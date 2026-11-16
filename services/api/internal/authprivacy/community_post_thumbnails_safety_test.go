package authprivacy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/posts"
)

func TestThumbnailReuseCanceledWaiterDoesNotCancelOthersPostgres(t *testing.T) {
	f := thumbnailFixture(t)
	started, release := make(chan struct{}), make(chan struct{})
	probe := installThumbnailProbe(f, func(_ context.Context, n int) {
		if n == 1 {
			close(started)
		}
		<-release
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first, second := make(chan error, 1), make(chan error, 1)
	go func() { _, e := f.l.c.ReadPostThumbnail(ctx, f.viewer.SessionToken, f.post, f.content); first <- e }()
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("encoder not reached")
	}
	go func() {
		r, e := f.l.c.ReadPostThumbnail(context.Background(), f.owner.SessionToken, f.post, f.content)
		if e == nil && r.Value == nil {
			e = ErrAuthorizationUnavailable
		}
		second <- e
	}()
	deadline := time.Now().Add(10 * time.Second)
	for f.l.c.postImages.thumbnails.misses.Load() != 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	cancel()
	if e := <-first; e == nil {
		t.Fatal("canceled waiter received bytes")
	}
	close(release)
	select {
	case e := <-second:
		if e != nil {
			t.Fatal("other waiter canceled", e)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("other waiter stuck")
	}
	if probe.calls.Load() != 1 {
		t.Fatal("shared cancellation duplicated encoding")
	}
}

func TestThumbnailReuseEditedSourceVersionReusePostgres(t *testing.T) {
	f := thumbnailFixture(t)
	content := uuid.NewString()
	stage := func(data []byte, mime string) {
		command := postTestKey(t)
		if _, e := f.l.c.StagePostEditImage(context.Background(), f.owner.SessionToken, f.post, command, content, mime, data); e != nil {
			t.Fatal(e)
		}
		response, e := f.l.c.ReadPostEdit(context.Background(), f.owner.SessionToken, f.post)
		if e != nil {
			t.Fatal(e)
		}
		s := response.Value.(posts.EditSnapshot)
		ids := []string{content}
		if _, e = f.l.c.SavePostEdit(context.Background(), f.owner.SessionToken, f.post, command, posts.EditInput{Revision: s.Revision, Title: s.Post.Title, Body: s.Post.Body, ImageIDs: &ids}); e != nil {
			t.Fatal(e)
		}
	}
	stage(f.data, "image/png")
	old, e := f.l.c.ReadPostThumbnail(context.Background(), f.viewer.SessionToken, f.post, content)
	if e != nil {
		t.Fatal(e)
	}
	if old.Value == nil {
		t.Fatal("missing initial derivative")
	}
	var beforeBinding []byte
	if e = f.l.cp.QueryRow(context.Background(), `SELECT source_binding FROM c_posts.thumbnails WHERE post_id=$1 AND content_version_id=$2`, f.post, content).Scan(&beforeBinding); e != nil {
		t.Fatal(e)
	}
	selection, replacement := imageFixture(t, "jpeg")
	expected, e := f.l.c.postImages.thumbnails.encoder.ThumbnailRecord(context.Background(), replacement)
	if e != nil {
		t.Fatal(e)
	}
	preparedReadPool(t, f.l, func(n int) {
		if n == 1 {
			empty := []string{}
			editPreparedMedia(t, f, &empty)
			stage(replacement, selection.MIME)
		}
	})
	current, e := f.l.c.ReadPostThumbnail(context.Background(), f.viewer.SessionToken, f.post, content)
	if e != nil || !bytes.Equal(expected.Bytes, current.Value.(ImageBytes).Bytes) {
		t.Fatal("reused edited ID returned stale derivative", e)
	}
	var afterBinding []byte
	if e = f.l.cp.QueryRow(context.Background(), `SELECT source_binding FROM c_posts.thumbnails WHERE post_id=$1 AND content_version_id=$2`, f.post, content).Scan(&afterBinding); e != nil || bytes.Equal(beforeBinding, afterBinding) {
		t.Fatal("derivative still bound to removed source", e)
	}
	original, e := f.l.c.ReadPostImages(context.Background(), f.viewer.SessionToken, f.post, content)
	if e != nil || !bytes.Equal(original.Value.(ImageBytes).Bytes, replacement) {
		t.Fatal("edited original changed", e)
	}
}

func TestThumbnailReuseLostCommitACKReturnsNoBytesPostgres(t *testing.T) {
	f := thumbnailFixture(t)
	pool, proxy := newCommitACKProxyPool(t, f.l.cp)
	f.l.c.pool = pool
	installThumbnailProbe(f, func(context.Context, int) { proxy.armed.Store(true) })
	r, e := f.l.c.ReadPostThumbnail(context.Background(), f.viewer.SessionToken, f.post, f.content)
	if e == nil || r.Value != nil || !r.ExpiresAt.IsZero() {
		t.Fatal("unknown commit released prepared bytes", e)
	}
	select {
	case <-proxy.dropped:
	default:
		t.Fatal("real COMMIT ACK was not dropped")
	}
}

func TestThumbnailReuseEncoderPanicStaysPrivatePostgres(t *testing.T) {
	f := thumbnailFixture(t)
	installThumbnailProbe(f, func(context.Context, int) { panic("synthetic encoder failure") })
	r, e := f.l.c.ReadPostThumbnail(context.Background(), f.viewer.SessionToken, f.post, f.content)
	if e == nil || r.Value != nil || !r.ExpiresAt.IsZero() {
		t.Fatal("encoder panic exposed prepared output", e)
	}
	if count(t, f.l.cp, `SELECT count(*) FROM c_posts.thumbnails`) != 0 || count(t, f.l.cp, `SELECT count(*) FROM c_auth.authorization_gate WHERE gate_state='OPEN'`) != 1 {
		t.Fatal("encoder failure persisted bytes or froze authority")
	}
}

func TestThumbnailReuseTwoInstancesUniqueStoragePostgres(t *testing.T) {
	f := thumbnailFixture(t)
	second, e := f.l.c.WithPostThumbnailReuse(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	ready, release := make(chan struct{}, 2), make(chan struct{})
	probe := installThumbnailProbe(f, func(context.Context, int) { ready <- struct{}{}; <-release })
	second.postImages.thumbnails.encoder = probe
	results := make(chan error, 2)
	go func() {
		_, e := f.l.c.ReadPostThumbnail(context.Background(), f.viewer.SessionToken, f.post, f.content)
		results <- e
	}()
	go func() {
		_, e := second.ReadPostThumbnail(context.Background(), f.owner.SessionToken, f.post, f.content)
		results <- e
	}()
	for i := 0; i < 2; i++ {
		select {
		case <-ready:
		case <-time.After(10 * time.Second):
			close(release)
			t.Fatal("separate instance encoder did not start")
		}
	}
	close(release)
	for i := 0; i < 2; i++ {
		select {
		case e := <-results:
			if e != nil {
				t.Fatal(e)
			}
		case <-time.After(15 * time.Second):
			t.Fatal("instance waiter stuck")
		}
	}
	if probe.calls.Load() != 2 || count(t, f.l.cp, `SELECT count(*) FROM c_posts.thumbnails`) != 1 ||
		count(t, f.l.cp, `SELECT count(*) FROM c_posts.thumbnail_storage WHERE used_bytes=(SELECT sum(byte_length) FROM c_posts.thumbnails)`) != 1 {
		t.Fatal("cross-instance duplicate storage/accounting")
	}
}

func TestThumbnailReuseWarmAuthorityPostgres(t *testing.T) {
	for _, mode := range []string{"delete", "hide", "block", "session_generation", "freeze", "edit_text"} {
		t.Run(mode, func(t *testing.T) {
			f := thumbnailFixture(t)
			if _, e := f.l.c.ReadPostThumbnail(context.Background(), f.viewer.SessionToken, f.post, f.content); e != nil {
				t.Fatal(e)
			}
			before := f.l.c.postImages.thumbnails.transforms.Load()
			original := f.l.c
			preparedReadPool(t, f.l, func(n int) {
				if n != 1 {
					return
				}
				switch mode {
				case "delete":
					postTestCommand(t, f.l, f.owner.SessionToken, postTestKey(t), posts.Intent{Operation: posts.DeletePost, PostID: f.post})
				case "hide":
					mustExec(t, f.l.cp, `UPDATE c_posts.posts SET moderation_hidden=true WHERE post_id=$1`, f.post)
				case "block":
					if _, e := f.l.c.BlockPostAuthor(context.Background(), f.viewer.SessionToken, f.post); e != nil {
						t.Fatal(e)
					}
				case "session_generation":
					mustExec(t, f.l.cp, `UPDATE c_auth.accounts SET session_generation=session_generation+1 WHERE account_id=$1`, f.viewer.AccountID)
				case "freeze":
					if e := original.gate.(*PostgresAuthorizationGate).Freeze(context.Background(), "warm thumbnail test"); e != nil {
						t.Fatal(e)
					}
				case "edit_text":
					editPreparedMedia(t, f, nil)
				}
			})
			r, e := f.l.c.ReadPostThumbnail(context.Background(), f.viewer.SessionToken, f.post, f.content)
			if mode == "edit_text" {
				if e != nil || r.Value == nil {
					t.Fatal("unchanged image lost after text edit", e)
				}
			} else if e == nil || r.Value != nil {
				t.Fatal("warm bytes bypassed current authority", e)
			}
			if f.l.c.postImages.thumbnails.transforms.Load() != before {
				t.Fatal("warm request encoded")
			}
		})
	}
}

func TestThumbnailReuseBudgetAndOrphanCleanupPostgres(t *testing.T) {
	f := thumbnailFixture(t)
	mustExec(t, f.l.cp, `UPDATE c_posts.thumbnail_storage SET capacity_bytes=524288`)
	// Fill the exact budget with a private orphan, then ensure a foreground
	// request can still use transient bytes without exceeding durable storage.
	payload := make([]byte, 524288)
	copy(payload, []byte("RIFF\x00\x00\x00\x00WEBP"))
	digest := sha256.Sum256(payload)
	mustExec(t, f.l.cp, `INSERT INTO c_posts.thumbnails(post_id,content_version_id,variant,encoder_version,source_binding,payload,digest,mime_type,width,height,byte_length,created_at)
 VALUES($1,$2,'thumbnail-v1-640',repeat('a',64),$3,$4,$5,'image/webp',1,1,524288,'2026-01-01T00:00:00Z')`, uuid.NewString(), uuid.NewString(), digest[:], payload, digest[:])
	if _, e := f.l.c.ReadPostThumbnail(context.Background(), f.viewer.SessionToken, f.post, f.content); e != nil {
		t.Fatal(e)
	}
	if f.l.c.postImages.thumbnails.capacitySkips.Load() != 1 || count(t, f.l.cp, `SELECT used_bytes FROM c_posts.thumbnail_storage`) != 524288 {
		t.Fatal("capacity ceiling ignored")
	}
	removed, e := f.l.c.CleanupPostThumbnails(context.Background(), 128)
	if e != nil || removed != 1 || count(t, f.l.cp, `SELECT used_bytes FROM c_posts.thumbnail_storage`) != 0 {
		t.Fatal("orphan cleanup/accounting", removed, e)
	}
	if _, e = f.l.c.ReadPostThumbnail(context.Background(), f.viewer.SessionToken, f.post, f.content); e != nil {
		t.Fatal(e)
	}
	if count(t, f.l.cp, `SELECT count(*) FROM c_posts.thumbnails WHERE post_id=$1`, f.post) != 1 {
		t.Fatal("storage did not resume after cleanup")
	}
}

func TestThumbnailReuseCleanupAdvancesPastValidOldestPostgres(t *testing.T) {
	f := thumbnailFixture(t)
	if _, e := f.l.c.ReadPostThumbnail(context.Background(), f.viewer.SessionToken, f.post, f.content); e != nil {
		t.Fatal(e)
	}
	orphan := uuid.NewString()
	mustExec(t, f.l.cp, `INSERT INTO c_posts.thumbnails
 SELECT $1,$2,variant,encoder_version,source_binding,payload,digest,mime_type,width,height,byte_length,created_at+interval '1 hour'
 FROM c_posts.thumbnails WHERE post_id=$3`, orphan, uuid.NewString(), f.post)
	first, e := f.l.c.CleanupPostThumbnails(context.Background(), 1)
	if e != nil || first != 0 {
		t.Fatal("valid oldest thumbnail erased", e)
	}
	second, e := f.l.c.CleanupPostThumbnails(context.Background(), 1)
	if e != nil || second != 1 || count(t, f.l.cp, `SELECT count(*) FROM c_posts.thumbnails WHERE post_id=$1`, orphan) != 0 {
		t.Fatal("bounded scanner did not advance to orphan", e)
	}
	if count(t, f.l.cp, `SELECT count(*) FROM c_posts.thumbnails WHERE post_id=$1`, f.post) != 1 {
		t.Fatal("scanner erased valid source")
	}
}

func TestThumbnailReuseStartupRejectsBrokenLifecyclePostgres(t *testing.T) {
	f := thumbnailFixture(t)
	mustExec(t, f.l.cp, `ALTER TABLE c_posts.posts DISABLE TRIGGER thumbnail_post_erase`)
	if _, e := f.l.c.WithPostThumbnailReuse(context.Background()); e == nil {
		t.Fatal("reuse accepted missing erasure")
	}
	if _, e := f.l.c.WithPostImages(context.Background(), f.l.c.postImages.validator); e == nil {
		t.Fatal("rollback accepted missing erasure")
	}
	mustExec(t, f.l.cp, `ALTER TABLE c_posts.posts ENABLE TRIGGER thumbnail_post_erase;
 ALTER TABLE c_posts.thumbnails DROP CONSTRAINT thumbnails_width_check;
 ALTER TABLE c_posts.thumbnails ADD CONSTRAINT thumbnails_width_check CHECK(true)`)
	if _, e := f.l.c.WithPostThumbnailReuse(context.Background()); e == nil {
		t.Fatal("no-op size guard accepted")
	}
}

func TestThumbnailReuseStartupRejectsDriftedStoragePostgres(t *testing.T) {
	f := thumbnailFixture(t)
	if _, e := f.l.c.ReadPostThumbnail(context.Background(), f.viewer.SessionToken, f.post, f.content); e != nil {
		t.Fatal(e)
	}
	mustExec(t, f.l.cp, `UPDATE c_posts.thumbnail_storage SET used_bytes=used_bytes+1`)
	if _, e := f.l.c.WithPostThumbnailReuse(context.Background()); e == nil {
		t.Fatal("inconsistent storage ledger accepted")
	}
	mustExec(t, f.l.cp, `UPDATE c_posts.thumbnail_storage SET used_bytes=used_bytes-1;
 DROP INDEX c_posts.thumbnail_oldest_idx; CREATE INDEX thumbnail_oldest_idx ON c_posts.thumbnails(post_id)`)
	if _, e := f.l.c.WithPostThumbnailReuse(context.Background()); e == nil {
		t.Fatal("unbounded cleanup index accepted")
	}
}

func TestThumbnailReuseRuntimeCannotChangeBudgetOrPayloadPostgres(t *testing.T) {
	f := thumbnailFixture(t)
	if _, e := f.l.c.ReadPostThumbnail(context.Background(), f.viewer.SessionToken, f.post, f.content); e != nil {
		t.Fatal(e)
	}
	dsn := os.Getenv("AUTHLAB_POSTS_RUNTIME_DSN")
	if dsn == "" {
		t.Fatal("isolated runtime role required")
	}
	pool, e := pgxpool.New(context.Background(), dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	for _, sql := range []string{
		`UPDATE c_posts.thumbnail_storage SET capacity_bytes=1099511627776`,
		`UPDATE c_posts.thumbnails SET payload='x'::bytea`,
		`ALTER TABLE c_posts.posts DISABLE TRIGGER thumbnail_post_erase`,
		`SELECT c_posts.erase_source_thumbnails()`,
	} {
		if _, e = pool.Exec(context.Background(), sql); e == nil {
			t.Fatal("runtime altered authoritative storage policy")
		}
	}
}
