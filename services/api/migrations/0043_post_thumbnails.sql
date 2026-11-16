-- +goose Up
-- +goose StatementBegin
-- Private derivative storage only. Apply with source writes quiesced.
LOCK TABLE c_posts.posts, c_posts.images, c_posts.post_edit_media IN SHARE ROW EXCLUSIVE MODE;
CREATE TABLE c_posts.thumbnail_storage (
    singleton_id integer PRIMARY KEY CHECK (singleton_id=1),
    capacity_bytes bigint NOT NULL CHECK (capacity_bytes BETWEEN 524288 AND 1099511627776),
    used_bytes bigint NOT NULL CHECK (used_bytes>=0 AND used_bytes<=capacity_bytes)
);
-- A bounded initial budget, not preallocated space. Only the migration owner
-- may change capacity; all instances observe the same database budget.
INSERT INTO c_posts.thumbnail_storage VALUES (1,268435456,0);
CREATE TABLE c_posts.thumbnails (
    post_id uuid NOT NULL,
    content_version_id uuid NOT NULL,
    variant text NOT NULL CHECK (variant='thumbnail-v1-640'),
    encoder_version text NOT NULL CHECK (encoder_version ~ '^[0-9a-f]{64}$'),
    source_binding bytea NOT NULL CHECK (octet_length(source_binding)=32),
    payload bytea NOT NULL,
    digest bytea NOT NULL CHECK (octet_length(digest)=32 AND sha256(payload)=digest),
    mime_type text NOT NULL CHECK (mime_type='image/webp'),
    width integer NOT NULL CHECK (width BETWEEN 1 AND 640),
    height integer NOT NULL CHECK (height BETWEEN 1 AND 640),
    byte_length integer NOT NULL CHECK (byte_length BETWEEN 12 AND 524288 AND octet_length(payload)=byte_length),
    created_at timestamptz NOT NULL,
    PRIMARY KEY(post_id,content_version_id,variant,encoder_version)
);
CREATE INDEX thumbnail_oldest_idx ON c_posts.thumbnails(created_at,post_id,content_version_id,variant,encoder_version);

CREATE FUNCTION c_posts.account_thumbnail_storage() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $$
BEGIN
    UPDATE c_posts.thumbnail_storage SET used_bytes=used_bytes+
        CASE WHEN TG_OP='INSERT' THEN NEW.byte_length ELSE -OLD.byte_length END
        WHERE singleton_id=1;
    IF NOT FOUND THEN RAISE EXCEPTION 'missing thumbnail storage budget' USING ERRCODE='23514'; END IF;
    RETURN NULL;
END
$$;
CREATE TRIGGER thumbnail_account_storage AFTER INSERT OR DELETE ON c_posts.thumbnails
    FOR EACH ROW EXECUTE FUNCTION c_posts.account_thumbnail_storage();

CREATE FUNCTION c_posts.erase_source_thumbnails() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $$
DECLARE target_post uuid;
BEGIN
    IF TG_TABLE_NAME='posts' THEN
        IF (NEW.visibility,NEW.moderation_hidden,NEW.published_attempt_version,NEW.edited_image_ids)
            IS DISTINCT FROM (OLD.visibility,OLD.moderation_hidden,OLD.published_attempt_version,OLD.edited_image_ids) THEN
            DELETE FROM c_posts.thumbnails WHERE post_id=OLD.post_id;
        END IF;
    ELSIF TG_TABLE_NAME='images' THEN
        IF TG_OP='DELETE' OR
            (NEW.payload IS NULL,NEW.erased_at,NEW.verified_at,NEW.digest,NEW.byte_length,NEW.mime_type,
             NEW.width,NEW.height,NEW.display_width,NEW.display_height,NEW.frames,
             NEW.cycle_duration_ms,NEW.max_frame_duration_ms)
            IS DISTINCT FROM
            (OLD.payload IS NULL,OLD.erased_at,OLD.verified_at,OLD.digest,OLD.byte_length,OLD.mime_type,
             OLD.width,OLD.height,OLD.display_width,OLD.display_height,OLD.frames,
             OLD.cycle_duration_ms,OLD.max_frame_duration_ms) THEN
            SELECT t.post_id INTO target_post FROM c_posts.publication_tasks t WHERE t.task_id=OLD.task_id;
            DELETE FROM c_posts.thumbnails WHERE post_id=target_post AND content_version_id=OLD.content_version_id;
        END IF;
    ELSE
        -- Editing may delete and reuse an ID. Invalidate on every transition,
        -- including staging, rather than trusting ID equality alone.
        DELETE FROM c_posts.thumbnails WHERE post_id=OLD.post_id AND content_version_id=OLD.content_version_id;
    END IF;
    RETURN NULL;
END
$$;
CREATE TRIGGER thumbnail_post_erase AFTER UPDATE ON c_posts.posts
    FOR EACH ROW EXECUTE FUNCTION c_posts.erase_source_thumbnails();
CREATE TRIGGER thumbnail_image_erase AFTER UPDATE OR DELETE ON c_posts.images
    FOR EACH ROW EXECUTE FUNCTION c_posts.erase_source_thumbnails();
CREATE TRIGGER thumbnail_edit_erase AFTER UPDATE OR DELETE ON c_posts.post_edit_media
    FOR EACH ROW EXECUTE FUNCTION c_posts.erase_source_thumbnails();

REVOKE ALL ON c_posts.thumbnails,c_posts.thumbnail_storage FROM PUBLIC;
REVOKE ALL ON FUNCTION c_posts.account_thumbnail_storage(),c_posts.erase_source_thumbnails() FROM PUBLIC;
DO $$ BEGIN
    IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='hnuhole_c_runtime') THEN
        GRANT SELECT,INSERT,DELETE ON c_posts.thumbnails TO hnuhole_c_runtime;
        GRANT SELECT ON c_posts.thumbnail_storage TO hnuhole_c_runtime;
    END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
    RAISE EXCEPTION 'thumbnail lifecycle must remain installed; disable reuse instead of rolling storage back';
END $$;
-- +goose StatementEnd
