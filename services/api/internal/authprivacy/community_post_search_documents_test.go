package authprivacy

import (
	"context"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/posts"
)

func TestPostSearchDocumentRouting(t *testing.T) {
	for keyword, want := range map[string]bool{"中": false, "中文": false, "中文字": true, "a_b": false, "ab cd": false, "a%bc": false, "😀😀😀": false, "a\\b": false, "ABC": true, "中文abc": true} {
		if got := postSearchHasTrigram(keyword); got != want {
			t.Fatalf("keyword %q: %v, want %v", keyword, got, want)
		}
	}
	if strings.Contains(documentSearchCandidatesSQL, "attempt_contents") || strings.Contains(documentSearchCandidatesSQL, "to_jsonb") || strings.Contains(documentSearchCandidatesSQL, "lower(tag") {
		t.Fatal("document query retained content assembly or tag normalization")
	}
}

// Re-run the established contract/fault scenarios through the new candidate
// path, including native Gate checkpoints, retries and off-page leaders.
func TestFeedPostSearchDocumentsExistingScenariosPostgres(t *testing.T) {
	t.Setenv("AUTHLAB_SEARCH_DOCUMENTS", "1")
	for name, run := range map[string]func(*testing.T){
		"ranking":                 TestFeedPostSearchScopeRankingAndStablePagesPostgres,
		"unlocked_candidates":     TestFeedPostSearchCandidateScanDoesNotHoldGatePostgres,
		"final_revalidation":      TestFeedPostSearchFinalRevalidationPostgres,
		"checkpoint":              TestFeedPostSearchPreparationPreservesCheckpointAndFinalCommitPostgres,
		"freeze":                  TestFeedPostSearchPreparationPersistsDetectedFreezePostgres,
		"cursor_time":             TestFeedPostSearchFinalCursorUpperTimeBoundPostgres,
		"edit_invalidates_cursor": TestFeedSearchContentEditInvalidatesContinuationPostgres,
	} {
		t.Run(name, run)
	}
}

func TestFeedPostSearchDocumentsBackfillDefersBusyPostgres(t *testing.T) {
	l := newPostLab(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	owner := sessionTestAccount(t, l, "docs_concurrent")
	identity := identityChange(t, l.c, owner.SessionToken, "CREATE", uuid.Nil, "并发作者").IdentityID
	id := postTestCommand(t, l, owner.SessionToken, postTestKey(t), postTestIntent(t, l, identity)).PostID
	postTestPublish(t, l)
	mustExec(t, l.cp, `UPDATE c_posts.post_search_document_state SET ready=false,last_post_id='00000000-0000-0000-0000-000000000000'`)
	writer, err := l.cp.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Rollback(context.Background())
	if _, err = writer.Exec(ctx, `UPDATE c_posts.posts SET edited_title='最新标题',edited_body='最新正文',edited_at=now(),edit_revision=gen_random_uuid() WHERE post_id=$1`, id); err != nil {
		t.Fatal(err)
	}
	var processed int
	if err = l.cp.QueryRow(ctx, `SELECT c_posts.backfill_post_search_documents(1)`).Scan(&processed); err != nil || processed != -1 {
		t.Fatal("backfill must defer a busy post", processed, err)
	}
	if count(t, l.cp, `SELECT count(*) FROM c_posts.post_search_document_state WHERE last_post_id='00000000-0000-0000-0000-000000000000'`) != 1 {
		t.Fatal("busy row advanced cursor")
	}
	if err = writer.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = l.cp.QueryRow(ctx, `SELECT c_posts.backfill_post_search_documents(1)`).Scan(&processed); err != nil || processed != 1 {
		t.Fatal("backfill did not resume", processed, err)
	}
	mustExec(t, l.cp, `SELECT c_posts.validate_post_search_documents()`)
	if count(t, l.cp, `SELECT count(*) FROM c_posts.post_search_documents WHERE post_id=$1 AND title_norm='最新标题' AND body_norm='最新正文'`, id) != 1 {
		t.Fatal("backfill overwrote newer content")
	}
	// A later rolled-back mutation must roll its projection back as well.
	tx, err := l.cp.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE c_posts.posts SET moderation_hidden=true WHERE post_id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	mustExec(t, l.cp, `SELECT c_posts.validate_post_search_documents()`)
}

func TestFeedPostSearchDocumentsSchemaAndPrivilegesPostgres(t *testing.T) {
	l := newLab(t)
	config := enablePostLab(t, l)
	config.SearchDocuments = true
	ctx := context.Background()
	if configured, err := l.c.WithPosts(ctx, config); err != nil || !configured.posts.searchDocuments {
		t.Fatal("explicit search configuration rejected", err)
	}
	var sourceSQL, refreshSQL string
	if err := l.cp.QueryRow(ctx, `SELECT pg_get_functiondef('c_posts.current_post_search_documents(uuid)'::regprocedure),pg_get_functiondef('c_posts.refresh_post_search_document(uuid)'::regprocedure)`).Scan(&sourceSQL, &refreshSQL); err != nil {
		t.Fatal(err)
	}
	grant, err := os.ReadFile("../../../../infra/postgres/grant-community.sql")
	if err != nil {
		t.Fatal(err)
	}
	mustExec(t, l.cp, `CREATE TABLE IF NOT EXISTS public.goose_db_version(version_id bigint,is_applied boolean); REVOKE CREATE,TEMP ON DATABASE hnuhole_c FROM PUBLIC,hnuhole_c_runtime,hnuhole_c_recovery,hnuhole_business`)
	mustExec(t, l.cp, string(grant))
	pool, err := pgxpool.New(ctx, os.Getenv("AUTHLAB_POSTS_RUNTIME_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	runtime := *l.c
	runtime.pool = pool
	if err = runtime.checkPostSearchDocumentSchema(ctx); err != nil {
		t.Fatal("runtime grants", err)
	}
	for _, statement := range []string{`DELETE FROM c_posts.post_search_documents`, `UPDATE c_posts.post_search_document_state SET ready=true`, `SELECT c_posts.refresh_post_search_document(gen_random_uuid())`, `SELECT c_posts.validate_post_search_documents()`, `SELECT c_posts.backfill_post_search_documents(1)`, `SELECT c_posts.current_post_search_documents(NULL)`} {
		if _, err = pool.Exec(ctx, statement); err == nil {
			t.Fatalf("runtime maintenance permitted: %s", statement)
		}
	}
	for _, mutation := range []struct{ change, restore string }{
		{`UPDATE c_posts.post_search_document_state SET ready=false`, `UPDATE c_posts.post_search_document_state SET ready=true`},
		{`UPDATE c_posts.post_search_document_state SET normalization_version='obsolete'`, `UPDATE c_posts.post_search_document_state SET normalization_version=pg_collation_actual_version('pg_catalog."und-x-icu"'::regcollation)`},
		{`ALTER TABLE c_posts.posts DISABLE TRIGGER post_search_document_post`, `ALTER TABLE c_posts.posts ENABLE TRIGGER post_search_document_post`},
		{`ALTER TABLE c_posts.attempt_contents DISABLE TRIGGER post_search_document_content`, `ALTER TABLE c_posts.attempt_contents ENABLE TRIGGER post_search_document_content`},
		{`DROP INDEX c_posts.post_search_documents_trgm_idx`, `CREATE INDEX post_search_documents_trgm_idx ON c_posts.post_search_documents USING gin(search_key public.gin_trgm_ops)`},
		{`GRANT UPDATE(ready) ON c_posts.post_search_document_state TO hnuhole_c_runtime`, `REVOKE UPDATE(ready) ON c_posts.post_search_document_state FROM hnuhole_c_runtime`},
		{`GRANT EXECUTE ON FUNCTION c_posts.backfill_post_search_documents(integer) TO hnuhole_c_runtime`, `REVOKE EXECUTE ON FUNCTION c_posts.backfill_post_search_documents(integer) FROM hnuhole_c_runtime`},
		{`ALTER FUNCTION c_posts.current_post_search_documents(uuid) VOLATILE`, `ALTER FUNCTION c_posts.current_post_search_documents(uuid) STABLE`},
		{`CREATE OR REPLACE FUNCTION c_posts.current_post_search_documents(selected uuid DEFAULT NULL) RETURNS SETOF c_posts.post_search_documents LANGUAGE sql STABLE SET search_path=pg_catalog AS $$ SELECT * FROM c_posts.post_search_documents WHERE false $$`, sourceSQL},
		{`CREATE OR REPLACE FUNCTION c_posts.refresh_post_search_document(selected uuid) RETURNS void LANGUAGE plpgsql SET search_path=pg_catalog AS $$ BEGIN RETURN; END $$`, refreshSQL},
	} {
		mustExec(t, l.cp, mutation.change)
		if err = runtime.checkPostSearchDocumentSchema(ctx); err == nil {
			t.Fatalf("damaged schema accepted: %s", mutation.change)
		}
		mustExec(t, l.cp, mutation.restore)
	}
	if err = runtime.checkPostSearchDocumentSchema(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestFeedPostSearchDocumentsPublishedInsertPostgres(t *testing.T) {
	l := newPostLab(t)
	ctx := context.Background()
	owner := sessionTestAccount(t, l, "docs_insert")
	identity := identityChange(t, l.c, owner.SessionToken, "CREATE", uuid.Nil, "插入作者").IdentityID
	channel := feedTestChannels(t, l)[0]
	post, task := uuid.New(), uuid.New()
	at := time.Now().UTC()
	tx, err := l.cp.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	for _, step := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO c_posts.posts(post_id,owner_account_id,channel_id,visibility,published_attempt_version,published_at,publication_ordinal) VALUES($1,$2,$3,'PUBLISHED',1,$4,nextval('c_posts.publication_ordinal_seq'))`, []any{post, owner.AccountID, channel, at}},
		{`INSERT INTO c_posts.post_identity_bindings(account_id,post_id,identity_id,bound_at) VALUES($1,$2,$3,$4)`, []any{owner.AccountID, post, identity, at}},
		{`INSERT INTO c_posts.publication_tasks(task_id,post_id,owner_account_id,identity_id,channel_id,latest_attempt_version) VALUES($1,$2,$3,$4,$5,1)`, []any{task, post, owner.AccountID, identity, channel}},
		{`INSERT INTO c_posts.publication_attempts(task_id,version,accepted_stop_generation,state,accepted_at,acceptance_ordinal,terminal_at) VALUES($1,1,0,'PUBLISHED',$2,nextval('c_posts.acceptance_ordinal_seq'),$2)`, []any{task, at}},
		{`INSERT INTO c_posts.attempt_contents(task_id,version,title,body,request_digest) VALUES($1,1,'插入式发布','公开正文',decode(repeat('11',32),'hex'))`, []any{task}},
	} {
		if _, err = tx.Exec(ctx, step.sql, step.args...); err != nil {
			t.Fatal(err)
		}
	}
	var n int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM c_posts.post_search_documents WHERE post_id=$1 AND title_norm='插入式发布'`, post).Scan(&n); err != nil || n != 1 {
		t.Fatal("content insert missed projection", err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	mustExec(t, l.cp, `SELECT c_posts.validate_post_search_documents()`)
}

func TestFeedPostSearchDocumentsEquivalentPostgres(t *testing.T) {
	l := newPostLab(t)
	ctx := context.Background()
	owner := sessionTestAccount(t, l, "docs_owner")
	reader := sessionTestAccount(t, l, "docs_reader")
	identity := identityChange(t, l.c, owner.SessionToken, "CREATE", uuid.Nil, "搜索作者").IdentityID
	channels := feedTestChannels(t, l)
	for i, content := range []struct {
		title, body string
		tags        []string
	}{
		{"中文字", "中文与正文 ABC %_\\ 上海交通大学", []string{"分项", "标签", "NBA"}},
		{"标题 ABC 上海交通大学", "不含旧内容 İSTANBUL", []string{"Alpha", "Beta"}},
		{"中文字符", "另一个频道 abc %_\\", []string{"中文", "测试"}},
		{"标点 .!?", "😀 e\u0301 ＡＢＣ", []string{"a_b", "百分"}},
		{"中文字", "ΣΟΣ σος σ LoW", nil},
	} {
		intent := postTestIntent(t, l, identity)
		intent.ChannelID, intent.Title, intent.Body, intent.Tags = channels[i%2], content.title, content.body, content.tags
		postTestCommand(t, l, owner.SessionToken, postTestKey(t), intent)
		postTestPublish(t, l)
	}
	if err := l.c.checkPostSearchDocumentSchema(ctx); err != nil {
		t.Fatal(err)
	}
	doc, service := *l.c, *l.c.posts
	service.searchDocuments = true
	doc.posts = &service
	for _, keyword := range []string{"中", "中文", "中文字", "上海交通大学", "abc", "ABC %_\\", "ABC%", "ABC_", "ABC\\", "%", "_", "\\", "%_\\", "nba", "σ", "σος", "İ", "a_b", "分项标签", "无命中", ".!?", "😀", "e\u0301"} {
		for _, scope := range []string{"", channels[0]} {
			for _, sort := range []string{posts.SearchRelevance, posts.SearchLatest} {
				a := posts.SearchQuery{Keyword: keyword, ChannelID: scope, Sort: sort, Limit: 1}
				b := a
				for page := 0; ; page++ {
					if page > 5 {
						t.Fatal("nonterminating cursor")
					}
					old, err := l.c.SearchPosts(ctx, reader.SessionToken, a)
					if err != nil {
						t.Fatalf("old %q: %v", keyword, err)
					}
					fresh, err := doc.SearchPosts(ctx, reader.SessionToken, b)
					if err != nil {
						t.Fatalf("new %q: %v", keyword, err)
					}
					x, y := old.Value.(posts.FeedPage), fresh.Value.(posts.FeedPage)
					if !reflect.DeepEqual(x.Items, y.Items) || (x.NextCursor == nil) != (y.NextCursor == nil) {
						t.Fatalf("diff keyword=%q scope=%s sort=%s page=%d: %+v / %+v", keyword, scope, sort, page, x, y)
					}
					if x.NextCursor == nil {
						break
					}
					// Exercise old -> new and new -> old cursor compatibility too.
					a.Cursor, b.Cursor = *y.NextCursor, *x.NextCursor
				}
			}
		}
	}
}

func TestFeedPostSearchDocumentsBackfillAndLifecyclePostgres(t *testing.T) {
	l := newPostLab(t)
	ctx := context.Background()
	owner := sessionTestAccount(t, l, "docs_lifecycle")
	identity := identityChange(t, l.c, owner.SessionToken, "CREATE", uuid.Nil, "搜索作者").IdentityID
	intent := postTestIntent(t, l, identity)
	accepted := postTestCommand(t, l, owner.SessionToken, postTestKey(t), intent)
	postTestPublish(t, l)
	id := accepted.PostID
	if count(t, l.cp, `SELECT count(*) FROM c_posts.post_search_documents WHERE post_id=$1`, id) != 1 {
		t.Fatal("publication missing")
	}
	mustExec(t, l.cp, `UPDATE c_posts.post_search_document_state SET ready=false; DELETE FROM c_posts.post_search_documents`)
	if err := l.c.checkPostSearchDocumentSchema(ctx); err == nil {
		t.Fatal("partial backfill enabled")
	}
	if _, err := l.cp.Exec(ctx, `SELECT c_posts.validate_post_search_documents()`); err == nil {
		t.Fatal("drift validated")
	}
	command := exec.CommandContext(ctx, "sh", "../../authlab/backfill-post-search.sh", "1")
	command.Env = append(os.Environ(), "POST_SEARCH_BACKFILL_DSN="+l.cp.Config().ConnString())
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("operator backfill script: %v\n%s", err, output)
	}
	if err := l.c.checkPostSearchDocumentSchema(ctx); err != nil {
		t.Fatal(err)
	}
	read, err := l.c.ReadPostEdit(ctx, owner.SessionToken, id)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := read.Value.(posts.EditSnapshot)
	tags := []string{"新标签"}
	if _, err = l.c.SavePostEdit(ctx, owner.SessionToken, id, postTestKey(t), posts.EditInput{Revision: snapshot.Revision, Title: "编辑后", Body: "新正文", Tags: &tags}); err != nil {
		t.Fatal(err)
	}
	mustExec(t, l.cp, `SELECT c_posts.validate_post_search_documents()`)
	if count(t, l.cp, `SELECT count(*) FROM c_posts.post_search_documents WHERE post_id=$1 AND title_norm='编辑后' AND body_norm='新正文' AND tags_norm=ARRAY['新标签']`, id) != 1 {
		t.Fatal("edit missing")
	}
	mustExec(t, l.cp, `UPDATE c_posts.posts SET moderation_hidden=true WHERE post_id=$1`, id)
	if count(t, l.cp, `SELECT count(*) FROM c_posts.post_search_documents WHERE post_id=$1`, id) != 0 {
		t.Fatal("hidden content retained")
	}
	mustExec(t, l.cp, `UPDATE c_posts.posts SET moderation_hidden=false WHERE post_id=$1`, id)
	mustExec(t, l.cp, `SELECT c_posts.validate_post_search_documents()`)
	// Check erasure before the matching deletion in the same transaction; the
	// deferred public-content invariant must still hold when it commits.
	tx, err := l.cp.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `UPDATE c_posts.attempt_contents SET title=NULL,body=NULL,erased_at=now() WHERE task_id=(SELECT task_id FROM c_posts.publication_tasks WHERE post_id=$1)`, id); err != nil {
		t.Fatal(err)
	}
	var retained int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM c_posts.post_search_documents WHERE post_id=$1`, id).Scan(&retained); err != nil || retained != 0 {
		t.Fatal("erasure retained edited document", err)
	}
	if _, err = tx.Exec(ctx, `UPDATE c_posts.posts SET visibility='DELETED',deleted_at=now() WHERE post_id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if count(t, l.cp, `SELECT count(*) FROM c_posts.post_search_documents WHERE post_id=$1`, id) != 0 {
		t.Fatal("erased content retained")
	}
	mustExec(t, l.cp, `SELECT c_posts.validate_post_search_documents()`)
}
