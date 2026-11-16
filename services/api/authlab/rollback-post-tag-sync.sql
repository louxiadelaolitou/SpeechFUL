-- Restore the original synchronization function; keep migration 0045 index and ledger.
CREATE OR REPLACE FUNCTION c_posts.sync_post_tag_terms() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    DELETE FROM c_posts.post_tag_terms WHERE post_id=NEW.post_id;
    IF NEW.visibility='PUBLISHED' AND NOT NEW.moderation_hidden THEN
        INSERT INTO c_posts.post_tag_terms(post_id,tag_key,label)
        SELECT NEW.post_id,lower(t COLLATE "C"),t FROM unnest(NEW.tags) t;
    END IF;
    RETURN NEW;
END $$;
