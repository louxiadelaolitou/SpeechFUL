package authprivacy

import (
	"bytes"
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/posts"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/readtiming"
)

// A feed proof covers the complete channel ranges and the viewer's filtering,
// as well as every bounded candidate (including the lookahead). Fixed-ceiling
// continuation ignores only newer publications, never membership changes.
type feedReadProof struct {
	channel  string
	initial  bool
	ids      []string
	versions []byte
}

const feedReadVersionsSQL = `WITH channels AS (
 SELECT id FROM public.channels WHERE $3::uuid IS NULL OR id=$3
), sources AS (
 SELECT p.post_id,p.owner_account_id,t.identity_id FROM c_posts.posts p
 JOIN c_posts.publication_tasks t ON t.post_id=p.post_id WHERE p.post_id=ANY($2::uuid[])
), identities AS (SELECT DISTINCT identity_id FROM sources),
 accounts AS (SELECT owner_account_id AS account_id FROM sources UNION SELECT $1::uuid)
 SELECT jsonb_build_object(
 'channels',(SELECT coalesce(jsonb_agg(jsonb_build_array(c.id,v.membership_revision,CASE WHEN $4 THEN v.publication_revision ELSE 0 END) ORDER BY c.id),'[]'::jsonb) FROM channels c LEFT JOIN c_posts.read_feed_scope_versions v ON v.channel_id=c.id),
 'posts',(SELECT coalesce(jsonb_agg(jsonb_build_array(s.post_id,v.projection_revision) ORDER BY s.post_id),'[]'::jsonb) FROM sources s LEFT JOIN c_posts.read_feed_post_versions v USING(post_id)),
 'identities',(SELECT coalesce(jsonb_agg(jsonb_build_array(i.identity_id,v.projection_revision) ORDER BY i.identity_id),'[]'::jsonb) FROM identities i LEFT JOIN c_posts.read_identity_versions v USING(identity_id)),
 'accounts',(SELECT coalesce(jsonb_agg(jsonb_build_array(a.account_id,v.projection_revision) ORDER BY a.account_id),'[]'::jsonb) FROM accounts a LEFT JOIN c_posts.read_account_versions v USING(account_id)),
 'viewer',(SELECT visibility_revision FROM c_posts.read_account_versions WHERE account_id=$1),
 'filters',coalesce((SELECT revision FROM c_posts.feed_preferences WHERE account_id=$1),0)),
 NOT EXISTS(SELECT 1 FROM channels c LEFT JOIN c_posts.read_feed_scope_versions v ON v.channel_id=c.id WHERE v.channel_id IS NULL)
 AND NOT EXISTS(SELECT 1 FROM sources s LEFT JOIN c_posts.read_feed_post_versions v USING(post_id) WHERE v.post_id IS NULL)
 AND NOT EXISTS(SELECT 1 FROM identities i LEFT JOIN c_posts.read_identity_versions v USING(identity_id) WHERE v.identity_id IS NULL)
 AND NOT EXISTS(SELECT 1 FROM accounts a LEFT JOIN c_posts.read_account_versions v USING(account_id) WHERE v.account_id IS NULL)`

func (p *feedReadProof) read(ctx context.Context, tx pgx.Tx, a uuid.UUID) ([]byte, error) {
	var proof []byte
	var intact bool
	e := tx.QueryRow(ctx, feedReadVersionsSQL, a, p.ids, nullablePostID(p.channel), p.initial).Scan(&proof, &intact)
	if e != nil {
		return nil, e
	}
	if !intact {
		return nil, ErrAuthorizationUnavailable
	}
	return proof, nil
}

func (p *feedReadProof) revalidate(ctx context.Context, tx pgx.Tx, a uuid.UUID) error {
	current, e := p.read(ctx, tx, a)
	if e != nil {
		return e
	}
	if !bytes.Equal(current, p.versions) {
		return errReadConflict
	}
	return nil
}

func (c *Community) readPreparedFeed(ctx context.Context, bearer [32]byte, q PostQuery) (PostResponse, error) {
	ctx = readtiming.Phase(ctx, "feed")
	return c.withPreparedReadPlan(ctx, bearer, postError("SERVICE_UNAVAILABLE", 503), func(prepareCtx context.Context, tx pgx.Tx, s readSubject) (*preparedReadPlan, error) {
		proof := &feedReadProof{channel: q.ID, initial: q.Cursor == ""}
		var position posts.CursorPosition
		value, pending := c.listPublicPostsCaptured(prepareCtx, tx, s.account, q, s.decision.TrustedAt, func(p posts.CursorPosition, ids []string) { position = p; proof.ids = append([]string{}, ids...) })
		var e error
		proof.versions, e = proof.read(prepareCtx, tx, s.account)
		if e != nil {
			pending = e
		}
		plan := &preparedReadPlan{
			Revalidate: func(tx pgx.Tx, a uuid.UUID, d AuthorizationDecision, _ restriction) error {
				if len(proof.ids) > q.Limit+1 {
					return ErrAuthorizationUnavailable
				}
				if e := proof.revalidate(ctx, tx, a); e != nil {
					return e
				}
				if q.Cursor != "" && position.Scope != "" {
					if _, e := c.posts.cursors.Decode(q.Cursor, position, d.TrustedAt); e != nil {
						return e
					}
				}
				return nil
			},
			Complete: func(tx pgx.Tx, a uuid.UUID, d AuthorizationDecision, _ restriction) (any, error) {
				if q.Cursor == "" {
					position.ExpiresAt = d.TrustedAt.Add(posts.CursorTTL)
				}
				var cursor *string
				switch page := value.(type) {
				case posts.CrossChannelFeedPage:
					cursor = page.NextCursor
				case posts.FeedPage:
					cursor = page.NextCursor
				}
				if cursor != nil {
					encoded, e := c.posts.cursors.Encode(position)
					if e != nil {
						return nil, e
					}
					cursor = &encoded
				}
				switch page := value.(type) {
				case posts.CrossChannelFeedPage:
					page.NextCursor = cursor
					ids := make([]string, len(page.Items))
					for i := range page.Items {
						ids[i] = page.Items[i].PostID
					}
					views, e := readPreparedViewCounts(ctx, tx, ids)
					if e != nil {
						return nil, e
					}
					for i := range page.Items {
						page.Items[i].ViewCount = views[page.Items[i].PostID]
					}
					return page, nil
				case posts.FeedPage:
					page.NextCursor = cursor
					return page, nil
				default:
					return nil, ErrAuthorizationUnavailable
				}
			},
		}
		return plan, pending
	})
}

// Mutable display counters do not invalidate candidate order or public body
// projection. Refresh only the returned page's counters under final authority.
func readPreparedViewCounts(ctx context.Context, tx pgx.Tx, ids []string) (map[string]int64, error) {
	counts := make(map[string]int64, len(ids))
	if len(ids) == 0 {
		return counts, nil
	}
	rows, e := tx.Query(ctx, `SELECT id::text,coalesce(v.view_count,0) FROM unnest($1::uuid[]) id LEFT JOIN c_posts.post_view_totals v ON v.post_id=id`, ids)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var n int64
		if e = rows.Scan(&id, &n); e != nil {
			return nil, e
		}
		counts[id] = n
	}
	return counts, rows.Err()
}
