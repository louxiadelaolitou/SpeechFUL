package postthumbnailcache

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/postmedia"
)

// TestCacheAgainstRealRedis is gated: it runs only when a disposable Redis
// instance is supplied. It proves the exact commands and expiry semantics used
// by the request, fill and deletion paths.
func TestCacheAgainstRealRedis(t *testing.T) {
	address := os.Getenv("THUMBNAIL_CACHE_TEST_ADDR")
	if address == "" {
		t.Skip("THUMBNAIL_CACHE_TEST_ADDR absent; real Redis test not executed")
	}
	password := os.Getenv("THUMBNAIL_CACHE_TEST_PASSWORD")
	username := os.Getenv("THUMBNAIL_CACHE_TEST_USERNAME")
	if username == "" {
		username = "default"
	}
	cfg := testConfig()
	cfg.Address = address
	cfg.Username = username
	cfg.Namespace = "hnuhole:post-thumbnail:v1:test:"
	cfg.TTLSeconds = 60
	if err := cfg.Validate("test"); err != nil {
		t.Fatal(err)
	}
	cache, err := New(cfg, password)
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()
	ctx := context.Background()
	if err := cache.Check(ctx); err != nil {
		t.Fatal(err)
	}
	observer := redis.NewClient(&redis.Options{Addr: address, Username: username, Password: password})
	defer observer.Close()

	post, content := uuid.NewString(), uuid.NewString()
	parts := KeyParts{PostID: post, ContentVersion: content, Variant: postmedia.ThumbnailVariant,
		EncoderVersion: randomHex(t), SourceBinding: randomHex(t), OutputDigest: randomHex(t)}
	key, err := cache.Key(parts)
	if err != nil {
		t.Fatal(err)
	}
	defer observer.Unlink(context.Background(), key)

	record := testRecord(128)
	digest := record.SHA256
	deadline := time.Now().Add(cache.TTL())
	if !cache.Enqueue(key, record, deadline) {
		t.Fatal("fill rejected")
	}
	waitFor(t, func() bool { return cache.Stats().FillsSet == 1 })
	raw, err := observer.Get(ctx, key).Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != string(record.Bytes) {
		t.Fatal("stored bytes differ")
	}
	ttl, err := observer.TTL(ctx, key).Result()
	if err != nil || ttl <= 0 || ttl > cache.TTL() {
		t.Fatal("absolute expiry not set", ttl, err)
	}
	// The deadline is absolute, so the observed TTL cannot exceed the remaining
	// lifetime computed before enqueueing.
	if ttl > time.Until(deadline)+2*time.Second {
		t.Fatal("expiry drifted from absolute authorization time", ttl)
	}

	// SET NX must never replace an existing copy.
	other := testRecord(256)
	if !cache.Enqueue(key, other, deadline) {
		t.Fatal("second fill rejected")
	}
	waitFor(t, func() bool { return cache.Stats().FillsExisting == 1 })
	raw, err = observer.Get(ctx, key).Bytes()
	if err != nil || string(raw) != string(record.Bytes) {
		t.Fatal("NX did not preserve original", err)
	}

	got, ok := cache.Get(ctx, key, record.Width, record.Height, len(record.Bytes), digest)
	if !ok || string(got.Bytes) != string(record.Bytes) || cache.Stats().Hits != 1 {
		t.Fatal("real Redis hit failed")
	}

	// Same-length corruption and an oversize value must both be rejected.
	corrupt := append([]byte(nil), record.Bytes...)
	corrupt[len(corrupt)-1] ^= 0xff
	if err = observer.Set(ctx, key, corrupt, time.Minute).Err(); err != nil {
		t.Fatal(err)
	}
	if _, ok = cache.Get(ctx, key, record.Width, record.Height, len(record.Bytes), digest); ok || cache.Stats().BadValues != 1 {
		t.Fatal("corrupt value accepted")
	}
	oversize := make([]byte, postmedia.MaxThumbnailBytes+1)
	if err = observer.Set(ctx, key, oversize, time.Minute).Err(); err != nil {
		t.Fatal(err)
	}
	if _, ok = cache.Get(ctx, key, record.Width, record.Height, postmedia.MaxThumbnailBytes, digest); ok {
		t.Fatal("oversize value accepted")
	}

	if err = cache.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
	if n, err := observer.Exists(ctx, key).Result(); err != nil || n != 0 {
		t.Fatal("key survived deletion", n, err)
	}
	if _, ok = cache.Get(ctx, key, record.Width, record.Height, len(record.Bytes), digest); ok {
		t.Fatal("deleted key served")
	}
}

func randomHex(t *testing.T) string {
	t.Helper()
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b[:])
}
