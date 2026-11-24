package authprivacy

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/postmedia"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/postthumbnailcache"
)

// thumbnailCacheProbe implements only the small hot-cache surface. It stores
// bytes in memory so the PostgreSQL read path can be observed without Redis.
type thumbnailCacheProbe struct {
	mu         sync.Mutex
	records    map[string]postmedia.ThumbnailRecord
	gets       int
	hits       int
	enqueues   int
	deletes    int
	corrupt    bool
	failDelete bool
	beforeGet  func()
}

func (p *thumbnailCacheProbe) Key(parts postthumbnailcache.KeyParts) (string, error) {
	return parts.PostID + "|" + parts.ContentVersion + "|" + parts.EncoderVersion + "|" + parts.SourceBinding + "|" + parts.OutputDigest, nil
}

func (p *thumbnailCacheProbe) Get(_ context.Context, key string, width, height, byteLength int, digest string) (postmedia.ThumbnailRecord, bool) {
	if p.beforeGet != nil {
		p.beforeGet()
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.gets++
	record, ok := p.records[key]
	if !ok || p.corrupt || record.Width != width || record.Height != height ||
		len(record.Bytes) != byteLength || record.SHA256 != digest {
		return postmedia.ThumbnailRecord{}, false
	}
	p.hits++
	record.Bytes = append([]byte(nil), record.Bytes...)
	return record, true
}

func (p *thumbnailCacheProbe) Enqueue(key string, record postmedia.ThumbnailRecord, _ time.Time) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.enqueues++
	if p.records == nil {
		p.records = map[string]postmedia.ThumbnailRecord{}
	}
	record.Bytes = append([]byte(nil), record.Bytes...)
	p.records[key] = record
	return true
}

func (p *thumbnailCacheProbe) Delete(_ context.Context, key string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.failDelete {
		return errors.New("delete unavailable")
	}
	p.deletes++
	delete(p.records, key)
	return nil
}

func (p *thumbnailCacheProbe) Stats() postthumbnailcache.Stats { return postthumbnailcache.Stats{} }

func (p *thumbnailCacheProbe) TTL() time.Duration { return time.Minute }

func (p *thumbnailCacheProbe) DeleteBatch() int { return 32 }

func redisThumbnailFixture(t *testing.T) (preparedMediaFixture, *thumbnailCacheProbe) {
	t.Helper()
	f := thumbnailFixture(t)
	probe := &thumbnailCacheProbe{}
	c, err := f.l.c.WithPostThumbnailRedisCache(context.Background(), probe)
	if err != nil {
		t.Fatal("redis cache startup", err)
	}
	f.l.c = c
	return f, probe
}

func readThumbnail(t *testing.T, f preparedMediaFixture) []byte {
	t.Helper()
	response, err := f.l.c.ReadPostThumbnail(context.Background(), f.viewer.SessionToken, f.post, f.content)
	if err != nil {
		t.Fatal(err)
	}
	return response.Value.(ImageBytes).Bytes
}

func TestThumbnailRedisHotPathAndFillPolicyPostgres(t *testing.T) {
	f, probe := redisThumbnailFixture(t)
	first := readThumbnail(t, f)
	if probe.gets != 0 || probe.enqueues != 0 {
		t.Fatal("generated output must not read or fill Redis")
	}
	base, err := f.l.c.PostThumbnailStats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	second := readThumbnail(t, f)
	if !bytes.Equal(first, second) {
		t.Fatal("filled read changed bytes")
	}
	if probe.gets != 1 || probe.enqueues != 1 {
		t.Fatalf("pre-existing PG row did not enter cache exactly once: gets=%d fills=%d", probe.gets, probe.enqueues)
	}
	afterFill, err := f.l.c.PostThumbnailStats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if afterFill.SourceReads != base.SourceReads || afterFill.Transforms != base.Transforms {
		t.Fatal("PG fallback entered source/transform path")
	}
	if afterFill.Hits != base.Hits+1 || afterFill.PayloadReads != base.PayloadReads+1 ||
		afterFill.PayloadBytes != base.PayloadBytes+uint64(len(first)) {
		t.Fatal("PG fallback not accounted as derivative payload read")
	}

	third := readThumbnail(t, f)
	if !bytes.Equal(first, third) || probe.hits != 1 {
		t.Fatal("Redis hit not served", probe.hits)
	}
	afterHit, err := f.l.c.PostThumbnailStats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if afterHit.SourceReads != afterFill.SourceReads || afterHit.Transforms != afterFill.Transforms ||
		afterHit.Hits != afterFill.Hits || afterHit.PayloadReads != afterFill.PayloadReads ||
		afterHit.PayloadBytes != afterFill.PayloadBytes {
		t.Fatal("Redis hit read PostgreSQL payload or transformed")
	}

	probe.mu.Lock()
	probe.corrupt = true
	probe.mu.Unlock()
	fourth := readThumbnail(t, f)
	if !bytes.Equal(first, fourth) {
		t.Fatal("corrupt value did not fall back")
	}
	afterBad, err := f.l.c.PostThumbnailStats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if afterBad.SourceReads != afterHit.SourceReads || afterBad.Transforms != afterHit.Transforms ||
		afterBad.Hits != afterHit.Hits+1 || afterBad.PayloadReads != afterHit.PayloadReads+1 {
		t.Fatal("bad Redis value did not use bounded PG payload fallback")
	}
}

func TestThumbnailRedisHitRequiresLivePGDerivativePostgres(t *testing.T) {
	f, probe := redisThumbnailFixture(t)
	readThumbnail(t, f) // generate the PostgreSQL derivative
	readThumbnail(t, f) // fill Redis from the bounded PG payload
	readThumbnail(t, f) // this read is served by Redis
	if probe.hits != 1 {
		t.Fatalf("expected one Redis hit, got %d", probe.hits)
	}
	before, err := f.l.c.PostThumbnailStats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	var deleteErr error
	probe.beforeGet = func() {
		once.Do(func() {
			_, deleteErr = f.l.cp.Exec(context.Background(), `DELETE FROM c_posts.thumbnails WHERE post_id=$1`, f.post)
		})
	}
	// The cached bytes were prepared while the row existed. Deleting the row
	// before final authorization must force a rebuild, never release the hit.
	readThumbnail(t, f)
	if deleteErr != nil {
		t.Fatal(deleteErr)
	}
	after, err := f.l.c.PostThumbnailStats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if after.Transforms != before.Transforms+1 || after.Hits != before.Hits {
		t.Fatalf("stale Redis hit was released: transforms %d->%d hits %d->%d", before.Transforms, after.Transforms, before.Hits, after.Hits)
	}
	if count(t, f.l.cp, `SELECT count(*) FROM c_posts.thumbnails WHERE post_id=$1`, f.post) != 1 {
		t.Fatal("rebuilt derivative missing")
	}
}

func TestThumbnailCacheDeletionOutboxPostgres(t *testing.T) {
	f, probe := redisThumbnailFixture(t)
	readThumbnail(t, f)
	readThumbnail(t, f)
	probe.mu.Lock()
	fills := len(probe.records)
	probe.mu.Unlock()
	if fills != 1 {
		t.Fatalf("expected one cached key, got %d", fills)
	}
	if _, err := f.l.cp.Exec(context.Background(), `DELETE FROM c_posts.thumbnails WHERE post_id=$1`, f.post); err != nil {
		t.Fatal(err)
	}
	var pending int
	if err := f.l.cp.QueryRow(context.Background(), `SELECT count(*) FROM c_posts.thumbnail_cache_outbox WHERE completed_at IS NULL`).Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if pending != 1 {
		t.Fatalf("derivative delete did not enqueue exactly one event: %d", pending)
	}
	completed, err := f.l.c.processThumbnailCacheDeletions(context.Background(), 32)
	if err != nil || completed != 1 {
		t.Fatalf("delete worker: completed=%d err=%v", completed, err)
	}
	probe.mu.Lock()
	deletes := probe.deletes
	remaining := len(probe.records)
	probe.mu.Unlock()
	if deletes != 1 || remaining != 0 {
		t.Fatalf("worker did not delete the exact key: deletes=%d remaining=%d", deletes, remaining)
	}
	if err = f.l.cp.QueryRow(context.Background(), `SELECT count(*) FROM c_posts.thumbnail_cache_outbox WHERE completed_at IS NULL`).Scan(&pending); err != nil || pending != 0 {
		t.Fatal("event not completed", pending, err)
	}
}

func TestThumbnailMaintenanceRunsWithReuseDisabledPostgres(t *testing.T) {
	// Durable storage is enabled while reuse and Redis are not. The periodic
	// pass must still clean orphans and retire abandoned outbox events instead
	// of dereferencing the absent cache.
	f := newPreparedMediaFixture(t)
	if f.l.c.postImages == nil || !f.l.c.postImages.thumbnailStorage || f.l.c.postImages.thumbnails != nil {
		t.Fatal("fixture must have storage without reuse")
	}
	stale, fresh := uuid.NewString(), uuid.NewString()
	insert := `INSERT INTO c_posts.thumbnail_cache_outbox
 (event_id,post_id,content_version_id,variant,encoder_version,source_binding,output_digest,created_at)
 VALUES($1,$2,$3,'thumbnail-v1-640',repeat('a',64),decode(repeat('00',32),'hex'),decode(repeat('11',32),'hex'),$4)`
	mustExec(t, f.l.cp, insert, stale, uuid.NewString(), uuid.NewString(), time.Now().Add(-thumbnailCacheStalePending-time.Hour))
	mustExec(t, f.l.cp, insert, fresh, uuid.NewString(), uuid.NewString(), time.Now())
	f.l.c.postThumbnailMaintenancePass(context.Background())
	if count(t, f.l.cp, `SELECT count(*) FROM c_posts.thumbnail_cache_outbox WHERE event_id=$1`, stale) != 0 {
		t.Fatal("stale pending event survived a disabled cache pass")
	}
	if count(t, f.l.cp, `SELECT count(*) FROM c_posts.thumbnail_cache_outbox WHERE event_id=$1`, fresh) != 1 {
		t.Fatal("fresh pending event was abandoned before its bound")
	}
}
