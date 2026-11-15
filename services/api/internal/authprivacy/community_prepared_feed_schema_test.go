package authprivacy

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/postmedia"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/posts"
)

func TestPreparedFeedVersionFactsPostgres(t *testing.T) {
	l := newLab(t)
	cfg := enablePostLab(t, l)
	ctx := context.Background()
	if err := l.c.checkPreparedFeedReadSchema(ctx); err != nil {
		t.Fatal("fresh feed facts", err)
	}
	for _, media := range []bool{false, true} {
		cfg.PreparedFeedReads, cfg.PreparedMediaReads = !media, media
		enabled, err := l.c.WithPosts(ctx, cfg)
		if err != nil || enabled.posts.preparedFeedReads != !media || enabled.posts.preparedMediaReads != media {
			t.Fatal("configured module did not enable independently", err)
		}
	}
	owner := sessionTestAccount(t, l, "feed_fact_owner")
	identity := identityChange(t, l.c, owner.SessionToken, "CREATE", uuid.Nil, "版本作者").IdentityID
	intent := postTestIntent(t, l, identity)
	post := postTestCommand(t, l, owner.SessionToken, postTestKey(t), intent).PostID
	scope := func() (int, int) {
		return count(t, l.cp, `SELECT membership_revision FROM c_posts.read_feed_scope_versions WHERE channel_id=$1`, intent.ChannelID), count(t, l.cp, `SELECT publication_revision FROM c_posts.read_feed_scope_versions WHERE channel_id=$1`, intent.ChannelID)
	}
	projection := func() int {
		return count(t, l.cp, `SELECT projection_revision FROM c_posts.read_feed_post_versions WHERE post_id=$1`, post)
	}
	membership, publication := scope()
	postTestPublish(t, l)
	m, p := scope()
	if m != membership || p != publication+1 {
		t.Fatal("new publication changed a frozen-ceiling membership fence", m, p)
	}
	before := projection()
	if _, err := l.c.SetPostInteraction(ctx, owner.SessionToken, post, posts.PostLike, true); err != nil {
		t.Fatal(err)
	}
	if projection() != before {
		t.Fatal("interaction invalidated payload projection")
	}
	// Controlled owner writes exercise the same trigger transactions as the
	// separately tested moderation/edit entry points.
	mustExec(t, l.cp, `UPDATE c_posts.posts SET moderation_hidden=true WHERE post_id=$1`, post)
	m, p = scope()
	if m != membership+1 || p != publication+1 || projection() <= before {
		t.Fatal("hiding an existing member lost its fence")
	}
	mustExec(t, l.cp, `UPDATE c_posts.posts SET moderation_hidden=false WHERE post_id=$1`, post)
	m, p = scope()
	if m != membership+2 || p != publication+1 {
		t.Fatal("restoring an existing member lost its fence")
	}
	before = projection()
	mustExec(t, l.cp, `UPDATE c_posts.posts SET edit_revision=gen_random_uuid(),edited_at=published_at,edited_title='edited',edited_body='payload',edited_image_ids='{}' WHERE post_id=$1`, post)
	m2, p2 := scope()
	if projection() <= before || m2 != m || p2 != p {
		t.Fatal("content edit did not exclusively change projection")
	}
	before = projection()
	tx, err := l.cp.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE c_posts.posts SET edited_title='rolled back' WHERE post_id=$1`, post); err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(ctx); err != nil || projection() != before {
		t.Fatal("rollback leaked a read revision", err)
	}
	// A damaged live version is not silently reconstructed by a write.
	mustExec(t, l.cp, `DELETE FROM c_posts.read_feed_post_versions WHERE post_id=$1`, post)
	if _, err = l.cp.Exec(ctx, `UPDATE c_posts.posts SET edited_title='must fail' WHERE post_id=$1`, post); err == nil {
		t.Fatal("source write silently accepted a missing version")
	}
	if err = l.c.checkPreparedFeedReadSchema(ctx); err == nil {
		t.Fatal("startup accepted missing live projection")
	}
	// Reapply the exact migration only after removing its owned increment.
	removePreparedFeedReadFixture(t, l)
	applyPreparedFeedFixture(t, l)
	if err = l.c.checkPreparedFeedReadSchema(ctx); err != nil || projection() != 1 {
		t.Fatal("historical backfill did not restore complete facts", err)
	}
	mustExec(t, l.cp, `DELETE FROM c_posts.read_feed_scope_versions WHERE channel_id=$1`, intent.ChannelID)
	if err = l.c.checkPreparedFeedReadSchema(ctx); err == nil {
		t.Fatal("startup accepted missing existing channel scope")
	}
}

func applyPreparedFeedFixture(t *testing.T, l *lab) {
	t.Helper()
	source, err := os.ReadFile("../../migrations/0042_prepared_feed_reads.sql")
	if err != nil {
		t.Fatal(err)
	}
	mustExec(t, l.cp, strings.SplitN(string(source), "-- +goose Down", 2)[0])
}

func TestPreparedFeedSelectedMediaVersionsPostgres(t *testing.T) {
	l := newPostLab(t)
	enableImageLab(t, l)
	owner := sessionTestAccount(t, l, "feed_media_owner")
	identity := identityChange(t, l.c, owner.SessionToken, "CREATE", uuid.Nil, "媒体作者").IdentityID
	intent := postTestIntent(t, l, identity)
	selection, data := imageFixture(t, "png")
	intent.Images = []postmedia.Selection{selection}
	accepted := postTestCommand(t, l, owner.SessionToken, postTestKey(t), intent)
	imageUpload(t, l, accepted, selection, imageGrant(t, l, owner.SessionToken, accepted, selection.MediaID), data)
	postTestPublish(t, l)
	post := accepted.PostID
	projection := func() int {
		return count(t, l.cp, `SELECT projection_revision FROM c_posts.read_feed_post_versions WHERE post_id=$1`, post)
	}
	before := projection()
	selected, unselected := uuid.NewString(), uuid.NewString()
	insert := func(id string) {
		mustExec(t, l.cp, `INSERT INTO c_posts.post_edit_media(post_id,content_version_id,command_digest,staged_at,payload,digest,mime_type,byte_length,width,height,display_width,display_height,frames)
VALUES($1,$2,decode(repeat('21',32),'hex'),now(),decode('616263','hex'),sha256(decode('616263','hex')),'image/png',3,1,1,1,1,1)`, post, id)
	}
	insert(selected)
	insert(unselected)
	if projection() != before {
		t.Fatal("unselected staging invalidated a public projection")
	}
	mustExec(t, l.cp, `UPDATE c_posts.posts SET edit_revision=gen_random_uuid(),edited_at=published_at,edited_title='edited',edited_body='body',edited_image_ids=ARRAY[$2::uuid] WHERE post_id=$1`, post, selected)
	if projection() <= before {
		t.Fatal("selected media list did not advance projection")
	}
	before = projection()
	mustExec(t, l.cp, `UPDATE c_posts.post_edit_media SET command_digest=NULL WHERE post_id=$1 AND content_version_id=$2`, post, selected)
	if projection() != before+1 {
		t.Fatal("publishing selected staged bytes did not advance projection")
	}
	before = projection()
	mustExec(t, l.cp, `DELETE FROM c_posts.post_edit_media WHERE post_id=$1 AND content_version_id=$2`, post, unselected)
	if projection() != before {
		t.Fatal("unselected staged cleanup invalidated a public projection")
	}
	// Delete/reinsert of the same selected content ID must invalidate the
	// original proof even when the public ID array never changed.
	mustExec(t, l.cp, `DELETE FROM c_posts.post_edit_media WHERE post_id=$1 AND content_version_id=$2`, post, selected)
	insert(selected)
	if projection() != before+2 {
		t.Fatal("selected content ID reuse escaped projection fencing")
	}
	before = projection()
	// Re-selecting the original set is itself versioned. Original erasure is
	// only legal with the terminal post transition, not while it is public.
	mustExec(t, l.cp, `UPDATE c_posts.posts SET edited_image_ids=NULL WHERE post_id=$1`, post)
	if projection() <= before {
		t.Fatal("original-selection change lost its proof")
	}
	before = projection()
	postTestCommand(t, l, owner.SessionToken, postTestKey(t), posts.Intent{Operation: posts.DeletePost, PostID: post})
	if projection() <= before || count(t, l.cp, `SELECT count(*) FROM c_posts.images WHERE task_id=$1 AND payload IS NULL AND erased_at IS NOT NULL`, accepted.TaskID) != 1 {
		t.Fatal("terminal original erasure lost its public projection fence")
	}
}

func TestPreparedFeedSchemaPrivilegeAndHooksPostgres(t *testing.T) {
	l := newPostLab(t)
	ctx := context.Background()
	mustExec(t, l.cp, `CREATE TABLE IF NOT EXISTS public.goose_db_version(version_id bigint,is_applied boolean);
REVOKE CREATE,TEMP ON DATABASE hnuhole_c FROM PUBLIC,hnuhole_c_runtime,hnuhole_c_recovery,hnuhole_business`)
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
	if err = runtime.checkPreparedFeedReadSchema(ctx); err != nil {
		t.Fatal("minimum runtime read grants rejected", err)
	}
	audit, err := os.ReadFile("../../authlab/privacy-role-audit.sql")
	if err != nil {
		t.Fatal(err)
	}
	auditSQL := strings.Replace(string(audit), ":'party'", "'c'", 1)
	if _, err = l.cp.Exec(ctx, auditSQL); err != nil {
		t.Fatal("feed grants failed role audit", err)
	}
	for _, statement := range []string{
		`UPDATE c_posts.read_feed_post_versions SET projection_revision=0`,
		`SELECT c_posts.advance_feed_read_versions()`,
	} {
		if _, err = pool.Exec(ctx, statement); err == nil {
			t.Fatal("runtime can forge feed facts")
		}
	}
	for _, damaged := range []struct{ mutate, restore string }{
		{`ALTER TABLE c_posts.posts DISABLE TRIGGER prepared_feed_post`, `ALTER TABLE c_posts.posts ENABLE TRIGGER prepared_feed_post`},
		{`GRANT UPDATE(projection_revision) ON c_posts.read_feed_post_versions TO hnuhole_c_runtime`, `REVOKE UPDATE(projection_revision) ON c_posts.read_feed_post_versions FROM hnuhole_c_runtime`},
		{`GRANT EXECUTE ON FUNCTION c_posts.advance_feed_read_versions() TO hnuhole_c_runtime`, `REVOKE EXECUTE ON FUNCTION c_posts.advance_feed_read_versions() FROM hnuhole_c_runtime`},
		{`ALTER TABLE c_posts.read_feed_scope_versions DROP CONSTRAINT read_feed_scope_versions_membership_revision_check`, `ALTER TABLE c_posts.read_feed_scope_versions ADD CONSTRAINT read_feed_scope_versions_membership_revision_check CHECK(membership_revision>=0)`},
	} {
		mustExec(t, l.cp, damaged.mutate)
		if err = runtime.checkPreparedFeedReadSchema(ctx); !errors.Is(err, errPostSchema) {
			t.Fatal("damaged feed schema accepted", err)
		}
		mustExec(t, l.cp, damaged.restore)
	}
	for _, grant := range []struct{ set, clear string }{
		{`GRANT SELECT ON c_posts.read_feed_post_versions TO hnuhole_business`, `REVOKE SELECT ON c_posts.read_feed_post_versions FROM hnuhole_business`},
		{`GRANT UPDATE(projection_revision) ON c_posts.read_feed_post_versions TO hnuhole_c_runtime`, `REVOKE UPDATE(projection_revision) ON c_posts.read_feed_post_versions FROM hnuhole_c_runtime`},
		{`GRANT EXECUTE ON FUNCTION c_posts.advance_feed_read_versions() TO hnuhole_business`, `REVOKE EXECUTE ON FUNCTION c_posts.advance_feed_read_versions() FROM hnuhole_business`},
	} {
		mustExec(t, l.cp, grant.set)
		if _, err = l.cp.Exec(ctx, auditSQL); err == nil {
			t.Fatal("role audit accepted excess feed fact authority")
		}
		mustExec(t, l.cp, grant.clear)
	}
	mustExec(t, l.cp, `DROP TRIGGER prepared_feed_channel ON public.channels;
CREATE TRIGGER prepared_feed_channel AFTER INSERT ON public.channels FOR EACH ROW WHEN(false) EXECUTE FUNCTION c_posts.advance_feed_read_versions()`)
	if err = runtime.checkPreparedFeedReadSchema(ctx); !errors.Is(err, errPostSchema) {
		t.Fatal("same-name no-op feed hook accepted", err)
	}
	mustExec(t, l.cp, `DROP TRIGGER prepared_feed_channel ON public.channels;
CREATE TRIGGER prepared_feed_channel AFTER INSERT ON public.channels FOR EACH ROW EXECUTE FUNCTION c_posts.advance_feed_read_versions();
CREATE OR REPLACE FUNCTION c_posts.advance_feed_read_versions() RETURNS trigger LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $$BEGIN RETURN NULL; END$$`)
	if err = runtime.checkPreparedFeedReadSchema(ctx); !errors.Is(err, errPostSchema) {
		t.Fatal("same-signature no-op feed function accepted", err)
	}
}
