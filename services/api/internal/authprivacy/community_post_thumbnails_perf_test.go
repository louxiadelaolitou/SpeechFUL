package authprivacy

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/jpeg"
	"math/rand"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/postmedia"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/postthumbnailcache"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/readtiming"
)

// A separate opt-in performance test: real PostgreSQL, actual controlled Python
// child, deterministic 1024x768 JPEG, 3 paired rounds of 40 requests per path.
// Both paths use REQ-001 prepared media; only derivative reuse differs.
func TestThumbnailReusePerformancePostgres(t *testing.T) {
	if os.Getenv("POST_THUMBNAIL_PERFORMANCE") != "1" {
		t.Skip("explicit performance run required")
	}
	f := thumbnailFixture(t)
	ctx := context.Background()
	imageData := image.NewRGBA(image.Rect(0, 0, 1024, 768))
	rng := rand.New(rand.NewSource(424242))
	for i := 0; i < len(imageData.Pix); i += 4 {
		v := rng.Uint32()
		imageData.Pix[i] = byte(v)
		imageData.Pix[i+1] = byte(v >> 8)
		imageData.Pix[i+2] = byte(v >> 16)
		imageData.Pix[i+3] = 255
	}
	var encoded bytes.Buffer
	if e := jpeg.Encode(&encoded, imageData, &jpeg.Options{Quality: 87}); e != nil {
		t.Fatal(e)
	}
	data := encoded.Bytes()
	m, e := f.l.c.postImages.validator.Inspect(ctx, data)
	if e != nil {
		t.Fatal(e)
	}
	var identity uuid.UUID
	if e = f.l.cp.QueryRow(ctx, `SELECT identity_id FROM c_posts.publication_tasks WHERE post_id=$1`, f.post).Scan(&identity); e != nil {
		t.Fatal(e)
	}
	intent := postTestIntent(t, f.l, identity)
	selection := postmedia.Selection{MediaID: uuid.NewString(), SHA256: m.SHA256, ByteLength: m.ByteLength, MIME: m.MIME, Width: m.Width, Height: m.Height}
	intent.Images = []postmedia.Selection{selection}
	accepted := postTestCommand(t, f.l, f.owner.SessionToken, postTestKey(t), intent)
	grant := imageGrant(t, f.l, f.owner.SessionToken, accepted, selection.MediaID)
	imageUpload(t, f.l, accepted, selection, grant, data)
	postTestPublish(t, f.l)
	response, e := f.l.c.ReadPostImages(ctx, f.viewer.SessionToken, accepted.PostID, "")
	if e != nil {
		t.Fatal(e)
	}
	f.post, f.content = accepted.PostID, response.Value.(postmedia.Group).Images[0].ContentVersionID
	cache := f.l.c.postImages.thumbnails
	started := readtiming.SampleClock()
	cold, e := f.l.c.ReadPostThumbnail(ctx, f.viewer.SessionToken, f.post, f.content)
	coldElapsed, coldClockValid := readtiming.ElapsedClock(started, readtiming.SampleClock())
	if e != nil {
		t.Fatal(e)
	}
	if !coldClockValid {
		t.Fatal("cold clock invalid")
	}
	expected := cold.Value.(ImageBytes).Bytes
	measure := func(reuse bool) int64 {
		if reuse {
			f.l.c.postImages.thumbnails = cache
		} else {
			f.l.c.postImages.thumbnails = nil
		}
		samples := make([]int64, 0, 40)
		for i := 0; i < 45; i++ {
			traceCtx, _ := readtiming.Start(ctx)
			started := readtiming.SampleClock()
			r, err := f.l.c.ReadPostThumbnail(traceCtx, f.viewer.SessionToken, f.post, f.content)
			end := readtiming.SampleClock()
			elapsed, valid := readtiming.ElapsedClock(started, end)
			if err != nil || !bytes.Equal(r.Value.(ImageBytes).Bytes, expected) {
				t.Fatal("performance request failed or changed bytes", err)
			}
			if !valid {
				t.Fatalf("service clock invalid: raw=%d goLower=%d goUpper=%d brackets=%d", elapsed, end.Before.Sub(started.After).Nanoseconds(), end.After.Sub(started.Before).Nanoseconds(), started.After.Sub(started.Before).Nanoseconds()+end.After.Sub(end.Before).Nanoseconds())
			}
			if i >= 5 {
				samples = append(samples, elapsed)
			}
		}
		sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
		return samples[37] // ceil(40*.95)-1
	}
	type round struct {
		Round         int     `json:"round"`
		BaselineP95NS int64   `json:"baselineP95NS"`
		ReuseP95NS    int64   `json:"reuseP95NS"`
		Reduction     float64 `json:"reduction"`
	}
	var rounds []round
	for i := 0; i < 3; i++ {
		var old, new int64
		if i%2 == 0 {
			old = measure(false)
			new = measure(true)
		} else {
			new = measure(true)
			old = measure(false)
		}
		reduction := 1 - float64(new)/float64(old)
		rounds = append(rounds, round{i + 1, old, new, reduction})
	}
	f.l.c.postImages.thumbnails = cache
	report := struct {
		Scenario    string  `json:"scenario"`
		Seed        int     `json:"seed"`
		SourceBytes int     `json:"sourceBytes"`
		ColdNS      int64   `json:"coldNS"`
		Rounds      []round `json:"rounds"`
	}{"prepared-thumbnail-warm-server-empty-client", 424242, len(data), coldElapsed, rounds}
	raw, _ := json.Marshal(report)
	t.Log("THUMBNAIL_PERF", string(raw))
	for _, r := range rounds {
		if r.Reduction < .6 {
			t.Errorf("round %d failed 60%% hard threshold: %.2f%%", r.Round, 100*r.Reduction)
		}
	}
}

// TestThumbnailRedisPerformancePostgres compares the same binary with only the
// hot-cache attachment changed: A = PostgreSQL derivative reuse, B = Redis
// hit. Real PostgreSQL and a disposable real Redis are required. Production
// SLO/adoption thresholds remain product decisions, so this test reports
// paired distributions instead of imposing an arbitrary reduction.
func TestThumbnailRedisPerformancePostgres(t *testing.T) {
	if os.Getenv("POST_THUMBNAIL_PERFORMANCE") != "1" {
		t.Skip("explicit performance run required")
	}
	address := os.Getenv("THUMBNAIL_PERFORMANCE_REDIS_ADDR")
	if address == "" {
		t.Skip("THUMBNAIL_PERFORMANCE_REDIS_ADDR absent")
	}
	password := os.Getenv("THUMBNAIL_PERFORMANCE_REDIS_PASSWORD")
	username := os.Getenv("THUMBNAIL_PERFORMANCE_REDIS_USERNAME")
	if username == "" {
		username = "default"
	}
	f := thumbnailFixture(t)
	ctx := context.Background()
	hot, err := postthumbnailcache.New(postthumbnailcache.Config{
		Address: address, Username: username, PasswordFile: "unused-for-performance",
		Namespace: "hnuhole:post-thumbnail:v1:dev:", TTLSeconds: 300,
	}, password)
	if err != nil {
		t.Fatal(err)
	}
	defer hot.Close()
	if err = hot.Check(ctx); err != nil {
		t.Fatal(err)
	}
	c, err := f.l.c.WithPostThumbnailRedisCache(ctx, hot)
	if err != nil {
		t.Fatal(err)
	}
	f.l.c = c

	imageData := image.NewRGBA(image.Rect(0, 0, 1024, 768))
	rng := rand.New(rand.NewSource(515151))
	for i := 0; i < len(imageData.Pix); i += 4 {
		v := rng.Uint32()
		imageData.Pix[i], imageData.Pix[i+1], imageData.Pix[i+2], imageData.Pix[i+3] = byte(v), byte(v>>8), byte(v>>16), 255
	}
	var encoded bytes.Buffer
	if err = jpeg.Encode(&encoded, imageData, &jpeg.Options{Quality: 87}); err != nil {
		t.Fatal(err)
	}
	data := encoded.Bytes()
	m, err := f.l.c.postImages.validator.Inspect(ctx, data)
	if err != nil {
		t.Fatal(err)
	}
	var identity uuid.UUID
	if err = f.l.cp.QueryRow(ctx, `SELECT identity_id FROM c_posts.publication_tasks WHERE post_id=$1`, f.post).Scan(&identity); err != nil {
		t.Fatal(err)
	}
	intent := postTestIntent(t, f.l, identity)
	selection := postmedia.Selection{MediaID: uuid.NewString(), SHA256: m.SHA256, ByteLength: m.ByteLength, MIME: m.MIME, Width: m.Width, Height: m.Height}
	intent.Images = []postmedia.Selection{selection}
	accepted := postTestCommand(t, f.l, f.owner.SessionToken, postTestKey(t), intent)
	grant := imageGrant(t, f.l, f.owner.SessionToken, accepted, selection.MediaID)
	imageUpload(t, f.l, accepted, selection, grant, data)
	postTestPublish(t, f.l)
	response, err := f.l.c.ReadPostImages(ctx, f.viewer.SessionToken, accepted.PostID, "")
	if err != nil {
		t.Fatal(err)
	}
	f.post, f.content = accepted.PostID, response.Value.(postmedia.Group).Images[0].ContentVersionID
	expected := readThumbnail(t, f) // generates and persists the derivative
	readThumbnail(t, f)             // bounded PG fallback fills Redis after commit
	deadline := time.Now().Add(5 * time.Second)
	for hot.Stats().FillsSet == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if hot.Stats().FillsSet == 0 {
		t.Fatal("hot cache fill did not reach Redis")
	}

	measure := func(redis bool) int64 {
		if redis {
			f.l.c.postImages.thumbnails.redis = hot
		} else {
			f.l.c.postImages.thumbnails.redis = nil
		}
		samples := make([]int64, 0, 40)
		for i := 0; i < 45; i++ {
			traceCtx, _ := readtiming.Start(ctx)
			started := readtiming.SampleClock()
			r, e := f.l.c.ReadPostThumbnail(traceCtx, f.viewer.SessionToken, f.post, f.content)
			end := readtiming.SampleClock()
			elapsed, valid := readtiming.ElapsedClock(started, end)
			if e != nil || !bytes.Equal(r.Value.(ImageBytes).Bytes, expected) {
				t.Fatal("performance request failed or changed bytes", e)
			}
			if !valid {
				t.Fatal("service clock invalid")
			}
			if i >= 5 {
				samples = append(samples, elapsed)
			}
		}
		sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
		return samples[37]
	}
	type round struct {
		Round         int     `json:"round"`
		PostgresP95NS int64   `json:"postgresP95NS"`
		RedisP95NS    int64   `json:"redisP95NS"`
		Difference    float64 `json:"difference"`
	}
	var rounds []round
	for i := 0; i < 3; i++ {
		var postgres, redis int64
		if i%2 == 0 {
			postgres, redis = measure(false), measure(true)
		} else {
			redis, postgres = measure(true), measure(false)
		}
		rounds = append(rounds, round{i + 1, postgres, redis, 1 - float64(redis)/float64(postgres)})
	}
	f.l.c.postImages.thumbnails.redis = hot
	stats := hot.Stats()
	report := struct {
		Scenario    string  `json:"scenario"`
		Seed        int     `json:"seed"`
		SourceBytes int     `json:"sourceBytes"`
		Rounds      []round `json:"rounds"`
		Gets        uint64  `json:"gets"`
		Hits        uint64  `json:"hits"`
		Misses      uint64  `json:"misses"`
	}{"prepared-thumbnail-postgres-vs-redis", 515151, len(data), rounds, stats.Gets, stats.Hits, stats.Misses}
	raw, _ := json.Marshal(report)
	t.Log("THUMBNAIL_REDIS_PERF", string(raw))
}
