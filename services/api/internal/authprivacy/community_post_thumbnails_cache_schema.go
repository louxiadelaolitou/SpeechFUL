package authprivacy

import "context"

// LF-normalized body from 0055. The trigger is intentionally separate from the
// 0043 lifecycle functions so older binaries and historical fingerprints keep
// working unchanged.
const thumbnailCacheOutboxDigest = "c709eae5525fb2dd8e9f0a7edd607be9a130e0e43438d6b46123f6b5a706a186"

// checkThumbnailCacheSchema fails closed before any request can use Redis. It
// verifies the transactional deletion outbox and the runtime's limited rights.
func (c *Community) checkThumbnailCacheSchema(ctx context.Context) error {
	var ok bool
	err := c.pool.QueryRow(ctx, `SELECT
 NOT EXISTS(SELECT 1 FROM (VALUES
 ('c_posts.thumbnail_cache_outbox','event_id','uuid',true),
 ('c_posts.thumbnail_cache_outbox','post_id','uuid',true),
 ('c_posts.thumbnail_cache_outbox','content_version_id','uuid',true),
 ('c_posts.thumbnail_cache_outbox','variant','text',true),
 ('c_posts.thumbnail_cache_outbox','encoder_version','text',true),
 ('c_posts.thumbnail_cache_outbox','source_binding','bytea',true),
 ('c_posts.thumbnail_cache_outbox','output_digest','bytea',true),
 ('c_posts.thumbnail_cache_outbox','created_at','timestamp with time zone',true),
 ('c_posts.thumbnail_cache_outbox','next_attempt_at','timestamp with time zone',true),
 ('c_posts.thumbnail_cache_outbox','attempts','integer',true),
 ('c_posts.thumbnail_cache_outbox','lease_token','uuid',false),
 ('c_posts.thumbnail_cache_outbox','lease_until','timestamp with time zone',false),
 ('c_posts.thumbnail_cache_outbox','completed_at','timestamp with time zone',false)) required(rel,col,typ,required_notnull)
 WHERE NOT EXISTS(SELECT 1 FROM pg_attribute a JOIN pg_class r ON r.oid=a.attrelid
 WHERE a.attrelid=to_regclass(required.rel) AND a.attname=required.col AND a.atttypid=to_regtype(required.typ)
 AND a.attnotnull=required.required_notnull AND NOT a.attisdropped AND r.relkind='r' AND r.relpersistence='p'))
 AND EXISTS(SELECT 1 FROM pg_constraint k WHERE k.conrelid=to_regclass('c_posts.thumbnail_cache_outbox')
 AND k.contype='p' AND k.convalidated AND NOT k.condeferrable
 AND k.conkey=ARRAY(SELECT a.attnum FROM pg_attribute a WHERE a.attrelid=k.conrelid AND a.attname='event_id'))
 AND (SELECT count(*) FROM pg_index i WHERE i.indexrelid=to_regclass('c_posts.thumbnail_cache_pending_idx')
 AND i.indrelid='c_posts.thumbnail_cache_outbox'::regclass AND i.indisvalid AND i.indisready
 AND NOT i.indisunique AND i.indpred IS NOT NULL AND i.indexprs IS NULL
 AND i.indkey::text=(SELECT string_agg(a.attnum::text,' ' ORDER BY col.pos)
 FROM unnest(ARRAY['next_attempt_at','event_id']) WITH ORDINALITY col(name,pos)
 JOIN pg_attribute a ON a.attrelid=i.indrelid AND a.attname=col.name AND NOT a.attisdropped))=1
 AND (SELECT count(*) FROM pg_index i WHERE i.indexrelid=to_regclass('c_posts.thumbnail_cache_completed_idx')
 AND i.indrelid='c_posts.thumbnail_cache_outbox'::regclass AND i.indisvalid AND i.indisready
 AND NOT i.indisunique AND i.indpred IS NOT NULL AND i.indexprs IS NULL
 AND i.indkey::text=(SELECT a.attnum::text FROM pg_attribute a
 WHERE a.attrelid=i.indrelid AND a.attname='completed_at' AND NOT a.attisdropped))=1
 AND NOT EXISTS(SELECT 1 FROM (VALUES
 ('c_posts.record_thumbnail_cache_erasure()',$1)) required(fn,digest)
 WHERE NOT EXISTS(SELECT 1 FROM pg_proc p WHERE p.oid=to_regprocedure(required.fn)
 AND p.prorettype='trigger'::regtype AND p.prosecdef AND p.proconfig=ARRAY['search_path=pg_catalog']
 AND encode(sha256(convert_to(replace(p.prosrc,chr(13),''),'UTF8')),'hex')=required.digest))
 AND NOT EXISTS(SELECT 1 FROM (VALUES
 ('c_posts.thumbnails','thumbnail_cache_erase',9,'c_posts.record_thumbnail_cache_erasure()')) required(rel,name,mask,fn)
 WHERE NOT EXISTS(SELECT 1 FROM pg_trigger t WHERE t.tgrelid=to_regclass(required.rel)
 AND t.tgname=required.name AND t.tgenabled='O' AND NOT t.tgisinternal
 AND t.tgfoid=to_regprocedure(required.fn) AND t.tgtype=required.mask AND t.tgnargs=0
 AND t.tgqual IS NULL AND t.tgattr::text='' AND t.tgconstraint=0 AND NOT t.tgdeferrable))
 AND (current_user<>'hnuhole_c_runtime' OR (
 has_table_privilege(current_user,'c_posts.thumbnail_cache_outbox','SELECT')
 AND has_table_privilege(current_user,'c_posts.thumbnail_cache_outbox','UPDATE')
 AND has_table_privilege(current_user,'c_posts.thumbnail_cache_outbox','DELETE')
 AND NOT has_table_privilege(current_user,'c_posts.thumbnail_cache_outbox','INSERT,TRUNCATE,REFERENCES,TRIGGER')
 AND NOT has_function_privilege(current_user,'c_posts.record_thumbnail_cache_erasure()','EXECUTE')))`,
		thumbnailCacheOutboxDigest).Scan(&ok)
	if err != nil || !ok {
		return ErrAuthorizationUnavailable
	}
	return nil
}
