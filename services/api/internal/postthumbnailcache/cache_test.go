package postthumbnailcache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/postmedia"
)

const (
	testPost    = "0f8fad5b-d9cb-469f-a165-70867728950e"
	testContent = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
)

type fakeCall struct{ args []interface{} }

type fakeDoer struct {
	mu      sync.Mutex
	calls   []fakeCall
	respond func([]interface{}) (interface{}, error)
	closed  bool
}

func (f *fakeDoer) Do(ctx context.Context, args ...interface{}) *redis.Cmd {
	cmd := redis.NewCmd(ctx, args...)
	f.mu.Lock()
	f.calls = append(f.calls, fakeCall{args: append([]interface{}(nil), args...)})
	respond := f.respond
	f.mu.Unlock()
	if respond != nil {
		value, err := respond(args)
		cmd.SetVal(value)
		cmd.SetErr(err)
	}
	return cmd
}

func (f *fakeDoer) Close() error {
	f.mu.Lock()
	f.closed = true
	f.mu.Unlock()
	return nil
}

func (f *fakeDoer) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *fakeDoer) call(i int) []interface{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[i].args
}

func testConfig() Config {
	return Config{
		Address:        "127.0.0.1:6380",
		Username:       "community_thumbnail_cache",
		PasswordFile:   "/run/secrets/thumbnail-cache",
		Namespace:      "hnuhole:post-thumbnail:v1:dev:",
		FillQueueBytes: 1 << 20,
	}
}

func newTestCache(t *testing.T, cfg Config, client redisDoer) *Cache {
	t.Helper()
	cfg = cfg.Effective()
	ctx, cancel := context.WithCancel(context.Background())
	c := &Cache{config: cfg, client: client, now: time.Now,
		fill: make(chan fillJob, cfg.FillQueueEntries), fillDone: make(chan struct{}), cancel: cancel}
	c.breaker = breaker{limit: cfg.CircuitFailures, cooldown: time.Duration(cfg.CircuitCooldownMilliseconds) * time.Millisecond}
	go c.runFill(ctx)
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func testRecord(size int) postmedia.ThumbnailRecord {
	b := make([]byte, size)
	copy(b, "RIFF")
	copy(b[8:], "WEBP")
	for i := 12; i < len(b); i++ {
		b[i] = byte(i)
	}
	sum := sha256.Sum256(b)
	return postmedia.ThumbnailRecord{Bytes: b, Width: 640, Height: 480, SHA256: hex.EncodeToString(sum[:])}
}

func testKey(t *testing.T, c *Cache) string {
	t.Helper()
	key, err := c.Key(KeyParts{PostID: testPost, ContentVersion: testContent,
		Variant: postmedia.ThumbnailVariant, EncoderVersion: strings.Repeat("a1", 32),
		SourceBinding: strings.Repeat("b2", 32), OutputDigest: strings.Repeat("c3", 32)})
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func TestConfigEffectiveAndValidate(t *testing.T) {
	valid := testConfig()
	if err := valid.Validate("dev"); err != nil {
		t.Fatal(err)
	}
	effective := Config{}.Effective()
	if effective.TTLSeconds != DefaultTTLSeconds || effective.DeadlineMilliseconds != DefaultDeadlineMilliseconds ||
		effective.FillQueueEntries != DefaultFillQueueEntries || effective.FillQueueBytes != DefaultFillQueueBytes ||
		effective.DeleteBatch != DefaultDeleteBatch || effective.CircuitFailures != DefaultCircuitFailures {
		t.Fatal("defaults not applied", effective)
	}
	for _, tc := range []struct {
		name   string
		env    string
		mutate func(*Config)
	}{
		{"other environment", "dev", func(c *Config) { c.Namespace = "hnuhole:post-thumbnail:v1:prod:" }},
		{"unterminated namespace", "dev", func(c *Config) { c.Namespace = "hnuhole:post-thumbnail:v1:dev" }},
		{"address without port", "dev", func(c *Config) { c.Address = "127.0.0.1" }},
		{"username whitespace", "dev", func(c *Config) { c.Username = "a b" }},
		{"missing password file", "dev", func(c *Config) { c.PasswordFile = "" }},
		{"short ttl", "dev", func(c *Config) { c.TTLSeconds = 1 }},
		{"long deadline", "dev", func(c *Config) { c.DeadlineMilliseconds = 2000 }},
		{"small queue", "dev", func(c *Config) { c.FillQueueBytes = 1024 }},
		{"large batch", "dev", func(c *Config) { c.DeleteBatch = 1000 }},
		{"bad database", "dev", func(c *Config) { c.Database = 16 }},
		{"bad cooldown", "dev", func(c *Config) { c.CircuitCooldownMilliseconds = 1 }},
		{"environment separator", "d:ev", func(c *Config) {}},
		{"missing environment", "", func(c *Config) {}},
	} {
		cfg := valid
		tc.mutate(&cfg)
		if err := cfg.Validate(tc.env); err == nil {
			t.Fatalf("%s accepted", tc.name)
		}
	}
}

func TestNewRejectsUnsafeConfiguration(t *testing.T) {
	if _, err := New(testConfig(), ""); err == nil {
		t.Fatal("empty password accepted")
	}
	cfg := testConfig()
	cfg.Namespace = "hnuhole:post-thumbnail:v1:"
	if _, err := New(cfg, "secret"); err == nil {
		t.Fatal("empty environment namespace accepted")
	}
	badQueue := testConfig()
	badQueue.FillQueueEntries = -1
	if _, err := New(badQueue, "secret"); err == nil {
		t.Fatal("negative fill queue accepted")
	}
	c, err := New(testConfig(), "secret")
	if err != nil {
		t.Fatal(err)
	}
	if err = c.Close(); err != nil {
		t.Fatal(err)
	}
	if c.Close() != nil {
		t.Fatal("close is not idempotent")
	}
}

func TestKeyCanonicalization(t *testing.T) {
	c := newTestCache(t, testConfig(), &fakeDoer{})
	key := testKey(t, c)
	want := "hnuhole:post-thumbnail:v1:dev:data:" + testPost + ":" + testContent + ":" +
		postmedia.ThumbnailVariant + ":" + strings.Repeat("a1", 32) + ":" + strings.Repeat("b2", 32) + ":" + strings.Repeat("c3", 32)
	if key != want {
		t.Fatalf("key %q", key)
	}
	for _, parts := range []KeyParts{
		{PostID: strings.ToUpper(testPost), ContentVersion: testContent, Variant: postmedia.ThumbnailVariant, EncoderVersion: strings.Repeat("a1", 32), SourceBinding: strings.Repeat("b2", 32), OutputDigest: strings.Repeat("c3", 32)},
		{PostID: testPost, ContentVersion: testContent, Variant: "thumbnail-v2-640", EncoderVersion: strings.Repeat("a1", 32), SourceBinding: strings.Repeat("b2", 32), OutputDigest: strings.Repeat("c3", 32)},
		{PostID: testPost, ContentVersion: testContent, Variant: postmedia.ThumbnailVariant, EncoderVersion: strings.Repeat("A1", 32), SourceBinding: strings.Repeat("b2", 32), OutputDigest: strings.Repeat("c3", 32)},
		{PostID: testPost, ContentVersion: testContent, Variant: postmedia.ThumbnailVariant, EncoderVersion: strings.Repeat("a1", 31), SourceBinding: strings.Repeat("b2", 32), OutputDigest: strings.Repeat("c3", 32)},
		{PostID: "not-a-uuid", ContentVersion: testContent, Variant: postmedia.ThumbnailVariant, EncoderVersion: strings.Repeat("a1", 32), SourceBinding: strings.Repeat("b2", 32), OutputDigest: strings.Repeat("c3", 32)},
	} {
		if _, err := c.Key(parts); err == nil {
			t.Fatal("unsafe key accepted", parts)
		}
	}
}

func TestGetValidatesMetadataAndContainer(t *testing.T) {
	record := testRecord(64)
	client := &fakeDoer{respond: func(args []interface{}) (interface{}, error) {
		return record.Bytes, nil
	}}
	c := newTestCache(t, testConfig(), client)
	key := testKey(t, c)
	got, ok := c.Get(context.Background(), key, 640, 480, len(record.Bytes), record.SHA256)
	if !ok || !got.Valid() || string(got.Bytes) != string(record.Bytes) {
		t.Fatal("valid value rejected")
	}
	call := client.call(0)
	if len(call) != 4 || call[0] != "GETRANGE" || call[1] != key || call[2] != 0 ||
		call[3] != int64(postmedia.MaxThumbnailBytes) {
		t.Fatal("unexpected bounded read", call)
	}
	stats := c.Stats()
	if stats.Gets != 1 || stats.Hits != 1 || stats.Misses != 0 || stats.BadValues != 0 {
		t.Fatal("hit metrics", stats)
	}

	client.respond = func([]interface{}) (interface{}, error) { return record.Bytes, nil }
	if _, ok = c.Get(context.Background(), key, 640, 480, len(record.Bytes)-1, record.SHA256); ok {
		t.Fatal("length mismatch accepted")
	}
	if c.Stats().BadValues != 1 {
		t.Fatal("length mismatch not counted")
	}

	oversize := make([]byte, postmedia.MaxThumbnailBytes+1)
	client.respond = func([]interface{}) (interface{}, error) { return oversize, nil }
	if _, ok = c.Get(context.Background(), key, 640, 480, 64, record.SHA256); ok {
		t.Fatal("oversize value accepted")
	}

	client.respond = func([]interface{}) (interface{}, error) { return nil, nil }
	if _, ok = c.Get(context.Background(), key, 640, 480, 64, record.SHA256); ok || c.Stats().Misses != 1 {
		t.Fatal("miss not reported", c.Stats())
	}

	client.respond = func([]interface{}) (interface{}, error) { return nil, context.DeadlineExceeded }
	if _, ok = c.Get(context.Background(), key, 640, 480, 64, record.SHA256); ok || c.Stats().Timeouts != 1 {
		t.Fatal("timeout not reported", c.Stats())
	}

	client.respond = func([]interface{}) (interface{}, error) { return nil, errors.New("boom") }
	if _, ok = c.Get(context.Background(), key, 640, 480, 64, record.SHA256); ok || c.Stats().Errors != 1 {
		t.Fatal("error not reported", c.Stats())
	}

	calls := client.count()
	if _, ok = c.Get(context.Background(), key, 640, 480, 64, strings.ToUpper(record.SHA256)); ok || client.count() != calls {
		t.Fatal("invalid digest reached Redis")
	}
	if _, ok = c.Get(context.Background(), "", 640, 480, 64, record.SHA256); ok || client.count() != calls {
		t.Fatal("empty key reached Redis")
	}
}

func TestBreakerBypassesThenProbes(t *testing.T) {
	record := testRecord(32)
	client := &fakeDoer{respond: func([]interface{}) (interface{}, error) { return nil, errors.New("down") }}
	cfg := testConfig()
	cfg.CircuitFailures = 2
	cfg.CircuitCooldownMilliseconds = 40
	c := newTestCache(t, cfg, client)
	now := time.Unix(1_700_000_000, 0)
	c.now = func() time.Time { return now }
	key := testKey(t, c)
	for i := 0; i < 2; i++ {
		if _, ok := c.Get(context.Background(), key, 640, 480, len(record.Bytes), record.SHA256); ok {
			t.Fatal("failure reported as hit")
		}
	}
	if client.count() != 2 {
		t.Fatal("unexpected calls", client.count())
	}
	if _, ok := c.Get(context.Background(), key, 640, 480, len(record.Bytes), record.SHA256); ok || client.count() != 2 {
		t.Fatal("open breaker did not bypass", client.count())
	}
	if c.Stats().Bypassed != 1 {
		t.Fatal("bypass not counted")
	}
	now = now.Add(41 * time.Millisecond)
	client.respond = func([]interface{}) (interface{}, error) { return nil, nil }
	if _, ok := c.Get(context.Background(), key, 640, 480, len(record.Bytes), record.SHA256); ok || client.count() != 3 {
		t.Fatal("probe not sent", client.count())
	}
	if _, ok := c.Get(context.Background(), key, 640, 480, len(record.Bytes), record.SHA256); ok || client.count() != 4 {
		t.Fatal("breaker did not close", client.count())
	}
}

func TestEnqueueFillsWithAbsoluteDeadline(t *testing.T) {
	client := &fakeDoer{respond: func(args []interface{}) (interface{}, error) { return "OK", nil }}
	c := newTestCache(t, testConfig(), client)
	key := testKey(t, c)
	record := testRecord(96)
	deadline := time.Now().Add(250 * time.Millisecond).Round(time.Millisecond)
	if !c.Enqueue(key, record, deadline) {
		t.Fatal("fill rejected")
	}
	waitFor(t, func() bool { return c.Stats().FillsSet == 1 })
	call := client.call(0)
	if len(call) != 6 || call[0] != "SET" || call[1] != key || call[3] != "NX" || call[4] != "PXAT" ||
		call[5] != fmt.Sprint(deadline.UnixMilli()) {
		t.Fatal("unexpected fill command", call)
	}
	if !c.Enqueue(key, record, time.Now().Add(-time.Second)) {
		// A past deadline must be dropped with a distinct counter.
	}
	if c.Stats().FillsExpired != 1 || c.Stats().FillsDropped != 0 {
		t.Fatal("expired fill counted incorrectly", c.Stats())
	}
	c.closed.Store(true)
	if c.Enqueue(key, record, time.Now().Add(time.Minute)) {
		t.Fatal("closed cache queued a fill")
	}
	if c.Stats().FillsDropped != 1 {
		t.Fatal("closed fill not counted", c.Stats())
	}
}

func TestFillExistingAndInvalidContainer(t *testing.T) {
	client := &fakeDoer{respond: func(args []interface{}) (interface{}, error) { return nil, redis.Nil }}
	c := newTestCache(t, testConfig(), client)
	key := testKey(t, c)
	if !c.Enqueue(key, testRecord(32), time.Now().Add(time.Minute)) {
		t.Fatal("fill rejected")
	}
	waitFor(t, func() bool { return c.Stats().FillsExisting == 1 })

	bad := testRecord(32)
	copy(bad.Bytes[8:], "NOPE")
	if c.Enqueue(key, bad, time.Now().Add(time.Minute)) {
		// Enqueue accepts it; the worker rejects the container.
	}
	waitFor(t, func() bool { return c.Stats().FillsFailed == 1 })
}

func TestEnqueueBoundedQueue(t *testing.T) {
	cfg := testConfig()
	cfg.FillQueueEntries = 1
	c := &Cache{config: cfg.Effective(), client: &fakeDoer{}, now: time.Now,
		fill: make(chan fillJob, 1), fillDone: make(chan struct{}), cancel: func() {}}
	record := testRecord(64)
	key := testKey(t, c)
	if !c.Enqueue(key, record, time.Now().Add(time.Minute)) {
		t.Fatal("first fill rejected")
	}
	if c.Enqueue(key, record, time.Now().Add(time.Minute)) {
		t.Fatal("queue entry cap ignored")
	}

	cfg.FillQueueEntries = 8
	cfg.FillQueueBytes = 1 << 20
	byBytes := &Cache{config: cfg.Effective(), client: &fakeDoer{}, now: time.Now,
		fill: make(chan fillJob, 8), fillDone: make(chan struct{}), cancel: func() {}}
	big := testRecord(400 << 10)
	k := testKey(t, byBytes)
	for i := 0; i < 2; i++ {
		if !byBytes.Enqueue(k, big, time.Now().Add(time.Minute)) {
			t.Fatal("fill within budget rejected", i)
		}
	}
	if byBytes.Enqueue(k, big, time.Now().Add(time.Minute)) {
		t.Fatal("byte budget ignored")
	}
	if byBytes.Stats().FillsDropped != 1 || byBytes.Stats().QueueBytes != 800<<10 {
		t.Fatal("byte accounting", byBytes.Stats())
	}
}

func TestDeleteAndCheck(t *testing.T) {
	client := &fakeDoer{respond: func(args []interface{}) (interface{}, error) {
		switch args[0] {
		case "UNLINK":
			return int64(1), nil
		case "SET":
			return "OK", nil
		case "GETRANGE":
			return "1", nil
		case "PING":
			return "PONG", nil
		default:
			return nil, errors.New("unexpected command")
		}
	}}
	c := newTestCache(t, testConfig(), client)
	key := testKey(t, c)
	if err := c.Delete(context.Background(), key); err != nil {
		t.Fatal(err)
	}
	call := client.call(0)
	if call[0] != "UNLINK" || call[1] != key {
		t.Fatal("delete command", call)
	}
	if c.Stats().Deletes != 1 {
		t.Fatal("delete not counted")
	}
	if err := c.Check(context.Background()); err != nil {
		t.Fatal(err)
	}

	failing := &fakeDoer{respond: func([]interface{}) (interface{}, error) { return nil, errors.New("down") }}
	broken := newTestCache(t, testConfig(), failing)
	if err := broken.Delete(context.Background(), key); err == nil || broken.Stats().DeleteErrors != 1 {
		t.Fatal("delete error not reported", broken.Stats())
	}
}

func TestCheckProvesWriteExpiryAndUnlink(t *testing.T) {
	ok := func(args []interface{}) (interface{}, error) {
		switch args[0] {
		case "SET":
			return "OK", nil
		case "GETRANGE":
			return "1", nil
		case "PING":
			return "PONG", nil
		case "UNLINK":
			return int64(1), nil
		default:
			return nil, errors.New("unexpected command")
		}
	}
	client := &fakeDoer{respond: ok}
	c := newTestCache(t, testConfig(), client)
	if err := c.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	setCall := client.call(0)
	if len(setCall) != 6 || setCall[0] != "SET" || setCall[3] != "NX" || setCall[4] != "PXAT" {
		t.Fatal("check did not probe the fill command", setCall)
	}
	if got := client.call(client.count() - 1); got[0] != "UNLINK" {
		t.Fatal("check did not clean up the probe", got)
	}

	// An ACL with +set but without +unlink must fail startup instead of
	// silently retaining erased keys until their TTL.
	denied := &fakeDoer{respond: func(args []interface{}) (interface{}, error) {
		if args[0] == "UNLINK" {
			return nil, errors.New("NOPERM no permissions to access a key")
		}
		return ok(args)
	}}
	if err := newTestCache(t, testConfig(), denied).Check(context.Background()); err == nil {
		t.Fatal("missing unlink permission accepted")
	}
}

func TestStatsNeverExposeKeys(t *testing.T) {
	record := testRecord(32)
	client := &fakeDoer{respond: func([]interface{}) (interface{}, error) { return record.Bytes, nil }}
	c := newTestCache(t, testConfig(), client)
	key := testKey(t, c)
	c.Get(context.Background(), key, 640, 480, len(record.Bytes), record.SHA256)
	rendered := fmt.Sprintf("%+v", c.Stats())
	for _, secret := range []string{testPost, testContent, key} {
		if strings.Contains(rendered, secret) {
			t.Fatal("stats exposed resource identity")
		}
	}
}

func waitFor(t *testing.T, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if done() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition not reached")
}
