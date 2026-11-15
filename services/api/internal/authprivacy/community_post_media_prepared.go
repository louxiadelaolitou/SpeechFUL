package authprivacy

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/postmedia"
)

// Only current selected media participate. Both a missing resource and a
// private preparation error retain this proof until final authorization.
// The feed versions detect selection/digest ABA, not just unequal end states.
const preparedPostMediaHeaderSQL = `SELECT
 COALESCE(p.visibility='PUBLISHED' AND NOT p.moderation_hidden AND NOT c_posts.accounts_blocked($2,p.owner_account_id),false),
 p.edited_image_ids IS NOT NULL,` + publicPostImageCount + `,
 jsonb_build_array(p.post_id,p.owner_account_id,p.visibility,p.moderation_hidden,p.published_attempt_version,
 p.edited_image_ids,x.erased_at,v.projection_revision,viewer.visibility_revision,author.projection_revision,
 c_posts.accounts_blocked($2,p.owner_account_id)),
 viewer.account_id IS NOT NULL AND (p.post_id IS NULL OR
 (v.post_id IS NOT NULL AND author.account_id IS NOT NULL AND
 (NOT COALESCE(p.visibility='PUBLISHED' AND NOT p.moderation_hidden AND NOT c_posts.accounts_blocked($2,p.owner_account_id),false)
 OR x.task_id IS NOT NULL)))
 FROM (SELECT 1) one
 LEFT JOIN c_posts.posts p ON p.post_id=$1
 LEFT JOIN c_posts.publication_tasks t ON t.post_id=p.post_id
 LEFT JOIN c_posts.attempt_contents x ON x.task_id=t.task_id AND x.version=p.published_attempt_version
 LEFT JOIN c_posts.read_feed_post_versions v ON v.post_id=p.post_id
 LEFT JOIN c_posts.read_account_versions viewer ON viewer.account_id=$2
 LEFT JOIN c_posts.read_account_versions author ON author.account_id=p.owner_account_id`

type preparedPostMediaItem struct {
	MediaID, ContentID string
	Position           int
	Metadata           postmedia.Metadata
}

type preparedPostMediaSnapshot struct {
	proof   []byte
	visible bool
	edited  bool
	items   []preparedPostMediaItem
}

// No payload is selected here, including during finalization. The set is
// bounded by the historical nine-image limit, with one corruption lookahead.
func readPreparedPostMediaSnapshot(ctx context.Context, tx pgx.Tx, post string, account uuid.UUID) (preparedPostMediaSnapshot, error) {
	s := preparedPostMediaSnapshot{}
	var header []byte
	var expected int
	var intact bool
	if err := tx.QueryRow(ctx, preparedPostMediaHeaderSQL, post, account).Scan(&s.visible, &s.edited, &expected, &header, &intact); err != nil {
		return s, err
	}
	if !intact {
		return s, ErrAuthorizationUnavailable
	}
	s.items = []preparedPostMediaItem{}
	if s.visible {
		if expected < 0 || expected > postmedia.LegacyMaxImages {
			return s, ErrAuthorizationUnavailable
		}
		rows, err := tx.Query(ctx, `SELECT media_id::text,content_version_id::text,position,digest,byte_length,mime_type,
 width,height,display_width,display_height,frames,cycle_duration_ms,max_frame_duration_ms
 FROM (`+currentPostImageRows(s.edited)+`) m ORDER BY position LIMIT 10`, post)
		if err != nil {
			return s, err
		}
		defer rows.Close()
		for rows.Next() {
			var item preparedPostMediaItem
			var digest []byte
			m := &item.Metadata
			if err = rows.Scan(&item.MediaID, &item.ContentID, &item.Position, &digest, &m.ByteLength, &m.MIME,
				&m.Width, &m.Height, &m.DisplayWidth, &m.DisplayHeight, &m.Frames, &m.CycleDurationMS, &m.MaxFrameDurationMS); err != nil {
				return s, err
			}
			m.SHA256, m.Animated = hex.EncodeToString(digest), m.Frames > 1
			selection := postmedia.Selection{MediaID: item.MediaID, SHA256: m.SHA256, ByteLength: m.ByteLength, MIME: m.MIME, Width: m.Width, Height: m.Height}
			if item.Position != len(s.items) || postmedia.ValidateStoredManifest([]postmedia.Selection{selection}) != nil ||
				m.Frames < 1 || m.Frames > postmedia.MaxFrames ||
				!((m.DisplayWidth == m.Width && m.DisplayHeight == m.Height) || (m.DisplayWidth == m.Height && m.DisplayHeight == m.Width)) {
				return s, ErrAuthorizationUnavailable
			}
			s.items = append(s.items, item)
		}
		if err = rows.Err(); err != nil {
			return s, err
		}
		if len(s.items) != expected {
			return s, ErrAuthorizationUnavailable
		}
	}
	var err error
	s.proof, err = json.Marshal([]any{json.RawMessage(header), s.items})
	return s, err
}

func (item preparedPostMediaItem) publicImage(post string) postmedia.Image {
	m := item.Metadata
	source := postmedia.Source{Path: "/api/v1/posts/" + post + "/images/" + item.ContentID + "/original", MIME: m.MIME, ByteLength: m.ByteLength}
	return postmedia.Image{MediaID: item.MediaID, ContentVersionID: item.ContentID, Position: item.Position,
		Width: m.Width, Height: m.Height, PixelWidth: m.DisplayWidth, PixelHeight: m.DisplayHeight,
		Frames: m.Frames, Animated: m.Animated, SaveAllowed: true, Thumbnail: source, Full: source, Original: source}
}

func (c *Community) readPostMediaPrepared(ctx context.Context, bearer [32]byte, post, content string, thumbnail bool) (PostResponse, error) {
	response, err := c.withPreparedReadPlan(ctx, bearer, postError("SERVICE_UNAVAILABLE", 503),
		func(prepareCtx context.Context, tx pgx.Tx, subject readSubject) (*preparedReadPlan, error) {
			snapshot, err := readPreparedPostMediaSnapshot(prepareCtx, tx, post, subject.account)
			if err != nil {
				return nil, ErrAuthorizationUnavailable
			}
			var original, output []byte
			var selected *preparedPostMediaItem
			images := make([]postmedia.Image, len(snapshot.items))
			for i := range snapshot.items {
				item := &snapshot.items[i]
				images[i] = item.publicImage(post)
				if item.ContentID == content {
					selected = item
				}
			}
			plan := &preparedReadPlan{
				Revalidate: func(tx pgx.Tx, a uuid.UUID, _ AuthorizationDecision, _ restriction) error {
					// Capture the outer request context: prepareCtx was canceled
					// when its independent RO transaction finished.
					current, e := readPreparedPostMediaSnapshot(ctx, tx, post, a)
					if e != nil {
						return ErrAuthorizationUnavailable
					}
					if !bytes.Equal(current.proof, snapshot.proof) {
						return errReadConflict
					}
					return nil
				},
				Complete: func(_ pgx.Tx, _ uuid.UUID, final AuthorizationDecision, _ restriction) (any, error) {
					if content != "" {
						mime := selected.Metadata.MIME
						if thumbnail {
							mime = "image/webp"
							return ImageBytes{Bytes: output, MIME: mime}, nil
						}
						return ImageBytes{Bytes: original, MIME: mime}, nil
					}
					return postmedia.Group{PostID: post, Images: images, AccessValidUntil: final.TrustedAt.Add(time.Minute).UTC().Format(time.RFC3339Nano)}, nil
				},
				Release: func() {
					// Detach private references; never wipe the backing arrays
					// transferred into a confirmed successful response.
					original, output, images = nil, nil, nil
					snapshot.items, snapshot.proof = nil, nil
				},
			}
			if !snapshot.visible || content != "" && selected == nil {
				return plan, postError("MEDIA_NOT_FOUND", 404)
			}
			if thumbnail && content == "" {
				return plan, postmedia.ErrUnavailable
			}
			if content == "" {
				return plan, nil
			}
			if err = tx.QueryRow(prepareCtx, `SELECT payload FROM (`+currentPostImageRows(snapshot.edited)+`) m WHERE content_version_id=$2`, post, content).Scan(&original); err != nil {
				return plan, ErrAuthorizationUnavailable
			}
			plan.AfterPrepare = func(cpuCtx context.Context) error {
				m := selected.Metadata
				selection := postmedia.Selection{MediaID: selected.MediaID, SHA256: m.SHA256, ByteLength: m.ByteLength, MIME: m.MIME, Width: m.Width, Height: m.Height}
				if !m.Matches(selection, original) {
					return ErrAuthorizationUnavailable
				}
				if thumbnail {
					var e error
					output, e = c.postImages.validator.(postmedia.Thumbnailer).Thumbnail(cpuCtx, original)
					return e
				}
				return nil
			}
			return plan, nil
		})
	if err == nil {
		if group, ok := response.Value.(postmedia.Group); ok {
			until, _ := time.Parse(time.RFC3339Nano, group.AccessValidUntil)
			if until.After(response.ExpiresAt) {
				group.AccessValidUntil = response.ExpiresAt.Format(time.RFC3339Nano)
			}
			response.Value = group
		}
	}
	return response, err
}
