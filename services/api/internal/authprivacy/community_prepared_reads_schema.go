package authprivacy

import "context"

// SHA-256 of the LF-normalized prosrc installed by 0041. A same-signature
// no-op trigger must not pass startup after an incomplete schema restore.
const preparedCommentReadVersionFunctionDigest = "11e5f1796150f174063e5118aa78e610bffee6314968fb087f0fb61079f24f58"

// Only explicitly enabled prepared reads require 0041. The original path can
// still run while an operator rolls the optional read optimization back.
func (c *Community) checkPreparedCommentReadSchema(ctx context.Context) error {
	var ok bool
	if err := c.pool.QueryRow(ctx, `SELECT
 NOT EXISTS (SELECT 1 FROM (VALUES
  ('c_comments.read_scope_versions','post_id','uuid'),
  ('c_comments.read_scope_versions','root_id','uuid'),
  ('c_comments.read_scope_versions','membership_revision','bigint'),
  ('c_comments.read_scope_versions','order_revision','bigint'),
  ('c_comments.read_post_versions','post_id','uuid'),
  ('c_comments.read_post_versions','visibility_revision','bigint'),
  ('c_comments.read_post_versions','count_revision','bigint'),
  ('c_comments.read_comment_versions','comment_id','uuid'),
  ('c_comments.read_comment_versions','projection_revision','bigint'),
  ('c_comments.read_comment_versions','like_count','bigint'),
  ('c_posts.read_identity_versions','identity_id','uuid'),
  ('c_posts.read_identity_versions','projection_revision','bigint'),
  ('c_posts.read_account_versions','account_id','uuid'),
  ('c_posts.read_account_versions','projection_revision','bigint'),
  ('c_posts.read_account_versions','visibility_revision','bigint')
 ) required(rel,col,typ) WHERE NOT EXISTS (
  SELECT 1 FROM pg_attribute a JOIN pg_class r ON r.oid=a.attrelid
  WHERE a.attrelid=to_regclass(required.rel) AND a.attname=required.col
    AND a.atttypid=to_regtype(required.typ) AND a.attnotnull AND NOT a.attisdropped
    AND r.relkind='r' AND r.relpersistence='p'))
 AND NOT EXISTS (SELECT 1 FROM (VALUES
  ('c_comments.read_scope_versions',ARRAY['post_id','root_id']),
  ('c_comments.read_post_versions',ARRAY['post_id']),
  ('c_comments.read_comment_versions',ARRAY['comment_id']),
  ('c_posts.read_identity_versions',ARRAY['identity_id']),
  ('c_posts.read_account_versions',ARRAY['account_id'])
 ) required(rel,cols) WHERE NOT EXISTS (
  SELECT 1 FROM pg_constraint k WHERE k.conrelid=to_regclass(required.rel)
    AND k.contype='p' AND k.convalidated AND NOT k.condeferrable
    AND k.conkey=ARRAY(SELECT a.attnum FROM unnest(required.cols) WITH ORDINALITY col(name,pos)
      JOIN pg_attribute a ON a.attrelid=k.conrelid AND a.attname=col.name AND NOT a.attisdropped
      ORDER BY col.pos)))
 AND NOT EXISTS (SELECT 1 FROM pg_constraint WHERE contype='f' AND conrelid IN (
  to_regclass('c_comments.read_scope_versions'),to_regclass('c_comments.read_post_versions'),
  to_regclass('c_comments.read_comment_versions'),to_regclass('c_posts.read_identity_versions'),
  to_regclass('c_posts.read_account_versions')))
 AND EXISTS (SELECT 1 FROM pg_proc p WHERE p.oid=to_regprocedure('c_comments.advance_read_versions()')
  AND p.prorettype='trigger'::regtype AND p.prosecdef
  AND p.proconfig=ARRAY['search_path=pg_catalog']
  AND encode(sha256(convert_to(replace(p.prosrc,chr(13),''),'UTF8')),'hex')=$1)
 AND EXISTS (SELECT 1 FROM pg_index i WHERE i.indexrelid=to_regclass('c_comments.comment_snapshot_owner_expiry_idx')
  AND i.indrelid=to_regclass('c_comments.snapshots') AND i.indisvalid AND i.indisready
  AND i.indpred IS NULL AND i.indexprs IS NULL
  AND i.indkey::text=(SELECT string_agg(a.attnum::text,' ' ORDER BY col.pos)
   FROM unnest(ARRAY['owner_account_id','expires_at']) WITH ORDINALITY col(name,pos)
   JOIN pg_attribute a ON a.attrelid=i.indrelid AND a.attname=col.name AND NOT a.attisdropped))
 AND NOT EXISTS (SELECT 1 FROM (VALUES
  ('c_posts.posts','prepared_read_post',21,ARRAY['visibility','moderation_hidden']),
  ('c_comments.tasks','prepared_read_task',21,ARRAY['deleted_at','moderation_hidden']),
  ('c_comments.attempts','prepared_read_attempt',21,ARRAY['state','body','terminal_at','image']),
  ('c_comments.images','prepared_read_image',21,ARRAY['payload','metadata','content_version_id','erased_at']),
  ('c_comments.likes','prepared_read_like',13,ARRAY[]::text[]),
  ('public.community_identities','prepared_read_identity',21,ARRAY['nickname','avatar','deleted_at','account_id']),
  ('c_auth.accounts','prepared_read_account',21,ARRAY['state']),
  ('c_posts.blocked_authors','prepared_read_block',13,ARRAY[]::text[])
 ) required(rel,name,mask,cols) WHERE NOT EXISTS (SELECT 1 FROM pg_trigger t
  WHERE t.tgrelid=to_regclass(required.rel) AND t.tgname=required.name AND t.tgenabled='O'
    AND NOT t.tgisinternal AND t.tgfoid=to_regprocedure('c_comments.advance_read_versions()')
    AND t.tgtype=required.mask AND t.tgnargs=0
    AND t.tgqual IS NULL AND t.tgconstraint=0 AND NOT t.tgdeferrable AND NOT t.tginitdeferred
    AND t.tgattr::text=(SELECT COALESCE(string_agg(a.attnum::text,' ' ORDER BY col.pos),'')
     FROM unnest(required.cols) WITH ORDINALITY col(name,pos)
     JOIN pg_attribute a ON a.attrelid=t.tgrelid AND a.attname=col.name AND NOT a.attisdropped)))`, preparedCommentReadVersionFunctionDigest).Scan(&ok); err != nil || !ok {
		return errCommentSchema
	}
	if err := c.pool.QueryRow(ctx, `SELECT current_user<>'hnuhole_c_runtime' OR (
 NOT has_function_privilege(current_user,'c_comments.advance_read_versions()','EXECUTE')
 AND NOT EXISTS (SELECT 1 FROM unnest(ARRAY[
  'c_comments.read_scope_versions','c_comments.read_post_versions','c_comments.read_comment_versions',
  'c_posts.read_identity_versions','c_posts.read_account_versions']) rel
 WHERE NOT has_table_privilege(current_user,rel,'SELECT')
    OR has_table_privilege(current_user,rel,'INSERT,UPDATE,DELETE,TRUNCATE,REFERENCES,TRIGGER')
    OR has_any_column_privilege(current_user,rel,'UPDATE,INSERT,REFERENCES')))`).Scan(&ok); err != nil || !ok {
		return errCommentSchema
	}
	return nil
}
