package authprivacy

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/postmedia"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/posts"
)

const postEditStageLifetime = 24 * time.Hour

func (c *Community) StagePostEditImage(ctx context.Context, bearer [32]byte, post, command, content, mime string, data []byte) (PostResponse, error) {
	if !c.PostImagesEnabled() || !c.posts.editEnabled {
		return PostResponse{}, postmedia.ErrUnavailable
	}
	if !postmedia.ValidID(post) || !postmedia.ValidID(content) || len(data) < 1 || len(data) > postmedia.MaxBytes {
		return PostResponse{}, postmedia.ErrInvalid
	}
	digest := sha256.Sum256(data)
	meta, err := c.postImages.validator.Inspect(ctx, data)
	if err != nil {
		return PostResponse{}, err
	}
	selection := postmedia.Selection{MediaID: content, SHA256: meta.SHA256, ByteLength: len(data), MIME: mime, Width: meta.Width, Height: meta.Height}
	if !meta.Matches(selection, data) {
		return PostResponse{}, postmedia.ErrInvalid
	}
	return c.withPostSession(ctx, bearer, func(tx pgx.Tx, account uuid.UUID, final AuthorizationDecision, mute restriction) (any, error) {
		if err := requireSharedPolicyFence(ctx, tx); err != nil {
			return nil, err
		}
		key, err := c.posts.commands.KeyDigest(account.String(), command)
		if err != nil {
			return nil, err
		}
		subject, err := readPostEditSubject(ctx, tx, account, post, mute, final.TrustedAt)
		if err != nil {
			return nil, err
		}
		if subject.blocked != "" {
			return nil, postError(subject.blocked, 409)
		}
		_, receipt, err := readPostEditReceipt(ctx, tx, post, key)
		if err != nil || receipt != nil {
			return struct{}{}, err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM c_posts.post_edit_media m USING c_posts.posts p
 WHERE m.post_id=p.post_id AND p.owner_account_id=$1 AND m.command_digest IS NOT NULL AND m.staged_at<=$2`,
			account, final.TrustedAt.Add(-postEditStageLifetime)); err != nil {
			return nil, err
		}
		var priorKey, priorDigest []byte
		err = tx.QueryRow(ctx, `SELECT command_digest,digest FROM c_posts.post_edit_media WHERE post_id=$1 AND content_version_id=$2`, post, content).Scan(&priorKey, &priorDigest)
		if err == nil {
			if !hmac.Equal(key[:], priorKey) || !hmac.Equal(digest[:], priorDigest) {
				return nil, postError("COMMAND_CONFLICT", 409)
			}
			return struct{}{}, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
		var duplicate bool
		var staged, thisCommand int
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM (`+originalPostImageRows+`) o WHERE o.content_version_id=$2),
 (SELECT count(*) FROM c_posts.post_edit_media m JOIN c_posts.posts p USING(post_id) WHERE p.owner_account_id=$3 AND m.command_digest IS NOT NULL),
 (SELECT count(*) FROM c_posts.post_edit_media WHERE post_id=$1 AND command_digest=$4)`,
			post, content, account, key[:]).Scan(&duplicate, &staged, &thisCommand); err != nil {
			return nil, err
		}
		if duplicate {
			return nil, postError("COMMAND_CONFLICT", 409)
		}
		if thisCommand >= postmedia.MaxImages || staged >= 2*postmedia.MaxImages {
			return nil, postError("POST_EDIT_MEDIA_LIMIT", 409)
		}
		_, err = tx.Exec(ctx, `INSERT INTO c_posts.post_edit_media
 (post_id,content_version_id,command_digest,staged_at,payload,digest,mime_type,byte_length,width,height,display_width,display_height,frames)
 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
			post, content, key[:], final.TrustedAt, data, digest[:], mime, len(data), meta.Width, meta.Height, meta.DisplayWidth, meta.DisplayHeight, meta.Frames)
		return struct{}{}, err
	})
}

func checkPostEditImages(ctx context.Context, tx pgx.Tx, post string, key [32]byte, edited bool, ids []string, at time.Time) (bool, error) {
	var complete bool
	err := tx.QueryRow(ctx, `SELECT NOT EXISTS (
 SELECT 1 FROM unnest($2::uuid[]) id WHERE NOT EXISTS (
  SELECT 1 FROM (`+currentPostImageRows(edited)+`) chosen WHERE chosen.content_version_id=id)
 AND NOT EXISTS (SELECT 1 FROM c_posts.post_edit_media m WHERE m.post_id=$1 AND m.content_version_id=id AND m.command_digest=$3 AND m.staged_at>$4)
 )`, post, ids, key[:], at.Add(-postEditStageLifetime)).Scan(&complete)
	return complete, err
}

func finishPostEditImages(ctx context.Context, tx pgx.Tx, post string, key [32]byte, selected *[]string) error {
	if selected != nil {
		if _, err := tx.Exec(ctx, `UPDATE c_posts.post_edit_media SET command_digest=NULL
 WHERE post_id=$1 AND content_version_id=ANY($2::uuid[]) AND command_digest=$3`, post, *selected, key[:]); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM c_posts.post_edit_media
 WHERE post_id=$1 AND command_digest IS NULL AND NOT(content_version_id=ANY($2::uuid[]))`, post, *selected); err != nil {
			return err
		}
	}
	_, err := tx.Exec(ctx, `DELETE FROM c_posts.post_edit_media WHERE post_id=$1 AND command_digest=$2`, post, key[:])
	return err
}

func (c *Community) cleanupPostEditImages(ctx context.Context, limit int) (int64, error) {
	if !c.PostsEnabled() || !c.posts.editEnabled {
		return 0, nil
	}
	decision, err := c.gate.Snapshot(ctx)
	if err != nil {
		return 0, err
	}
	tx, err := begin(ctx, c.pool)
	if err != nil {
		return 0, ErrAuthorizationUnavailable
	}
	defer observedAuthorizationAbort(ctx, c.gate, tx)
	var removed int64
	_, err = c.gate.CommitAuthorized(ctx, tx, decision.Generation, func(final AuthorizationDecision) error {
		result, err := tx.Exec(ctx, `WITH expired AS (
 SELECT post_id,content_version_id FROM c_posts.post_edit_media
 WHERE command_digest IS NOT NULL AND staged_at<=$1 ORDER BY staged_at,post_id,content_version_id LIMIT $2
 ) DELETE FROM c_posts.post_edit_media m USING expired e
 WHERE m.post_id=e.post_id AND m.content_version_id=e.content_version_id AND m.command_digest IS NOT NULL`,
			final.TrustedAt.Add(-postEditStageLifetime), limit)
		removed = result.RowsAffected()
		return err
	})
	return removed, err
}

// A nil list means keep the public images. An empty list removes them all.
func validPostEditImageIDs(ids *[]string) bool {
	if ids == nil {
		return true
	}
	if *ids == nil || len(*ids) > postmedia.MaxImages {
		return false
	}
	seen := make(map[string]bool, len(*ids))
	for _, id := range *ids {
		if !postmedia.ValidID(id) || seen[id] {
			return false
		}
		seen[id] = true
	}
	return true
}

func preparePostEditImages(ctx context.Context, tx pgx.Tx, post string, key [32]byte, current posts.PublicPost, ids *[]string, at time.Time) (int, string, error) {
	if ids == nil {
		return current.ImageCount, "", nil
	}
	complete, err := checkPostEditImages(ctx, tx, post, key, current.ImagesEdited, *ids, at)
	if !complete {
		return 0, "POST_EDIT_MEDIA_MISSING", err
	}
	return len(*ids), "", err
}
