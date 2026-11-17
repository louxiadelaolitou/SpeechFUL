package authprivacy

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/posts"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/readtiming"
)

type publicListRow struct {
	post    posts.FeedPost
	ordinal int64
}

// Cursor metadata, filtering and the publication ceiling share one round trip.
// Continuations do not scan MAX again. Missing channels keep the old error.
func (c *Community) publicListPosition(ctx context.Context, tx pgx.Tx, account uuid.UUID, q PostQuery, at time.Time) (posts.CursorPosition, []string, error) {
	defer readtiming.Span(ctx, "list_metadata")()
	p := posts.CursorPosition{Scope: posts.CursorChannel, AccountID: account.String(), ChannelID: q.ID, Limit: q.Limit, IdentityID: q.IdentityID, ExpiresAt: at.Add(posts.CursorTTL)}
	switch q.Kind {
	case PostFeed:
		p.Scope = posts.CursorFeed
		if q.Incremental {
			p.Scope = posts.CursorFeedWindow
		}
	case PostOwnList:
		p.Scope = posts.CursorOwn
		p.ChannelID = ""
	case PostLikedList:
		p.Scope = posts.CursorLiked
	case PostFavoriteList:
		p.Scope = posts.CursorFavorites
	case PostAuthor:
		p.Scope = posts.CursorAuthor
		p.ChannelID = ""
	}
	blocked := []string{}
	var exists bool
	channel := ""
	if q.Kind == PostFeed || q.Kind == PostChannelFeed {
		channel = q.ID
	}
	e := tx.QueryRow(ctx, `/* list:metadata */ SELECT
 CASE WHEN $2 THEN (SELECT coalesce(max(publication_ordinal),0) FROM c_posts.posts) ELSE 0 END,
 CASE WHEN $3 THEN coalesce((SELECT blocked_channel_ids FROM c_posts.feed_preferences WHERE account_id=$1),'{}'::uuid[]) ELSE '{}'::uuid[] END,
 CASE WHEN $3 THEN coalesce((SELECT revision FROM c_posts.feed_preferences WHERE account_id=$1),0) ELSE 0 END,
 ($4::uuid IS NULL OR EXISTS(SELECT 1 FROM public.channels WHERE id=$4))`, account, q.Cursor == "", q.Kind == PostFeed, nullablePostID(channel)).Scan(&p.Ceiling, &blocked, &p.FilterRevision, &exists)
	if e != nil {
		return p, blocked, e
	}
	if !exists {
		return p, blocked, postError("POST_CHANNEL_UNAVAILABLE", 404)
	}
	if q.Cursor != "" {
		p, e = c.posts.cursors.Decode(q.Cursor, p, at)
	}
	return p, blocked, e
}

// Each scenario adds only its actual predicates. Values are always parameters.
func (c *Community) publicListCandidates(ctx context.Context, tx pgx.Tx, account uuid.UUID, q PostQuery, p posts.CursorPosition, blocked []string, refresh *posts.FeedRefreshPosition) ([]string, error) {
	defer readtiming.Span(ctx, "list_candidates")()
	args := []any{}
	arg := func(v any) string { args = append(args, v); return fmt.Sprintf("$%d", len(args)) }
	viewer := arg(account)
	sql := `/* list:candidates */ SELECT p.post_id FROM c_posts.posts p JOIN c_posts.publication_tasks t ON t.post_id=p.post_id JOIN public.community_identities i ON i.identity_id=t.identity_id
 WHERE p.visibility='PUBLISHED' AND NOT p.moderation_hidden AND ` + publicListVisibleAuthorSQL(viewer) + `
 AND p.publication_ordinal<=` + arg(p.Ceiling)
	if q.Kind == PostChannelFeed || q.Kind == PostFeed && q.ID != "" {
		sql += ` AND p.channel_id=` + arg(q.ID) + `::uuid`
	}
	if q.Kind == PostOwnList {
		sql += ` AND p.owner_account_id=` + viewer + ` AND i.deleted_at IS NULL`
	}
	if q.IdentityID != "" {
		sql += ` AND t.identity_id=` + arg(q.IdentityID) + `::uuid`
	}
	if q.Kind == PostFeed {
		sql += ` AND NOT(p.channel_id=ANY(` + arg(blocked) + `::uuid[]))`
	}
	if c.posts.hiddenPostsEnabled && q.Kind != PostOwnList {
		sql += ` AND NOT EXISTS(SELECT 1 FROM c_posts.hidden_posts h WHERE h.account_id=` + viewer + ` AND h.post_id=p.post_id AND h.hidden)`
	}
	if q.Kind == PostLikedList {
		sql += ` AND EXISTS(SELECT 1 FROM c_posts.post_likes l WHERE l.post_id=p.post_id AND l.account_id=` + viewer + `)`
	}
	if q.Kind == PostFavoriteList {
		sql += ` AND EXISTS(SELECT 1 FROM c_posts.post_favorites f WHERE f.post_id=p.post_id AND f.account_id=` + viewer + `)`
	}
	if q.Kind == PostLikedList || q.Kind == PostFavoriteList || q.Kind == PostAuthor {
		sql += ` AND EXISTS(SELECT 1 FROM c_posts.attempt_contents x WHERE x.task_id=t.task_id AND x.version=p.published_attempt_version AND x.erased_at IS NULL)`
	}
	if refresh != nil {
		floorAt, floorOrdinal := refresh.HeadAt, refresh.HeadOrdinal
		if refresh.InsertAfter != "" {
			floorAt, floorOrdinal = refresh.FloorAt, refresh.FloorOrdinal
			sql += ` AND (p.published_at,p.publication_ordinal)<(` + arg(refresh.BeforeAt) + `::timestamptz,` + arg(refresh.BeforeOrdinal) + `::bigint)`
		}
		sql += ` AND (p.published_at,p.publication_ordinal)>(` + arg(floorAt) + `::timestamptz,` + arg(floorOrdinal) + `::bigint)`
	} else if !p.LastAt.IsZero() {
		sql += ` AND (p.published_at,p.publication_ordinal)<(` + arg(p.LastAt) + `::timestamptz,` + arg(p.LastOrdinal) + `::bigint)`
	}
	sql += ` ORDER BY p.published_at DESC,p.publication_ordinal DESC LIMIT ` + arg(q.Limit+1)
	rows, e := tx.Query(ctx, sql, args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			return nil, e
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// Keep the exact symmetric predicate of c_posts.accounts_blocked, but expose
// the two indexed anti-joins to the bulk query planner. The SQL function's
// per-row SET search_path boundary otherwise repeats for scanned posts. Both
// candidates and projection still check fresh rows in their own SQL snapshot;
// this does not assume a prepared/RR path or reuse an eligibility decision.
// viewer is an internally generated placeholder, never user-supplied SQL.
func publicListVisibleAuthorSQL(viewer string) string {
	return `NOT EXISTS(SELECT 1 FROM c_posts.blocked_authors b WHERE b.account_id=` + viewer + ` AND b.author_account_id=p.owner_account_id)
 AND NOT EXISTS(SELECT 1 FROM c_posts.blocked_authors b WHERE b.account_id=p.owner_account_id AND b.author_account_id=` + viewer + `)`
}

func (c *Community) publicListProjection(ctx context.Context, tx pgx.Tx, viewer uuid.UUID, ids []string, q PostQuery, proof *feedReadProof) ([]publicListRow, error) {
	defer readtiming.Span(ctx, "list_projection")()
	result := make([]publicListRow, 0, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	if !c.posts.batchListReads {
		for _, id := range ids {
			item, ordinal, e := readPublicPost(ctx, tx, viewer, id)
			if e != nil {
				return nil, e
			}
			row := publicListRow{post: posts.FeedPost{PublicPost: item}, ordinal: ordinal}
			if q.Kind == PostFeed || q.Kind == PostAuthor {
				row.post.ViewCount, e = readPostViewCount(ctx, tx, id)
				if e != nil {
					return nil, e
				}
			}
			result = append(result, row)
		}
	} else {
		views, join := `0::bigint`, ``
		if !q.deferViewCounts && (q.Kind == PostFeed || q.Kind == PostAuthor) {
			views = `coalesce(v.view_count,0)`
			join = ` LEFT JOIN c_posts.post_view_totals v ON v.post_id=p.post_id`
		}
		projectionSQL := `/* list:projection */ SELECT p.post_id,p.channel_id,coalesce(p.edited_title,x.title),coalesce(p.edited_body,x.body),p.published_at,p.publication_ordinal,
 a.state,i.deleted_at,i.nickname,i.avatar,l.short_code,coalesce(cardinality(p.edited_image_ids),x.image_count,0),p.edited_at,p.edited_image_ids IS NOT NULL,p.tags,` + views + `
 FROM unnest($1::uuid[]) WITH ORDINALITY requested(id,position)
 JOIN c_posts.posts p ON p.post_id=requested.id JOIN c_posts.publication_tasks t ON t.post_id=p.post_id
 JOIN c_posts.attempt_contents x ON x.task_id=t.task_id AND x.version=p.published_attempt_version
 JOIN public.community_identities i ON i.identity_id=t.identity_id JOIN c_auth.accounts a ON a.account_id=p.owner_account_id
 JOIN c_posts.identity_public_labels l ON l.identity_id=i.identity_id` + join + `
 WHERE p.visibility='PUBLISHED' AND NOT p.moderation_hidden AND x.erased_at IS NULL AND ` + publicListVisibleAuthorSQL("$2") + `
 ORDER BY requested.position`
		e := func() (err error) {
			var batch pgx.BatchResults
			var rows pgx.Rows
			if proof != nil {
				// Both SELECTs use this already-bound native preparation's RR snapshot.
				// Queue all candidate IDs (including lookahead), not just displayed IDs.
				commands := &pgx.Batch{}
				commands.Queue(projectionSQL, ids, viewer)
				commands.Queue(feedReadVersionsSQL, viewer, proof.ids, nullablePostID(proof.channel), proof.initial)
				batch = tx.SendBatch(ctx, commands)
				defer func() {
					if e := batch.Close(); e != nil {
						err = e
					}
				}()
				rows, err = batch.Query()
			} else {
				rows, err = tx.Query(ctx, projectionSQL, ids, viewer)
			}
			if err != nil {
				return err
			}
			defer rows.Close()
			var e error
			for rows.Next() {
				var row publicListRow
				var state, short string
				var deleted *time.Time
				var name, avatar *string
				p := &row.post
				if e = rows.Scan(&p.PostID, &p.ChannelID, &p.Title, &p.Body, &p.PublishedAt, &row.ordinal, &state, &deleted, &name, &avatar, &short, &p.ImageCount, &p.EditedAt, &p.ImagesEdited, &p.Tags, &p.ViewCount); e != nil {
					rows.Close()
					return e
				}
				p.Author, e = projectPublicPostAuthor(state, deleted, name, avatar, short)
				if e != nil {
					rows.Close()
					return e
				}
				p.PublishedAt = p.PublishedAt.UTC()
				if p.EditedAt != nil {
					at := p.EditedAt.UTC()
					p.EditedAt = &at
				}
				result = append(result, row)
			}
			e = rows.Err()
			rows.Close()
			if e != nil {
				return e
			}
			if proof != nil {
				finish := readtiming.Span(ctx, "list_proof")
				material, e := scanFeedReadProof(batch.QueryRow())
				finish()
				if e != nil {
					return e
				}
				if e = batch.Close(); e != nil {
					return e
				}
				proof.versions, proof.prepared = material, true
			}
			return nil
		}()
		if e != nil {
			return nil, e
		}
		if len(result) != len(ids) {
			return nil, pgx.ErrNoRows
		}
	}
	imageIDs := []string{}
	for _, row := range result {
		if row.post.ImageCount > 0 {
			imageIDs = append(imageIDs, row.post.PostID)
		}
	}
	if q.ImageSizes && len(imageIDs) > 0 && (q.Kind == PostFeed || q.Kind == PostAuthor) {
		sizes, e := readPublicListImageSizes(ctx, tx, imageIDs)
		if e != nil {
			return nil, e
		}
		for n := range result {
			p := &result[n].post
			p.ImageSizes = sizes[p.PostID]
			if len(p.ImageSizes) != p.ImageCount {
				return nil, ErrAuthorizationUnavailable
			}
		}
	}
	return result, nil
}

// Only geometry crosses this query. Image payloads are never projected or
// multiplied into the post rows. Edited original and new images share ordinal.
func readPublicListImageSizes(ctx context.Context, tx pgx.Tx, ids []string) (map[string][]posts.ImageSize, error) {
	defer readtiming.Span(ctx, "list_geometry")()
	out := make(map[string][]posts.ImageSize)
	rows, e := tx.Query(ctx, `/* list:images */ WITH requested AS (SELECT p.post_id,p.published_attempt_version,p.edited_image_ids,t.task_id FROM unnest($1::uuid[]) id JOIN c_posts.posts p ON p.post_id=id JOIN c_posts.publication_tasks t ON t.post_id=p.post_id),
 selected AS (SELECT r.*,image.id AS content_id,(image.ordinality-1)::integer AS position FROM requested r CROSS JOIN LATERAL unnest(r.edited_image_ids) WITH ORDINALITY image(id,ordinality)),
 geometry AS (
 SELECT r.post_id,m.position,m.display_width,m.display_height FROM requested r JOIN c_posts.images m ON m.task_id=r.task_id AND m.version=r.published_attempt_version
 WHERE r.edited_image_ids IS NULL AND m.erased_at IS NULL AND m.verified_at IS NOT NULL AND m.payload IS NOT NULL
 UNION ALL
 SELECT s.post_id,s.position,m.display_width,m.display_height FROM selected s JOIN c_posts.images m ON m.task_id=s.task_id AND m.version=s.published_attempt_version AND m.content_version_id=s.content_id
 WHERE m.erased_at IS NULL AND m.verified_at IS NOT NULL AND m.payload IS NOT NULL
 UNION ALL
 SELECT s.post_id,s.position,m.display_width,m.display_height FROM selected s JOIN c_posts.post_edit_media m ON m.post_id=s.post_id AND m.content_version_id=s.content_id WHERE m.command_digest IS NULL
 ) SELECT post_id,display_width,display_height FROM geometry ORDER BY post_id,position`, ids)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var size posts.ImageSize
		if e = rows.Scan(&id, &size.Width, &size.Height); e != nil {
			return nil, e
		}
		out[id] = append(out[id], size)
	}
	return out, rows.Err()
}

func (c *Community) listPublicPostsBatchCaptured(ctx context.Context, tx pgx.Tx, account uuid.UUID, q PostQuery, at time.Time, capture func(posts.CursorPosition, []string)) (any, error) {
	return c.listPublicPostsBatchWithProof(ctx, tx, account, q, at, capture, nil)
}

// proof is supplied only by native RR prepared feed reads, after subject validation.
func (c *Community) listPublicPostsBatchWithProof(ctx context.Context, tx pgx.Tx, account uuid.UUID, q PostQuery, at time.Time, capture func(posts.CursorPosition, []string), proof *feedReadProof) (any, error) {
	p, blocked, e := c.publicListPosition(ctx, tx, account, q, at)
	if e != nil {
		return nil, e
	}
	if capture != nil {
		capture(p, nil)
	}
	var refresh *posts.FeedRefreshPosition
	if q.RefreshCursor != "" {
		r, e := c.posts.cursors.DecodeFeedRefresh(q.RefreshCursor, posts.FeedRefreshPosition{AccountID: account.String(), ChannelID: q.ID, Revision: p.FilterRevision}, at)
		if e != nil {
			return nil, e
		}
		refresh = &r
	}
	ids, e := c.publicListCandidates(ctx, tx, account, q, p, blocked, refresh)
	if capture != nil {
		capture(p, ids)
	}
	if e != nil {
		return nil, e
	}
	more := len(ids) > q.Limit
	pageIDs := ids
	if more {
		pageIDs = pageIDs[:q.Limit]
	}
	entries, e := c.publicListProjection(ctx, tx, account, pageIDs, q, proof)
	if e != nil {
		return nil, e
	}
	if q.Incremental {
		return c.incrementalFeedPage(entries, refresh, p, q, more, at, capture, ids)
	}
	page := posts.FeedPage{Items: []posts.PublicPostCard{}}
	feed := posts.CrossChannelFeedPage{Items: []posts.FeedPost{}}
	own := posts.OwnPostPage{Items: []posts.OwnPostCard{}}
	author := posts.AuthorProfilePage{Items: []posts.FeedPost{}}
	if q.Kind == PostAuthor {
		author.Author, e = readPublicAuthor(ctx, tx, q.IdentityID)
		if e != nil {
			return nil, e
		}
	}
	for _, r := range entries {
		card := postCard(r.post.PublicPost)
		page.Items = append(page.Items, card)
		feed.Items = append(feed.Items, r.post)
		own.Items = append(own.Items, posts.OwnPostCard{Post: card, ServerSortAt: r.post.PublishedAt})
		author.Items = append(author.Items, r.post)
		p.LastAt = r.post.PublishedAt
		p.LastOrdinal = r.ordinal
	}
	if capture != nil {
		capture(p, ids)
	}
	if more {
		token, e := c.posts.cursors.Encode(p)
		if e != nil {
			return nil, e
		}
		page.NextCursor = &token
		feed.NextCursor = &token
		own.NextCursor = &token
		author.NextCursor = &token
	}
	switch q.Kind {
	case PostFeed:
		return feed, nil
	case PostOwnList:
		return own, nil
	case PostAuthor:
		return author, nil
	default:
		return page, nil
	}
}

func (c *Community) incrementalFeedPage(entries []publicListRow, r *posts.FeedRefreshPosition, p posts.CursorPosition, q PostQuery, more bool, at time.Time, capture func(posts.CursorPosition, []string), ids []string) (any, error) {
	page := posts.IncrementalFeedPage{Items: []posts.FeedPost{}}
	for _, row := range entries {
		page.Items = append(page.Items, row.post)
		p.LastAt = row.post.PublishedAt
		p.LastOrdinal = row.ordinal
	}
	if r != nil {
		next := *r
		if r.InsertAfter != "" {
			anchor := r.InsertAfter
			page.InsertAfter = &anchor
		} else if len(entries) > 0 {
			next.HeadAt = entries[0].post.PublishedAt
			next.HeadOrdinal = entries[0].ordinal
			next.HeadID = entries[0].post.PostID
			next.FloorAt = r.HeadAt
			next.FloorOrdinal = r.HeadOrdinal
		}
		if more {
			last := entries[len(entries)-1]
			next.BeforeAt = last.post.PublishedAt
			next.BeforeOrdinal = last.ordinal
			next.InsertAfter = last.post.PostID
		} else {
			next.FloorAt = time.Time{}
			next.FloorOrdinal = 0
			next.BeforeAt = time.Time{}
			next.BeforeOrdinal = 0
			next.InsertAfter = ""
		}
		page.RefreshPosition = next
	} else if q.Cursor == "" {
		next := posts.FeedRefreshPosition{AccountID: p.AccountID, ChannelID: q.ID, Revision: p.FilterRevision, ExpiresAt: at.Add(posts.CursorTTL), HeadAt: time.Unix(0, 0).UTC()}
		if len(entries) > 0 {
			next.HeadAt = entries[0].post.PublishedAt
			next.HeadOrdinal = entries[0].ordinal
			next.HeadID = entries[0].post.PostID
		}
		page.RefreshPosition = next
	}
	// Opt-in cursors encode the continuation's size, while the initial fetch
	// stays at ten. Old scopes still enforce their exact requested page size.
	p.Limit = 5
	if capture != nil {
		capture(p, ids)
	}
	if r == nil && more {
		token, e := c.posts.cursors.Encode(p)
		if e != nil {
			return nil, e
		}
		page.NextCursor = &token
	}
	if !page.RefreshPosition.HeadAt.IsZero() {
		token, e := c.posts.cursors.EncodeFeedRefresh(page.RefreshPosition)
		if e != nil {
			return nil, e
		}
		page.RefreshCursor = &token
	}
	return page, nil
}
