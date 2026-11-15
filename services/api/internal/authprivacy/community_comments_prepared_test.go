package authprivacy

import (
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/comments"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/commentsnapshot"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/readtiming"
)

func preparedComment(t *testing.T, l *lab, b [32]byte, post string, identity uuid.UUID, root string) string {
	t.Helper()
	task := uuid.NewString()
	commentTextSend(t, l, b, postTestKey(t), comments.Intent{Operation: comments.Create, TaskID: task, PostID: post, IdentityID: identity.String(), RootCommentID: root, TargetCommentID: root, Body: "prepared body"})
	commentTextPublish(t, l)
	return *commentTextOwn(t, l, b, task).PublishedCommentID
}

type preparedReadTracer struct {
	calls  int
	change func(int)
	prefix string
}

func (q *preparedReadTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	prefix := q.prefix
	if prefix == "" {
		prefix = "SELECT state,session_generation FROM c_auth.accounts"
	}
	if strings.HasPrefix(d.SQL, prefix) {
		q.calls++
		q.change(q.calls)
	}
	return ctx
}

func TestCommentPreparedPrivateErrorAuthorizationPostgres(t *testing.T) {
	for _, kind := range []string{"missing_post", "missing_snapshot"} {
		t.Run(kind, func(t *testing.T) {
			l := commentTextLab(t)
			l.c.comments.preparedReads = true
			reader := sessionTestAccount(t, l, "prepared_private_error")
			identity := identityChange(t, l.c, reader.SessionToken, "CREATE", uuid.Nil, "错误作者").IdentityID
			post := commentTextPost(t, l, reader.SessionToken, identity)
			q := CommentQuery{ID: post}
			if kind == "missing_post" {
				q.ID = uuid.NewString()
			} else {
				q.Cursor = l.c.commentCursor(uuid.New(), 0)
			}
			original := *l.c
			preparedReadPool(t, l, func(n int) {
				if n == 1 {
					if e := original.RevokeSession(context.Background(), reader.RevokeSecret); e != nil {
						t.Fatal(e)
					}
				}
			})
			r, e := l.c.ReadComments(context.Background(), reader.SessionToken, q)
			if !errors.Is(e, ErrSessionInvalid) || r.Value != nil {
				t.Fatal("private error bypassed final authorization", e)
			}
		})
	}
}

func TestCommentPreparedErrorContextRebuildPostgres(t *testing.T) {
	l := commentTextLab(t)
	l.c.comments.preparedReads = true
	reader := sessionTestAccount(t, l, "prepared_context")
	identity := identityChange(t, l.c, reader.SessionToken, "CREATE", uuid.Nil, "恢复作者").IdentityID
	post := commentTextPost(t, l, reader.SessionToken, identity)
	preparedComment(t, l, reader.SessionToken, post, identity, "")
	mustExec(t, l.cp, `UPDATE c_posts.posts SET moderation_hidden=true WHERE post_id=$1`, post)
	preparedReadPool(t, l, func(n int) {
		if n == 1 {
			mustExec(t, l.cp, `UPDATE c_posts.posts SET moderation_hidden=false WHERE post_id=$1`, post)
		}
	})
	r, e := l.c.ReadComments(context.Background(), reader.SessionToken, CommentQuery{ID: post})
	if e != nil || len(r.Value.(TextCommentPage).Comments) != 1 {
		t.Fatal("stale private 404 was final", e)
	}
}

func TestCommentPreparedLocateErrorRebuildPostgres(t *testing.T) {
	l := commentTextLab(t)
	l.c.comments.preparedReads = true
	reader := sessionTestAccount(t, l, "prepared_locate_error")
	identity := identityChange(t, l.c, reader.SessionToken, "CREATE", uuid.Nil, "定位作者").IdentityID
	post := commentTextPost(t, l, reader.SessionToken, identity)
	root := preparedComment(t, l, reader.SessionToken, post, identity, "")
	reply := preparedComment(t, l, reader.SessionToken, post, identity, root)
	mustExec(t, l.cp, `UPDATE c_comments.tasks SET moderation_hidden=true WHERE comment_id=$1`, reply)
	calls := 0
	preparedReadPool(t, l, func(n int) {
		calls = n
		if n == 1 {
			mustExec(t, l.cp, `UPDATE c_comments.tasks SET moderation_hidden=false WHERE comment_id=$1`, reply)
		}
	})
	r, e := l.c.LocateComment(context.Background(), reader.SessionToken, post, reply)
	if e != nil {
		t.Fatal("hidden reply's old 404 survived restore", e)
	}
	location := r.Value.(CommentLocation)
	if calls != 2 || location.CommentID != reply || location.RootCommentID == nil || *location.RootCommentID != root || location.Cursor == nil {
		t.Fatal("locate error did not rebuild its resource binding", location, calls)
	}
}

func TestCommentPreparedCustomGateFallbackPostgres(t *testing.T) {
	l := commentTextLab(t)
	l.c.comments.preparedReads = true
	reader := sessionTestAccount(t, l, "prepared_custom")
	identity := identityChange(t, l.c, reader.SessionToken, "CREATE", uuid.Nil, "兼容作者").IdentityID
	post := commentTextPost(t, l, reader.SessionToken, identity)
	gate := &postSessionCommitGate{AuthorizationGate: l.gate}
	l.c.gate = gate
	ctx, trace := readtiming.Start(context.Background())
	_, e := l.c.ReadComments(ctx, reader.SessionToken, CommentQuery{ID: post})
	if e != nil || gate.snapshots != 1 || gate.commits != 1 || trace.Snapshot().Stages["prepare_read"].Calls != 0 {
		t.Fatal("custom Gate did not retain old contract", e)
	}
}

func TestCommentPreparedRedisPanicCompensationPostgres(t *testing.T) {
	l := commentTextLab(t)
	l.c.comments.preparedReads = true
	reader := sessionTestAccount(t, l, "prepared_panic")
	identity := identityChange(t, l.c, reader.SessionToken, "CREATE", uuid.Nil, "补偿作者").IdentityID
	post := commentTextPost(t, l, reader.SessionToken, identity)
	preparedComment(t, l, reader.SessionToken, post, identity, "")
	store, cfg := snapshotSQLStore(t, l)
	sentinel := errors.New("prepared final panic")
	config := l.cp.Config()
	config.ConnConfig.Tracer = &preparedReadTracer{prefix: "SELECT v.comment_id,v.like_count", change: func(int) { panic(sentinel) }}
	pool, e := pgxpool.NewWithConfig(context.Background(), config)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(pool.Close)
	l.c.pool = pool
	var caught any
	func() {
		defer func() { caught = recover() }()
		_, _ = l.c.ReadComments(context.Background(), reader.SessionToken, CommentQuery{ID: post})
	}()
	if caught != sentinel {
		t.Fatal("panic lost", caught)
	}
	owner := commentsnapshot.OwnerToken(l.c.comments.codec.IntentFingerprint, reader.AccountID, cfg.Redis.Namespace)
	usage, e := store.Usage(context.Background(), owner)
	if e != nil || usage.AccountCount != 0 {
		t.Fatal("panic leaked a new reservation", e, usage)
	}
}

func TestCommentPreparedFinalFencePostgres(t *testing.T) {
	l := commentTextLab(t)
	l.c.comments.preparedReads = true
	owner := sessionTestAccount(t, l, "prepared_fence_owner")
	reader := sessionTestAccount(t, l, "prepared_fence_reader")
	identity := identityChange(t, l.c, owner.SessionToken, "CREATE", uuid.Nil, "围栏作者").IdentityID
	post := commentTextPost(t, l, owner.SessionToken, identity)
	id := preparedComment(t, l, owner.SessionToken, post, identity, "")
	original := *l.c
	done := make(chan error, 1)
	blocked := false
	config := l.cp.Config()
	config.ConnConfig.Tracer = &preparedReadTracer{prefix: "SELECT v.comment_id,v.like_count", change: func(n int) {
		if n != 1 {
			return
		}
		go func() {
			_, e := original.ExecuteCommentCommand(context.Background(), owner.SessionToken, postTestKey(t), comments.Intent{Operation: comments.DeleteComment, CommentID: id})
			done <- e
		}()
		waitLock(t, l.cp, "transactionid", 1)
		blocked = true
		select {
		case e := <-done:
			t.Fatal("writer passed final Gate fence", e)
		default:
		}
	}}
	pool, e := pgxpool.NewWithConfig(context.Background(), config)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(pool.Close)
	l.c.pool = pool
	r, e := l.c.ReadComments(context.Background(), reader.SessionToken, CommentQuery{ID: post})
	if e != nil || !blocked || len(r.Value.(TextCommentPage).Comments) != 1 {
		t.Fatal("serialized read failed", e)
	}
	select {
	case e := <-done:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("fenced writer did not resume")
	}
	r, e = l.c.ReadComments(context.Background(), reader.SessionToken, CommentQuery{ID: post})
	if e != nil || len(r.Value.(TextCommentPage).Comments) != 0 {
		t.Fatal("committed deletion escaped version fence", e)
	}
}

func TestCommentPreparedFrozenPagingPostgres(t *testing.T) {
	l := commentTextLab(t)
	l.c.comments.preparedReads = true
	owner := sessionTestAccount(t, l, "prepared_frozen")
	identity := identityChange(t, l.c, owner.SessionToken, "CREATE", uuid.Nil, "冻结顺序").IdentityID
	post := commentTextPost(t, l, owner.SessionToken, identity)
	for i := 0; i < 6; i++ {
		preparedComment(t, l, owner.SessionToken, post, identity, "")
	}
	cfg := commentsnapshot.DefaultConfig()
	cfg.CreateBackend = "postgres"
	cfg.ReadRedis = false
	cfg.PostgresCursorVersion = 2
	l.c.comments.snapshotConfig = &cfg
	first := snapshotRead(t, l, owner.SessionToken, CommentQuery{ID: post, Limit: 1})
	ref, _, e := commentsnapshot.DecodeCursor(l.c.comments.codec.IntentFingerprint, *first.NextCursor)
	if e != nil {
		t.Fatal(e)
	}
	var ids []uuid.UUID
	if e = l.cp.QueryRow(context.Background(), `SELECT comment_ids FROM c_comments.snapshots WHERE snapshot_id=$1`, ref.ID).Scan(&ids); e != nil {
		t.Fatal(e)
	}
	for _, id := range ids[1:4] {
		mustExec(t, l.cp, `UPDATE c_comments.tasks SET moderation_hidden=true WHERE comment_id=$1`, id)
	}
	preparedComment(t, l, owner.SessionToken, post, identity, "") // must not enter the frozen order
	empty := snapshotRead(t, l, owner.SessionToken, CommentQuery{ID: post, Cursor: *first.NextCursor, Limit: 1})
	if len(empty.Comments) != 0 || empty.NextCursor == nil {
		t.Fatal("three filtered blocks ended frozen snapshot early")
	}
	last := snapshotRead(t, l, owner.SessionToken, CommentQuery{ID: post, Cursor: *empty.NextCursor, Limit: 1})
	if len(last.Comments) != 1 || last.Comments[0].ID != ids[4].String() {
		t.Fatal("frozen order changed")
	}
	original := *l.c
	target := ids[5].String()
	preparedReadPool(t, l, func(n int) {
		if n == 1 {
			yes := true
			commentTextSend(t, &lab{c: &original}, owner.SessionToken, postTestKey(t), comments.Intent{Operation: comments.SetLike, CommentID: target, Liked: &yes})
		}
	})
	ctx, trace := readtiming.Start(context.Background())
	r, e := l.c.ReadComments(ctx, owner.SessionToken, CommentQuery{ID: post, Cursor: *last.NextCursor, Limit: 1})
	if e != nil {
		t.Fatal(e)
	}
	page := r.Value.(TextCommentPage)
	if len(page.Comments) != 1 || page.Comments[0].ID != target || page.Comments[0].LikeCount != 1 || !page.Comments[0].LikedByMe || page.NextCursor != nil {
		t.Fatal("final likes/frozen end changed")
	}
	if trace.Snapshot().Stages["comments.prepared_read_conflict"].Calls != 0 {
		t.Fatal("frozen snapshot compared historical likes order")
	}
}

func TestCommentPreparedAuthorityChangesPostgres(t *testing.T) {
	for _, change := range []string{"session_expiry", "gate_expiry", "gate_generation", "ban", "closure"} {
		t.Run(change, func(t *testing.T) {
			control := newGateControl(t)
			l := control.lab
			enablePostLab(t, l)
			cfg := CommentConfig{PreparedReads: true}
			cfg.KeyKey, _ = random32()
			cfg.FingerprintKey, _ = random32()
			var e error
			l.c, e = l.c.WithComments(context.Background(), cfg)
			if e != nil {
				t.Fatal(e)
			}
			owner := sessionTestAccount(t, l, "prepared_authority_owner")
			reader := sessionTestAccount(t, l, "prep_authority_reader")
			identity := identityChange(t, l.c, owner.SessionToken, "CREATE", uuid.Nil, "权限作者").IdentityID
			post := commentTextPost(t, l, owner.SessionToken, identity)
			preparedComment(t, l, owner.SessionToken, post, identity, "")
			original := *l.c
			token := sha256.Sum256(reader.SessionToken[:])
			if change == "session_expiry" {
				mustExec(t, l.cp, `UPDATE c_auth.sessions SET expires_at=$2 WHERE token_digest=$1`, token[:], control.clock.now().Add(time.Minute))
			}
			var expiry time.Time
			if e = l.cp.QueryRow(context.Background(), `SELECT expires_at FROM c_auth.sessions WHERE token_digest=$1`, token[:]).Scan(&expiry); e != nil {
				t.Fatal(e)
			}
			passwords := sessionTestPasswordWorker(t, 2)
			preparedReadPool(t, l, func(n int) {
				if n != 1 {
					return
				}
				switch change {
				case "session_expiry":
					control.clock.set(control.clock.now().Add(2 * time.Minute))
				case "gate_expiry":
					control.clock.set(control.clock.now().Add(5*time.Minute + time.Nanosecond))
				case "gate_generation":
					if e := l.gate.Freeze(context.Background(), "prepared generation"); e != nil {
						t.Fatal(e)
					}
					control.recover(t, AuthorizationRecoveryBreakGlass, 3, 3)
				case "ban":
					if e := original.ApplyAccountBan(context.Background(), reader.AccountID, 1, nil); e != nil {
						t.Fatal(e)
					}
				case "closure":
					request, _ := closureTestRequest(t, reader.SessionToken)
					if _, e := original.RequestAccountClosure(context.Background(), request, passwords); e != nil {
						t.Fatal(e)
					}
				}
			})
			r, e := l.c.ReadComments(context.Background(), reader.SessionToken, CommentQuery{ID: post})
			want := ErrSessionInvalid
			if change == "gate_expiry" || change == "gate_generation" {
				want = ErrAuthorizationUnavailable
			}
			if !errors.Is(e, want) || r.Value != nil {
				t.Fatal("changed authority released prepared body", e)
			}
			var after time.Time
			if qe := l.cp.QueryRow(context.Background(), `SELECT expires_at FROM c_auth.sessions WHERE token_digest=$1`, token[:]).Scan(&after); qe != nil || !after.Equal(expiry) {
				t.Fatal("authority denial renewed prepared reader", qe)
			}
		})
	}
}
func (*preparedReadTracer) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}
func preparedReadPool(t *testing.T, l *lab, change func(int)) {
	t.Helper()
	tracer := &preparedReadTracer{change: change}
	cfg := l.cp.Config()
	cfg.ConnConfig.Tracer = tracer
	pool, e := pgxpool.NewWithConfig(context.Background(), cfg)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(pool.Close)
	l.c.pool = pool
}

func TestCommentPreparedPagesPostgres(t *testing.T) {
	for _, backend := range []string{"legacy", "postgres", "redis"} {
		t.Run(backend, func(t *testing.T) {
			l := commentTextLab(t)
			l.c.comments.preparedReads = true
			owner := sessionTestAccount(t, l, "prepared_owner")
			identity := identityChange(t, l.c, owner.SessionToken, "CREATE", uuid.Nil, "准备作者").IdentityID
			post := commentTextPost(t, l, owner.SessionToken, identity)
			root := preparedComment(t, l, owner.SessionToken, post, identity, "")
			for i := 0; i < 7; i++ {
				preparedComment(t, l, owner.SessionToken, post, identity, root)
			}
			if backend == "redis" {
				snapshotSQLStore(t, l)
			} else if backend == "postgres" {
				cfg := commentsnapshot.DefaultConfig()
				cfg.CreateBackend = "postgres"
				cfg.ReadRedis = false
				cfg.PostgresCursorVersion = 2
				l.c.comments.snapshotConfig = &cfg
			}
			ctx, trace := readtiming.Start(context.Background())
			r, e := l.c.ReadComments(ctx, owner.SessionToken, CommentQuery{ID: post, Limit: 1})
			if e != nil {
				t.Fatal(e)
			}
			page := r.Value.(TextCommentPage)
			if len(page.Comments) != 1 || page.Comments[0].ReplyCount != 7 || *page.TotalCount != 8 {
				t.Fatalf("bad prepared main: %+v", page)
			}
			if trace.Snapshot().Stages["comments.anchor_write"].Calls != 1 || trace.Snapshot().Stages["comments.sql_commit"].Calls != 1 {
				t.Fatal("prepared read checkpoint count", trace.Snapshot().Stages)
			}
			preview := snapshotRead(t, l, owner.SessionToken, CommentQuery{ID: root, PostID: post, Replies: true})
			if len(preview.Comments) != 5 || preview.NextCursor == nil || *preview.RemainingCount != 2 {
				t.Fatal("preview changed")
			}
			rest := snapshotRead(t, l, owner.SessionToken, CommentQuery{ID: root, PostID: post, Replies: true, Expanded: true, Cursor: *preview.NextCursor})
			if len(rest.Comments) != 2 || rest.NextCursor != nil || *rest.RemainingCount != 0 {
				t.Fatal("continuation changed")
			}
			location, e := l.c.LocateComment(context.Background(), owner.SessionToken, post, rest.Comments[0].ID)
			if e != nil || location.Value.(CommentLocation).Cursor == nil {
				t.Fatal("locate", e)
			}
			if backend == "redis" {
				nonce := postTestKey(t)
				first := snapshotRead(t, l, owner.SessionToken, CommentQuery{ID: post, SnapshotRequest: nonce})
				again := snapshotRead(t, l, owner.SessionToken, CommentQuery{ID: post, SnapshotRequest: nonce})
				if first.SnapshotID != again.SnapshotID {
					t.Fatal("nonce changed")
				}
			}
			l.c.comments.preparedReads = false
			if _, e = l.c.ReadComments(context.Background(), owner.SessionToken, CommentQuery{ID: root, PostID: post, Replies: true, Expanded: true, Cursor: *preview.NextCursor}); e != nil {
				t.Fatal("old-path rollback lost cursor", e)
			}
		})
	}
}

func TestCommentPreparedInterleavingPostgres(t *testing.T) {
	for _, change := range []string{"revoke", "freeze", "generation", "hide", "delete", "post_hidden", "block", "new_candidate", "edit", "twice", "missing_version"} {
		t.Run(change, func(t *testing.T) {
			l := commentTextLab(t)
			l.c.comments.preparedReads = true
			owner := sessionTestAccount(t, l, "prepared_race_owner")
			reader := sessionTestAccount(t, l, "prepared_race_reader")
			identity := identityChange(t, l.c, owner.SessionToken, "CREATE", uuid.Nil, "交错作者").IdentityID
			post := commentTextPost(t, l, owner.SessionToken, identity)
			id := preparedComment(t, l, owner.SessionToken, post, identity, "")
			original := *l.c
			token := sha256.Sum256(reader.SessionToken[:])
			var oldExpiry time.Time
			if e := l.cp.QueryRow(context.Background(), `SELECT expires_at FROM c_auth.sessions WHERE token_digest=$1`, token[:]).Scan(&oldExpiry); e != nil {
				t.Fatal(e)
			}
			calls := 0
			preparedReadPool(t, l, func(n int) {
				calls = n
				if n > 1 && change != "twice" {
					return
				}
				switch change {
				case "revoke":
					if e := original.RevokeSession(context.Background(), reader.RevokeSecret); e != nil {
						t.Fatal(e)
					}
				case "freeze":
					if e := l.gate.Freeze(context.Background(), "prepared race"); e != nil {
						t.Fatal(e)
					}
				case "generation":
					mustExec(t, l.cp, `UPDATE c_auth.accounts SET session_generation=session_generation+1 WHERE account_id=$1`, reader.AccountID)
				case "hide":
					mustExec(t, l.cp, `UPDATE c_comments.tasks SET moderation_hidden=true WHERE comment_id=$1`, id)
				case "post_hidden":
					mustExec(t, l.cp, `UPDATE c_posts.posts SET moderation_hidden=true WHERE post_id=$1`, post)
				case "block":
					if _, e := original.BlockPostAuthor(context.Background(), reader.SessionToken, post); e != nil {
						t.Fatal(e)
					}
				case "delete":
					commentTextSend(t, &lab{c: &original}, owner.SessionToken, postTestKey(t), comments.Intent{Operation: comments.DeleteComment, CommentID: id})
				case "new_candidate":
					preparedComment(t, &lab{c: &original}, owner.SessionToken, post, identity, "")
				case "edit":
					identityChange(t, &original, owner.SessionToken, "RENAME", identity, "更新作者")
				case "twice":
					mustExec(t, l.cp, `UPDATE c_comments.tasks SET moderation_hidden=NOT moderation_hidden WHERE comment_id=$1`, id)
				case "missing_version":
					mustExec(t, l.cp, `DELETE FROM c_comments.read_comment_versions WHERE comment_id=$1`, id)
				}
			})
			r, e := l.c.ReadComments(context.Background(), reader.SessionToken, CommentQuery{ID: post})
			switch change {
			case "revoke", "generation":
				if !errors.Is(e, ErrSessionInvalid) || r.Value != nil {
					t.Fatal("session race released", e)
				}
			case "freeze", "missing_version":
				if !errors.Is(e, ErrAuthorizationUnavailable) || r.Value != nil {
					t.Fatal("authority race released", e)
				}
			case "post_hidden", "block":
				postTestError(t, e, "POST_NOT_FOUND", 404)
			case "twice":
				postTestError(t, e, "COMMENT_SNAPSHOT_UNAVAILABLE", 503)
				if calls != 2 {
					t.Fatal("unbounded rebuild", calls)
				}
			default:
				if e != nil {
					t.Fatal(e)
				}
				page := r.Value.(TextCommentPage)
				if calls != 2 {
					t.Fatal("did not rebuild", calls)
				}
				switch change {
				case "hide", "delete":
					if len(page.Comments) != 0 || *page.TotalCount != 0 {
						t.Fatal("deleted projection escaped")
					}
				case "new_candidate":
					if len(page.Comments) != 2 || *page.TotalCount != 2 {
						t.Fatal("new candidate silently lost")
					}
				case "edit":
					if page.Comments[0].Author.Nickname != "更新作者" {
						t.Fatal("old author escaped")
					}
				}
			}
			if e != nil {
				var expiry time.Time
				if qe := l.cp.QueryRow(context.Background(), `SELECT expires_at FROM c_auth.sessions WHERE token_digest=$1`, token[:]).Scan(&expiry); qe != nil || !expiry.Equal(oldExpiry) {
					t.Fatal("denial renewed session", qe)
				}
			}
			if change == "twice" {
				var state string
				if e := l.cp.QueryRow(context.Background(), `SELECT gate_state FROM c_auth.authorization_gate`).Scan(&state); e != nil || state != "OPEN" {
					t.Fatal("normal conflicts froze gate", e, state)
				}
			}
		})
	}
}
