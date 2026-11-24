-- +goose Up
-- +goose StatementBegin
-- Discardable Redis thumbnail cache: transactional deletion outbox only.
-- Apply with source writes quiesced. Old binaries keep working because the
-- existing derivative lifecycle triggers and checks are unchanged; they simply
-- do not consume these events until the new runtime is deployed.
LOCK TABLE c_posts.thumbnails IN SHARE ROW EXCLUSIVE MODE;
CREATE TABLE c_posts.thumbnail_cache_outbox (
    event_id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    post_id uuid NOT NULL,
    content_version_id uuid NOT NULL,
    variant text NOT NULL CHECK (variant='thumbnail-v1-640'),
    encoder_version text NOT NULL CHECK (encoder_version ~ '^[0-9a-f]{64}$'),
    source_binding bytea NOT NULL CHECK (octet_length(source_binding)=32),
    output_digest bytea NOT NULL CHECK (octet_length(output_digest)=32),
    created_at timestamptz NOT NULL DEFAULT now(),
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts>=0),
    lease_token uuid,
    lease_until timestamptz,
    completed_at timestamptz
);
CREATE INDEX thumbnail_cache_pending_idx ON c_posts.thumbnail_cache_outbox(next_attempt_at,event_id)
    WHERE completed_at IS NULL;
CREATE INDEX thumbnail_cache_completed_idx ON c_posts.thumbnail_cache_outbox(completed_at)
    WHERE completed_at IS NOT NULL;

CREATE FUNCTION c_posts.record_thumbnail_cache_erasure() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $$
BEGIN
    INSERT INTO c_posts.thumbnail_cache_outbox
        (post_id,content_version_id,variant,encoder_version,source_binding,output_digest)
    VALUES (OLD.post_id,OLD.content_version_id,OLD.variant,OLD.encoder_version,OLD.source_binding,OLD.digest);
    RETURN NULL;
END
$$;
CREATE TRIGGER thumbnail_cache_erase AFTER DELETE ON c_posts.thumbnails
    FOR EACH ROW EXECUTE FUNCTION c_posts.record_thumbnail_cache_erasure();

REVOKE ALL ON c_posts.thumbnail_cache_outbox FROM PUBLIC;
REVOKE ALL ON FUNCTION c_posts.record_thumbnail_cache_erasure() FROM PUBLIC;
DO $$ BEGIN
    IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='hnuhole_c_runtime') THEN
        -- The definer trigger inserts; the runtime may only claim, complete,
        -- retry and purge its own queue. It cannot fabricate deletion events.
        GRANT SELECT,UPDATE,DELETE ON c_posts.thumbnail_cache_outbox TO hnuhole_c_runtime;
    END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
    RAISE EXCEPTION 'thumbnail cache outbox must remain installed; disable the cache and drain events instead of rolling back';
END $$;
-- +goose StatementEnd
