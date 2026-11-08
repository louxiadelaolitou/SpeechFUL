package authprivacy

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
)

// A pre-0036 installation can continue serving old clients. A partially applied
// migration or missing closure privileges must fail closed at startup.
func checkHiddenPostSchema(ctx context.Context, pool *pgxpool.Pool) error {
	var present, tracker, runtime, valid bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass('c_posts.hidden_posts') IS NOT NULL,to_regclass('public.goose_db_version') IS NOT NULL,current_user='hnuhole_c_runtime'`).Scan(&present, &tracker, &runtime); err != nil {
		return errPostSchema
	}
	if !present {
		if tracker {
			if err := pool.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM public.goose_db_version WHERE version_id>=36 AND is_applied)`).Scan(&valid); err != nil || !valid {
				return errPostSchema
			}
		}
		return nil
	}
	if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_class WHERE oid=to_regclass('c_posts.hidden_posts') AND relkind='r' AND relpersistence='p')
 AND EXISTS(SELECT 1 FROM pg_constraint k WHERE k.conrelid=to_regclass('c_posts.hidden_posts') AND k.contype='p' AND NOT k.condeferrable AND k.convalidated
 AND k.conkey=ARRAY(SELECT a.attnum FROM unnest(ARRAY['account_id','post_id']) WITH ORDINALITY required(name,position) JOIN pg_attribute a ON a.attrelid=k.conrelid AND a.attname=required.name AND NOT a.attisdropped ORDER BY required.position))
 AND NOT EXISTS(SELECT 1 FROM (VALUES ('account_id','uuid'),('post_id','uuid'),('hidden','boolean'),('version','uuid')) AS required(name,typ)
 WHERE NOT EXISTS(SELECT 1 FROM pg_attribute a WHERE a.attrelid=to_regclass('c_posts.hidden_posts') AND a.attname=required.name AND a.atttypid=to_regtype(required.typ) AND a.attnotnull AND NOT a.attisdropped))
 AND EXISTS(SELECT 1 FROM pg_trigger t JOIN pg_proc p ON p.oid=t.tgfoid WHERE t.tgrelid='c_auth.accounts'::regclass AND t.tgname='hidden_posts_account_closed' AND t.tgenabled='O' AND NOT t.tgisinternal AND t.tgtype=17
 AND t.tgattr::text=(SELECT attnum::text FROM pg_attribute WHERE attrelid=t.tgrelid AND attname='state' AND NOT attisdropped)
 AND p.oid=to_regprocedure('c_posts.clear_closed_account_hidden_posts()') AND NOT p.prosecdef
 AND position('DELETE FROM c_posts.hidden_posts WHERE account_id = NEW.account_id' IN p.prosrc)>0)`).Scan(&valid); err != nil || !valid {
		return errPostSchema
	}
	if runtime {
		if err := pool.QueryRow(ctx, `SELECT has_table_privilege(current_user,'c_posts.hidden_posts','SELECT') AND has_table_privilege(current_user,'c_posts.hidden_posts','INSERT') AND has_table_privilege(current_user,'c_posts.hidden_posts','DELETE')
 AND has_column_privilege(current_user,'c_posts.hidden_posts','hidden','UPDATE') AND has_column_privilege(current_user,'c_posts.hidden_posts','version','UPDATE')
 AND NOT has_column_privilege(current_user,'c_posts.hidden_posts','account_id','UPDATE') AND NOT has_column_privilege(current_user,'c_posts.hidden_posts','post_id','UPDATE')
 AND NOT has_table_privilege(current_user,'c_posts.hidden_posts','UPDATE,TRUNCATE,TRIGGER,REFERENCES')`).Scan(&valid); err != nil || !valid {
			return errPostSchema
		}
	}
	return nil
}
