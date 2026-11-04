-- +goose Up
-- +goose StatementBegin
ALTER TABLE c_posts.posts
    ADD COLUMN edit_revision uuid,
    ADD COLUMN edited_at timestamptz,
    ADD COLUMN edited_title text,
    ADD COLUMN edited_body text,
    ADD COLUMN edited_image_ids uuid[] CHECK (cardinality(edited_image_ids) BETWEEN 0 AND 9),
    ADD CONSTRAINT post_edit_content_shape CHECK (
        (edited_at IS NULL AND edited_title IS NULL AND edited_body IS NULL)
        OR (visibility='PUBLISHED' AND edit_revision IS NOT NULL AND edited_at IS NOT NULL
            AND edited_title IS NOT NULL AND edited_body IS NOT NULL
            AND octet_length(edited_title) BETWEEN 1 AND 4096 AND octet_length(edited_body)<=65536)
    );

CREATE TABLE c_posts.post_edit_receipts (
    post_id uuid NOT NULL REFERENCES c_posts.posts(post_id),
    command_digest bytea NOT NULL CHECK(octet_length(command_digest)=32),
    fingerprint bytea NOT NULL CHECK(octet_length(fingerprint)=32),
    result_revision uuid,
    rejection text,
    PRIMARY KEY (post_id,command_digest),
    CHECK ((result_revision IS NULL) <> (rejection IS NULL))
);

CREATE TABLE c_posts.post_edit_media (
    post_id uuid NOT NULL REFERENCES c_posts.posts(post_id),
    content_version_id uuid NOT NULL,
    command_digest bytea CHECK(octet_length(command_digest)=32),
    staged_at timestamptz NOT NULL,
    payload bytea NOT NULL,
    digest bytea NOT NULL CHECK(octet_length(digest)=32),
    mime_type text NOT NULL CHECK(mime_type IN ('image/jpeg','image/png','image/gif','image/webp')),
    byte_length integer NOT NULL CHECK(byte_length BETWEEN 1 AND 10485760),
    width integer NOT NULL CHECK(width BETWEEN 1 AND 16384),
    height integer NOT NULL CHECK(height BETWEEN 1 AND 16384),
    display_width integer NOT NULL,
    display_height integer NOT NULL,
    frames integer NOT NULL CHECK(frames BETWEEN 1 AND 120),
    PRIMARY KEY (post_id,content_version_id),
    CHECK (octet_length(payload)=byte_length AND sha256(payload)=digest),
    CHECK ((display_width=width AND display_height=height) OR (display_width=height AND display_height=width))
);
CREATE INDEX post_edit_staged_expiry ON c_posts.post_edit_media (staged_at,post_id) WHERE command_digest IS NOT NULL;

CREATE FUNCTION c_posts.fence_post_edit() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.visibility='DELETED' THEN
        NEW.edit_revision := NULL;
        NEW.edited_at := NULL;
        NEW.edited_title := NULL;
        NEW.edited_body := NULL;
        NEW.edited_image_ids := NULL;
        DELETE FROM c_posts.post_edit_media WHERE post_id=NEW.post_id;
    ELSIF OLD.moderation_hidden IS DISTINCT FROM NEW.moderation_hidden THEN
        NEW.edit_revision := gen_random_uuid();
        DELETE FROM c_posts.post_edit_media WHERE post_id=NEW.post_id AND command_digest IS NOT NULL;
    END IF;
    RETURN NEW;
END
$$;
CREATE TRIGGER fence_post_edit BEFORE UPDATE OF visibility,moderation_hidden ON c_posts.posts
    FOR EACH ROW EXECUTE FUNCTION c_posts.fence_post_edit();

REVOKE ALL ON TABLE c_posts.post_edit_receipts,c_posts.post_edit_media FROM PUBLIC;
REVOKE ALL ON FUNCTION c_posts.fence_post_edit() FROM PUBLIC;
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname='hnuhole_c_runtime') THEN
        GRANT SELECT,INSERT ON c_posts.post_edit_receipts TO hnuhole_c_runtime;
        GRANT UPDATE(edit_revision,edited_at,edited_title,edited_body,edited_image_ids) ON c_posts.posts TO hnuhole_c_runtime;
        GRANT SELECT,INSERT,DELETE ON c_posts.post_edit_media TO hnuhole_c_runtime;
        GRANT UPDATE(command_digest) ON c_posts.post_edit_media TO hnuhole_c_runtime;
    END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'post edit receipts require forward repair'; END $$;
-- +goose StatementEnd
