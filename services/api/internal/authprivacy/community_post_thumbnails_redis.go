package authprivacy

import (
	"context"
	"encoding/hex"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/postmedia"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/postthumbnailcache"
)

// thumbnailPGResolveTimeout bounds the cache-miss fallback. The normal hot
// path never takes this connection; only a Redis miss/failure does.
const thumbnailPGResolveTimeout = 2 * time.Second

// thumbnailHotCache is the small surface used by request and deletion paths.
// The concrete Redis client is assembled only by the explicit runtime opt-in.
type thumbnailHotCache interface {
	Key(postthumbnailcache.KeyParts) (string, error)
	Get(context.Context, string, int, int, int, string) (postmedia.ThumbnailRecord, bool)
	Enqueue(string, postmedia.ThumbnailRecord, time.Time) bool
	Delete(context.Context, string) error
	Stats() postthumbnailcache.Stats
	TTL() time.Duration
	DeleteBatch() int
}

// storedThumbnailMeta is the PostgreSQL authority for one derivative row. It
// never contains image bytes; Redis candidates are checked against it.
type storedThumbnailMeta struct {
	width, height, byteLength int
	digest                    string
	binding                   [32]byte
}

type thumbnailRowQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

// WithPostThumbnailRedisCache attaches an independent, discardable Redis copy
// above the existing PostgreSQL derivative path. PostgreSQL remains required
// for the source proof, derivative identity and every final authorization.
func (c *Community) WithPostThumbnailRedisCache(ctx context.Context, cache thumbnailHotCache) (*Community, error) {
	if cache == nil || !c.PostImageThumbnailsEnabled() || !c.posts.preparedMediaReads || !c.nativePreparedReads() {
		return nil, ErrAuthorizationUnavailable
	}
	if c.postImages == nil || c.postImages.thumbnails == nil {
		return nil, ErrAuthorizationUnavailable
	}
	if err := c.checkThumbnailCacheSchema(ctx); err != nil {
		return nil, err
	}
	clone := *c
	images := *c.postImages
	// Copy fields explicitly: thumbnailReuse owns a singleflight.Group and
	// counters that must not be duplicated by struct assignment.
	reuse := thumbnailReuse{encoder: c.postImages.thumbnails.encoder, version: c.postImages.thumbnails.version,
		redis: cache, workers: c.postImages.thumbnails.workers, queue: c.postImages.thumbnails.queue}
	images.thumbnails = &reuse
	clone.postImages = &images
	return &clone, nil
}

// readStoredThumbnailMetadata reads only the bounded row identity. The request
// path uses it to build the Redis key without touching payload.
func readStoredThumbnailMetadata(ctx context.Context, q thumbnailRowQuerier, post string, item preparedPostMediaItem, version string) (storedThumbnailMeta, bool, error) {
	var meta storedThumbnailMeta
	meta.binding = thumbnailSourceBinding(item)
	err := q.QueryRow(ctx, `SELECT width,height,encode(digest,'hex'),byte_length FROM c_posts.thumbnails
 WHERE post_id=$1 AND content_version_id=$2 AND variant=$3 AND encoder_version=$4 AND source_binding=$5`,
		post, item.ContentID, postmedia.ThumbnailVariant, version, meta.binding[:]).Scan(
		&meta.width, &meta.height, &meta.digest, &meta.byteLength)
	if errors.Is(err, pgx.ErrNoRows) {
		return storedThumbnailMeta{}, false, nil
	}
	if err != nil {
		return storedThumbnailMeta{}, false, err
	}
	if meta.width < 1 || meta.width > 640 || meta.height < 1 || meta.height > 640 ||
		meta.byteLength < 12 || meta.byteLength > postmedia.MaxThumbnailBytes || !thumbnailLowerHex64(meta.digest) {
		return storedThumbnailMeta{}, false, nil
	}
	return meta, true, nil
}

// readStoredThumbnailPayload is the bounded PG fallback after a Redis miss. It
// re-reads the exact identity and rejects any row that changed since prepare;
// stale metadata never vouches for newer bytes.
func readStoredThumbnailPayload(ctx context.Context, pool *pgxpool.Pool, post string, item preparedPostMediaItem, version string, meta storedThumbnailMeta) (postmedia.ThumbnailRecord, bool, error) {
	bounded, cancel := context.WithTimeout(ctx, thumbnailPGResolveTimeout)
	defer cancel()
	digest, err := hex.DecodeString(meta.digest)
	if err != nil || len(digest) != 32 {
		return postmedia.ThumbnailRecord{}, false, nil
	}
	var record postmedia.ThumbnailRecord
	var width, height, byteLength int
	var storedDigest []byte
	err = pool.QueryRow(bounded, `SELECT payload,width,height,digest,byte_length FROM c_posts.thumbnails
 WHERE post_id=$1 AND content_version_id=$2 AND variant=$3 AND encoder_version=$4 AND source_binding=$5 AND digest=$6`,
		post, item.ContentID, postmedia.ThumbnailVariant, version, meta.binding[:], digest).Scan(
		&record.Bytes, &width, &height, &storedDigest, &byteLength)
	if errors.Is(err, pgx.ErrNoRows) {
		return postmedia.ThumbnailRecord{}, false, nil
	}
	if err != nil {
		return postmedia.ThumbnailRecord{}, false, err
	}
	if width != meta.width || height != meta.height || byteLength != meta.byteLength ||
		byteLength != len(record.Bytes) || hex.EncodeToString(storedDigest) != meta.digest {
		return postmedia.ThumbnailRecord{}, false, nil
	}
	record.Width, record.Height, record.SHA256 = width, height, meta.digest
	if !record.Valid() {
		return postmedia.ThumbnailRecord{}, false, nil
	}
	return record, true, nil
}

func thumbnailLowerHex64(value string) bool {
	if len(value) != 64 {
		return false
	}
	for i := 0; i < len(value); i++ {
		c := value[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

const (
	thumbnailCacheLeaseSeconds       = 30
	thumbnailCacheRetryCapSeconds    = 300
	thumbnailCacheCompletedRetention = time.Hour
	thumbnailCacheStalePending       = 24 * time.Hour
)

type thumbnailCacheEvent struct {
	EventID, LeaseToken, PostID, ContentVersion uuid.UUID
	Variant, EncoderVersion                     string
	SourceBinding, OutputDigest                 []byte
	Attempts                                    int
}

// RunPostThumbnailCacheMaintenance consumes transactional deletion events.
// Redis commands run outside the claim transaction; an unknown result leaves
// the event leased and retryable by this or another instance.
func (c *Community) RunPostThumbnailCacheMaintenance(ctx context.Context) {
	if c == nil || c.postImages == nil || c.postImages.thumbnails == nil || c.postImages.thumbnails.redis == nil {
		return
	}
	cache := c.postImages.thumbnails.redis
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	ticks := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			step, cancel := context.WithTimeout(ctx, 5*time.Second)
			if _, err := c.processThumbnailCacheDeletions(step, cache.DeleteBatch()); err != nil {
				slog.Warn("thumbnail cache deletion worker unavailable")
			}
			cancel()
			ticks++
			if ticks%30 == 0 {
				step, cancel := context.WithTimeout(ctx, 5*time.Second)
				if stats, ok, err := c.PostThumbnailCacheStats(step); err == nil && ok {
					slog.Info("thumbnail cache metrics", "gets", stats.Gets, "hits", stats.Hits, "misses", stats.Misses,
						"timeouts", stats.Timeouts, "errors", stats.Errors, "bad_values", stats.BadValues,
						"bypassed", stats.Bypassed, "fills_queued", stats.FillsQueued, "fills_set", stats.FillsSet,
						"fills_existing", stats.FillsExisting, "fills_dropped", stats.FillsDropped,
						"fills_expired", stats.FillsExpired, "fills_failed", stats.FillsFailed,
						"deletes", stats.Deletes, "delete_errors", stats.DeleteErrors,
						"queue_entries", stats.QueueEntries, "queue_bytes", stats.QueueBytes,
						"outbox_pending", stats.Pending, "outbox_retrying", stats.Retrying,
						"outbox_oldest_seconds", stats.OldestSeconds)
				}
				cancel()
			}
		}
	}
}

// ThumbnailCacheStats combines in-process counters with bounded outbox depth.
type ThumbnailCacheStats struct {
	postthumbnailcache.Stats
	Pending, Retrying, OldestSeconds int64
}

func (c *Community) PostThumbnailCacheStats(ctx context.Context) (ThumbnailCacheStats, bool, error) {
	var stats ThumbnailCacheStats
	if c == nil || c.postImages == nil || c.postImages.thumbnails == nil || c.postImages.thumbnails.redis == nil {
		return stats, false, nil
	}
	stats.Stats = c.postImages.thumbnails.redis.Stats()
	err := c.pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE completed_at IS NULL),
 count(*) FILTER (WHERE completed_at IS NULL AND attempts>0),
 COALESCE(EXTRACT(EPOCH FROM now()-min(created_at) FILTER (WHERE completed_at IS NULL)),0)::bigint
 FROM c_posts.thumbnail_cache_outbox`).Scan(&stats.Pending, &stats.Retrying, &stats.OldestSeconds)
	return stats, true, err
}

func (c *Community) processThumbnailCacheDeletions(ctx context.Context, limit int) (int, error) {
	if limit < 1 || limit > 256 {
		limit = 32
	}
	events, err := c.claimThumbnailCacheEvents(ctx, limit)
	if err != nil {
		return 0, err
	}
	completed := 0
	for _, event := range events {
		cache := c.postImages.thumbnails.redis
		key, e := cache.Key(postthumbnailcache.KeyParts{
			PostID:         event.PostID.String(),
			ContentVersion: event.ContentVersion.String(),
			Variant:        event.Variant,
			EncoderVersion: event.EncoderVersion,
			SourceBinding:  hex.EncodeToString(event.SourceBinding),
			OutputDigest:   hex.EncodeToString(event.OutputDigest),
		})
		if e != nil {
			// Trigger constraints already prevent malformed rows. If one appears
			// anyway it can never address a real key; complete it rather than
			// block the bounded queue forever.
			if c.completeThumbnailCacheEvent(ctx, event) {
				completed++
			}
			continue
		}
		if e = cache.Delete(ctx, key); e != nil {
			c.retryThumbnailCacheEvent(ctx, event)
			continue
		}
		if c.completeThumbnailCacheEvent(ctx, event) {
			completed++
		}
	}
	return completed, nil
}

func (c *Community) claimThumbnailCacheEvents(ctx context.Context, limit int) ([]thumbnailCacheEvent, error) {
	tx, err := begin(ctx, c.pool)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(context.Background())
	rows, err := tx.Query(ctx, `WITH claimed AS (
 SELECT event_id FROM c_posts.thumbnail_cache_outbox
 WHERE completed_at IS NULL AND next_attempt_at<=now()
 AND (lease_until IS NULL OR lease_until<now())
 ORDER BY next_attempt_at,event_id
 FOR UPDATE SKIP LOCKED LIMIT $1)
 UPDATE c_posts.thumbnail_cache_outbox o
 SET lease_token=gen_random_uuid(), lease_until=now()+make_interval(secs=>$2), attempts=o.attempts+1
 FROM claimed WHERE o.event_id=claimed.event_id
 RETURNING o.event_id,o.lease_token,o.post_id,o.content_version_id,o.variant,o.encoder_version,
 o.source_binding,o.output_digest,o.attempts`, limit, thumbnailCacheLeaseSeconds)
	if err != nil {
		return nil, err
	}
	var events []thumbnailCacheEvent
	for rows.Next() {
		var event thumbnailCacheEvent
		if err = rows.Scan(&event.EventID, &event.LeaseToken, &event.PostID, &event.ContentVersion,
			&event.Variant, &event.EncoderVersion, &event.SourceBinding, &event.OutputDigest, &event.Attempts); err != nil {
			rows.Close()
			return nil, err
		}
		events = append(events, event)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return events, nil
}

func (c *Community) completeThumbnailCacheEvent(ctx context.Context, event thumbnailCacheEvent) bool {
	tag, err := c.pool.Exec(ctx, `UPDATE c_posts.thumbnail_cache_outbox
 SET completed_at=now(), lease_token=NULL, lease_until=NULL
 WHERE event_id=$1 AND lease_token=$2 AND completed_at IS NULL`, event.EventID, event.LeaseToken)
	return err == nil && tag.RowsAffected() == 1
}

func (c *Community) retryThumbnailCacheEvent(ctx context.Context, event thumbnailCacheEvent) {
	seconds := thumbnailCacheRetrySeconds(event.Attempts)
	_, _ = c.pool.Exec(ctx, `UPDATE c_posts.thumbnail_cache_outbox
 SET lease_token=NULL, lease_until=NULL, next_attempt_at=now()+make_interval(secs=>$3)
 WHERE event_id=$1 AND lease_token=$2 AND completed_at IS NULL`, event.EventID, event.LeaseToken, seconds)
}

func thumbnailCacheRetrySeconds(attempts int) int {
	seconds := 1
	for i := 0; i < attempts && seconds < thumbnailCacheRetryCapSeconds; i++ {
		seconds *= 2
	}
	if seconds > thumbnailCacheRetryCapSeconds {
		seconds = thumbnailCacheRetryCapSeconds
	}
	return seconds
}

// retireThumbnailCacheEvents purges completed receipts and, only while the
// cache is disabled, stale pending events. While Redis is active, pending
// deletions are retried instead of silently abandoned.
func (c *Community) retireThumbnailCacheEvents(ctx context.Context, active bool, limit int) (int64, error) {
	if limit < 1 || limit > 256 {
		limit = 128
	}
	var exists bool
	if err := c.pool.QueryRow(ctx, `SELECT to_regclass('c_posts.thumbnail_cache_outbox') IS NOT NULL`).Scan(&exists); err != nil || !exists {
		return 0, err
	}
	tag, err := c.pool.Exec(ctx, `DELETE FROM c_posts.thumbnail_cache_outbox WHERE event_id IN (
 SELECT event_id FROM c_posts.thumbnail_cache_outbox
 WHERE completed_at IS NOT NULL AND completed_at < now()-make_interval(secs=>$1)
 ORDER BY completed_at LIMIT $2)`, int64(thumbnailCacheCompletedRetention/time.Second), limit)
	if err != nil {
		return 0, err
	}
	removed := tag.RowsAffected()
	if active {
		return removed, nil
	}
	tag, err = c.pool.Exec(ctx, `DELETE FROM c_posts.thumbnail_cache_outbox WHERE event_id IN (
 SELECT event_id FROM c_posts.thumbnail_cache_outbox
 WHERE completed_at IS NULL AND created_at < now()-make_interval(secs=>$1)
 ORDER BY created_at LIMIT $2)`, int64(thumbnailCacheStalePending/time.Second), limit)
	if err != nil {
		return removed, err
	}
	return removed + tag.RowsAffected(), nil
}
