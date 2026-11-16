-- +goose Up
-- Empty projection first: install transactional writers before owner-only,
-- separately committed backfill batches. Never scan the history in this DDL.
-- +goose StatementBegin
-- Do not wait behind a live multi-table writer while retaining another source
-- lock. A busy install rolls back intact and can be retried by the operator.
LOCK TABLE c_posts.posts,c_posts.attempt_contents IN SHARE ROW EXCLUSIVE MODE NOWAIT;
CREATE EXTENSION IF NOT EXISTS pg_trgm WITH SCHEMA public;

CREATE TABLE c_posts.post_search_documents (
    post_id uuid PRIMARY KEY,
    channel_id uuid NOT NULL,
    published_at timestamptz NOT NULL,
    publication_ordinal bigint NOT NULL,
    source_version integer NOT NULL,
    content_revision uuid NOT NULL,
    title_norm text COLLATE "C" NOT NULL,
    body_norm text COLLATE "C" NOT NULL,
    tags_norm text[] COLLATE "C" NOT NULL,
    search_key text COLLATE "C" NOT NULL
);
CREATE INDEX post_search_documents_trgm_idx ON c_posts.post_search_documents USING gin(search_key public.gin_trgm_ops);

CREATE TABLE c_posts.post_search_document_state (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    ready boolean NOT NULL,
    normalization_version text NOT NULL,
    last_post_id uuid NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000'
);

-- One definition for synchronization, backfill and the bidirectional diff.
-- Source visibility remains authoritative; normalization is exactly the old
-- search's ICU lower, including contextual Unicode casing of each field/tag.
CREATE FUNCTION c_posts.current_post_search_documents(selected uuid DEFAULT NULL)
RETURNS SETOF c_posts.post_search_documents LANGUAGE plpgsql STABLE SET search_path=pg_catalog AS $$
DECLARE source_sql constant text := $source$
SELECT p.post_id,p.channel_id,p.published_at,p.publication_ordinal,
       p.published_attempt_version AS source_version,COALESCE(p.edit_revision,p.post_id) AS content_revision,
       w.title_norm,w.body_norm,w.tags_norm,
       encode(convert_to(w.title_norm || chr(1) || w.body_norm || chr(1) || array_to_string(w.tags_norm,chr(1)),'UTF8'),'hex') COLLATE "C" AS search_key
FROM c_posts.posts p
JOIN c_posts.publication_tasks t ON t.post_id=p.post_id
JOIN c_posts.attempt_contents x ON x.task_id=t.task_id AND x.version=p.published_attempt_version
CROSS JOIN LATERAL (
    SELECT lower(COALESCE(p.edited_title,x.title) COLLATE pg_catalog."und-x-icu") COLLATE "C" AS title_norm,
           lower(COALESCE(p.edited_body,x.body) COLLATE pg_catalog."und-x-icu") COLLATE "C" AS body_norm,
           ARRAY(SELECT lower(tag COLLATE pg_catalog."und-x-icu") FROM unnest(p.tags) tag) COLLATE "C" AS tags_norm
) w
WHERE p.visibility='PUBLISHED' AND NOT p.moderation_hidden AND x.erased_at IS NULL
$source$;
BEGIN
    -- Separate constant statement shapes: a point refresh must never inherit
    -- an optional-parameter generic plan that scans all source history.
    IF selected IS NULL THEN RETURN QUERY EXECUTE source_sql;
    ELSE RETURN QUERY EXECUTE source_sql || ' AND p.post_id=$1' USING selected;
    END IF;
END
$$;

CREATE FUNCTION c_posts.refresh_post_search_document(selected uuid) RETURNS void
LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
    -- The same post-row fence for source triggers and backfill. Re-read source
    -- AFTER waiting; never upsert a pre-lock snapshot over a concurrent edit.
    PERFORM 1 FROM c_posts.posts WHERE post_id=selected FOR NO KEY UPDATE;
    INSERT INTO c_posts.post_search_documents
        SELECT * FROM c_posts.current_post_search_documents(selected)
    ON CONFLICT(post_id) DO UPDATE SET
        channel_id=EXCLUDED.channel_id,published_at=EXCLUDED.published_at,
        publication_ordinal=EXCLUDED.publication_ordinal,source_version=EXCLUDED.source_version,
        content_revision=EXCLUDED.content_revision,title_norm=EXCLUDED.title_norm,
        body_norm=EXCLUDED.body_norm,tags_norm=EXCLUDED.tags_norm,search_key=EXCLUDED.search_key
    WHERE (post_search_documents.channel_id,post_search_documents.published_at,
           post_search_documents.publication_ordinal,post_search_documents.source_version,
           post_search_documents.content_revision,post_search_documents.title_norm,
           post_search_documents.body_norm,post_search_documents.tags_norm,post_search_documents.search_key)
       IS DISTINCT FROM (EXCLUDED.channel_id,EXCLUDED.published_at,EXCLUDED.publication_ordinal,
           EXCLUDED.source_version,EXCLUDED.content_revision,EXCLUDED.title_norm,
           EXCLUDED.body_norm,EXCLUDED.tags_norm,EXCLUDED.search_key);
    IF NOT FOUND THEN
        DELETE FROM c_posts.post_search_documents d WHERE d.post_id=selected
          AND NOT EXISTS (SELECT 1 FROM c_posts.current_post_search_documents(selected));
    END IF;
END
$$;

CREATE FUNCTION c_posts.sync_post_search_document() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $$
DECLARE selected uuid;
BEGIN
    IF TG_TABLE_NAME='posts' THEN
        IF TG_OP='DELETE' THEN selected := OLD.post_id;
        ELSE
            IF TG_OP='UPDATE' AND
               (NEW.visibility,NEW.moderation_hidden,NEW.published_attempt_version,NEW.edit_revision,
                NEW.edited_title,NEW.edited_body,NEW.tags,NEW.channel_id,NEW.published_at,NEW.publication_ordinal)
               IS NOT DISTINCT FROM
               (OLD.visibility,OLD.moderation_hidden,OLD.published_attempt_version,OLD.edit_revision,
                OLD.edited_title,OLD.edited_body,OLD.tags,OLD.channel_id,OLD.published_at,OLD.publication_ordinal)
            THEN RETURN NULL; END IF;
            selected := NEW.post_id;
        END IF;
    ELSE
        SELECT p.post_id INTO selected FROM c_posts.publication_tasks t
        JOIN c_posts.posts p ON p.post_id=t.post_id
        WHERE t.task_id=NEW.task_id AND p.published_attempt_version=NEW.version;
    END IF;
    IF selected IS NOT NULL THEN PERFORM c_posts.refresh_post_search_document(selected); END IF;
    RETURN NULL;
END
$$;
CREATE TRIGGER post_search_document_post AFTER INSERT OR DELETE OR UPDATE OF
    visibility,moderation_hidden,published_attempt_version,edit_revision,edited_title,edited_body,tags,channel_id,published_at,publication_ordinal
    ON c_posts.posts FOR EACH ROW EXECUTE FUNCTION c_posts.sync_post_search_document();
CREATE TRIGGER post_search_document_content AFTER INSERT OR UPDATE OF title,body,erased_at
    ON c_posts.attempt_contents FOR EACH ROW EXECUTE FUNCTION c_posts.sync_post_search_document();

INSERT INTO c_posts.post_search_document_state(singleton,ready,normalization_version)
    SELECT true,NOT EXISTS(SELECT 1 FROM c_posts.posts),pg_collation_actual_version('pg_catalog."und-x-icu"'::regcollation);

CREATE FUNCTION c_posts.backfill_post_search_documents(batch_size integer DEFAULT 1000) RETURNS integer
LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE last_id uuid; selected uuid; processed integer := 0; complete boolean;
BEGIN
    IF batch_size NOT BETWEEN 1 AND 10000 OR batch_size IS NULL THEN RAISE EXCEPTION 'invalid search backfill batch'; END IF;
    SELECT last_post_id,ready INTO STRICT last_id,complete FROM c_posts.post_search_document_state WHERE singleton FOR UPDATE;
    IF complete THEN RETURN 0; END IF;
    FOR selected IN SELECT post_id FROM c_posts.posts WHERE post_id>last_id ORDER BY post_id LIMIT batch_size LOOP
        -- Never wait while holding earlier post locks: account cleanup may
        -- update several posts in a different order. Stop BEFORE a busy row,
        -- retaining the keyset cursor so it cannot silently miss old history.
        PERFORM 1 FROM c_posts.posts WHERE post_id=selected FOR NO KEY UPDATE SKIP LOCKED;
        IF NOT FOUND THEN
            IF processed=0 THEN RETURN -1; ELSE EXIT; END IF;
        END IF;
        PERFORM c_posts.refresh_post_search_document(selected);
        last_id := selected;
        processed := processed+1;
    END LOOP;
    UPDATE c_posts.post_search_document_state SET last_post_id=last_id WHERE singleton;
    RETURN processed;
END
$$;

CREATE FUNCTION c_posts.validate_post_search_documents() RETURNS void
LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
    -- Both sides use ONE statement snapshot. Concurrent business writes keep
    -- this invariant in their own transaction; no global/table write lock.
    IF EXISTS (
        (SELECT * FROM c_posts.current_post_search_documents(NULL) EXCEPT SELECT * FROM c_posts.post_search_documents)
        UNION ALL
        (SELECT * FROM c_posts.post_search_documents EXCEPT SELECT * FROM c_posts.current_post_search_documents(NULL))
    ) THEN RAISE EXCEPTION 'post search projection inconsistent'; END IF;
    UPDATE c_posts.post_search_document_state SET ready=true,
        normalization_version=pg_collation_actual_version('pg_catalog."und-x-icu"'::regcollation) WHERE singleton;
    IF NOT FOUND THEN RAISE EXCEPTION 'post search readiness missing'; END IF;
END
$$;

REVOKE ALL ON c_posts.post_search_documents,c_posts.post_search_document_state FROM PUBLIC;
REVOKE ALL ON FUNCTION c_posts.refresh_post_search_document(uuid),c_posts.sync_post_search_document(),
    c_posts.backfill_post_search_documents(integer),c_posts.validate_post_search_documents(),
    c_posts.current_post_search_documents(uuid) FROM PUBLIC;
DO $$ BEGIN
    IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='hnuhole_c_runtime') THEN
        GRANT SELECT ON c_posts.post_search_documents,c_posts.post_search_document_state TO hnuhole_c_runtime;
    END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'disable searchDocuments; keep synchronization until reviewed retirement'; END $$;
-- +goose StatementEnd
