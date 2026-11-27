package authprivacy

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNotificationInboxGooseMigrationPostgres(t *testing.T) {
	tool := os.Getenv("COMMENT_TEXT_GOOSE")
	if tool == "" {
		t.Skip("owned Goose fixture absent")
	}
	for _, mode := range []string{"upgrade", "fresh"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			dsn := os.Getenv("NOTIFICATION_" + strings.ToUpper(mode) + "_DSN")
			if dsn == "" {
				t.Skip("owned notification migration fixture absent")
			}
			cfg, e := pgxpool.ParseConfig(dsn)
			if e != nil || os.Getenv("AUTHLAB_ALLOW_SCHEMA_RESET") != "1" || !strings.HasPrefix(os.Getenv("AUTHLAB_RUNTIME_TAG"), "hnuhole_authlab_") || cfg.ConnConfig.RuntimeParams["application_name"] != os.Getenv("AUTHLAB_RUNTIME_TAG") {
				t.Fatal("owned fixture marker required")
			}
			pool, e := pgxpool.NewWithConfig(ctx, cfg)
			if e != nil {
				t.Fatal(e)
			}
			defer pool.Close()
			var safe bool
			e = pool.QueryRow(ctx, `SELECT current_database()=$1 AND current_user='hnuhole_c' AND NOT (SELECT rolsuper FROM pg_roles WHERE rolname=current_user) AND inet_server_addr() IS NULL AND to_regnamespace('c_auth') IS NULL AND to_regclass('public.goose_db_version') IS NULL`, "hnuhole_notifications_"+mode).Scan(&safe)
			if e != nil || !safe {
				t.Fatal("dedicated empty private-socket database required", e)
			}
			dir, e := filepath.Abs("../../migrations")
			if e != nil {
				t.Fatal(e)
			}
			run := func(args ...string) error {
				return exec.Command(tool, append([]string{"-dir", dir, "postgres", dsn}, args...)...).Run()
			}
			if mode == "upgrade" {
				if e = run("up-to", "27"); e != nil {
					t.Fatal("main migration chain", e)
				}
				mustExec(t, pool, `INSERT INTO c_auth.accounts(account_id,platform_number,username,password_hash,password_salt,password_params_version) VALUES('d2c920f5-5563-452d-901c-be133844b0fa','001829','notice_upgrade',decode(repeat('11',32),'hex'),decode(repeat('22',16),'hex'),1)`)
			}
			if e = run("up-to", "28"); e != nil {
				t.Fatal("notification migration", e)
			}
			if count(t, pool, `SELECT count(*) FROM public.goose_db_version WHERE is_applied`) != 29 {
				t.Fatal("incomplete migration chain")
			}
			if mode == "upgrade" && count(t, pool, `SELECT count(*) FROM c_auth.accounts WHERE username='notice_upgrade'`) != 1 {
				t.Fatal("lost historical account")
			}
			if e = checkInboxLifecycleSchema(ctx, pool); e != nil {
				t.Fatal(e)
			}
			if mode == "upgrade" {
				// One historical like record with two live events must be
				// backfilled to the newest event version by 0050.
				mustExec(t, pool, `INSERT INTO c_auth.accounts(account_id,platform_number,username,password_hash,password_salt,password_params_version) VALUES('d2c920f5-5563-452d-901c-be133844b0fb','001830','notice_liker',decode(repeat('11',32),'hex'),decode(repeat('22',16),'hex'),1),('d2c920f5-5563-452d-901c-be133844b0fc','001831','notice_liker_two',decode(repeat('11',32),'hex'),decode(repeat('22',16),'hex'),1);
INSERT INTO public.identity_account_state VALUES('d2c920f5-5563-452d-901c-be133844b0fa',1,now());
INSERT INTO public.community_identities(identity_id,account_id,nickname,avatar,is_original,created_at) VALUES('11111111-1111-4111-8111-111111111111','d2c920f5-5563-452d-901c-be133844b0fa','notice owner','default-v1',true,now());
INSERT INTO c_posts.posts(post_id,owner_account_id,channel_id) SELECT '22222222-2222-4222-8222-222222222222','d2c920f5-5563-452d-901c-be133844b0fa',id FROM public.channels ORDER BY id LIMIT 1;
INSERT INTO c_posts.post_identity_bindings(account_id,post_id,identity_id,bound_at) VALUES('d2c920f5-5563-452d-901c-be133844b0fa','22222222-2222-4222-8222-222222222222','11111111-1111-4111-8111-111111111111',now());
INSERT INTO c_posts.publication_tasks(task_id,post_id,owner_account_id,identity_id,channel_id,latest_attempt_version) SELECT '44444444-4444-4444-8444-444444444444',post_id,owner_account_id,'11111111-1111-4111-8111-111111111111',channel_id,1 FROM c_posts.posts WHERE post_id='22222222-2222-4222-8222-222222222222';
INSERT INTO c_posts.publication_attempts(task_id,version,accepted_stop_generation,state,accepted_at,acceptance_ordinal) VALUES('44444444-4444-4444-8444-444444444444',1,0,'ACCEPTED',now(),nextval('c_posts.acceptance_ordinal_seq'));
INSERT INTO c_posts.attempt_contents(task_id,version,title,body,request_digest) VALUES('44444444-4444-4444-8444-444444444444',1,'notice source','notice content',decode(repeat('cd',32),'hex'));
UPDATE c_posts.publication_attempts SET state='PUBLISHED',terminal_at=now() WHERE task_id='44444444-4444-4444-8444-444444444444';
UPDATE c_posts.posts SET visibility='PUBLISHED',published_attempt_version=1,published_at=now(),publication_ordinal=nextval('c_posts.publication_ordinal_seq') WHERE post_id='22222222-2222-4222-8222-222222222222';
INSERT INTO c_inbox.records(notification_id,account_id,identity_id,post_id,content_id,kind,version,created_at,updated_at) VALUES('33333333-3333-4333-8333-333333333333','d2c920f5-5563-452d-901c-be133844b0fa','11111111-1111-4111-8111-111111111111','22222222-2222-4222-8222-222222222222','22222222-2222-4222-8222-222222222222','LIKE_POST',2,now(),now());
INSERT INTO c_inbox.like_events VALUES('33333333-3333-4333-8333-333333333333','d2c920f5-5563-452d-901c-be133844b0fb',1),('33333333-3333-4333-8333-333333333333','d2c920f5-5563-452d-901c-be133844b0fc',2)`)
			}
			if e = run("up"); e != nil {
				t.Fatal("unread index migration", e)
			}
			var latest int64
			if e = pool.QueryRow(ctx, `SELECT latest_like_version FROM c_inbox.records WHERE notification_id='33333333-3333-4333-8333-333333333333'`).Scan(&latest); mode == "upgrade" && (e != nil || latest != 2) {
				t.Fatalf("like version backfill: %d %v", latest, e)
			}
			var ok bool
			if e = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='c_inbox' AND table_name='records' AND column_name='latest_like_version' AND is_nullable='NO' AND column_default='0')`).Scan(&ok); e != nil || !ok {
				t.Fatal("denormalized like version column absent", e)
			}
			if count(t, pool, `SELECT count(*) FROM pg_indexes WHERE schemaname='c_inbox' AND indexname='inbox_unread' AND indexdef LIKE '%latest_like_version%'`) != 1 {
				t.Fatal("unread-only page index absent")
			}
			if e = run("down-to", "49"); e == nil {
				t.Fatal("recipient facts rolled back")
			}
			if count(t, pool, `SELECT max(version_id) FROM public.goose_db_version WHERE is_applied`) != 58 {
				t.Fatal("failed down changed ledger")
			}
		})
	}
}
