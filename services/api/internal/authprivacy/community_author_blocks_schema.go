package authprivacy

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// A pre-0039 installation can still serve an owner fixture; the formal runtime
// additionally requires schema version 39. A partially applied migration or
// missing closure/predicate privileges must fail closed at startup.
func checkAuthorBlockSchema(ctx context.Context, pool *pgxpool.Pool) error {
	var present, tracker, runtime, valid bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass('c_posts.blocked_authors') IS NOT NULL,to_regclass('public.goose_db_version') IS NOT NULL,current_user='hnuhole_c_runtime'`).Scan(&present, &tracker, &runtime); err != nil {
		return errPostSchema
	}
	if !present {
		if tracker {
			if err := pool.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM public.goose_db_version WHERE version_id>=39 AND is_applied)`).Scan(&valid); err != nil || !valid {
				return errPostSchema
			}
		}
		return nil
	}
	if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_class WHERE oid=to_regclass('c_posts.blocked_authors') AND relkind='r' AND relpersistence='p')
 AND EXISTS(SELECT 1 FROM pg_constraint k WHERE k.conrelid=to_regclass('c_posts.blocked_authors') AND k.contype='p' AND NOT k.condeferrable AND k.convalidated
 AND k.conkey=ARRAY(SELECT a.attnum FROM unnest(ARRAY['block_id']) WITH ORDINALITY required(name,position) JOIN pg_attribute a ON a.attrelid=k.conrelid AND a.attname=required.name AND NOT a.attisdropped ORDER BY required.position))
 AND EXISTS(SELECT 1 FROM pg_constraint k WHERE k.conrelid=to_regclass('c_posts.blocked_authors') AND k.contype='u' AND NOT k.condeferrable AND k.convalidated
 AND k.conkey=ARRAY(SELECT a.attnum FROM unnest(ARRAY['account_id','author_account_id']) WITH ORDINALITY required(name,position) JOIN pg_attribute a ON a.attrelid=k.conrelid AND a.attname=required.name AND NOT a.attisdropped ORDER BY required.position))
 AND EXISTS(SELECT 1 FROM pg_constraint k WHERE k.conrelid=to_regclass('c_posts.blocked_authors') AND k.contype='c' AND k.convalidated AND position('account_id <> author_account_id' IN pg_get_constraintdef(k.oid))>0)
 AND EXISTS(SELECT 1 FROM pg_constraint k WHERE k.conrelid=to_regclass('c_posts.blocked_authors') AND k.contype='c' AND k.convalidated AND position('author_state' IN pg_get_constraintdef(k.oid))>0 AND position('ACTIVE' IN pg_get_constraintdef(k.oid))>0)
 AND NOT EXISTS(SELECT 1 FROM (VALUES ('block_id','uuid'),('account_id','uuid'),('author_account_id','uuid'),('author_state','text'),('nickname','text'),('avatar','text'),('short_code','text'),('created_at','timestamp with time zone')) AS required(name,typ)
 WHERE NOT EXISTS(SELECT 1 FROM pg_attribute a WHERE a.attrelid=to_regclass('c_posts.blocked_authors') AND a.attname=required.name AND a.atttypid=to_regtype(required.typ) AND a.attnotnull AND NOT a.attisdropped))
 AND to_regclass('c_posts.blocked_authors_author_idx') IS NOT NULL
 AND EXISTS(SELECT 1 FROM pg_trigger t JOIN pg_proc p ON p.oid=t.tgfoid WHERE t.tgrelid='c_auth.accounts'::regclass AND t.tgname='blocked_authors_account_closed' AND t.tgenabled='O' AND NOT t.tgisinternal AND t.tgtype=17
 AND t.tgattr::text=(SELECT attnum::text FROM pg_attribute WHERE attrelid=t.tgrelid AND attname='state' AND NOT attisdropped)
 AND p.oid=to_regprocedure('c_posts.clear_closed_account_author_blocks()') AND NOT p.prosecdef
 AND position('DELETE FROM c_posts.blocked_authors' IN p.prosrc)>0
 AND position('author_account_id = NEW.account_id' IN p.prosrc)>0)
 AND EXISTS(SELECT 1 FROM pg_proc p WHERE p.oid=to_regprocedure('c_posts.accounts_blocked(uuid,uuid)') AND p.prokind='f' AND p.prorettype='boolean'::regtype AND p.provolatile='s' AND NOT p.prosecdef
 AND position('b.account_id = viewer' IN p.prosrc)>0 AND position('b.account_id = author' IN p.prosrc)>0)`).Scan(&valid); err != nil || !valid {
		return errPostSchema
	}
	if runtime {
		if err := pool.QueryRow(ctx, `SELECT has_table_privilege(current_user,'c_posts.blocked_authors','SELECT') AND has_table_privilege(current_user,'c_posts.blocked_authors','INSERT') AND has_table_privilege(current_user,'c_posts.blocked_authors','DELETE')
 AND NOT has_table_privilege(current_user,'c_posts.blocked_authors','UPDATE,TRUNCATE,TRIGGER,REFERENCES')
 AND has_function_privilege(current_user,'c_posts.accounts_blocked(uuid,uuid)','EXECUTE')`).Scan(&valid); err != nil || !valid {
			return errPostSchema
		}
	}
	return nil
}
