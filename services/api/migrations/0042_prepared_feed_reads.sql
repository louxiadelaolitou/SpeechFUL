-- +goose Up
-- +goose StatementBegin
-- Install with source writes quiesced. Runtime source writes use the same C
-- Gate; these FK-free counters do not acquire another author's account lock.
LOCK TABLE public.channels, c_posts.posts, c_posts.attempt_contents,
    c_posts.images, c_posts.post_edit_media IN SHARE ROW EXCLUSIVE MODE;

CREATE TABLE c_posts.read_feed_scope_versions (
    channel_id uuid PRIMARY KEY,
    membership_revision bigint NOT NULL CHECK (membership_revision >= 0),
    publication_revision bigint NOT NULL CHECK (publication_revision >= 0)
);
CREATE TABLE c_posts.read_feed_post_versions (
    post_id uuid PRIMARY KEY,
    projection_revision bigint NOT NULL CHECK (projection_revision >= 0)
);
INSERT INTO c_posts.read_feed_scope_versions SELECT id, 1, 1 FROM public.channels;
INSERT INTO c_posts.read_feed_post_versions SELECT post_id, 1 FROM c_posts.posts;

CREATE FUNCTION c_posts.advance_feed_read_versions() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog AS $$
DECLARE
    source text := TG_TABLE_SCHEMA || '.' || TG_TABLE_NAME;
    target_post uuid;
    target_channel uuid;
    projection_changed boolean := false;
    membership_changed boolean := false;
    publication_changed boolean := false;
BEGIN
    IF source = 'public.channels' THEN
        INSERT INTO c_posts.read_feed_scope_versions VALUES (NEW.id, 1, 1);
        RETURN NULL;
    ELSIF source = 'c_posts.posts' THEN
        target_post := NEW.post_id;
        target_channel := NEW.channel_id;
        IF TG_OP = 'INSERT' THEN
            INSERT INTO c_posts.read_feed_post_versions VALUES (NEW.post_id, 1);
            publication_changed := NEW.published_at IS NOT NULL;
        ELSE
            -- Arrival above a frozen publication ceiling must not invalidate
            -- a continuation. Existing member/order changes still do.
            publication_changed := OLD.published_at IS NULL AND NEW.published_at IS NOT NULL;
            membership_changed := NOT publication_changed AND (
                (NEW.visibility='PUBLISHED' AND NOT NEW.moderation_hidden)
                  IS DISTINCT FROM (OLD.visibility='PUBLISHED' AND NOT OLD.moderation_hidden)
                OR (OLD.published_at IS NOT NULL AND
                    (NEW.channel_id,NEW.published_at,NEW.publication_ordinal)
                    IS DISTINCT FROM (OLD.channel_id,OLD.published_at,OLD.publication_ordinal)));
            projection_changed := (NEW.owner_account_id,NEW.channel_id,NEW.visibility,NEW.moderation_hidden,
                NEW.published_attempt_version,NEW.published_at,NEW.publication_ordinal,
                NEW.edit_revision,NEW.edited_at,NEW.edited_title,NEW.edited_body,NEW.edited_image_ids,NEW.tags)
                IS DISTINCT FROM (OLD.owner_account_id,OLD.channel_id,OLD.visibility,OLD.moderation_hidden,
                OLD.published_attempt_version,OLD.published_at,OLD.publication_ordinal,
                OLD.edit_revision,OLD.edited_at,OLD.edited_title,OLD.edited_body,OLD.edited_image_ids,OLD.tags);
        END IF;
    ELSIF source = 'c_posts.attempt_contents' THEN
        SELECT p.post_id,p.channel_id INTO target_post,target_channel
            FROM c_posts.posts p JOIN c_posts.publication_tasks t USING(post_id)
            WHERE t.task_id=NEW.task_id AND p.published_attempt_version=NEW.version;
        projection_changed := target_post IS NOT NULL AND
            (NEW.title,NEW.body,NEW.erased_at,NEW.image_count)
            IS DISTINCT FROM (OLD.title,OLD.body,OLD.erased_at,OLD.image_count);
        membership_changed := target_post IS NOT NULL AND NEW.erased_at IS DISTINCT FROM OLD.erased_at;
    ELSIF source = 'c_posts.images' THEN
        SELECT p.post_id INTO target_post
            FROM c_posts.posts p JOIN c_posts.publication_tasks t USING(post_id)
            WHERE t.task_id=NEW.task_id AND p.published_attempt_version=NEW.version
              AND (p.edited_image_ids IS NULL OR NEW.content_version_id=ANY(p.edited_image_ids));
        projection_changed := target_post IS NOT NULL AND (TG_OP='INSERT' OR
            (NEW.payload IS NOT NULL,NEW.digest,NEW.byte_length,NEW.mime_type,NEW.width,NEW.height,
             NEW.display_width,NEW.display_height,NEW.frames,NEW.erased_at,NEW.verified_at,
             NEW.position,NEW.content_version_id,NEW.cycle_duration_ms,NEW.max_frame_duration_ms)
            IS DISTINCT FROM
            (OLD.payload IS NOT NULL,OLD.digest,OLD.byte_length,OLD.mime_type,OLD.width,OLD.height,
             OLD.display_width,OLD.display_height,OLD.frames,OLD.erased_at,OLD.verified_at,
             OLD.position,OLD.content_version_id,OLD.cycle_duration_ms,OLD.max_frame_duration_ms));
    ELSIF source = 'c_posts.post_edit_media' THEN
        -- Staging and expiry of an unselected image cannot invalidate a feed.
        -- A selected ID can be deleted/reinserted by staging, so bind each
        -- selected row transition, not merely content_version_id equality.
        SELECT p.post_id INTO target_post FROM c_posts.posts p
            WHERE p.post_id=CASE WHEN TG_OP='DELETE' THEN OLD.post_id ELSE NEW.post_id END
              AND CASE WHEN TG_OP='DELETE' THEN OLD.content_version_id ELSE NEW.content_version_id END
                  =ANY(p.edited_image_ids);
        projection_changed := target_post IS NOT NULL;
    END IF;

    IF projection_changed THEN
        UPDATE c_posts.read_feed_post_versions SET projection_revision=projection_revision+1
            WHERE post_id=target_post;
        IF NOT FOUND THEN RAISE EXCEPTION 'missing prepared feed post version' USING ERRCODE='23514'; END IF;
    END IF;
    IF membership_changed OR publication_changed THEN
        UPDATE c_posts.read_feed_scope_versions
            SET membership_revision=membership_revision+membership_changed::integer,
                publication_revision=publication_revision+publication_changed::integer
            WHERE channel_id=target_channel;
        IF NOT FOUND THEN RAISE EXCEPTION 'missing prepared feed scope version' USING ERRCODE='23514'; END IF;
    END IF;
    RETURN NULL;
END
$$;

CREATE TRIGGER prepared_feed_channel AFTER INSERT ON public.channels
    FOR EACH ROW EXECUTE FUNCTION c_posts.advance_feed_read_versions();
CREATE TRIGGER prepared_feed_post AFTER INSERT OR UPDATE OF owner_account_id,channel_id,visibility,moderation_hidden,
    published_attempt_version,published_at,publication_ordinal,edit_revision,edited_at,edited_title,edited_body,edited_image_ids,tags
    ON c_posts.posts FOR EACH ROW EXECUTE FUNCTION c_posts.advance_feed_read_versions();
CREATE TRIGGER prepared_feed_content AFTER UPDATE OF title,body,erased_at,image_count ON c_posts.attempt_contents
    FOR EACH ROW EXECUTE FUNCTION c_posts.advance_feed_read_versions();
CREATE TRIGGER prepared_feed_image AFTER INSERT OR UPDATE OF payload,digest,byte_length,mime_type,width,height,
    display_width,display_height,frames,erased_at,verified_at,position,content_version_id,cycle_duration_ms,max_frame_duration_ms
    ON c_posts.images FOR EACH ROW EXECUTE FUNCTION c_posts.advance_feed_read_versions();
CREATE TRIGGER prepared_feed_edit_media AFTER INSERT OR UPDATE OR DELETE ON c_posts.post_edit_media
    FOR EACH ROW EXECUTE FUNCTION c_posts.advance_feed_read_versions();

REVOKE ALL ON c_posts.read_feed_scope_versions,c_posts.read_feed_post_versions FROM PUBLIC;
REVOKE ALL ON FUNCTION c_posts.advance_feed_read_versions() FROM PUBLIC;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Prepared feed versions require freeze and forward repair'; END $$;
-- +goose StatementEnd
