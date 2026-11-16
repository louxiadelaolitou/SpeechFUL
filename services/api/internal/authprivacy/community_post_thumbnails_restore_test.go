package authprivacy

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestThumbnailStorageDumpRestoreIntegrityPostgres(t *testing.T) {
	f := thumbnailFixture(t)
	dump := filepath.Join(t.TempDir(), "posts.dump")
	dsn := os.Getenv("AUTHLAB_C_DSN")
	defs := func() map[string]string {
		m := map[string]string{}
		rows, e := f.l.cp.Query(context.Background(), `SELECT r.relname||':'||k.conname,pg_get_constraintdef(k.oid) FROM pg_constraint k JOIN pg_class r ON r.oid=k.conrelid WHERE k.conrelid IN ('c_posts.thumbnails'::regclass,'c_posts.thumbnail_storage'::regclass) AND k.contype='c'`)
		if e != nil {
			t.Fatal(e)
		}
		defer rows.Close()
		for rows.Next() {
			var k, v string
			rows.Scan(&k, &v)
			m[k] = v
		}
		return m
	}
	beforeDefs := defs()
	if e := exec.Command("pg_dump", "--format=custom", "--schema=c_posts", "--no-owner", "--no-privileges", "--file", dump, dsn).Run(); e != nil {
		t.Fatal("owned schema dump", e)
	}
	mustExec(t, f.l.cp, `DROP SCHEMA c_posts CASCADE`)
	if e := exec.Command("pg_restore", "--no-owner", "--no-privileges", "--dbname", dsn, dump).Run(); e != nil {
		t.Fatal("owned schema restore", e)
	}
	if e := f.l.c.checkThumbnailStorageSchema(context.Background(), false); e != nil {
		for k, v := range defs() {
			if beforeDefs[k] != v {
				t.Log("constraint normalization", k, "before", beforeDefs[k], "after", v)
			}
		}
		var constraints string
		f.l.cp.QueryRow(context.Background(), `SELECT encode(sha256(convert_to(string_agg(r.relname||':'||k.conname||':'||pg_get_constraintdef(k.oid),E'\n' ORDER BY r.relname,k.conname),'UTF8')),'hex') FROM pg_constraint k JOIN pg_class r ON r.oid=k.conrelid WHERE k.conrelid IN ('c_posts.thumbnails'::regclass,'c_posts.thumbnail_storage'::regclass) AND k.contype='c'`).Scan(&constraints)
		t.Log("restored constraints", constraints)
		rows, _ := f.l.cp.Query(context.Background(), `SELECT p.proname,encode(sha256(convert_to(replace(p.prosrc,chr(13),''),'UTF8')),'hex'),p.proconfig::text FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname='c_posts' AND p.proname IN ('account_thumbnail_storage','erase_source_thumbnails')`)
		for rows.Next() {
			var name, hash, cfg string
			rows.Scan(&name, &hash, &cfg)
			t.Log(name, hash, cfg)
		}
		rows.Close()
		t.Fatal("restored thumbnail integrity", e)
	}
	const executeCount = `SELECT count(*) FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace
 WHERE n.nspname='c_posts' AND p.proname IN ('account_thumbnail_storage','erase_source_thumbnails')
 AND has_function_privilege('hnuhole_c_runtime',p.oid,'EXECUTE')`
	if count(t, f.l.cp, executeCount) != 2 {
		t.Fatal("fixture did not reproduce default execute after ACL-free restore")
	}
	grants, e := os.ReadFile("../../../../infra/postgres/grant-post-images.sql")
	if e != nil {
		t.Fatal(e)
	}
	mustExec(t, f.l.cp, string(grants))
	if count(t, f.l.cp, executeCount) != 0 {
		t.Fatal("restored trigger function acquired runtime execute")
	}
	canonical, e := os.ReadFile("../../../../infra/postgres/grant-community.sql")
	if e != nil {
		t.Fatal(e)
	}
	// newLab applies raw migration bodies; this ledger marker is only for
	// canonical grant compatibility, not a claim of formal Goose acceptance.
	mustExec(t, f.l.cp, `CREATE TABLE IF NOT EXISTS public.goose_db_version(version_id bigint,is_applied boolean); INSERT INTO public.goose_db_version VALUES(43,true)`)
	mustExec(t, f.l.cp, string(canonical))
	// Match the actual startup order: general grants reset the schema, then
	// the optional image grant restores just its approved permissions.
	mustExec(t, f.l.cp, string(grants))
	runtime, e := pgxpool.New(context.Background(), os.Getenv("AUTHLAB_POSTS_RUNTIME_DSN"))
	if e != nil {
		t.Fatal(e)
	}
	defer runtime.Close()
	clone := *f.l.c
	clone.pool = runtime
	if e = clone.checkThumbnailStorageSchema(context.Background(), false); e != nil {
		t.Fatal("restored runtime thumbnail integrity", e)
	}
}
