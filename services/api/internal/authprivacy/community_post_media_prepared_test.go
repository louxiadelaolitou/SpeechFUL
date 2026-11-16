package authprivacy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/postmedia"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/posts"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/readtiming"
)

type preparedMediaFixture struct {
	l             *lab
	config        PostConfig
	owner, viewer SignupResult
	post, content string
	data          []byte
}

func newPreparedMediaFixture(t *testing.T) preparedMediaFixture {
	t.Helper()
	return newPreparedMediaPostFixture(t, true)
}

func newPreparedMediaPostFixture(t *testing.T, publish bool) preparedMediaFixture {
	t.Helper()
	l := newLab(t)
	config := enablePostLab(t, l)
	config.PreparedMediaReads = true
	var err error
	l.c, err = l.c.WithPosts(context.Background(), config)
	if err != nil {
		t.Fatal("prepared media must pass its real startup schema checks", err)
	}
	enableImageLab(t, l)
	owner := sessionTestAccount(t, l, "prepared_media_owner")
	viewer := sessionTestAccount(t, l, "prepared_media_reader")
	id := identityChange(t, l.c, owner.SessionToken, "CREATE", uuid.Nil, "媒体作者").IdentityID
	intent := postTestIntent(t, l, id)
	selection, data := imageFixture(t, "png")
	intent.Images = []postmedia.Selection{selection}
	accepted := postTestCommand(t, l, owner.SessionToken, postTestKey(t), intent)
	grant := imageGrant(t, l, owner.SessionToken, accepted, selection.MediaID)
	imageUpload(t, l, accepted, selection, grant, data)
	if !publish {
		var content string
		if err = l.cp.QueryRow(context.Background(), `SELECT content_version_id::text FROM c_posts.images WHERE task_id=$1 AND version=1`, accepted.TaskID).Scan(&content); err != nil {
			t.Fatal(err)
		}
		return preparedMediaFixture{l: l, config: config, owner: owner, viewer: viewer, post: accepted.PostID, content: content, data: data}
	}
	postTestPublish(t, l)
	r, err := l.c.ReadPostImages(context.Background(), viewer.SessionToken, accepted.PostID, "")
	if err != nil {
		t.Fatal(err)
	}
	return preparedMediaFixture{l: l, config: config, owner: owner, viewer: viewer, post: accepted.PostID,
		content: r.Value.(postmedia.Group).Images[0].ContentVersionID, data: data}
}

func readPreparedMediaRoute(ctx context.Context, f preparedMediaFixture, route string) (PostResponse, error) {
	switch route {
	case "manifest":
		return f.l.c.ReadPostImages(ctx, f.viewer.SessionToken, f.post, "")
	case "original":
		return f.l.c.ReadPostImages(ctx, f.viewer.SessionToken, f.post, f.content)
	default:
		return f.l.c.ReadPostThumbnail(ctx, f.viewer.SessionToken, f.post, f.content)
	}
}

func TestPreparedPostMediaPrivatePublicationStatesMatchLegacyPostgres(t *testing.T) {
	for _, state := range []string{"accepted", "failed", "cancelled", "deleted"} {
		t.Run(state, func(t *testing.T) {
			f := newPreparedMediaPostFixture(t, state == "deleted")
			if state == "failed" || state == "cancelled" {
				var task string
				if err := f.l.cp.QueryRow(context.Background(), `SELECT task_id::text FROM c_posts.publication_tasks WHERE post_id=$1`, f.post).Scan(&task); err != nil {
					t.Fatal(err)
				}
				if state == "cancelled" {
					postTestCommand(t, f.l, f.owner.SessionToken, postTestKey(t), posts.Intent{Operation: posts.Cancel, TaskID: task, ExpectedAttemptVersion: 1})
				} else {
					// The same legal terminal transition used by the publication
					// worker, inside its final business Gate transaction.
					_, err := f.l.c.withPostSession(context.Background(), f.owner.SessionToken,
						func(tx pgx.Tx, _ uuid.UUID, d AuthorizationDecision, _ restriction) (any, error) {
							_, e := tx.Exec(context.Background(), `UPDATE c_posts.publication_attempts SET state='FAILED',terminal_at=$2,failure_code='PUBLICATION_FAILED' WHERE task_id=$1 AND version=1`, task, d.TrustedAt)
							return nil, e
						})
					if err != nil {
						t.Fatal(err)
					}
				}
			} else if state == "deleted" {
				postTestCommand(t, f.l, f.owner.SessionToken, postTestKey(t), posts.Intent{Operation: posts.DeletePost, PostID: f.post})
			}
			for _, enabled := range []bool{false, true} {
				f.config.PreparedMediaReads = enabled
				var err error
				f.l.c, err = f.l.c.WithPosts(context.Background(), f.config)
				if err != nil {
					t.Fatal(err)
				}
				for _, route := range []string{"manifest", "original", "thumbnail"} {
					r, e := readPreparedMediaRoute(context.Background(), f, route)
					postTestError(t, e, "MEDIA_NOT_FOUND", 404)
					if r.Value != nil || !r.ExpiresAt.IsZero() {
						t.Fatal("private publication state returned content or renewal", state, route, enabled)
					}
				}
			}
		})
	}
}

func TestPreparedPostMediaPublicationChangesRebuildPrivateProofPostgres(t *testing.T) {
	for _, publish := range []bool{true, false} {
		for _, route := range []string{"manifest", "original", "thumbnail"} {
			t.Run(route+map[bool]string{true: "_publish", false: "_delete"}[publish], func(t *testing.T) {
				f := newPreparedMediaPostFixture(t, !publish)
				preparedReadPool(t, f.l, func(n int) {
					if n == 1 {
						if publish {
							postTestPublish(t, f.l)
						} else {
							postTestCommand(t, f.l, f.owner.SessionToken, postTestKey(t), posts.Intent{Operation: posts.DeletePost, PostID: f.post})
						}
					}
				})
				ctx, trace := readtiming.Start(context.Background())
				r, err := readPreparedMediaRoute(ctx, f, route)
				if publish {
					if err != nil || r.Value == nil {
						t.Fatal("stale private absence won over newly published selection", err)
					}
				} else {
					postTestError(t, err, "MEDIA_NOT_FOUND", 404)
					if r.Value != nil || !r.ExpiresAt.IsZero() {
						t.Fatal("deleted media released a private plan")
					}
				}
				if preparedMediaCalls(trace.Snapshot(), "prepared_read_rebuild") != 1 {
					t.Fatal("publication change skipped its one bounded rebuild")
				}
			})
		}
	}
}

func preparedMediaCalls(observation readtiming.Observation, suffix string) int64 {
	var count int64
	for name, stage := range observation.Stages {
		if strings.HasSuffix(name, suffix) {
			count += stage.Calls
		}
	}
	return count
}

type preparedMediaTransform struct {
	postmedia.Validator
	preview postmedia.Thumbnailer
	before  func(context.Context, []byte, int) error
	calls   int
}

func (v *preparedMediaTransform) Thumbnail(ctx context.Context, data []byte) ([]byte, error) {
	v.calls++
	if err := v.before(ctx, data, v.calls); err != nil {
		return nil, err
	}
	return v.preview.Thumbnail(ctx, data)
}

func installPreparedMediaTransform(f preparedMediaFixture, before func(context.Context, []byte, int) error) *preparedMediaTransform {
	v := f.l.c.postImages.validator
	probe := &preparedMediaTransform{Validator: v, preview: v.(postmedia.Thumbnailer), before: before}
	f.l.c.postImages = &postImageServices{validator: probe}
	return probe
}

func editPreparedMedia(t *testing.T, f preparedMediaFixture, ids *[]string) {
	t.Helper()
	r, err := f.l.c.ReadPostEdit(context.Background(), f.owner.SessionToken, f.post)
	if err != nil {
		t.Fatal(err)
	}
	s := r.Value.(posts.EditSnapshot)
	r, err = f.l.c.SavePostEdit(context.Background(), f.owner.SessionToken, f.post, postTestKey(t),
		posts.EditInput{Revision: s.Revision, Title: s.Post.Title + "改", Body: s.Post.Body, ImageIDs: ids})
	if err != nil || r.Value.(posts.EditResult).State != "SAVED" {
		t.Fatal("competing edit failed", err, r.Value)
	}
}

func TestPreparedPostMediaRoutesAndFallbackPostgres(t *testing.T) {
	f := newPreparedMediaFixture(t)
	for _, enabled := range []bool{true, false} {
		f.config.PreparedMediaReads = enabled
		var err error
		f.l.c, err = f.l.c.WithPosts(context.Background(), f.config)
		if err != nil {
			t.Fatal(err)
		}
		for _, route := range []string{"manifest", "original", "thumbnail"} {
			ctx, trace := readtiming.Start(context.Background())
			var r PostResponse
			switch route {
			case "manifest":
				r, err = f.l.c.ReadPostImages(ctx, f.viewer.SessionToken, f.post, "")
				if err == nil {
					group := r.Value.(postmedia.Group)
					if len(group.Images) != 1 || group.Images[0].ContentVersionID != f.content || group.AccessValidUntil == "" {
						t.Fatal("manifest lost its protected selection or lease")
					}
				}
			case "original":
				r, err = f.l.c.ReadPostImages(ctx, f.viewer.SessionToken, f.post, f.content)
				if err == nil && !bytes.Equal(r.Value.(ImageBytes).Bytes, f.data) {
					t.Fatal("original bytes changed or were cleared by Release")
				}
			case "thumbnail":
				r, err = f.l.c.ReadPostThumbnail(ctx, f.viewer.SessionToken, f.post, f.content)
				if err == nil {
					image := r.Value.(ImageBytes)
					if image.MIME != "image/webp" || len(image.Bytes) < 12 || string(image.Bytes[8:12]) != "WEBP" {
						t.Fatal("real bounded thumbnail failed or was cleared by Release")
					}
				}
			}
			if err != nil || r.ExpiresAt.IsZero() {
				t.Fatal(route, enabled, err)
			}
			wantPrepare, wantCommit := int64(0), int64(1)
			if enabled {
				wantPrepare = 1
			} else if route == "thumbnail" {
				wantCommit = 2
			}
			if preparedMediaCalls(trace.Snapshot(), "prepare_read") != wantPrepare || preparedMediaCalls(trace.Snapshot(), "anchor_write") != wantCommit {
				t.Fatal("wrong route or final checkpoint count", route, enabled, trace.Snapshot())
			}
		}
	}
}

func TestPreparedPostMediaRestrictedRuntimePostgres(t *testing.T) {
	f := newPreparedMediaFixture(t)
	mustExec(t, f.l.cp, `CREATE TABLE IF NOT EXISTS public.goose_db_version(version_id bigint,is_applied boolean);
REVOKE CREATE,TEMP ON DATABASE hnuhole_c FROM PUBLIC,hnuhole_c_runtime,hnuhole_c_recovery,hnuhole_business`)
	for _, name := range []string{"grant-community.sql", "grant-comments.sql", "grant-post-images.sql"} {
		body, err := os.ReadFile("../../../../infra/postgres/" + name)
		if err != nil {
			t.Fatal(err)
		}
		mustExec(t, f.l.cp, string(body))
	}
	dsn := os.Getenv("AUTHLAB_POSTS_RUNTIME_DSN")
	if dsn == "" {
		t.Fatal("prepared media requires its owned least-privilege runtime fixture")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	clone := *f.l.c
	clone.pool, clone.postImages = pool, nil
	enabled, err := clone.WithPosts(context.Background(), f.config)
	if err != nil {
		t.Fatal("runtime prepared-media schema checks failed", err)
	}
	enabled, err = enabled.WithPostImages(context.Background(), f.l.c.postImages.validator)
	if err != nil {
		t.Fatal(err)
	}
	enabled, err = enabled.WithPostThumbnailReuse(context.Background())
	if err != nil {
		t.Fatal("restricted role derivative startup", err)
	}
	f.l.c = enabled
	for _, route := range []string{"manifest", "original", "thumbnail"} {
		ctx, trace := readtiming.Start(context.Background())
		r, e := readPreparedMediaRoute(ctx, f, route)
		if e != nil || r.Value == nil || preparedMediaCalls(trace.Snapshot(), "prepare_read") != 1 {
			t.Fatal("runtime media did not use its formally enabled prepared path", route, e)
		}
	}
	_, err = pool.Exec(context.Background(), `UPDATE c_posts.read_feed_post_versions SET projection_revision=projection_revision+1`)
	var denied *pgconn.PgError
	if !errors.As(err, &denied) || denied.Code != "42501" {
		t.Fatal("runtime unexpectedly wrote authoritative read versions")
	}
}

func TestPreparedPostMediaCPUAuthorityAndConflictsPostgres(t *testing.T) {
	for _, mode := range []string{"success", "delete", "delete_cpu_error", "edit_once", "edit_twice", "cancel", "session_generation", "freeze", "commit_ack_loss"} {
		t.Run(mode, func(t *testing.T) {
			f := newPreparedMediaFixture(t)
			ctx, trace := readtiming.Start(context.Background())
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()
			var proxy *commitACKProxy
			if mode == "commit_ack_loss" {
				f.l.c.pool, proxy = newCommitACKProxyPool(t, f.l.cp)
			}
			probe := installPreparedMediaTransform(f, func(cpuCtx context.Context, original []byte, n int) error {
				if cpuCtx.Err() != nil || !bytes.Equal(original, f.data) || f.l.c.pool.Stat().AcquiredConns() != 0 {
					t.Fatal("CPU did not run after RO transaction and connection release")
				}
				switch mode {
				case "delete", "delete_cpu_error":
					postTestCommand(t, f.l, f.owner.SessionToken, postTestKey(t), posts.Intent{Operation: posts.DeletePost, PostID: f.post})
					if mode == "delete_cpu_error" {
						return postmedia.ErrUnavailable
					}
				case "edit_once", "edit_twice":
					if n == 1 || mode == "edit_twice" {
						editPreparedMedia(t, f, nil)
					}
				case "cancel":
					cancel()
				case "session_generation":
					mustExec(t, f.l.cp, `UPDATE c_auth.accounts SET session_generation=session_generation+1 WHERE account_id=$1`, f.viewer.AccountID)
				case "freeze":
					if err := f.l.c.gate.(*PostgresAuthorizationGate).Freeze(context.Background(), "synthetic media CPU freeze"); err != nil {
						t.Fatal(err)
					}
				case "commit_ack_loss":
					proxy.armed.Store(true)
				}
				return nil
			})
			r, err := f.l.c.ReadPostThumbnail(ctx, f.viewer.SessionToken, f.post, f.content)
			success := mode == "success" || mode == "edit_once"
			if success {
				if err != nil || len(r.Value.(ImageBytes).Bytes) < 12 || preparedMediaCalls(trace.Snapshot(), "anchor_write") != 1 {
					t.Fatal("prepared media success failed or used extra final checkpoints", err)
				}
			} else if err == nil || r.Value != nil || !r.ExpiresAt.IsZero() {
				t.Fatal("failed private media released bytes or renewal", err)
			}
			switch mode {
			case "delete", "delete_cpu_error":
				postTestError(t, err, "MEDIA_NOT_FOUND", 404)
			case "edit_twice":
				postTestError(t, err, "SERVICE_UNAVAILABLE", 503)
				if probe.calls != 2 || preparedMediaCalls(trace.Snapshot(), "prepared_read_rebuild") != 1 || count(t, f.l.cp, `SELECT count(*) FROM c_auth.authorization_gate WHERE gate_state='OPEN'`) != 1 {
					t.Fatal("normal conflicts retried without bound or froze the Gate")
				}
			case "session_generation":
				if !errors.Is(err, ErrSessionInvalid) {
					t.Fatal("CPU bytes survived a session change", err)
				}
			case "commit_ack_loss":
				if !errors.Is(err, ErrAuthorizationUnavailable) {
					t.Fatal(err)
				}
				if preparedMediaCalls(trace.Snapshot(), "prepare_read") != 1 || preparedMediaCalls(trace.Snapshot(), "anchor_write") != 1 || preparedMediaCalls(trace.Snapshot(), "sql_commit") != 1 {
					t.Fatal("ACK loss did not reach the one native final checkpoint")
				}
				select {
				case <-proxy.dropped:
				default:
					t.Fatal("test did not drop a real durable COMMIT response")
				}
			}
		})
	}
}

func TestPreparedPostMediaMissingResourceStillChecksSessionPostgres(t *testing.T) {
	f := newPreparedMediaFixture(t)
	preparedReadPool(t, f.l, func(n int) {
		if n == 1 {
			mustExec(t, f.l.cp, `UPDATE c_auth.accounts SET session_generation=session_generation+1 WHERE account_id=$1`, f.viewer.AccountID)
		}
	})
	r, err := f.l.c.ReadPostImages(context.Background(), f.viewer.SessionToken, uuid.NewString(), "")
	if !errors.Is(err, ErrSessionInvalid) || r.Value != nil {
		t.Fatal("private media absence bypassed final authorization", err)
	}
}

func TestPreparedPostMediaSelectedContentIDReuseRebuildPostgres(t *testing.T) {
	f := newPreparedMediaFixture(t)
	content := uuid.NewString()
	stage := func(data []byte, mime string) {
		command := postTestKey(t)
		if _, err := f.l.c.StagePostEditImage(context.Background(), f.owner.SessionToken, f.post, command, content, mime, data); err != nil {
			t.Fatal(err)
		}
		r, err := f.l.c.ReadPostEdit(context.Background(), f.owner.SessionToken, f.post)
		if err != nil {
			t.Fatal(err)
		}
		s := r.Value.(posts.EditSnapshot)
		ids := []string{content}
		if _, err = f.l.c.SavePostEdit(context.Background(), f.owner.SessionToken, f.post, command,
			posts.EditInput{Revision: s.Revision, Title: s.Post.Title, Body: s.Post.Body, ImageIDs: &ids}); err != nil {
			t.Fatal(err)
		}
	}
	stage(f.data, "image/png")
	selection, replacement := imageFixture(t, "jpeg")
	preparedReadPool(t, f.l, func(n int) {
		if n == 1 {
			empty := []string{}
			editPreparedMedia(t, f, &empty)
			stage(replacement, selection.MIME)
		}
	})
	ctx, trace := readtiming.Start(context.Background())
	r, err := f.l.c.ReadPostImages(ctx, f.viewer.SessionToken, f.post, content)
	if err != nil || !bytes.Equal(r.Value.(ImageBytes).Bytes, replacement) || preparedMediaCalls(trace.Snapshot(), "prepared_read_rebuild") != 1 {
		t.Fatal("same content ID reused stale bytes or failed to rebuild", err)
	}
	if sha256.Sum256(f.data) == sha256.Sum256(replacement) {
		t.Fatal("ABA fixture did not change actual bytes")
	}
}
