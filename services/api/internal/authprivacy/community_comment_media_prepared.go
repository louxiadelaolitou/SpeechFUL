package authprivacy

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/postmedia"
)

// A single comment image has a bounded proof including its actual published
// selection and metadata, parent/root visibility, author and viewer versions.
// Source bytes are deliberately excluded from this final-transaction query.
const preparedCommentMediaProofSQL = `SELECT
 jsonb_build_array(p.post_id,p.visibility,p.moderation_hidden,p.owner_account_id,pv.visibility_revision,pa.projection_revision,
 t.comment_id,t.post_id,t.root_comment_id,t.deleted_at,to_jsonb(t)->'moderation_hidden',cv.projection_revision,
 root.comment_id,root.deleted_at,to_jsonb(root)->'moderation_hidden',rv.projection_revision,
 i.identity_id,i.account_id,i.deleted_at,iv.projection_revision,author.projection_revision,viewer.visibility_revision,
 x.task_id,x.version,x.state,x.image,m.content_version_id,m.erased_at,m.metadata,m.payload IS NOT NULL,
 c_posts.accounts_blocked($4,p.owner_account_id),c_posts.accounts_blocked($4,i.account_id)),
 COALESCE(p.visibility='PUBLISHED' AND NOT p.moderation_hidden AND NOT c_posts.accounts_blocked($4,p.owner_account_id)
 AND t.post_id=p.post_id AND t.deleted_at IS NULL AND NOT COALESCE((to_jsonb(t)->>'moderation_hidden')::boolean,false)
 AND root.deleted_at IS NULL AND NOT COALESCE((to_jsonb(root)->>'moderation_hidden')::boolean,false)
 AND NOT c_posts.accounts_blocked($4,i.account_id) AND x.state='PUBLISHED'
 AND m.erased_at IS NULL AND m.payload IS NOT NULL,false),
 m.metadata,x.image,
 viewer.account_id IS NOT NULL
 AND (p.post_id IS NULL OR (pv.post_id IS NOT NULL AND pa.account_id IS NOT NULL))
 AND (t.comment_id IS NULL OR (cv.comment_id IS NOT NULL AND i.identity_id IS NOT NULL AND iv.identity_id IS NOT NULL AND author.account_id IS NOT NULL))
 AND (root.comment_id IS NULL OR rv.comment_id IS NOT NULL)
 FROM (SELECT 1) one
 LEFT JOIN c_posts.posts p ON p.post_id=$1
 LEFT JOIN c_comments.tasks t ON t.comment_id=$2
 LEFT JOIN c_comments.tasks root ON root.comment_id=t.root_comment_id
 LEFT JOIN public.community_identities i ON i.identity_id=t.identity_id
 LEFT JOIN c_comments.attempts x ON x.task_id=t.task_id AND x.state='PUBLISHED'
 LEFT JOIN c_comments.images m ON m.task_id=x.task_id AND m.version=x.version AND m.content_version_id=$3
 LEFT JOIN c_comments.read_post_versions pv ON pv.post_id=p.post_id
 LEFT JOIN c_posts.read_account_versions pa ON pa.account_id=p.owner_account_id
 LEFT JOIN c_comments.read_comment_versions cv ON cv.comment_id=t.comment_id
 LEFT JOIN c_comments.read_comment_versions rv ON rv.comment_id=root.comment_id
 LEFT JOIN c_posts.read_identity_versions iv ON iv.identity_id=i.identity_id
 LEFT JOIN c_posts.read_account_versions author ON author.account_id=i.account_id
 LEFT JOIN c_posts.read_account_versions viewer ON viewer.account_id=$4`

type preparedCommentMediaProof struct {
	binding, metadata, selection []byte
	visible                      bool
}

func readPreparedCommentMediaProof(ctx context.Context, tx pgx.Tx, post, comment, content string, account uuid.UUID) (preparedCommentMediaProof, error) {
	p := preparedCommentMediaProof{}
	var intact bool
	err := tx.QueryRow(ctx, preparedCommentMediaProofSQL, post, comment, content, account).
		Scan(&p.binding, &p.visible, &p.metadata, &p.selection, &intact)
	if err != nil {
		return p, err
	}
	if !intact {
		return p, ErrAuthorizationUnavailable
	}
	return p, nil
}

func (c *Community) readCommentMediaPrepared(ctx context.Context, bearer [32]byte, post, comment, content string) (PostResponse, error) {
	return c.withPreparedReadPlan(ctx, bearer, postError("SERVICE_UNAVAILABLE", 503),
		func(prepareCtx context.Context, tx pgx.Tx, subject readSubject) (*preparedReadPlan, error) {
			proof, err := readPreparedCommentMediaProof(prepareCtx, tx, post, comment, content, subject.account)
			if err != nil {
				return nil, ErrAuthorizationUnavailable
			}
			var original []byte
			var metadata postmedia.Metadata
			plan := &preparedReadPlan{
				Revalidate: func(tx pgx.Tx, a uuid.UUID, _ AuthorizationDecision, _ restriction) error {
					if e := c.checkCommentKeys(ctx, tx); e != nil {
						return e
					}
					current, e := readPreparedCommentMediaProof(ctx, tx, post, comment, content, a)
					if e != nil {
						return ErrAuthorizationUnavailable
					}
					if !bytes.Equal(current.binding, proof.binding) {
						return errReadConflict
					}
					return nil
				},
				Complete: func(pgx.Tx, uuid.UUID, AuthorizationDecision, restriction) (any, error) {
					return ImageBytes{Bytes: original, MIME: metadata.MIME}, nil
				},
				Release: func() {
					original = nil
					proof.binding, proof.metadata, proof.selection = nil, nil, nil
				},
			}
			if !proof.visible {
				return plan, postError("MEDIA_NOT_FOUND", 404)
			}
			if err = tx.QueryRow(prepareCtx, `SELECT payload FROM c_comments.images WHERE task_id=(
 SELECT task_id FROM c_comments.tasks WHERE comment_id=$1) AND content_version_id=$2 AND erased_at IS NULL AND payload IS NOT NULL`, comment, content).Scan(&original); err != nil {
				return plan, ErrAuthorizationUnavailable
			}
			plan.AfterPrepare = func(context.Context) error {
				var selection postmedia.Selection
				if json.Unmarshal(proof.metadata, &metadata) != nil || json.Unmarshal(proof.selection, &selection) != nil || !metadata.Matches(selection, original) {
					return ErrAuthorizationUnavailable
				}
				return nil
			}
			return plan, nil
		})
}
