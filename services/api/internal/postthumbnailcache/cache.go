package postthumbnailcache

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/postmedia"
)

var (
	errKey      = errors.New("invalid post thumbnail cache key")
	errBypassed = errors.New("post thumbnail cache temporarily bypassed")
	errClosed   = errors.New("post thumbnail cache closed")
	errBadValue = errors.New("invalid post thumbnail cache value")
)

// KeyParts are the immutable identities of one stored derivative. The
// constructor rejects anything that could change the physical key shape.
type KeyParts struct {
	PostID         string
	ContentVersion string
	Variant        string
	EncoderVersion string
	SourceBinding  string
	OutputDigest   string
}

// Stats contains aggregate counters only, never keys or resource identifiers.
type Stats struct {
	Gets, Hits, Misses, Timeouts, Errors, BadValues, Bypassed uint64
	FillsQueued, FillsSet, FillsExisting, FillsDropped        uint64
	FillsExpired, FillsFailed, Deletes, DeleteErrors          uint64
	QueueEntries                                              int
	QueueBytes                                                int64
}

type fillJob struct {
	key      string
	value    []byte
	deadline time.Time
}

// Cache is safe for concurrent request, fill and deletion workers. It owns no
// protected authorization result; callers must independently authorize.
type Cache struct {
	config Config
	client redisDoer
	now    func() time.Time

	gets, hits, misses, timeouts, errorsCount, badValues, bypassed atomic.Uint64
	fillsQueued, fillsSet, fillsExisting, fillsDropped             atomic.Uint64
	fillsExpired, fillsFailed, deletes, deleteErrors               atomic.Uint64

	fillMu    sync.Mutex
	fillBytes int64
	fill      chan fillJob
	fillDone  chan struct{}
	cancel    context.CancelFunc
	closeOnce sync.Once
	closed    atomic.Bool

	breaker breaker
}

// redisDoer is the narrow command surface used here. Keeping it small makes
// the cache testable without a live Redis process.
type redisDoer interface {
	Do(ctx context.Context, args ...interface{}) *redis.Cmd
	Close() error
}

// New applies defaults, validates the key namespace, opens one bounded client
// and starts the fill worker. Credentials are passed separately from Config so
// they never enter JSON or logs.
func New(config Config, password string) (*Cache, error) {
	config = config.Effective()
	if password == "" || !hasNamespacePrefix(config.Namespace) {
		return nil, errConfig
	}
	if err := config.validateRuntime(); err != nil {
		return nil, err
	}
	deadline := time.Duration(config.DeadlineMilliseconds) * time.Millisecond
	client := redis.NewClient(&redis.Options{
		Addr:                  config.Address,
		Username:              config.Username,
		Password:              password,
		DB:                    config.Database,
		PoolSize:              8,
		MaxRetries:            -1,
		DialTimeout:           deadline,
		ReadTimeout:           deadline,
		WriteTimeout:          deadline,
		PoolTimeout:           deadline,
		ContextTimeoutEnabled: true,
	})
	ctx, cancel := context.WithCancel(context.Background())
	c := &Cache{config: config, client: client, now: time.Now,
		fill: make(chan fillJob, config.FillQueueEntries), fillDone: make(chan struct{}), cancel: cancel}
	c.breaker = breaker{limit: config.CircuitFailures, cooldown: time.Duration(config.CircuitCooldownMilliseconds) * time.Millisecond}
	go c.runFill(ctx)
	return c, nil
}

func hasNamespacePrefix(namespace string) bool {
	if len(namespace) <= len(namespacePrefix) || namespace[:len(namespacePrefix)] != namespacePrefix {
		return false
	}
	return namespace[len(namespace)-1] == ':'
}

// Key builds the one physical key used by GET, backfill and deletion. All
// variable parts are canonicalized and length/character checked.
func (c *Cache) Key(p KeyParts) (string, error) {
	post, err := uuid.Parse(p.PostID)
	if err != nil || post.String() != p.PostID {
		return "", errKey
	}
	content, err := uuid.Parse(p.ContentVersion)
	if err != nil || content.String() != p.ContentVersion {
		return "", errKey
	}
	if p.Variant != postmedia.ThumbnailVariant || !isLowerHex64(p.EncoderVersion) ||
		!isLowerHex64(p.SourceBinding) || !isLowerHex64(p.OutputDigest) {
		return "", errKey
	}
	return c.config.Namespace + "data:" + post.String() + ":" + content.String() + ":" +
		p.Variant + ":" + p.EncoderVersion + ":" + p.SourceBinding + ":" + p.OutputDigest, nil
}

func isLowerHex64(s string) bool {
	if len(s) != 64 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

// Get returns a candidate only when its length, dimensions, WebP container and
// digest match the PostgreSQL metadata supplied by the caller. Every failure
// is a miss; no error detail crosses the request boundary.
func (c *Cache) Get(ctx context.Context, key string, width, height, byteLength int, digest string) (postmedia.ThumbnailRecord, bool) {
	var zero postmedia.ThumbnailRecord
	if key == "" || width < 1 || width > 640 || height < 1 || height > 640 ||
		byteLength < 12 || byteLength > postmedia.MaxThumbnailBytes || !isLowerHex64(digest) {
		c.badValues.Add(1)
		return zero, false
	}
	if !c.breaker.allow(c.now()) {
		c.bypassed.Add(1)
		return zero, false
	}
	c.gets.Add(1)
	// One command reads at most max+1 bytes. Redis range ends are inclusive, so
	// a value larger than the protocol cap is detected without buffering it all.
	commandCtx, cancel := c.commandContext(ctx)
	defer cancel()
	raw, err := cmdBytes(c.client.Do(commandCtx, "GETRANGE", key, 0, int64(postmedia.MaxThumbnailBytes)))
	if err != nil {
		c.fail(err)
		return zero, false
	}
	if len(raw) == 0 {
		c.misses.Add(1)
		c.breaker.succeeded()
		return zero, false
	}
	if len(raw) > postmedia.MaxThumbnailBytes || len(raw) != byteLength {
		c.badValues.Add(1)
		return zero, false
	}
	record := postmedia.ThumbnailRecord{Bytes: raw, Width: width, Height: height, SHA256: digest}
	if !record.Valid() {
		c.badValues.Add(1)
		return zero, false
	}
	c.breaker.succeeded()
	c.hits.Add(1)
	return record, true
}

// Enqueue schedules a post-commit fill. It never blocks the HTTPS request: a
// full queue, a closed cache or a past deadline drops the fill and is counted.
func (c *Cache) Enqueue(key string, record postmedia.ThumbnailRecord, deadline time.Time) bool {
	if c == nil || c.closed.Load() || key == "" || len(record.Bytes) < 12 ||
		len(record.Bytes) > postmedia.MaxThumbnailBytes || !isLowerHex64(record.SHA256) {
		c.dropped()
		return false
	}
	if !deadline.After(c.now()) {
		c.fillsExpired.Add(1)
		return false
	}
	n := int64(len(record.Bytes))
	c.fillMu.Lock()
	if len(c.fill) >= cap(c.fill) || c.fillBytes+n > c.config.FillQueueBytes {
		c.fillMu.Unlock()
		c.dropped()
		return false
	}
	c.fillBytes += n
	c.fillMu.Unlock()
	job := fillJob{key: key, value: record.Bytes, deadline: deadline}
	select {
	case c.fill <- job:
		c.fillsQueued.Add(1)
		return true
	default:
		c.releaseBytes(n)
		c.dropped()
		return false
	}
}

func (c *Cache) dropped() {
	if c != nil {
		c.fillsDropped.Add(1)
	}
}

// Delete removes one exact key. Callers retry the event on an error; an absent
// key is a successful deletion. UNLINK keeps large values off the main thread.
func (c *Cache) Delete(ctx context.Context, key string) error {
	if c == nil || c.closed.Load() {
		return errClosed
	}
	if key == "" {
		return errKey
	}
	if !c.breaker.allow(c.now()) {
		c.bypassed.Add(1)
		return errBypassed
	}
	commandCtx, cancel := c.commandContext(ctx)
	defer cancel()
	if _, err := c.client.Do(commandCtx, "UNLINK", key).Int64(); err != nil {
		c.fail(err)
		c.deleteErrors.Add(1)
		return err
	}
	c.deletes.Add(1)
	c.breaker.succeeded()
	return nil
}

// Check performs a bounded write/read/unlink probe. It proves the address,
// credentials, absolute-expiry write syntax and the exact command ACL before
// the runtime starts serving.
func (c *Cache) Check(ctx context.Context) (retErr error) {
	if c == nil || c.client == nil {
		return errClosed
	}
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return errConfig
	}
	probe := c.config.Namespace + "probe:" + hex.EncodeToString(buf)
	checkCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	// PXAT is the expiry form used by fills. Probing it here fails closed on a
	// server that would otherwise accept the connection but reject every fill.
	expires := strconv.FormatInt(time.Now().Add(30*time.Second).UnixMilli(), 10)
	if err := c.client.Do(checkCtx, "SET", probe, "1", "NX", "PXAT", expires).Err(); err != nil {
		return fmt.Errorf("thumbnail redis write probe failed: %w", err)
	}
	// UNLINK is required by the deletion outbox worker. Ignoring its result
	// would let an ACL without +unlink pass startup and retain bytes until TTL.
	defer func() {
		if err := c.client.Do(context.WithoutCancel(ctx), "UNLINK", probe).Err(); err != nil && retErr == nil {
			retErr = fmt.Errorf("thumbnail redis unlink probe failed: %w", err)
		}
	}()
	value, err := cmdBytes(c.client.Do(checkCtx, "GETRANGE", probe, 0, 0))
	if err != nil || string(value) != "1" {
		return fmt.Errorf("thumbnail redis range probe failed")
	}
	if _, err := c.client.Do(checkCtx, "PING").Result(); err != nil {
		return fmt.Errorf("thumbnail redis ping failed: %w", err)
	}
	return nil
}

func (c *Cache) Stats() Stats {
	if c == nil {
		return Stats{}
	}
	c.fillMu.Lock()
	entries, bytes := len(c.fill), c.fillBytes
	c.fillMu.Unlock()
	return Stats{
		Gets: c.gets.Load(), Hits: c.hits.Load(), Misses: c.misses.Load(),
		Timeouts: c.timeouts.Load(), Errors: c.errorsCount.Load(), BadValues: c.badValues.Load(),
		Bypassed:    c.bypassed.Load(),
		FillsQueued: c.fillsQueued.Load(), FillsSet: c.fillsSet.Load(),
		FillsExisting: c.fillsExisting.Load(), FillsDropped: c.fillsDropped.Load(),
		FillsExpired: c.fillsExpired.Load(), FillsFailed: c.fillsFailed.Load(),
		Deletes: c.deletes.Load(), DeleteErrors: c.deleteErrors.Load(),
		QueueEntries: entries, QueueBytes: bytes,
	}
}

// TTL is the maximum logical lifetime of a filled key. The request path anchors
// the absolute deadline to its final authorization time, never to fill time.
func (c *Cache) TTL() time.Duration {
	return time.Duration(c.config.TTLSeconds) * time.Second
}

// DeleteBatch is the bounded size used by the PostgreSQL outbox claimer.
func (c *Cache) DeleteBatch() int {
	return c.config.DeleteBatch
}

// Close stops new fills, drains the bounded queue for a short grace period,
// then closes the client. It is idempotent.
func (c *Cache) Close() error {
	if c == nil {
		return nil
	}
	var err error
	c.closeOnce.Do(func() {
		c.closed.Store(true)
		c.cancel()
		<-c.fillDone
		err = c.client.Close()
	})
	return err
}

func (c *Cache) commandContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, time.Duration(c.config.DeadlineMilliseconds)*time.Millisecond)
}

func (c *Cache) fail(err error) {
	if errors.Is(err, redis.Nil) {
		return
	}
	var timeout interface{ Timeout() bool }
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &timeout) && timeout.Timeout()) {
		c.timeouts.Add(1)
	} else {
		c.errorsCount.Add(1)
	}
	c.breaker.failed(c.now())
}

func (c *Cache) runFill(ctx context.Context) {
	defer close(c.fillDone)
	for {
		select {
		case <-ctx.Done():
			c.drainFill()
			return
		case job := <-c.fill:
			c.setFill(job)
			c.releaseBytes(int64(len(job.value)))
		}
	}
}

func (c *Cache) drainFill() {
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		select {
		case job := <-c.fill:
			c.setFill(job)
			c.releaseBytes(int64(len(job.value)))
		default:
			return
		}
	}
}

func (c *Cache) setFill(job fillJob) {
	if !job.deadline.After(c.now()) {
		c.fillsExpired.Add(1)
		return
	}
	// Length, container and deadline are rechecked on the worker; the digest
	// and source binding were checked against PostgreSQL before enqueueing.
	if len(job.value) < 12 || len(job.value) > postmedia.MaxThumbnailBytes ||
		string(job.value[:4]) != "RIFF" || string(job.value[8:12]) != "WEBP" {
		c.fillsFailed.Add(1)
		return
	}
	commandCtx, cancel := c.commandContext(context.Background())
	defer cancel()
	cmd := c.client.Do(commandCtx, "SET", job.key, job.value, "NX", "PXAT", strconv.FormatInt(job.deadline.UnixMilli(), 10))
	if err := cmd.Err(); err != nil {
		if errors.Is(err, redis.Nil) {
			c.fillsExisting.Add(1)
			return
		}
		c.fillsFailed.Add(1)
		c.fail(err)
		return
	}
	c.fillsSet.Add(1)
	c.breaker.succeeded()
}

func (c *Cache) releaseBytes(n int64) {
	c.fillMu.Lock()
	c.fillBytes -= n
	if c.fillBytes < 0 {
		c.fillBytes = 0
	}
	c.fillMu.Unlock()
}

func cmdBytes(cmd *redis.Cmd) ([]byte, error) {
	result, err := cmd.Result()
	if err != nil {
		return nil, err
	}
	switch value := result.(type) {
	case string:
		return []byte(value), nil
	case []byte:
		return value, nil
	case nil:
		return nil, nil
	default:
		return nil, errBadValue
	}
}

// breaker short-circuits the request path for a short interval after repeated
// failures, then lets one probe through. It never delays a request.
type breaker struct {
	mu       sync.Mutex
	failures int
	limit    int
	cooldown time.Duration
	openAt   time.Time
	probing  bool
}

func (b *breaker) allow(now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.openAt.IsZero() {
		return true
	}
	if now.Before(b.openAt) {
		return false
	}
	if b.probing {
		return false
	}
	b.probing = true
	return true
}

func (b *breaker) failed(now time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.probing = false
	b.failures++
	if b.limit > 0 && b.failures >= b.limit {
		b.openAt = now.Add(b.cooldown)
		b.failures = 0
	}
}

func (b *breaker) succeeded() {
	b.mu.Lock()
	b.failures = 0
	b.openAt = time.Time{}
	b.probing = false
	b.mu.Unlock()
}
