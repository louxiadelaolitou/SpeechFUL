package authprivacy

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/postmedia"
)

const originalPostImageRows = `
 SELECT m.media_id,m.content_version_id,m.position,m.width,m.height,m.display_width,m.display_height,
        m.frames,m.mime_type,m.byte_length,m.payload
 FROM c_posts.images m JOIN c_posts.publication_tasks t USING(task_id) JOIN c_posts.posts p USING(post_id)
 WHERE p.post_id=$1 AND m.version=p.published_attempt_version
   AND m.erased_at IS NULL AND m.verified_at IS NOT NULL AND m.payload IS NOT NULL`

const editedPostImageRows = `
 WITH selected AS (
   SELECT image.id,(image.ordinality-1)::integer AS position
   FROM c_posts.posts p CROSS JOIN LATERAL unnest(p.edited_image_ids) WITH ORDINALITY AS image(id,ordinality)
   WHERE p.post_id=$1
 ), original AS (` + originalPostImageRows + `)
 SELECT o.media_id,o.content_version_id,s.position,o.width,o.height,o.display_width,o.display_height,
        o.frames,o.mime_type,o.byte_length,o.payload
 FROM selected s JOIN original o ON o.content_version_id=s.id
 UNION ALL
 SELECT m.content_version_id,m.content_version_id,s.position,m.width,m.height,m.display_width,m.display_height,
        m.frames,m.mime_type,m.byte_length,m.payload
 FROM selected s JOIN c_posts.post_edit_media m ON m.post_id=$1 AND m.content_version_id=s.id
 WHERE m.command_digest IS NULL`

func currentPostImageRows(edited bool) string {
	if edited {
		return editedPostImageRows
	}
	return originalPostImageRows
}

func readPostImageItems(ctx context.Context, tx pgx.Tx, post string, edited bool, expected int) ([]postmedia.Image, error) {
	items := make([]postmedia.Image, 0, expected)
	if expected == 0 {
		return items, nil
	}
	rows, err := tx.Query(ctx, `SELECT media_id,content_version_id,position,width,height,display_width,display_height,frames,mime_type,byte_length
 FROM (`+currentPostImageRows(edited)+`) m ORDER BY position`, post)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var item postmedia.Image
		var mime string
		var size int
		if err := rows.Scan(&item.MediaID, &item.ContentVersionID, &item.Position, &item.Width, &item.Height, &item.PixelWidth, &item.PixelHeight, &item.Frames, &mime, &size); err != nil {
			return nil, err
		}
		if item.Position != len(items) {
			return nil, ErrAuthorizationUnavailable
		}
		item.Animated = item.Frames > 1
		item.SaveAllowed = true
		source := postmedia.Source{Path: "/api/v1/posts/" + post + "/images/" + item.ContentVersionID + "/original", MIME: mime, ByteLength: size}
		item.Thumbnail, item.Full, item.Original = source, source, source
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(items) != expected {
		return nil, ErrAuthorizationUnavailable
	}
	return items, nil
}
