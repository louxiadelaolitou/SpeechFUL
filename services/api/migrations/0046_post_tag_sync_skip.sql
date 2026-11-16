-- +goose Up
-- Hold writers until the check and function replacement commit together.
LOCK TABLE c_posts.posts, c_posts.post_tag_terms IN SHARE ROW EXCLUSIVE MODE;
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_index i JOIN pg_class c ON c.oid=i.indexrelid
        JOIN pg_namespace n ON n.oid=c.relnamespace
        WHERE n.nspname='c_posts' AND c.relname='post_tag_terms_post_idx'
          AND i.indisvalid AND i.indisready
          AND i.indrelid='c_posts.post_tag_terms'::regclass
          AND pg_get_indexdef(i.indexrelid) =
              'CREATE INDEX post_tag_terms_post_idx ON c_posts.post_tag_terms USING btree (post_id)'
    ) THEN
        RAISE EXCEPTION 'valid post tag post_id index required';
    END IF;
    IF EXISTS (
        (SELECT p.post_id, lower(t.tag COLLATE "C") AS tag_key, t.tag COLLATE "C" AS label
         FROM c_posts.posts p CROSS JOIN LATERAL unnest(p.tags) AS t(tag)
         WHERE p.visibility='PUBLISHED' AND NOT p.moderation_hidden
         EXCEPT SELECT post_id, tag_key, label COLLATE "C" FROM c_posts.post_tag_terms)
        UNION ALL
        (SELECT post_id, tag_key, label COLLATE "C" FROM c_posts.post_tag_terms
         EXCEPT SELECT p.post_id, lower(t.tag COLLATE "C"), t.tag COLLATE "C"
         FROM c_posts.posts p CROSS JOIN LATERAL unnest(p.tags) AS t(tag)
         WHERE p.visibility='PUBLISHED' AND NOT p.moderation_hidden)
    ) THEN
        RAISE EXCEPTION 'post tag projection inconsistent; repair before enabling skip';
    END IF;
END $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION c_posts.sync_post_tag_terms() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF (OLD.visibility='PUBLISHED' AND NOT OLD.moderation_hidden) =
       (NEW.visibility='PUBLISHED' AND NOT NEW.moderation_hidden)
       AND (NOT (NEW.visibility='PUBLISHED' AND NOT NEW.moderation_hidden)
            OR OLD.tags COLLATE "C" IS NOT DISTINCT FROM NEW.tags COLLATE "C") THEN
        RETURN NEW;
    END IF;
    DELETE FROM c_posts.post_tag_terms WHERE post_id=NEW.post_id;
    IF NEW.visibility='PUBLISHED' AND NOT NEW.moderation_hidden THEN
        INSERT INTO c_posts.post_tag_terms(post_id,tag_key,label)
        SELECT NEW.post_id,lower(t COLLATE "C"),t FROM unnest(NEW.tags) t;
    END IF;
    RETURN NEW;
END $$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'use the reviewed post tag sync rollback script'; END $$;
-- +goose StatementEnd
