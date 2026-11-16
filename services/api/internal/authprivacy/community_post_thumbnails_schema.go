package authprivacy

import "context"

// LF-normalized bodies from 0043, including their lifecycle source map.
const thumbnailAccountingDigest = "f0788011c13ad2d0dd697bbefd7c1952cb6562a4ae7434250fd125c0f1233e41"
const thumbnailErasureDigest = "b5f7c447c0cac32a7c2fd5042ef3a6ef441c655d8e4a89aa0cfd1c9851613d72"
const thumbnailConstraintsDigest = "1e1ba6bdabbbff2f08f757f27b01b95c12f1268e3ffaabed3b3d9357d4bcaa11"

// PG16 dump/restore flattens (length BETWEEN low AND high) AND hash-length
// into the same three-term AND. Both exact forms retain every predicate.
const thumbnailRestoredConstraintsDigest = "12998d55742c4d862912bda691555543b538b6fccc260a0624d56f8ae9bf095d"

func (c *Community) checkThumbnailStorageSchema(ctx context.Context, access bool) error {
	var ok bool
	err := c.pool.QueryRow(ctx, `SELECT
 NOT EXISTS(SELECT 1 FROM (VALUES
 ('c_posts.thumbnails','post_id','uuid'),('c_posts.thumbnails','content_version_id','uuid'),
 ('c_posts.thumbnails','variant','text'),('c_posts.thumbnails','encoder_version','text'),
 ('c_posts.thumbnails','source_binding','bytea'),('c_posts.thumbnails','payload','bytea'),
 ('c_posts.thumbnails','digest','bytea'),('c_posts.thumbnails','mime_type','text'),
 ('c_posts.thumbnails','width','integer'),('c_posts.thumbnails','height','integer'),
 ('c_posts.thumbnails','byte_length','integer'),('c_posts.thumbnails','created_at','timestamp with time zone'),
 ('c_posts.thumbnail_storage','singleton_id','integer'),('c_posts.thumbnail_storage','capacity_bytes','bigint'),
 ('c_posts.thumbnail_storage','used_bytes','bigint')) required(rel,col,typ)
 WHERE NOT EXISTS(SELECT 1 FROM pg_attribute a JOIN pg_class r ON r.oid=a.attrelid
 WHERE a.attrelid=to_regclass(required.rel) AND a.attname=required.col AND a.atttypid=to_regtype(required.typ)
 AND a.attnotnull AND NOT a.attisdropped AND r.relkind='r' AND r.relpersistence='p'))
 AND NOT EXISTS(SELECT 1 FROM (VALUES
 ('c_posts.thumbnails',ARRAY['post_id','content_version_id','variant','encoder_version']),
 ('c_posts.thumbnail_storage',ARRAY['singleton_id'])) required(rel,cols)
 WHERE NOT EXISTS(SELECT 1 FROM pg_constraint k WHERE k.conrelid=to_regclass(required.rel)
 AND k.contype='p' AND k.convalidated AND NOT k.condeferrable
 AND k.conkey=ARRAY(SELECT a.attnum FROM unnest(required.cols) WITH ORDINALITY col(name,pos)
 JOIN pg_attribute a ON a.attrelid=k.conrelid AND a.attname=col.name ORDER BY col.pos)))
 AND NOT EXISTS(SELECT 1 FROM (VALUES
 ('c_posts.account_thumbnail_storage()',$1),('c_posts.erase_source_thumbnails()',$2)) required(fn,digest)
 WHERE NOT EXISTS(SELECT 1 FROM pg_proc p WHERE p.oid=to_regprocedure(required.fn)
 AND p.prorettype='trigger'::regtype AND p.prosecdef AND p.proconfig=ARRAY['search_path=pg_catalog']
 AND encode(sha256(convert_to(replace(p.prosrc,chr(13),''),'UTF8')),'hex')=required.digest))
 AND NOT EXISTS(SELECT 1 FROM (VALUES
 ('c_posts.thumbnails','thumbnail_account_storage',13,'c_posts.account_thumbnail_storage()'),
 ('c_posts.posts','thumbnail_post_erase',17,'c_posts.erase_source_thumbnails()'),
 ('c_posts.images','thumbnail_image_erase',25,'c_posts.erase_source_thumbnails()'),
 ('c_posts.post_edit_media','thumbnail_edit_erase',25,'c_posts.erase_source_thumbnails()')) required(rel,name,mask,fn)
 WHERE NOT EXISTS(SELECT 1 FROM pg_trigger t WHERE t.tgrelid=to_regclass(required.rel)
 AND t.tgname=required.name AND t.tgenabled='O' AND NOT t.tgisinternal
 AND t.tgfoid=to_regprocedure(required.fn) AND t.tgtype=required.mask AND t.tgnargs=0
 AND t.tgqual IS NULL AND t.tgattr::text='' AND t.tgconstraint=0 AND NOT t.tgdeferrable))`,
		thumbnailAccountingDigest, thumbnailErasureDigest).Scan(&ok)
	if err != nil || !ok {
		return ErrAuthorizationUnavailable
	}
	// Constraint expressions are pinned independently of the table names.
	err = c.pool.QueryRow(ctx, `SELECT
 (SELECT count(*) FROM pg_constraint WHERE conrelid='c_posts.thumbnails'::regclass AND contype='c' AND convalidated)=8
 AND (SELECT count(*) FROM pg_constraint WHERE conrelid='c_posts.thumbnail_storage'::regclass AND contype='c' AND convalidated)=3
 AND (SELECT encode(sha256(convert_to(string_agg(r.relname||':'||k.conname||':'||pg_get_constraintdef(k.oid),E'\n' ORDER BY r.relname,k.conname),'UTF8')),'hex')
 FROM pg_constraint k JOIN pg_class r ON r.oid=k.conrelid
 WHERE k.conrelid IN ('c_posts.thumbnails'::regclass,'c_posts.thumbnail_storage'::regclass) AND k.contype='c')=ANY($2::text[])
 AND EXISTS(SELECT 1 FROM c_posts.thumbnail_storage WHERE singleton_id=1 AND used_bytes>=0 AND used_bytes<=capacity_bytes)
 AND (SELECT used_bytes=(SELECT COALESCE(sum(byte_length),0) FROM c_posts.thumbnails) FROM c_posts.thumbnail_storage WHERE singleton_id=1)
 AND EXISTS(SELECT 1 FROM pg_index i WHERE i.indexrelid=to_regclass('c_posts.thumbnail_oldest_idx')
 AND i.indrelid='c_posts.thumbnails'::regclass AND i.indisvalid AND i.indisready AND i.indpred IS NULL AND i.indexprs IS NULL
 AND i.indkey::text=(SELECT string_agg(a.attnum::text,' ' ORDER BY col.pos)
 FROM unnest(ARRAY['created_at','post_id','content_version_id','variant','encoder_version']) WITH ORDINALITY col(name,pos)
 JOIN pg_attribute a ON a.attrelid=i.indrelid AND a.attname=col.name AND NOT a.attisdropped))
 AND (current_user<>'hnuhole_c_runtime' OR (
 NOT has_function_privilege(current_user,'c_posts.account_thumbnail_storage()','EXECUTE')
 AND NOT has_function_privilege(current_user,'c_posts.erase_source_thumbnails()','EXECUTE')
 AND NOT has_table_privilege(current_user,'c_posts.thumbnail_storage','INSERT,UPDATE,DELETE,TRUNCATE,REFERENCES,TRIGGER')
 AND NOT has_any_column_privilege(current_user,'c_posts.thumbnail_storage','INSERT,UPDATE,REFERENCES')
 AND NOT has_table_privilege(current_user,'c_posts.thumbnails','UPDATE,TRUNCATE,REFERENCES,TRIGGER')
 AND NOT has_any_column_privilege(current_user,'c_posts.thumbnails','UPDATE,REFERENCES')
 AND has_table_privilege(current_user,'c_posts.thumbnails','SELECT') AND has_table_privilege(current_user,'c_posts.thumbnails','DELETE')
 AND (NOT $1 OR (has_table_privilege(current_user,'c_posts.thumbnails','SELECT')
 AND has_table_privilege(current_user,'c_posts.thumbnails','INSERT') AND has_table_privilege(current_user,'c_posts.thumbnails','DELETE')
 AND has_table_privilege(current_user,'c_posts.thumbnail_storage','SELECT')))))`, access, []string{thumbnailConstraintsDigest, thumbnailRestoredConstraintsDigest}).Scan(&ok)
	if err != nil || !ok {
		return ErrAuthorizationUnavailable
	}
	return nil
}
