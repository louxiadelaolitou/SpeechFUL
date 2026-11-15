package authprivacy

import "context"

// LF-normalized prosrc installed by 0042, including its complete source map.
const preparedFeedReadVersionFunctionDigest = "2c9f712d44c990947cc47563ab8513d3478ad65fd23cad40889ab7f091d7c1ce"

func (c *Community) checkPreparedFeedReadSchema(ctx context.Context) error {
	// Feed and media share M1's identity/account/block facts. Optional read
	// switches alone call these checks; the unoptimized minimum stays at 39.
	if err := c.checkPreparedCommentReadSchema(ctx); err != nil {
		return err
	}
	var ok bool
	if err := c.pool.QueryRow(ctx, `SELECT
 NOT EXISTS (SELECT 1 FROM (VALUES
  ('c_posts.read_feed_scope_versions','channel_id','uuid'),
  ('c_posts.read_feed_scope_versions','membership_revision','bigint'),
  ('c_posts.read_feed_scope_versions','publication_revision','bigint'),
  ('c_posts.read_feed_post_versions','post_id','uuid'),
  ('c_posts.read_feed_post_versions','projection_revision','bigint')
 ) required(rel,col,typ) WHERE NOT EXISTS (
  SELECT 1 FROM pg_attribute a JOIN pg_class r ON r.oid=a.attrelid
  WHERE a.attrelid=to_regclass(required.rel) AND a.attname=required.col
    AND a.atttypid=to_regtype(required.typ) AND a.attnotnull AND NOT a.attisdropped
    AND r.relkind='r' AND r.relpersistence='p'))
 AND NOT EXISTS (SELECT 1 FROM (VALUES
  ('c_posts.read_feed_scope_versions','channel_id'),('c_posts.read_feed_post_versions','post_id')
 ) required(rel,col) WHERE NOT EXISTS (
  SELECT 1 FROM pg_constraint k JOIN pg_attribute a ON a.attrelid=k.conrelid AND a.attname=required.col
  WHERE k.conrelid=to_regclass(required.rel) AND k.contype='p' AND k.convalidated AND NOT k.condeferrable
    AND k.conkey=ARRAY[a.attnum]))
 AND NOT EXISTS (SELECT 1 FROM pg_constraint WHERE contype='f' AND conrelid IN (
  to_regclass('c_posts.read_feed_scope_versions'),to_regclass('c_posts.read_feed_post_versions')))
 AND NOT EXISTS (SELECT 1 FROM (VALUES
  ('c_posts.read_feed_scope_versions','membership_revision'),
  ('c_posts.read_feed_scope_versions','publication_revision'),
  ('c_posts.read_feed_post_versions','projection_revision')
 ) required(rel,col) WHERE NOT EXISTS (
  SELECT 1 FROM pg_constraint k WHERE k.conrelid=to_regclass(required.rel) AND k.contype='c'
    AND k.convalidated AND pg_get_expr(k.conbin,k.conrelid)=format('(%I >= 0)',required.col)))
 AND EXISTS (SELECT 1 FROM pg_proc p WHERE p.oid=to_regprocedure('c_posts.advance_feed_read_versions()')
  AND p.prorettype='trigger'::regtype AND p.prosecdef
  AND p.proconfig=ARRAY['search_path=pg_catalog']
  AND encode(sha256(convert_to(replace(p.prosrc,chr(13),''),'UTF8')),'hex')=$1)
 AND NOT EXISTS (SELECT 1 FROM (VALUES
  ('public.channels','prepared_feed_channel',5,ARRAY[]::text[]),
  ('c_posts.posts','prepared_feed_post',21,ARRAY['owner_account_id','channel_id','visibility','moderation_hidden',
    'published_attempt_version','published_at','publication_ordinal','edit_revision','edited_at','edited_title','edited_body','edited_image_ids','tags']),
  ('c_posts.attempt_contents','prepared_feed_content',17,ARRAY['title','body','erased_at','image_count']),
  ('c_posts.images','prepared_feed_image',21,ARRAY['payload','digest','byte_length','mime_type','width','height',
    'display_width','display_height','frames','erased_at','verified_at','position','content_version_id','cycle_duration_ms','max_frame_duration_ms']),
  ('c_posts.post_edit_media','prepared_feed_edit_media',29,ARRAY[]::text[])
 ) required(rel,name,mask,cols) WHERE NOT EXISTS (SELECT 1 FROM pg_trigger t
  WHERE t.tgrelid=to_regclass(required.rel) AND t.tgname=required.name AND t.tgenabled='O'
    AND NOT t.tgisinternal AND t.tgfoid=to_regprocedure('c_posts.advance_feed_read_versions()')
    AND t.tgtype=required.mask AND t.tgnargs=0 AND t.tgqual IS NULL
    AND t.tgconstraint=0 AND NOT t.tgdeferrable AND NOT t.tginitdeferred
    AND t.tgattr::text=(SELECT COALESCE(string_agg(a.attnum::text,' ' ORDER BY col.pos),'')
     FROM unnest(required.cols) WITH ORDINALITY col(name,pos)
     JOIN pg_attribute a ON a.attrelid=t.tgrelid AND a.attname=col.name AND NOT a.attisdropped)))`, preparedFeedReadVersionFunctionDigest).
		Scan(&ok); err != nil || !ok {
		return errPostSchema
	}
	if err := c.pool.QueryRow(ctx, `SELECT
 NOT EXISTS (SELECT 1 FROM public.channels c LEFT JOIN c_posts.read_feed_scope_versions v ON v.channel_id=c.id WHERE v.channel_id IS NULL)
 AND NOT EXISTS (SELECT 1 FROM c_posts.posts p LEFT JOIN c_posts.read_feed_post_versions v USING(post_id) WHERE v.post_id IS NULL)
 AND (current_user<>'hnuhole_c_runtime' OR (
  NOT has_function_privilege(current_user,'c_posts.advance_feed_read_versions()','EXECUTE')
  AND NOT EXISTS (SELECT 1 FROM unnest(ARRAY['c_posts.read_feed_scope_versions','c_posts.read_feed_post_versions']) rel
   WHERE NOT has_table_privilege(current_user,rel,'SELECT')
      OR has_table_privilege(current_user,rel,'INSERT,UPDATE,DELETE,TRUNCATE,REFERENCES,TRIGGER')
      OR has_any_column_privilege(current_user,rel,'UPDATE,INSERT,REFERENCES'))))`).Scan(&ok); err != nil || !ok {
		return errPostSchema
	}
	return nil
}

func (c *Community) checkPreparedMediaReadSchema(ctx context.Context) error {
	// Post manifests use 0042's selected-media projection; comment originals
	// also require 0041's post/root/comment/identity and account dependencies.
	return c.checkPreparedFeedReadSchema(ctx)
}
