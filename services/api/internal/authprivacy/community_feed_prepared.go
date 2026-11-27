package authprivacy

import (
	"bytes"
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/posts"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/readtiming"
	rw "github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/recommendation/transport"
)

// A feed proof covers the complete channel ranges and the viewer's filtering,
// as well as every bounded candidate (including the lookahead). Fixed-ceiling
// continuation ignores only newer publications, never membership changes.
type feedReadProof struct {
	channel  string
	initial  bool
	ids      []string
	versions []byte
	prepared bool // Complete fresh proof read with the projection in this plan only.
}

// Each fact has an unambiguous 36-byte representation: kind (4), UUID (16),
// revision (8), secondary revision (8), ordered by kind and UUID. The separate
// completeness flag distinguishes an absent version row from a valid zero.
// These bytes live only in this prepared plan, never in storage or a cursor.
const feedReadVersionsSQL = `WITH facts AS (
WITH sources AS (
 SELECT p.post_id,p.owner_account_id,t.identity_id FROM c_posts.posts p
 JOIN c_posts.publication_tasks t ON t.post_id=p.post_id WHERE p.post_id=ANY($2::uuid[])
), identities AS (SELECT DISTINCT identity_id FROM sources),
 accounts AS (SELECT owner_account_id AS account_id FROM sources UNION SELECT $1::uuid)
 SELECT 0::integer AS kind,c.id AS id,coalesce(v.membership_revision,0) AS revision,
 CASE WHEN $4 THEN coalesce(v.publication_revision,0) ELSE 0 END AS secondary,v.channel_id IS NOT NULL AS intact
 FROM public.channels c LEFT JOIN c_posts.read_feed_scope_versions v ON v.channel_id=c.id WHERE $3::uuid IS NULL OR c.id=$3
 UNION ALL
 SELECT 1,s.post_id,coalesce(v.projection_revision,0),0,v.post_id IS NOT NULL FROM sources s LEFT JOIN c_posts.read_feed_post_versions v USING(post_id)
 UNION ALL
 SELECT 2,i.identity_id,coalesce(v.projection_revision,0),0,v.identity_id IS NOT NULL FROM identities i LEFT JOIN c_posts.read_identity_versions v USING(identity_id)
 UNION ALL
 SELECT 3,a.account_id,coalesce(v.projection_revision,0),CASE WHEN a.account_id=$1 THEN coalesce(v.visibility_revision,0) ELSE 0 END,v.account_id IS NOT NULL FROM accounts a LEFT JOIN c_posts.read_account_versions v USING(account_id)
 UNION ALL
 SELECT 4,$1::uuid,coalesce((SELECT revision FROM c_posts.feed_preferences WHERE account_id=$1),0),0,true
) SELECT coalesce(string_agg(pg_catalog.int4send(kind)||pg_catalog.uuid_send(id)||pg_catalog.int8send(revision)||pg_catalog.int8send(secondary),''::bytea ORDER BY kind,id),''::bytea),coalesce(bool_and(intact),true) FROM facts`

func (p *feedReadProof) read(ctx context.Context, tx pgx.Tx, a uuid.UUID) ([]byte, error) {
	defer readtiming.Span(ctx, "list_proof")()
	return scanFeedReadProof(tx.QueryRow(ctx, feedReadVersionsSQL, a, p.ids, nullablePostID(p.channel), p.initial))
}

func scanFeedReadProof(row pgx.Row) ([]byte, error) {
	var proof []byte
	var intact bool
	e := row.Scan(&proof, &intact)
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
	q.deferViewCounts = c.posts.batchListReads
	var capability *rw.Capability
	response, err := c.withPreparedReadPlan(ctx, bearer, postError("SERVICE_UNAVAILABLE", 503), func(prepareCtx context.Context, tx pgx.Tx, s readSubject) (*preparedReadPlan, error) {
		proof := &feedReadProof{channel: q.ID, initial: q.Cursor == ""}
		var position posts.CursorPosition
		capture := func(p posts.CursorPosition, ids []string) { position = p; proof.ids = append([]string{}, ids...) }
		var value any
		var pending error
		if c.posts.batchListReads {
			value, pending = c.listPublicPostsBatchWithProof(prepareCtx, tx, s.account, q, s.decision.TrustedAt, capture, proof)
		} else {
			value, pending = c.listPublicPostsCaptured(prepareCtx, tx, s.account, q, s.decision.TrustedAt, capture)
		}
		// Empty/error pages and the legacy path still need a complete fresh proof.
		if !proof.prepared {
			var e error
			proof.versions, e = proof.read(prepareCtx, tx, s.account)
			if e != nil {
				pending = e
			}
		}
		var finalViews map[string]int64
		var pageIDs []string
		batchViews := c.posts.batchListReads && q.Kind == PostFeed
		if batchViews {
			switch page := value.(type) {
			case posts.IncrementalFeedPage:
				for _, item := range page.Items {
					pageIDs = append(pageIDs, item.PostID)
				}
			case posts.CrossChannelFeedPage:
				for _, item := range page.Items {
					pageIDs = append(pageIDs, item.PostID)
				}
			}
		}
		plan := &preparedReadPlan{
			Revalidate: func(tx pgx.Tx, a uuid.UUID, d AuthorizationDecision, _ restriction) error {
				if len(proof.ids) > q.Limit+1 {
					return ErrAuthorizationUnavailable
				}
				var views map[string]int64
				var e error
				if batchViews && len(pageIDs) > 0 {
					views, e = proof.revalidateWithViewCounts(ctx, tx, a, pageIDs)
				} else {
					e = proof.revalidate(ctx, tx, a)
				}
				if e != nil {
					return e
				}
				if q.Cursor != "" && position.Scope != "" {
					if _, e := c.posts.cursors.Decode(q.Cursor, position, d.TrustedAt); e != nil {
						return e
					}
				}
				if q.RefreshCursor != "" && position.Scope != "" {
					if _, e := c.posts.cursors.DecodeFeedRefresh(q.RefreshCursor, posts.FeedRefreshPosition{AccountID: a.String(), ChannelID: q.ID, Revision: position.FilterRevision}, d.TrustedAt); e != nil {
						return e
					}
				}
				// A conflicting or expired plan never publishes its queued counts.
				finalViews = views
				return nil
			},
			Complete: func(tx pgx.Tx, a uuid.UUID, d AuthorizationDecision, _ restriction) (any, error) {
				if q.Kind == PostFeed {
					var e error
					capability, e = c.recommendationFeedCapability(ctx, tx, a, bearer, d, q.ID)
					if e != nil {
						return nil, e
					}
				}
				if q.Cursor == "" {
					position.ExpiresAt = d.TrustedAt.Add(posts.CursorTTL)
				}
				var cursor *string
				switch page := value.(type) {
				case posts.IncrementalFeedPage:
					cursor = page.NextCursor
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
				case posts.IncrementalFeedPage:
					page.NextCursor = cursor
					if !page.RefreshPosition.HeadAt.IsZero() {
						page.RefreshPosition.ExpiresAt = d.TrustedAt.Add(posts.CursorTTL)
						token, e := c.posts.cursors.EncodeFeedRefresh(page.RefreshPosition)
						if e != nil {
							return nil, e
						}
						page.RefreshCursor = &token
					}
					ids := make([]string, len(page.Items))
					for i := range page.Items {
						ids[i] = page.Items[i].PostID
					}
					views := finalViews
					if !batchViews {
						var e error
						views, e = readPreparedViewCounts(ctx, tx, ids)
						if e != nil {
							return nil, e
						}
					}
					for i := range page.Items {
						page.Items[i].ViewCount = views[page.Items[i].PostID]
					}
					if q.WindowPreviews && q.Cursor != "" && len(ids) > 0 && c.PostImageThumbnailsEnabled() {
						return c.continuationPreviews(ctx, tx, a, d.TrustedAt, page, ids)
					}
					return page, nil
				case posts.CrossChannelFeedPage:
					page.NextCursor = cursor
					ids := make([]string, len(page.Items))
					for i := range page.Items {
						ids[i] = page.Items[i].PostID
					}
					views := finalViews
					if !batchViews {
						var e error
						views, e = readPreparedViewCounts(ctx, tx, ids)
						if e != nil {
							return nil, e
						}
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
	if err == nil {
		response.Recommendation = capability
	}
	if window, ok := response.Value.(FeedWindowPreviews); err == nil && ok {
		for i := range window.Previews.ImageManifests {
			group := &window.Previews.ImageManifests[i]
			until, _ := time.Parse(time.RFC3339Nano, group.AccessValidUntil)
			if until.After(response.ExpiresAt) {
				group.AccessValidUntil = response.ExpiresAt.UTC().Format(time.RFC3339Nano)
			}
		}
		response.Value = window
	}
	return response, err
}

// Mutable display counters do not invalidate candidate order or public body
// projection. Refresh only the returned page's counters under final authority.
const preparedViewCountsSQL = `SELECT id::text,coalesce(v.view_count,0) FROM unnest($1::uuid[]) id LEFT JOIN c_posts.post_view_totals v ON v.post_id=id`

func readPreparedViewCounts(ctx context.Context, tx pgx.Tx, ids []string) (map[string]int64, error) {
	counts := make(map[string]int64, len(ids))
	if len(ids) == 0 {
		return counts, nil
	}
	rows, e := tx.Query(ctx, preparedViewCountsSQL, ids)
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
