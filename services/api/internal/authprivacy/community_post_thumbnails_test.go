package authprivacy

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/postmedia"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/posts"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/readtiming"
)

func thumbnailFixture(t *testing.T) preparedMediaFixture {
	t.Helper()
	f := newPreparedMediaFixture(t)
	var err error
	f.l.c, err = f.l.c.WithPostThumbnailReuse(context.Background())
	if err != nil {
		t.Fatal("reuse startup", err)
	}
	return f
}

func TestThumbnailReuseHitAndEncoderIsolationPostgres(t *testing.T) {
	f := thumbnailFixture(t)
	ctx := context.Background()
	var constraints string
	if e := f.l.cp.QueryRow(ctx, `SELECT encode(sha256(convert_to(string_agg(r.relname||':'||k.conname||':'||pg_get_constraintdef(k.oid),E'\n' ORDER BY r.relname,k.conname),'UTF8')),'hex')
 FROM pg_constraint k JOIN pg_class r ON r.oid=k.conrelid WHERE k.conrelid IN ('c_posts.thumbnails'::regclass,'c_posts.thumbnail_storage'::regclass) AND k.contype='c'`).Scan(&constraints); e != nil {
		t.Fatal(e)
	}
	t.Log("thumbnail constraint digest", constraints)
	first, err := f.l.c.ReadPostThumbnail(ctx, f.viewer.SessionToken, f.post, f.content)
	if err != nil {
		t.Fatal(err)
	}
	before, err := f.l.c.PostThumbnailStats(ctx)
	if err != nil || before.SourceReads != 1 || before.Transforms != 1 || before.StoredBytes < 12 {
		t.Fatal("cold metrics", before, err)
	}
	traceCtx, trace := readtiming.Start(ctx)
	next, err := f.l.c.ReadPostThumbnail(traceCtx, f.viewer.SessionToken, f.post, f.content)
	if err != nil || !bytes.Equal(first.Value.(ImageBytes).Bytes, next.Value.(ImageBytes).Bytes) {
		t.Fatal("hit differs", err)
	}
	after, err := f.l.c.PostThumbnailStats(ctx)
	if err != nil || after.SourceReads != before.SourceReads || after.Transforms != before.Transforms || after.Hits != 1 ||
		preparedMediaCalls(trace.Snapshot(), "anchor_write") != 1 {
		t.Fatal("hit read source/encoded/checkpoint", after, err)
	}
	original, err := f.l.c.ReadPostImages(ctx, f.viewer.SessionToken, f.post, f.content)
	if err != nil || !bytes.Equal(original.Value.(ImageBytes).Bytes, f.data) {
		t.Fatal("original changed", err)
	}
	f.l.c.postImages.thumbnails.version = strings.Repeat("a", 64)
	if _, err = f.l.c.ReadPostThumbnail(ctx, f.viewer.SessionToken, f.post, f.content); err != nil {
		t.Fatal(err)
	}
	if count(t, f.l.cp, `SELECT count(*) FROM c_posts.thumbnails WHERE post_id=$1`, f.post) != 2 {
		t.Fatal("encoder versions conflated")
	}
	if count(t, f.l.cp, `SELECT count(*) FROM c_posts.thumbnail_storage WHERE singleton_id=1 AND used_bytes=(SELECT sum(byte_length) FROM c_posts.thumbnails)`) != 1 {
		t.Fatal("storage accounting")
	}
}

type thumbnailProbe struct {
	postmedia.Validator
	postmedia.ReusableThumbnailer
	calls atomic.Int32
	hook  func(context.Context, int)
}

func (p *thumbnailProbe) ThumbnailRecord(ctx context.Context, data []byte) (postmedia.ThumbnailRecord, error) {
	n := int(p.calls.Add(1))
	if p.hook != nil {
		p.hook(ctx, n)
	}
	return p.ReusableThumbnailer.ThumbnailRecord(ctx, data)
}
func installThumbnailProbe(f preparedMediaFixture, hook func(context.Context, int)) *thumbnailProbe {
	cache := f.l.c.postImages.thumbnails
	p := &thumbnailProbe{Validator: f.l.c.postImages.validator, ReusableThumbnailer: cache.encoder, hook: hook}
	cache.encoder = p
	return p
}

func TestThumbnailReuseTwentyColdRequestsPostgres(t *testing.T) {
	f := thumbnailFixture(t)
	release := make(chan struct{})
	started := make(chan struct{})
	probe := installThumbnailProbe(f, func(_ context.Context, n int) {
		if n == 1 {
			close(started)
		}
		<-release
	})
	result := make(chan error, 20)
	for i := 0; i < 20; i++ {
		go func() {
			r, e := f.l.c.ReadPostThumbnail(context.Background(), f.viewer.SessionToken, f.post, f.content)
			if e == nil && r.Value == nil {
				e = errors.New("missing protected output")
			}
			result <- e
		}()
	}
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("encoder not reached")
	}
	deadline := time.Now().Add(10 * time.Second)
	for f.l.c.postImages.thumbnails.misses.Load() != 20 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if f.l.c.postImages.thumbnails.misses.Load() != 20 {
		close(release)
		t.Fatal("requests did not reach cold preparation")
	}
	close(release)
	for i := 0; i < 20; i++ {
		select {
		case e := <-result:
			if e != nil {
				t.Fatal(e)
			}
		case <-time.After(15 * time.Second):
			t.Fatal("cold waiter stuck")
		}
	}
	s, e := f.l.c.PostThumbnailStats(context.Background())
	if e != nil || probe.calls.Load() != 1 || s.SourceReads != 1 || count(t, f.l.cp, `SELECT count(*) FROM c_posts.thumbnails`) != 1 {
		t.Fatal("cold work not coalesced", s, probe.calls.Load(), e)
	}
}

func TestThumbnailReuseColdAuthorityAndErasePostgres(t *testing.T) {
	for _, mode := range []string{"delete", "hide", "edit_remove", "session_generation", "freeze", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			f := thumbnailFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			installThumbnailProbe(f, func(_ context.Context, n int) {
				if n != 1 {
					return
				}
				switch mode {
				case "delete":
					postTestCommand(t, f.l, f.owner.SessionToken, postTestKey(t), posts.Intent{Operation: posts.DeletePost, PostID: f.post})
				case "hide":
					mustExec(t, f.l.cp, `UPDATE c_posts.posts SET moderation_hidden=true WHERE post_id=$1`, f.post)
				case "edit_remove":
					empty := []string{}
					editPreparedMedia(t, f, &empty)
				case "session_generation":
					mustExec(t, f.l.cp, `UPDATE c_auth.accounts SET session_generation=session_generation+1 WHERE account_id=$1`, f.viewer.AccountID)
				case "freeze":
					if e := f.l.c.gate.(*PostgresAuthorizationGate).Freeze(context.Background(), "thumbnail test"); e != nil {
						t.Error(e)
					}
				case "cancel":
					cancel()
				}
			})
			r, e := f.l.c.ReadPostThumbnail(ctx, f.viewer.SessionToken, f.post, f.content)
			if e == nil || r.Value != nil || !r.ExpiresAt.IsZero() {
				t.Fatal("authority loss released thumbnail", e)
			}
			// Canceled shared jobs may finish privately, but cannot persist or
			// outlive the isolated fixture while it resets the next schema.
			deadline := time.Now().Add(10 * time.Second)
			for len(f.l.c.postImages.thumbnails.queue) > 0 && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
			if count(t, f.l.cp, `SELECT count(*) FROM c_posts.thumbnails`) != 0 {
				t.Fatal("denied work persisted a thumbnail")
			}
		})
	}
}

func TestThumbnailReuseDisabledStillErasesPostgres(t *testing.T) {
	f := thumbnailFixture(t)
	if _, e := f.l.c.ReadPostThumbnail(context.Background(), f.viewer.SessionToken, f.post, f.content); e != nil {
		t.Fatal(e)
	}
	f.l.c.postImages.thumbnails = nil
	postTestCommand(t, f.l, f.owner.SessionToken, postTestKey(t), posts.Intent{Operation: posts.DeletePost, PostID: f.post})
	if count(t, f.l.cp, `SELECT count(*) FROM c_posts.thumbnails`) != 0 || count(t, f.l.cp, `SELECT used_bytes FROM c_posts.thumbnail_storage`) != 0 {
		t.Fatal("rollback disabled lifecycle erasure")
	}
}
