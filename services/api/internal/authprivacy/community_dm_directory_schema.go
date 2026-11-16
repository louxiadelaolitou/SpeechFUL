package authprivacy

import "context"

// Only the explicitly enabled prepared directory requires 0043 plus the 0041
// and 0042 version bases. The original directory query stays available while
// an operator rolls the optional read optimization back.
func (c *Community) checkPreparedDMDirectorySchema(ctx context.Context) error {
	if err := c.checkPreparedFeedReadSchema(ctx); err != nil {
		return err
	}
	var ok bool
	if err := c.pool.QueryRow(ctx, `SELECT
 EXISTS (SELECT 1 FROM pg_index i WHERE i.indexrelid=to_regclass('c_dm.dm_conversations_low_cursor')
  AND i.indrelid=to_regclass('c_dm.conversations') AND i.indisvalid AND i.indisready
  AND i.indpred IS NULL AND i.indexprs IS NULL
  AND i.indkey::text=(SELECT string_agg(a.attnum::text,' ' ORDER BY col.pos)
   FROM unnest(ARRAY['low_identity','conversation_id']) WITH ORDINALITY col(name,pos)
   JOIN pg_attribute a ON a.attrelid=i.indrelid AND a.attname=col.name AND NOT a.attisdropped))
 AND EXISTS (SELECT 1 FROM pg_index i WHERE i.indexrelid=to_regclass('c_dm.dm_conversations_high_cursor')
  AND i.indrelid=to_regclass('c_dm.conversations') AND i.indisvalid AND i.indisready
  AND i.indpred IS NULL AND i.indexprs IS NULL
  AND i.indkey::text=(SELECT string_agg(a.attnum::text,' ' ORDER BY col.pos)
   FROM unnest(ARRAY['high_identity','conversation_id']) WITH ORDINALITY col(name,pos)
   JOIN pg_attribute a ON a.attrelid=i.indrelid AND a.attname=col.name AND NOT a.attisdropped))
 AND to_regclass('c_dm.blocks') IS NOT NULL
 AND to_regclass('c_posts.blocked_authors') IS NOT NULL
 AND EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='c_dm.conversations'::regclass AND contype='u'
  AND pg_get_constraintdef(oid)='UNIQUE (low_identity, high_identity)')`).Scan(&ok); err != nil || !ok {
		return errPostSchema
	}
	return nil
}
