package authprivacy

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
)

type thumbnailCleanupCursor struct {
	at               time.Time
	post, content    uuid.UUID
	variant, encoder string
	valid            bool
}
type thumbnailCleanupState struct {
	sync.Mutex
	cursor thumbnailCleanupCursor
}

// CleanupPostThumbnails remains active when reuse is disabled. Source hooks
// erase immediately; this bounded pass is a backstop for orphans and capacity.
func (c *Community) CleanupPostThumbnails(ctx context.Context, limit int) (int64, error) {
	if c == nil || c.postImages == nil || !c.postImages.thumbnailStorage {
		return 0, nil
	}
	if limit < 1 || limit > 256 {
		limit = 128
	}
	state := c.postImages.thumbnailCleanup
	state.Lock()
	defer state.Unlock()
	decision, err := c.gate.Snapshot(ctx)
	if err != nil {
		return 0, err
	}
	tx, err := begin(ctx, c.pool)
	if err != nil {
		return 0, err
	}
	defer observedAuthorizationAbort(ctx, c.gate, tx)
	var removed int64
	next := thumbnailCleanupCursor{}
	_, err = c.gate.CommitAuthorized(ctx, tx, decision.Generation, func(AuthorizationDecision) error {
		if e := requireSharedPolicyFence(ctx, tx); e != nil {
			return e
		}
		var pressure bool
		if e := tx.QueryRow(ctx, `SELECT used_bytes>capacity_bytes*9/10 FROM c_posts.thumbnail_storage WHERE singleton_id=1`).Scan(&pressure); e != nil {
			return e
		}
		query := `SELECT created_at,post_id,content_version_id,variant,encoder_version FROM c_posts.thumbnails`
		args := []any{limit}
		if state.cursor.valid && !pressure {
			query += ` WHERE (created_at,post_id,content_version_id,variant,encoder_version)>($1,$2,$3,$4,$5)`
			args = []any{state.cursor.at, state.cursor.post, state.cursor.content, state.cursor.variant, state.cursor.encoder, limit}
			query += ` ORDER BY created_at,post_id,content_version_id,variant,encoder_version LIMIT $6`
		} else {
			query += ` ORDER BY created_at,post_id,content_version_id,variant,encoder_version LIMIT $1`
		}
		rows, e := tx.Query(ctx, query, args...)
		if e != nil {
			return e
		}
		var posts, contents []uuid.UUID
		var variants, encoders []string
		for rows.Next() {
			if e = rows.Scan(&next.at, &next.post, &next.content, &next.variant, &next.encoder); e != nil {
				rows.Close()
				return e
			}
			next.valid = true
			posts = append(posts, next.post)
			contents = append(contents, next.content)
			variants = append(variants, next.variant)
			encoders = append(encoders, next.encoder)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		if len(posts) == 0 {
			return nil
		}
		// Examine at most this page. Filtering before LIMIT would scan all
		// historical cache entries under the global Gate when no orphan exists.
		result, e := tx.Exec(ctx, `DELETE FROM c_posts.thumbnails d USING
 unnest($1::uuid[],$2::uuid[],$3::text[],$4::text[]) e(post_id,content_version_id,variant,encoder_version)
 WHERE (d.post_id,d.content_version_id,d.variant,d.encoder_version)=(e.post_id,e.content_version_id,e.variant,e.encoder_version)
 AND ($5::boolean OR NOT EXISTS(SELECT 1 FROM c_posts.posts p WHERE p.post_id=d.post_id AND p.visibility='PUBLISHED' AND NOT p.moderation_hidden
 AND ((EXISTS(SELECT 1 FROM c_posts.images m JOIN c_posts.publication_tasks t USING(task_id)
 WHERE t.post_id=p.post_id AND m.version=p.published_attempt_version AND m.content_version_id=d.content_version_id
 AND m.payload IS NOT NULL AND m.erased_at IS NULL AND m.verified_at IS NOT NULL)
 AND (p.edited_image_ids IS NULL OR d.content_version_id=ANY(p.edited_image_ids)))
 OR (d.content_version_id=ANY(p.edited_image_ids) AND EXISTS(SELECT 1 FROM c_posts.post_edit_media m
 WHERE m.post_id=p.post_id AND m.content_version_id=d.content_version_id AND m.command_digest IS NULL)))))`, posts, contents, variants, encoders, pressure)
		if e == nil {
			removed = result.RowsAffected()
		}
		return e
	})
	if err == nil {
		state.cursor = next
	}
	if err == nil && c.postImages.thumbnails != nil {
		c.postImages.thumbnails.cleaned.Add(uint64(removed))
	}
	return removed, err
}

func (c *Community) RunPostThumbnailMaintenance(ctx context.Context) {
	if c == nil || c.postImages == nil || !c.postImages.thumbnailStorage {
		return
	}
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.postThumbnailMaintenancePass(ctx)
		}
	}
}

// postThumbnailMaintenancePass runs one bounded pass. Reuse may be disabled
// while durable thumbnail storage is enabled, so only the optional in-memory
// cache surface is conditionally available here.
func (c *Community) postThumbnailMaintenancePass(ctx context.Context) {
	step, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	removed, err := c.CleanupPostThumbnails(step, 128)
	if err != nil {
		slog.Warn("thumbnail maintenance unavailable")
		return
	}
	var used, capacity int64
	if c.pool.QueryRow(step, `SELECT used_bytes,capacity_bytes FROM c_posts.thumbnail_storage WHERE singleton_id=1`).Scan(&used, &capacity) == nil {
		slog.Info("private thumbnail storage", "bytes", used, "capacity_bytes", capacity, "cleaned", removed)
	}
	cacheActive := false
	if cache := c.postImages.thumbnails; cache != nil {
		cacheActive = cache.redis != nil
		if s, e := c.PostThumbnailStats(step); e == nil {
			slog.Info("private thumbnail reuse", "hits", s.Hits, "misses", s.Misses, "source_reads", s.SourceReads,
				"source_bytes", s.SourceBytes, "transforms", s.Transforms, "shared", s.SharedWaiters,
				"payload_reads", s.PayloadReads, "payload_bytes", s.PayloadBytes,
				"queued", len(cache.queue), "workers", len(cache.workers), "capacity_skips", s.CapacitySkips)
		}
	}
	// Completed receipts expire quickly. Pending events survive while Redis is
	// active; only a disabled cache abandons stale ones.
	if n, e := c.retireThumbnailCacheEvents(step, cacheActive, 128); e != nil {
		slog.Warn("thumbnail cache outbox maintenance failed")
	} else if n > 0 {
		slog.Info("thumbnail cache outbox retired", "events", n)
	}
}
