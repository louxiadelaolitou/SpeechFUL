package authprivacy

import "context"

// Digests bind the actual LF-normalized function bodies, not only signatures.
// Filled from migration 0049 and checked independently against PostgreSQL.
var postSearchDocumentFunctionDigests = map[string]string{
	"current_post_search_documents(uuid)":     "f990d5993abe3da683c137c00a71d444a951565211797a16e9e3c8beddacfa2b",
	"refresh_post_search_document(uuid)":      "a1608492dee81fc965777b21d6d98942420b4358bbb92aa7a37d85a1203f77e4",
	"sync_post_search_document()":             "ac1d548a350c2a4da45a99cfa5d7ebb2023d717c8c49ace245e6e8310873db31",
	"backfill_post_search_documents(integer)": "311bfb0d72b677c97844e99ae9af59bd1f01897c8f4cc8111d79b921d536a454",
	"validate_post_search_documents()":        "2ee7d30b59015346ad133993adc8fe95327c6282286db50be20d80149fc8313d",
}

func (c *Community) checkPostSearchDocumentSchema(ctx context.Context) error {
	if len(postSearchDocumentFunctionDigests) != 5 {
		return errPostSchema
	}
	var ok bool
	if err := c.pool.QueryRow(ctx, `SELECT
 EXISTS(SELECT 1 FROM pg_extension e JOIN pg_namespace n ON n.oid=e.extnamespace WHERE e.extname='pg_trgm' AND n.nspname='public')
 AND NOT EXISTS(SELECT 1 FROM (VALUES
  ('post_search_documents','post_id','uuid'),('post_search_documents','channel_id','uuid'),
  ('post_search_documents','published_at','timestamp with time zone'),('post_search_documents','publication_ordinal','bigint'),
  ('post_search_documents','source_version','integer'),('post_search_documents','content_revision','uuid'),
  ('post_search_documents','title_norm','text'),('post_search_documents','body_norm','text'),
  ('post_search_documents','tags_norm','text[]'),('post_search_documents','search_key','text'),
  ('post_search_document_state','singleton','boolean'),('post_search_document_state','ready','boolean'),
  ('post_search_document_state','normalization_version','text'),
  ('post_search_document_state','last_post_id','uuid')
 ) req(rel,col,typ) WHERE NOT EXISTS(SELECT 1 FROM pg_attribute a JOIN pg_class r ON r.oid=a.attrelid
   WHERE a.attrelid=to_regclass('c_posts.'||req.rel) AND a.attname=req.col AND a.atttypid=to_regtype(req.typ)
    AND a.attnotnull AND NOT a.attisdropped AND r.relkind='r' AND r.relpersistence='p'))
 AND NOT EXISTS(SELECT 1 FROM (VALUES ('post_search_documents','post_id'),('post_search_document_state','singleton')) req(rel,col)
  WHERE NOT EXISTS(SELECT 1 FROM pg_constraint k JOIN pg_attribute a ON a.attrelid=k.conrelid AND a.attname=req.col
   WHERE k.conrelid=to_regclass('c_posts.'||req.rel) AND k.contype='p' AND k.convalidated AND NOT k.condeferrable AND k.conkey=ARRAY[a.attnum]))
 AND NOT EXISTS(SELECT 1 FROM pg_attribute WHERE attrelid=to_regclass('c_posts.post_search_documents')
  AND attname IN ('title_norm','body_norm','tags_norm','search_key') AND attcollation<>'pg_catalog."C"'::regcollation)
 AND EXISTS(SELECT 1 FROM pg_index i JOIN pg_class r ON r.oid=i.indexrelid JOIN pg_am am ON am.oid=r.relam
  JOIN pg_opclass op ON op.oid=i.indclass[0] JOIN pg_namespace n ON n.oid=op.opcnamespace
  WHERE i.indexrelid=to_regclass('c_posts.post_search_documents_trgm_idx') AND i.indrelid=to_regclass('c_posts.post_search_documents')
   AND i.indisvalid AND i.indisready AND i.indpred IS NULL AND i.indexprs IS NULL AND i.indnkeyatts=1 AND i.indnatts=1
   AND i.indkey[0]=(SELECT attnum FROM pg_attribute WHERE attrelid=i.indrelid AND attname='search_key')
   AND am.amname='gin' AND op.opcname='gin_trgm_ops' AND n.nspname='public')
 AND NOT EXISTS(SELECT 1 FROM (VALUES
  ('posts','post_search_document_post',29,ARRAY['visibility','moderation_hidden','published_attempt_version','edit_revision','edited_title','edited_body','tags','channel_id','published_at','publication_ordinal']),
  ('attempt_contents','post_search_document_content',21,ARRAY['title','body','erased_at'])
 ) req(rel,name,mask,cols) WHERE NOT EXISTS(SELECT 1 FROM pg_trigger t WHERE t.tgrelid=to_regclass('c_posts.'||req.rel)
  AND t.tgname=req.name AND t.tgenabled='O' AND NOT t.tgisinternal AND t.tgtype=req.mask
  AND t.tgfoid=to_regprocedure('c_posts.sync_post_search_document()') AND t.tgnargs=0 AND t.tgqual IS NULL
  AND t.tgconstraint=0 AND NOT t.tgdeferrable AND NOT t.tginitdeferred
  AND t.tgattr::text=(SELECT COALESCE(string_agg(a.attnum::text,' ' ORDER BY col.pos),'')
   FROM unnest(req.cols) WITH ORDINALITY col(name,pos)
   JOIN pg_attribute a ON a.attrelid=t.tgrelid AND a.attname=col.name AND NOT a.attisdropped)))`).Scan(&ok); err != nil || !ok {
		return errPostSchema
	}
	for name, digest := range postSearchDocumentFunctionDigests {
		if err := c.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_proc p WHERE p.oid=to_regprocedure('c_posts.'||$1)
 AND p.prosecdef=($1='sync_post_search_document()') AND p.proconfig=ARRAY['search_path=pg_catalog']
 AND p.provolatile::text=CASE WHEN $1='current_post_search_documents(uuid)' THEN 's' ELSE 'v' END
 AND encode(sha256(convert_to(replace(p.prosrc,chr(13),''),'UTF8')),'hex')=$2)`, name, digest).Scan(&ok); err != nil || !ok {
			return errPostSchema
		}
	}
	if err := c.pool.QueryRow(ctx, `SELECT count(*)=1 AND bool_and(singleton AND ready AND normalization_version=pg_collation_actual_version('pg_catalog."und-x-icu"'::regcollation)) FROM c_posts.post_search_document_state`).Scan(&ok); err != nil || !ok {
		return errPostSchema
	}
	if err := c.pool.QueryRow(ctx, `SELECT current_user<>'hnuhole_c_runtime' OR (
 has_table_privilege(current_user,'c_posts.post_search_documents','SELECT')
 AND has_table_privilege(current_user,'c_posts.post_search_document_state','SELECT')
 AND NOT EXISTS(SELECT 1 FROM unnest(ARRAY['c_posts.post_search_documents','c_posts.post_search_document_state']) rel
   WHERE has_table_privilege(current_user,rel,'INSERT,UPDATE,DELETE,TRUNCATE,TRIGGER')
    OR has_any_column_privilege(current_user,rel,'INSERT,UPDATE'))
 AND NOT EXISTS(SELECT 1 FROM unnest(ARRAY['c_posts.refresh_post_search_document(uuid)','c_posts.sync_post_search_document()',
  'c_posts.backfill_post_search_documents(integer)','c_posts.validate_post_search_documents()',
  'c_posts.current_post_search_documents(uuid)']) f
  WHERE has_function_privilege(current_user,f,'EXECUTE')))`).Scan(&ok); err != nil || !ok {
		return errPostSchema
	}
	return nil
}
