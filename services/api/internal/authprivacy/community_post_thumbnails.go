package authprivacy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/postmedia"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/readtiming"
	"golang.org/x/sync/singleflight"
)

type thumbnailReuse struct {
	encoder                                                                      postmedia.ReusableThumbnailer
	version                                                                      string
	redis                                                                        thumbnailHotCache
	group                                                                        singleflight.Group
	workers, queue                                                               chan struct{}
	hits, misses, reads, sourceBytes, transforms, shared, capacitySkips, cleaned atomic.Uint64
	payloadReads, payloadBytes                                                   atomic.Uint64
}

// WithPostThumbnailReuse explicitly enables private persistent derivatives.
// It requires the native two-phase path; custom Gates retain their old path.
func (c *Community) WithPostThumbnailReuse(ctx context.Context) (*Community, error) {
	if !c.PostImageThumbnailsEnabled() || !c.posts.preparedMediaReads || !c.nativePreparedReads() {
		return nil, ErrAuthorizationUnavailable
	}
	encoder, ok := c.postImages.validator.(postmedia.ReusableThumbnailer)
	if !ok {
		return nil, postmedia.ErrUnavailable
	}
	if err := c.checkThumbnailStorageSchema(ctx, true); err != nil {
		return nil, err
	}
	version, err := encoder.ThumbnailEncoderVersion(ctx)
	if err != nil || len(version) != 64 {
		return nil, postmedia.ErrUnavailable
	}
	if b, e := hex.DecodeString(version); e != nil || len(b) != 32 || hex.EncodeToString(b) != version {
		return nil, postmedia.ErrUnavailable
	}
	clone := *c
	images := *c.postImages
	images.thumbnails = &thumbnailReuse{encoder: encoder, version: version, workers: make(chan struct{}, 2), queue: make(chan struct{}, 32)}
	clone.postImages = &images
	return &clone, nil
}

func thumbnailSourceBinding(item preparedPostMediaItem) [32]byte {
	raw, _ := json.Marshal(item)
	return sha256.Sum256(raw)
}

func readStoredThumbnail(ctx context.Context, tx pgx.Tx, post string, item preparedPostMediaItem, version string) (postmedia.ThumbnailRecord, bool, error) {
	binding := thumbnailSourceBinding(item)
	var r postmedia.ThumbnailRecord
	err := tx.QueryRow(ctx, `SELECT payload,width,height,encode(digest,'hex') FROM c_posts.thumbnails
 WHERE post_id=$1 AND content_version_id=$2 AND variant=$3 AND encoder_version=$4 AND source_binding=$5`,
		post, item.ContentID, postmedia.ThumbnailVariant, version, binding[:]).Scan(&r.Bytes, &r.Width, &r.Height, &r.SHA256)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, false, nil
	}
	return r, err == nil, err
}

type preparedThumbnail struct {
	record    postmedia.ThumbnailRecord
	generated bool
}

func (c *Community) preparePostThumbnail(ctx context.Context, post string, item preparedPostMediaItem) (preparedThumbnail, error) {
	cache := c.postImages.thumbnails
	binding := thumbnailSourceBinding(item)
	key := post + ":" + item.ContentID + ":" + postmedia.ThumbnailVariant + ":" + cache.version + ":" + hex.EncodeToString(binding[:])
	ch := cache.group.DoChan(key, func() (value any, err error) {
		// Encoding is shared asynchronous work. Convert a worker panic into a
		// private failure; the caller must still pass final authorization to
		// receive even that failure. Never log encoder panic values.
		defer func() {
			if recover() != nil {
				value = nil
				err = postmedia.ErrUnavailable
			}
		}()
		// The shared job contains no Bearer, session or authorization result.
		// A canceled waiter cannot cancel another waiter's private CPU work.
		jobCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		select {
		case cache.queue <- struct{}{}:
			defer func() { <-cache.queue }()
		default:
			return nil, postmedia.ErrUnavailable
		}
		select {
		case cache.workers <- struct{}{}:
			defer func() { <-cache.workers }()
		case <-jobCtx.Done():
			return nil, postmedia.ErrUnavailable
		}
		tx, err := c.pool.BeginTx(jobCtx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
		if err != nil {
			return nil, ErrAuthorizationUnavailable
		}
		defer tx.Rollback(context.Background())
		cached, hit, err := readStoredThumbnail(jobCtx, tx, post, item, cache.version)
		if err != nil {
			return nil, ErrAuthorizationUnavailable
		}
		if hit {
			if err = tx.Rollback(jobCtx); err != nil {
				return nil, ErrAuthorizationUnavailable
			}
			if !cached.Valid() {
				return nil, postmedia.ErrUnavailable
			}
			return preparedThumbnail{record: cached}, nil
		}
		var original []byte
		// Source payload loading also participates in request coalescing.
		// The final per-request proof, not this private read, authorizes release.
		var edited bool
		if err = tx.QueryRow(jobCtx, `SELECT edited_image_ids IS NOT NULL FROM c_posts.posts WHERE post_id=$1`, post).Scan(&edited); err != nil {
			return nil, ErrAuthorizationUnavailable
		}
		err = tx.QueryRow(jobCtx, `SELECT payload FROM (`+currentPostImageRows(edited)+`) m WHERE content_version_id=$2`, post, item.ContentID).Scan(&original)
		if err != nil {
			return nil, ErrAuthorizationUnavailable
		}
		cache.reads.Add(1)
		cache.sourceBytes.Add(uint64(len(original)))
		if err = tx.Rollback(jobCtx); err != nil {
			return nil, ErrAuthorizationUnavailable
		}
		m := item.Metadata
		selection := postmedia.Selection{MediaID: item.MediaID, SHA256: m.SHA256, ByteLength: m.ByteLength, MIME: m.MIME, Width: m.Width, Height: m.Height}
		if !m.Matches(selection, original) {
			return nil, ErrAuthorizationUnavailable
		}
		cache.transforms.Add(1)
		r, err := cache.encoder.ThumbnailRecord(jobCtx, original)
		if err != nil {
			return nil, err
		}
		if !r.Valid() {
			return nil, postmedia.ErrUnavailable
		}
		return preparedThumbnail{record: r, generated: true}, nil
	})
	defer readtiming.Span(ctx, "thumbnail_prepare_wait")()
	select {
	case <-ctx.Done():
		return preparedThumbnail{}, ctx.Err()
	case result := <-ch:
		if result.Shared {
			cache.shared.Add(1)
		}
		if result.Err != nil {
			return preparedThumbnail{}, result.Err
		}
		return result.Val.(preparedThumbnail), nil
	}
}

// Called only inside Complete after the unchanged final source proof, under C
// Gate. Source erasure and this insertion therefore cannot resurrect one another.
func (c *Community) storePostThumbnail(ctx context.Context, tx pgx.Tx, post string, item preparedPostMediaItem, r postmedia.ThumbnailRecord, at time.Time) error {
	cache := c.postImages.thumbnails
	binding := thumbnailSourceBinding(item)
	digest, _ := hex.DecodeString(r.SHA256)
	if _, err := tx.Exec(ctx, `DELETE FROM c_posts.thumbnails WHERE post_id=$1 AND content_version_id=$2
 AND variant=$3 AND encoder_version=$4 AND source_binding<>$5`, post, item.ContentID, postmedia.ThumbnailVariant, cache.version, binding[:]); err != nil {
		return err
	}
	result, err := tx.Exec(ctx, `INSERT INTO c_posts.thumbnails
 (post_id,content_version_id,variant,encoder_version,source_binding,payload,digest,mime_type,width,height,byte_length,created_at)
 SELECT $1,$2,$3,$4,$5,$6,$7,'image/webp',$8,$9,$10::integer,$11 FROM c_posts.thumbnail_storage
 WHERE singleton_id=1 AND used_bytes+$10::integer<=capacity_bytes
 ON CONFLICT(post_id,content_version_id,variant,encoder_version) DO NOTHING`,
		post, item.ContentID, postmedia.ThumbnailVariant, cache.version, binding[:], r.Bytes, digest, r.Width, r.Height, len(r.Bytes), at)
	if err == nil && result.RowsAffected() == 0 {
		var full bool
		if e := tx.QueryRow(ctx, `SELECT used_bytes+$1::integer>capacity_bytes FROM c_posts.thumbnail_storage WHERE singleton_id=1`, len(r.Bytes)).Scan(&full); e != nil {
			return e
		}
		if full {
			cache.capacitySkips.Add(1)
		}
	}
	return err
}

// ThumbnailReuseStats contains only aggregate counters, no resource identifiers.
type ThumbnailReuseStats struct {
	Hits, Misses, SourceReads, SourceBytes, Transforms, SharedWaiters, CapacitySkips, CleanedRows uint64
	PayloadReads, PayloadBytes                                                                    uint64
	StoredBytes, CapacityBytes                                                                    int64
}

func (c *Community) PostThumbnailStats(ctx context.Context) (ThumbnailReuseStats, error) {
	var s ThumbnailReuseStats
	if c.postImages == nil || c.postImages.thumbnails == nil {
		return s, postmedia.ErrUnavailable
	}
	cache := c.postImages.thumbnails
	s.Hits, s.Misses = cache.hits.Load(), cache.misses.Load()
	s.SourceReads, s.SourceBytes, s.Transforms = cache.reads.Load(), cache.sourceBytes.Load(), cache.transforms.Load()
	s.SharedWaiters, s.CapacitySkips, s.CleanedRows = cache.shared.Load(), cache.capacitySkips.Load(), cache.cleaned.Load()
	s.PayloadReads, s.PayloadBytes = cache.payloadReads.Load(), cache.payloadBytes.Load()
	err := c.pool.QueryRow(ctx, `SELECT used_bytes,capacity_bytes FROM c_posts.thumbnail_storage WHERE singleton_id=1`).Scan(&s.StoredBytes, &s.CapacityBytes)
	return s, err
}
