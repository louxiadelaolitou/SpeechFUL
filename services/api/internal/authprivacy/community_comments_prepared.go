package authprivacy

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/comments"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/commentsnapshot"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/posts"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/readtiming"
)

type commentReadPlan struct {
	query                             CommentQuery
	locate                            string
	config                            *commentsnapshot.Config
	snapshot                          commentsnapshot.Snapshot
	create, legacyCursor              bool
	value                             any
	next                              *int
	scanned, dependencies, countRoots []uuid.UUID
	versions                          []byte
	loadedProof                       commentsnapshot.LoadedProof
	contextVersions                   []byte
}

// This small binding is recorded even for a missing/hidden context. A private
// 404 is only final after the caller is authorized and this binding is current.
const commentReadContextVersionsSQL = `WITH root AS (
 SELECT t.comment_id,t.post_id FROM c_comments.tasks t WHERE $3 AND t.comment_id=$1
), post AS (
 SELECT p.post_id FROM c_posts.posts p WHERE p.post_id=CASE WHEN $3 THEN (SELECT post_id FROM root) ELSE $1::uuid END
), located AS (
 SELECT t.comment_id,t.post_id,t.root_comment_id FROM c_comments.tasks t WHERE t.comment_id=$4::uuid
 UNION SELECT r.comment_id,r.post_id,r.root_comment_id FROM c_comments.tasks t JOIN c_comments.tasks r ON r.comment_id=t.root_comment_id WHERE t.comment_id=$4::uuid
)
SELECT jsonb_build_object(
 'post',(SELECT jsonb_build_array(p.post_id,v.visibility_revision) FROM post p LEFT JOIN c_comments.read_post_versions v USING(post_id)),
 'root',(SELECT jsonb_build_array(r.comment_id,v.projection_revision) FROM root r LEFT JOIN c_comments.read_comment_versions v USING(comment_id)),
 'located',(SELECT coalesce(jsonb_agg(jsonb_build_array(l.comment_id,l.post_id,l.root_comment_id,v.projection_revision) ORDER BY l.comment_id),'[]'::jsonb) FROM located l LEFT JOIN c_comments.read_comment_versions v USING(comment_id)),
 'viewer',(SELECT visibility_revision FROM c_posts.read_account_versions WHERE account_id=$2)),
 NOT EXISTS(SELECT 1 FROM post p LEFT JOIN c_comments.read_post_versions v USING(post_id) WHERE v.post_id IS NULL)
 AND NOT EXISTS(SELECT 1 FROM root r LEFT JOIN c_comments.read_comment_versions v USING(comment_id) WHERE v.comment_id IS NULL)
 AND NOT EXISTS(SELECT 1 FROM located l LEFT JOIN c_comments.read_comment_versions v USING(comment_id) WHERE v.comment_id IS NULL)
 AND EXISTS(SELECT 1 FROM c_posts.read_account_versions WHERE account_id=$2)`

func (p *commentReadPlan) readContextVersions(ctx context.Context, tx pgx.Tx, account uuid.UUID) ([]byte, error) {
	var data []byte
	var intact bool
	e := tx.QueryRow(ctx, commentReadContextVersionsSQL, p.query.ID, account, p.query.Replies, nullablePostID(p.locate)).Scan(&data, &intact)
	if e != nil {
		return nil, e
	}
	if !intact {
		return nil, ErrAuthorizationUnavailable
	}
	return data, nil
}

// Every collection below is bounded by the scanned page (at most 150 IDs),
// plus its root/target relationships. The only whole-order work is in prepare.
const commentReadVersionsSQL = `WITH deps AS (
 SELECT t.comment_id,t.identity_id FROM c_comments.tasks t WHERE t.comment_id=ANY($3::uuid[])
 UNION SELECT target.comment_id,target.identity_id FROM c_comments.tasks t JOIN c_comments.tasks target ON target.comment_id=t.target_comment_id WHERE t.comment_id=ANY($3::uuid[])
), identities AS (SELECT DISTINCT i.identity_id,i.account_id FROM deps d JOIN public.community_identities i USING(identity_id)),
 accounts AS (SELECT account_id FROM identities UNION SELECT $2::uuid)
 SELECT jsonb_build_object(
 'post',(SELECT jsonb_build_array(v.visibility_revision,CASE WHEN $6 THEN v.count_revision ELSE 0 END) FROM c_comments.read_post_versions v WHERE v.post_id=$1),
 'viewer',(SELECT visibility_revision FROM c_posts.read_account_versions WHERE account_id=$2),
 'comments',(SELECT coalesce(jsonb_agg(jsonb_build_array(d.comment_id,v.projection_revision) ORDER BY d.comment_id),'[]'::jsonb) FROM deps d LEFT JOIN c_comments.read_comment_versions v USING(comment_id)),
 'identities',(SELECT coalesce(jsonb_agg(jsonb_build_array(i.identity_id,v.projection_revision) ORDER BY i.identity_id),'[]'::jsonb) FROM identities i LEFT JOIN c_posts.read_identity_versions v USING(identity_id)),
 'accounts',(SELECT coalesce(jsonb_agg(jsonb_build_array(a.account_id,v.projection_revision) ORDER BY a.account_id),'[]'::jsonb) FROM accounts a LEFT JOIN c_posts.read_account_versions v USING(account_id)),
 'counts',(SELECT coalesce(jsonb_agg(jsonb_build_array(r,coalesce(v.membership_revision,0)) ORDER BY r),'[]'::jsonb) FROM unnest($4::uuid[]) r LEFT JOIN c_comments.read_scope_versions v ON v.post_id=$1 AND v.root_id=r),
 'scope',CASE WHEN $5 THEN (SELECT jsonb_build_array(coalesce(v.membership_revision,0),CASE WHEN $7 THEN coalesce(v.order_revision,0) ELSE 0 END) FROM (SELECT 1) one LEFT JOIN c_comments.read_scope_versions v ON v.post_id=$1 AND v.root_id=$8) ELSE 'null'::jsonb END
 ),
 EXISTS(SELECT 1 FROM c_comments.read_post_versions WHERE post_id=$1)
 AND NOT EXISTS(SELECT 1 FROM deps d LEFT JOIN c_comments.read_comment_versions v USING(comment_id) WHERE v.comment_id IS NULL)
 AND NOT EXISTS(SELECT 1 FROM identities i LEFT JOIN c_posts.read_identity_versions v USING(identity_id) WHERE v.identity_id IS NULL)
 AND NOT EXISTS(SELECT 1 FROM accounts a LEFT JOIN c_posts.read_account_versions v USING(account_id) WHERE v.account_id IS NULL)
 AND EXISTS(SELECT 1 FROM c_comments.read_scope_versions WHERE post_id=$1 AND root_id='00000000-0000-0000-0000-000000000000')
 AND NOT EXISTS(SELECT 1 FROM unnest($4::uuid[]) r LEFT JOIN c_comments.read_scope_versions v ON v.post_id=$1 AND v.root_id=r WHERE v.root_id IS NULL)
 AND ($8='00000000-0000-0000-0000-000000000000' OR EXISTS(SELECT 1 FROM c_comments.read_scope_versions WHERE post_id=$1 AND root_id=$8))`

func (p *commentReadPlan) readVersions(ctx context.Context, tx pgx.Tx, account uuid.UUID) ([]byte, error) {
	var data []byte
	var intact bool
	e := tx.QueryRow(ctx, commentReadVersionsSQL, p.snapshot.Scope.Post, account, p.dependencies, p.countRoots,
		p.create, !p.query.Replies && p.locate == "", p.create && p.snapshot.Scope.Sort != comments.Recent, p.snapshot.Scope.Root).Scan(&data, &intact)
	if e != nil {
		return nil, e
	}
	if !intact {
		return nil, ErrAuthorizationUnavailable
	}
	return data, nil
}

func (c *Community) readCommentsPrepared(ctx context.Context, b [32]byte, q CommentQuery, locate string, cfg *commentsnapshot.Config) (PostResponse, error) {
	ctx = readtiming.Phase(ctx, "comments")
	defer readtiming.Span(ctx, "total")()
	var timing CommentReadTiming
	started := time.Now()
	if c.comments.observeRead != nil {
		timing.Path = "initial"
		if q.Cursor != "" {
			timing.Path = "continuation"
		}
		if q.Replies {
			timing.Path = "reply_preview"
			if q.Expanded {
				timing.Path = "reply_expanded"
			}
		}
		defer func() { timing.Total = time.Since(started); c.comments.observeRead(timing) }()
	}
	if _, e := posts.ParseResourceID(q.ID); e != nil {
		return PostResponse{}, e
	}
	if locate != "" {
		if _, e := posts.ParseResourceID(locate); e != nil {
			return PostResponse{}, e
		}
	}
	if !q.Sort.Valid() || q.Sort != "" && (q.Cursor != "" || q.Replies) || cfg != nil && q.Cursor != "" && q.SnapshotRequest != "" {
		return PostResponse{}, comments.ErrInvalid
	}
	if q.Limit == 0 {
		q.Limit = 50
	}
	if q.Limit < 1 || q.Limit > 50 {
		return PostResponse{}, comments.ErrInvalid
	}
	var nonce []byte
	if cfg != nil {
		ctx = commentsnapshot.WithBudget(ctx, cfg.OperationBudgetMilliseconds)
		var e error
		nonce, e = snapshotNonce(q.SnapshotRequest)
		if e != nil {
			return PostResponse{}, commentSnapshotError(e)
		}
	}
	if c.comments.observeRead != nil {
		ctx = context.WithValue(ctx, preparedReadObserverKey{}, func(elapsed time.Duration) { timing.Transaction += elapsed })
	}
	var reservations []commentsnapshot.Prepared
	committed := false
	defer func() {
		if committed {
			return
		}
		for _, p := range reservations {
			if p.NewlyCreated {
				cleanup, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
				_ = c.comments.snapshotStore.Abort(cleanup, p)
				cancel()
			}
		}
	}()
	var completed *commentReadPlan
	response, e := c.withPreparedRead(ctx, b, func(prepareCtx context.Context, tx pgx.Tx, s readSubject) (preparedReadAction, error) {
		plan, pending := c.prepareCommentRead(prepareCtx, tx, s, q, locate, cfg, nonce)
		if pending != nil && plan.snapshot.Scope.Post != uuid.Nil {
			versions, ve := plan.readVersions(prepareCtx, tx, s.account)
			if ve != nil {
				pending = ve
			} else {
				plan.versions = versions
			}
		}
		return func(tx pgx.Tx, a uuid.UUID, d AuthorizationDecision, _ restriction) (any, error) {
			storeAt := commentsnapshot.BudgetElapsed(ctx)
			defer func() { timing.StoreInTransaction += commentsnapshot.BudgetElapsed(ctx) - storeAt }()
			if e := c.checkCommentKeys(ctx, tx); e != nil {
				return nil, e
			}
			contextVersions, e := plan.readContextVersions(ctx, tx, a)
			if e != nil {
				return nil, e
			}
			if !bytes.Equal(contextVersions, plan.contextVersions) {
				return nil, errReadConflict
			}
			if pending != nil {
				if len(plan.versions) > 0 {
					versions, e := plan.readVersions(ctx, tx, a)
					if e != nil {
						return nil, e
					}
					if !bytes.Equal(versions, plan.versions) {
						return nil, errReadConflict
					}
				}
				return nil, pending
			}
			value, e := c.finalizeCommentRead(ctx, tx, a, d, plan, nonce, &reservations)
			if e == nil {
				completed = plan
			}
			return value, e
		}, nil
	})
	if e != nil {
		return PostResponse{}, e
	}
	committed = true
	timing.Backend = "postgres"
	if completed.snapshot.Ref.Backend == commentsnapshot.Redis {
		timing.Backend = "redis"
	}
	timing.SQLGateProjection = max(0, timing.Transaction-timing.StoreInTransaction)
	for _, p := range reservations {
		publishAt := time.Now()
		e = c.comments.snapshotStore.Publish(ctx, p)
		timing.Publish += time.Since(publishAt)
		if e != nil {
			return PostResponse{}, commentSnapshotError(e)
		}
	}
	if completed.next != nil {
		var cursor string
		if completed.legacyCursor {
			cursor = c.commentCursor(completed.snapshot.Ref.ID, *completed.next)
		} else {
			cursor, e = commentsnapshot.EncodeCursor(c.comments.codec.IntentFingerprint, completed.snapshot.Ref, *completed.next)
			if e != nil {
				return PostResponse{}, commentSnapshotError(e)
			}
		}
		switch v := response.Value.(type) {
		case TextCommentPage:
			v.NextCursor = &cursor
			response.Value = v
		case CommentLocation:
			v.Cursor = &cursor
			response.Value = v
		}
	}
	timing.Succeeded = true
	return response, nil
}

func (c *Community) prepareCommentRead(ctx context.Context, tx pgx.Tx, s readSubject, q CommentQuery, locate string, cfg *commentsnapshot.Config, nonce []byte) (*commentReadPlan, error) {
	p := &commentReadPlan{query: q, locate: locate, config: cfg, legacyCursor: cfg == nil}
	var contextError error
	p.contextVersions, contextError = p.readContextVersions(ctx, tx, s.account)
	if contextError != nil {
		return p, contextError
	}
	post := q.ID
	root := uuid.Nil
	kind := comments.MainComments
	if q.Replies {
		root = uuid.MustParse(q.ID)
		kind = comments.Replies
		e := tx.QueryRow(ctx, `SELECT t.post_id::text FROM c_comments.tasks t JOIN c_comments.attempts x USING(task_id) JOIN public.community_identities ci ON ci.identity_id=t.identity_id WHERE t.comment_id=$1 AND t.root_comment_id IS NULL AND NOT c_posts.accounts_blocked($2,ci.account_id) AND t.deleted_at IS NULL AND NOT COALESCE((to_jsonb(t)->>'moderation_hidden')::boolean,false) AND x.state='PUBLISHED'`, root, s.account).Scan(&post)
		if errors.Is(e, pgx.ErrNoRows) || e == nil && q.PostID != "" && q.PostID != post {
			return p, postError("COMMENT_NOT_FOUND", 404)
		}
		if e != nil {
			return p, e
		}
		p.dependencies = append(p.dependencies, root)
	}
	var visible bool
	if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM c_posts.posts WHERE post_id=$1 AND visibility='PUBLISHED' AND NOT moderation_hidden AND NOT c_posts.accounts_blocked($2,owner_account_id))`, post, s.account).Scan(&visible); e != nil {
		return p, e
	}
	if !visible {
		return p, postError("POST_NOT_FOUND", 404)
	}
	namespace := ""
	if cfg != nil {
		namespace = cfg.Redis.Namespace
	}
	scope := commentsnapshot.Scope{Owner: commentsnapshot.OwnerToken(c.comments.codec.IntentFingerprint, s.account, namespace), Post: uuid.MustParse(post), Root: root, Kind: kind, Sort: commentsnapshot.NormalizeSort(q.Sort)}
	p.snapshot.Scope = scope
	p.create = q.Cursor == ""
	offset := 0
	if q.Cursor != "" {
		var ref commentsnapshot.Ref
		decoded, de := base64.RawURLEncoding.Strict().DecodeString(q.Cursor)
		if cfg == nil || de == nil && len(decoded) == 52 {
			id, o, e := c.parseCommentCursor(q.Cursor)
			if e != nil {
				return p, e
			}
			offset = o
			p.legacyCursor = true
			ref = commentsnapshot.Ref{Backend: commentsnapshot.Postgres, ID: id, Kind: kind, Sort: scope.Sort}
		} else {
			var e error
			ref, offset, e = commentsnapshot.DecodeCursor(c.comments.codec.IntentFingerprint, q.Cursor)
			if e != nil {
				return p, commentSnapshotError(e)
			}
			if ref.Kind != kind || !ref.Expires.After(s.decision.TrustedAt) {
				return p, postError("CURSOR_INVALID", 400)
			}
			scope.Sort = ref.Sort
		}
		var e error
		if ref.Backend == commentsnapshot.Redis {
			if cfg == nil || !cfg.ReadRedis {
				return p, postError("CURSOR_INVALID", 400)
			}
			p.snapshot, e = c.comments.snapshotStore.Load(ctx, scope, ref, s.decision.TrustedAt)
			if e != nil {
				e = commentSnapshotError(e)
			}
		} else {
			if cfg != nil && !cfg.ReadPostgres {
				return p, postError("CURSOR_INVALID", 400)
			}
			p.snapshot, e = c.loadPGSnapshot(ctx, tx, s.account, scope, ref, s.decision.TrustedAt, p.legacyCursor)
		}
		if e != nil {
			return p, e
		}
		if offset > len(p.snapshot.IDs) {
			return p, postError("CURSOR_INVALID", 400)
		}
	} else {
		found := false
		if cfg != nil && cfg.CreateBackend == "redis" {
			var e error
			p.snapshot, found, e = c.comments.snapshotStore.Lookup(ctx, scope, commentsnapshot.Request(c.comments.codec.IntentFingerprint, scope.Owner, scope, nonce), s.decision.TrustedAt)
			if e != nil {
				return p, commentSnapshotError(e)
			}
		}
		p.create = !found
		if !found {
			p.snapshot.Scope = scope
			ids, e := c.orderedCommentIDs(ctx, tx, s.account, scope)
			if e != nil {
				if cfg == nil {
					var business *PostError
					if errors.As(e, &business) && business.Code == "COMMENT_SNAPSHOT_TOO_LARGE" {
						e = postError("RATE_LIMITED", 429)
					}
				}
				return p, e
			}
			p.create = true
			p.snapshot = commentsnapshot.Snapshot{Scope: scope, IDs: ids, Ref: commentsnapshot.Ref{Backend: commentsnapshot.Postgres, ID: uuid.New(), Kind: kind, Sort: scope.Sort}}
			if cfg != nil {
				p.legacyCursor = cfg.CreateBackend == "postgres" && cfg.PostgresCursorVersion != 2
			}
		}
		if q.Replies && q.Expanded {
			offset = min(5, len(p.snapshot.IDs))
		}
	}
	if locate != "" {
		// Keep author/image/target dependencies even when projection itself
		// produces a private error before yielding a visible comment.
		p.dependencies = append(p.dependencies, uuid.MustParse(locate))
		row, e := readTextComment(ctx, tx, locate, s.account)
		if errors.Is(e, pgx.ErrNoRows) || e == nil && row.PostID != post {
			return p, postError("COMMENT_NOT_FOUND", 404)
		}
		if e != nil {
			return p, e
		}
		floor := row.ID
		if row.RootCommentID != nil {
			floor = *row.RootCommentID
			p.dependencies = append(p.dependencies, uuid.MustParse(floor))
		}
		found := false
		for i, id := range p.snapshot.IDs {
			if id.String() == floor {
				n := i / 50 * 50
				p.next = &n
				found = true
				break
			}
		}
		if !found {
			return p, postError("COMMENT_NOT_FOUND", 404)
		}
		p.value = CommentLocation{CommentID: locate, RootCommentID: row.RootCommentID}
	} else {
		limit := q.Limit
		if q.Replies && !q.Expanded {
			limit = 5
			if offset != 0 {
				return p, postError("CURSOR_INVALID", 400)
			}
		}
		page := TextCommentPage{Comments: []TextComment{}}
		scans := 3
		if cfg == nil {
			scans = 1
		}
		for scanned := 0; scanned < scans && offset < len(p.snapshot.IDs); scanned++ {
			end := min(offset+limit, len(p.snapshot.IDs))
			for _, id := range p.snapshot.IDs[offset:end] {
				p.scanned = append(p.scanned, id)
				p.dependencies = append(p.dependencies, id)
				item, e := readTextComment(ctx, tx, id.String(), s.account)
				if errors.Is(e, pgx.ErrNoRows) {
					continue
				}
				if e != nil {
					return p, e
				}
				if cfg != nil && (item.PostID != post || root == uuid.Nil && item.RootCommentID != nil || root != uuid.Nil && (item.RootCommentID == nil || *item.RootCommentID != root.String())) {
					return p, postError("COMMENT_SNAPSHOT_INTEGRITY", 503)
				}
				page.Comments = append(page.Comments, item)
				if item.RootCommentID == nil {
					p.countRoots = append(p.countRoots, id)
				}
			}
			offset = end
			if len(page.Comments) > 0 {
				break
			}
		}
		if offset < len(p.snapshot.IDs) {
			n := offset
			p.next = &n
		}
		if q.Replies {
			remaining := len(p.snapshot.IDs) - offset
			page.RootCommentID = &q.ID
			page.RemainingCount = &remaining
		} else {
			counts, e := readVisibleCommentCounts(ctx, tx, s.account, []string{post})
			if e != nil {
				return p, e
			}
			count := counts[post]
			page.TotalCount = &count
		}
		p.value = page
	}
	if !p.create && p.snapshot.Ref.Backend == commentsnapshot.Redis {
		var e error
		p.loadedProof, e = c.comments.snapshotStore.ProveLoaded(p.snapshot)
		if e != nil {
			return p, commentSnapshotError(e)
		}
	}
	var e error
	p.versions, e = p.readVersions(ctx, tx, s.account)
	return p, e
}

func (c *Community) finalizeCommentRead(ctx context.Context, tx pgx.Tx, a uuid.UUID, d AuthorizationDecision, p *commentReadPlan, nonce []byte, reservations *[]commentsnapshot.Prepared) (any, error) {
	versions, e := p.readVersions(ctx, tx, a)
	if e != nil {
		return nil, e
	}
	if !bytes.Equal(versions, p.versions) {
		return nil, errReadConflict
	}
	if !p.create {
		if !p.snapshot.Ref.Expires.After(d.TrustedAt) {
			return nil, postError("CURSOR_INVALID", 400)
		}
		if p.snapshot.Ref.Backend == commentsnapshot.Redis {
			if e = c.comments.snapshotStore.ValidateLoaded(ctx, p.loadedProof, d.TrustedAt); e != nil {
				return nil, commentSnapshotError(e)
			}
		} else {
			var root any
			if p.snapshot.Scope.Root != uuid.Nil {
				root = p.snapshot.Scope.Root
			}
			var exists bool
			e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM c_comments.snapshots WHERE snapshot_id=$1 AND owner_account_id=$2 AND post_id=$3 AND root_comment_id IS NOT DISTINCT FROM $4::uuid AND expires_at=$5 AND expires_at>$6 AND cardinality(comment_ids)=$7)`, p.snapshot.Ref.ID, a, p.snapshot.Scope.Post, root, p.snapshot.Ref.Expires, d.TrustedAt, len(p.snapshot.IDs)).Scan(&exists)
			if e != nil {
				return nil, e
			}
			if !exists {
				return nil, postError("CURSOR_INVALID", 400)
			}
		}
	} else {
		at := d.TrustedAt.UTC()
		ttl := 5 * time.Minute
		if p.config != nil {
			at = at.Truncate(time.Millisecond)
			ttl = 30 * time.Minute
		}
		p.snapshot.Created = at
		p.snapshot.Ref.Expires = at.Add(ttl)
		if p.config == nil {
			if _, e = tx.Exec(ctx, `DELETE FROM c_comments.snapshots WHERE owner_account_id=$1 AND expires_at<=$2`, a, d.TrustedAt); e != nil {
				return nil, e
			}
			var n int
			if e = tx.QueryRow(ctx, `SELECT count(*) FROM c_comments.snapshots WHERE owner_account_id=$1 AND expires_at>$2`, a, d.TrustedAt).Scan(&n); e != nil {
				return nil, e
			}
			if n >= 128 {
				return nil, &PostError{Code: "RATE_LIMITED", Status: 429, RetryAfter: 60}
			}
		} else {
			cfg := p.config
			legacy := commentsnapshot.LegacyUsage{}
			if cfg.ReadPostgres {
				legacy, e = c.legacySnapshotUsage(ctx, tx, a, d.TrustedAt)
				if e != nil {
					return nil, e
				}
			}
			if cfg.CreateBackend == "redis" {
				reservation, pe := c.comments.snapshotStore.Prepare(ctx, p.snapshot, commentsnapshot.Request(c.comments.codec.IntentFingerprint, p.snapshot.Scope.Owner, p.snapshot.Scope, nonce), legacy)
				if reservation.NewlyCreated {
					*reservations = append(*reservations, reservation)
				}
				if pe != nil {
					return nil, commentSnapshotError(pe)
				}
				if !reservation.NewlyCreated {
					return nil, errReadConflict
				}
				p.snapshot = reservation.Snapshot
			} else {
				if _, e = commentsnapshot.Encode(c.comments.codec.IntentFingerprint, p.snapshot, cfg.MaxSnapshotBytes); e != nil {
					return nil, commentSnapshotError(e)
				}
				usage := commentsnapshot.Usage{}
				if cfg.ReadRedis {
					usage, e = c.comments.snapshotStore.Usage(ctx, p.snapshot.Scope.Owner)
					if e != nil {
						return nil, commentSnapshotError(e)
					}
				}
				if legacy.Count+usage.AccountCount >= cfg.MaxPerAccount {
					wait := usage.RetryAfter
					if legacy.Count > 0 {
						wait = legacy.RetryAfter
					}
					if legacy.Count > 0 && usage.AccountCount > 0 {
						wait = min(wait, usage.RetryAfter)
					}
					return nil, &PostError{Code: "RATE_LIMITED", Status: 429, RetryAfter: max(1, wait)}
				}
				if _, e = tx.Exec(ctx, `DELETE FROM c_comments.snapshots WHERE owner_account_id=$1 AND expires_at<=$2`, a, d.TrustedAt); e != nil {
					return nil, e
				}
			}
		}
		if p.config == nil || p.config.CreateBackend != "redis" {
			var root any
			if p.snapshot.Scope.Root != uuid.Nil {
				root = p.snapshot.Scope.Root
			}
			if _, e = tx.Exec(ctx, `INSERT INTO c_comments.snapshots(snapshot_id,owner_account_id,post_id,root_comment_id,comment_ids,expires_at) VALUES($1,$2,$3,$4,$5,$6)`, p.snapshot.Ref.ID, a, p.snapshot.Scope.Post, root, p.snapshot.IDs, p.snapshot.Ref.Expires); e != nil {
				return nil, e
			}
		}
	}
	if len(p.scanned) > 150 {
		return nil, ErrAuthorizationUnavailable
	}
	if page, ok := p.value.(TextCommentPage); ok {
		if len(p.scanned) > 0 {
			rows, e := tx.Query(ctx, `SELECT v.comment_id,v.like_count,EXISTS(SELECT 1 FROM c_comments.likes l WHERE l.account_id=$2 AND l.comment_id=v.comment_id) FROM c_comments.read_comment_versions v WHERE v.comment_id=ANY($1::uuid[])`, p.scanned, a)
			if e != nil {
				return nil, e
			}
			type likes struct {
				n    int64
				mine bool
			}
			current := map[uuid.UUID]likes{}
			for rows.Next() {
				var id uuid.UUID
				var l likes
				if e = rows.Scan(&id, &l.n, &l.mine); e != nil {
					rows.Close()
					return nil, e
				}
				current[id] = l
			}
			e = rows.Err()
			rows.Close()
			if e != nil {
				return nil, e
			}
			for i := range page.Comments {
				l, found := current[uuid.MustParse(page.Comments[i].ID)]
				if !found {
					return nil, ErrAuthorizationUnavailable
				}
				page.Comments[i].LikeCount = l.n
				page.Comments[i].LikedByMe = l.mine
			}
		}
		page.SnapshotID = base64.RawURLEncoding.EncodeToString(p.snapshot.Ref.ID[:])
		return page, nil
	}
	location := p.value.(CommentLocation)
	location.SnapshotID = base64.RawURLEncoding.EncodeToString(p.snapshot.Ref.ID[:])
	return location, nil
}
