package authprivacy

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/comments"
)

func TestCommentPreparedVersionFactsPostgres(t *testing.T) {
	l := commentTextLab(t)
	ctx := context.Background()
	if err := l.c.checkPreparedCommentReadSchema(ctx); err != nil {
		t.Fatal("fresh prepared read schema", err)
	}
	owner := sessionTestAccount(t, l, "read_facts_owner")
	reader := sessionTestAccount(t, l, "read_facts_reader")
	identity := identityChange(t, l.c, owner.SessionToken, "CREATE", uuid.Nil, "作者").IdentityID
	post := commentTextPost(t, l, owner.SessionToken, identity)
	rootTask := uuid.NewString()
	commentTextSend(t, l, owner.SessionToken, postTestKey(t), comments.Intent{Operation: comments.Create, TaskID: rootTask, PostID: post, IdentityID: identity.String(), Body: "root"})
	before := count(t, l.cp, `SELECT membership_revision FROM c_comments.read_scope_versions WHERE post_id=$1 AND root_id=$2`, post, uuid.Nil)
	commentTextPublish(t, l)
	root := *commentTextOwn(t, l, owner.SessionToken, rootTask).PublishedCommentID
	member := count(t, l.cp, `SELECT membership_revision FROM c_comments.read_scope_versions WHERE post_id=$1 AND root_id=$2`, post, uuid.Nil)
	if member != before+1 {
		t.Fatal("publication did not advance candidate range", before, member)
	}
	projection := count(t, l.cp, `SELECT projection_revision FROM c_comments.read_comment_versions WHERE comment_id=$1`, root)
	postCount := count(t, l.cp, `SELECT count_revision FROM c_comments.read_post_versions WHERE post_id=$1`, post)
	order := count(t, l.cp, `SELECT order_revision FROM c_comments.read_scope_versions WHERE post_id=$1 AND root_id=$2`, post, uuid.Nil)
	liked := true
	commentTextSend(t, l, reader.SessionToken, postTestKey(t), comments.Intent{Operation: comments.SetLike, CommentID: root, Liked: &liked})
	if count(t, l.cp, `SELECT like_count FROM c_comments.read_comment_versions WHERE comment_id=$1`, root) != 1 ||
		count(t, l.cp, `SELECT projection_revision FROM c_comments.read_comment_versions WHERE comment_id=$1`, root) != projection ||
		count(t, l.cp, `SELECT count_revision FROM c_comments.read_post_versions WHERE post_id=$1`, post) != postCount ||
		count(t, l.cp, `SELECT membership_revision FROM c_comments.read_scope_versions WHERE post_id=$1 AND root_id=$2`, post, uuid.Nil) != member ||
		count(t, l.cp, `SELECT order_revision FROM c_comments.read_scope_versions WHERE post_id=$1 AND root_id=$2`, post, uuid.Nil) != order+1 {
		t.Fatal("likes must only advance the cached count and sorting dependency")
	}
	// The business write and all version changes must roll back together.
	rejected := errors.New("reject this prepared test write")
	_, err := l.c.withCommentSession(ctx, reader.SessionToken, func(tx pgx.Tx, _ uuid.UUID, _ AuthorizationDecision, _ restriction) (any, error) {
		if _, err := tx.Exec(ctx, `DELETE FROM c_comments.likes WHERE comment_id=$1 AND account_id=$2`, root, reader.AccountID); err != nil {
			return nil, err
		}
		return nil, rejected
	})
	if !errors.Is(err, rejected) || count(t, l.cp, `SELECT like_count FROM c_comments.read_comment_versions WHERE comment_id=$1`, root) != 1 ||
		count(t, l.cp, `SELECT count(*) FROM c_comments.likes WHERE comment_id=$1`, root) != 1 {
		t.Fatal("failed final transaction changed read facts", err)
	}
	commentTextSend(t, l, owner.SessionToken, postTestKey(t), comments.Intent{Operation: comments.Create, TaskID: uuid.NewString(), PostID: post, IdentityID: identity.String(), RootCommentID: root, TargetCommentID: root, Body: "reply"})
	repliesBefore := count(t, l.cp, `SELECT membership_revision FROM c_comments.read_scope_versions WHERE post_id=$1 AND root_id=$2`, post, root)
	commentTextPublish(t, l)
	if count(t, l.cp, `SELECT membership_revision FROM c_comments.read_scope_versions WHERE post_id=$1 AND root_id=$2`, post, root) != repliesBefore+1 {
		t.Fatal("reply publication did not advance root count dependency")
	}
	// Seed a pre-comment-facts image of this owned database, then apply the real
	// migration Up again to prove upgrade backfill, including historical likes.
	removePreparedFeedReadFixture(t, l)
	mustExec(t, l.cp, `DROP FUNCTION c_comments.advance_read_versions() CASCADE;
 DROP TABLE c_comments.read_scope_versions,c_comments.read_post_versions,c_comments.read_comment_versions,
 c_posts.read_identity_versions,c_posts.read_account_versions;
 DROP INDEX c_comments.comment_snapshot_owner_expiry_idx`)
	sql, err := os.ReadFile("../../migrations/0041_prepared_comment_reads.sql")
	if err != nil {
		t.Fatal(err)
	}
	mustExec(t, l.cp, strings.SplitN(string(sql), "-- +goose Down", 2)[0])
	if err := l.c.checkPreparedCommentReadSchema(ctx); err != nil || count(t, l.cp, `SELECT like_count FROM c_comments.read_comment_versions WHERE comment_id=$1`, root) != 1 {
		t.Fatal("historical read facts were not backfilled", err)
	}
}

func TestCommentPreparedSchemaPrivilegeAndHooksPostgres(t *testing.T) {
	l := commentTextLab(t)
	ctx := context.Background()
	mustExec(t, l.cp, `CREATE TABLE IF NOT EXISTS public.goose_db_version(version_id bigint,is_applied boolean)`)
	mustExec(t, l.cp, `REVOKE CREATE,TEMP ON DATABASE hnuhole_c FROM PUBLIC,hnuhole_c_runtime,hnuhole_c_recovery,hnuhole_business`)
	for _, path := range []string{"../../../../infra/postgres/grant-community.sql", "../../../../infra/postgres/grant-comments.sql"} {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		mustExec(t, l.cp, string(body))
	}
	pool, err := pgxpool.New(ctx, os.Getenv("AUTHLAB_POSTS_RUNTIME_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	runtime := *l.c
	runtime.pool = pool
	if err = runtime.checkPreparedCommentReadSchema(ctx); err != nil {
		t.Fatal("minimum runtime grants rejected", err)
	}
	audit, err := os.ReadFile("../../authlab/privacy-role-audit.sql")
	if err != nil {
		t.Fatal(err)
	}
	auditSQL := strings.Replace(string(audit), ":'party'", "'c'", 1)
	if _, err = l.cp.Exec(ctx, auditSQL); err != nil {
		t.Fatal("private version read grants failed role audit", err)
	}
	mustExec(t, l.cp, `GRANT SELECT ON c_comments.read_comment_versions TO hnuhole_business`)
	if _, err = l.cp.Exec(ctx, auditSQL); err == nil {
		t.Fatal("role audit accepted version reads by business")
	}
	mustExec(t, l.cp, `REVOKE SELECT ON c_comments.read_comment_versions FROM hnuhole_business`)
	if _, err = pool.Exec(ctx, `UPDATE c_comments.read_comment_versions SET like_count=0`); err == nil {
		t.Fatal("runtime may forge read facts")
	}
	if _, err = pool.Exec(ctx, `SELECT c_comments.advance_read_versions()`); err == nil {
		t.Fatal("runtime may invoke a trigger-only function")
	}
	mustExec(t, l.cp, `ALTER TABLE c_comments.likes DISABLE TRIGGER prepared_read_like`)
	if err = runtime.checkPreparedCommentReadSchema(ctx); !errors.Is(err, errCommentSchema) {
		t.Fatal("disabled source hook accepted", err)
	}
	mustExec(t, l.cp, `ALTER TABLE c_comments.likes ENABLE TRIGGER prepared_read_like;
 GRANT UPDATE(projection_revision) ON c_comments.read_comment_versions TO hnuhole_c_runtime`)
	if err = runtime.checkPreparedCommentReadSchema(ctx); !errors.Is(err, errCommentSchema) {
		t.Fatal("column-level version write grant accepted", err)
	}
	if _, err = l.cp.Exec(ctx, auditSQL); err == nil {
		t.Fatal("role audit accepted a column-level version write")
	}
	mustExec(t, l.cp, `REVOKE UPDATE(projection_revision) ON c_comments.read_comment_versions FROM hnuhole_c_runtime;
 DROP TRIGGER prepared_read_like ON c_comments.likes;
 CREATE TRIGGER prepared_read_like AFTER INSERT OR DELETE ON c_comments.likes
 FOR EACH ROW WHEN (false) EXECUTE FUNCTION c_comments.advance_read_versions()`)
	if err = runtime.checkPreparedCommentReadSchema(ctx); !errors.Is(err, errCommentSchema) {
		t.Fatal("a same-name no-op hook passed startup", err)
	}
	mustExec(t, l.cp, `DROP TRIGGER prepared_read_like ON c_comments.likes;
 CREATE TRIGGER prepared_read_like AFTER INSERT OR DELETE ON c_comments.likes
 FOR EACH ROW EXECUTE FUNCTION c_comments.advance_read_versions();
 CREATE OR REPLACE FUNCTION c_comments.advance_read_versions() RETURNS trigger
 LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $$BEGIN RETURN NULL; END$$`)
	if err = runtime.checkPreparedCommentReadSchema(ctx); !errors.Is(err, errCommentSchema) {
		t.Fatal("a same-signature no-op function passed startup", err)
	}
}
