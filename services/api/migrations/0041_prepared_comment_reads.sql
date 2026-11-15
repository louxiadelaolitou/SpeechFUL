-- +goose Up
-- +goose StatementBegin
-- These private, FK-free facts follow the same C Gate transaction as their
-- sources. No version write may acquire another account's FK lock after Gate.
-- Apply with source writes quiesced. These locks also prevent any concurrent
-- write from slipping between backfill and installation of the hooks.
LOCK TABLE c_auth.accounts, public.community_identities, c_posts.posts,
    c_comments.tasks, c_comments.attempts, c_comments.images, c_comments.likes,
    c_posts.blocked_authors IN SHARE ROW EXCLUSIVE MODE;

CREATE INDEX comment_snapshot_owner_expiry_idx ON c_comments.snapshots(owner_account_id, expires_at);
CREATE TABLE c_comments.read_scope_versions (
    post_id uuid NOT NULL,
    root_id uuid NOT NULL,
    membership_revision bigint NOT NULL CHECK (membership_revision >= 0),
    order_revision bigint NOT NULL CHECK (order_revision >= 0),
    PRIMARY KEY (post_id, root_id)
);
CREATE TABLE c_comments.read_post_versions (
    post_id uuid PRIMARY KEY,
    visibility_revision bigint NOT NULL CHECK (visibility_revision >= 0),
    count_revision bigint NOT NULL CHECK (count_revision >= 0)
);
CREATE TABLE c_comments.read_comment_versions (
    comment_id uuid PRIMARY KEY,
    projection_revision bigint NOT NULL CHECK (projection_revision >= 0),
    like_count bigint NOT NULL CHECK (like_count >= 0)
);
CREATE TABLE c_posts.read_identity_versions (
    identity_id uuid PRIMARY KEY,
    projection_revision bigint NOT NULL CHECK (projection_revision >= 0)
);
CREATE TABLE c_posts.read_account_versions (
    account_id uuid PRIMARY KEY,
    projection_revision bigint NOT NULL CHECK (projection_revision >= 0),
    visibility_revision bigint NOT NULL CHECK (visibility_revision >= 0)
);

INSERT INTO c_comments.read_post_versions SELECT post_id, 1, 1 FROM c_posts.posts;
INSERT INTO c_comments.read_scope_versions
SELECT post_id, '00000000-0000-0000-0000-000000000000'::uuid, 1, 1 FROM c_posts.posts
UNION SELECT post_id, COALESCE(root_comment_id, comment_id), 1, 1 FROM c_comments.tasks;
INSERT INTO c_comments.read_comment_versions
SELECT t.comment_id, 1, COALESCE(l.n, 0)
FROM c_comments.tasks t LEFT JOIN (
    SELECT comment_id, count(*) AS n FROM c_comments.likes GROUP BY comment_id
) l USING (comment_id);
INSERT INTO c_posts.read_identity_versions SELECT identity_id, 1 FROM public.community_identities;
INSERT INTO c_posts.read_account_versions SELECT account_id, 1, 1 FROM c_auth.accounts;

CREATE FUNCTION c_comments.advance_read_versions() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog AS $$
DECLARE
    t c_comments.tasks%ROWTYPE;
    member_changed boolean := false;
    projection_changed boolean := false;
    order_changed boolean := false;
    source text := TG_TABLE_SCHEMA || '.' || TG_TABLE_NAME;
    delta bigint;
    endpoint uuid;
BEGIN
    IF source = 'c_posts.posts' THEN
        IF TG_OP = 'INSERT' THEN
            INSERT INTO c_comments.read_post_versions VALUES (NEW.post_id, 1, 1);
            INSERT INTO c_comments.read_scope_versions VALUES
                (NEW.post_id, '00000000-0000-0000-0000-000000000000', 1, 1);
        ELSIF (NEW.visibility, NEW.moderation_hidden) IS DISTINCT FROM
              (OLD.visibility, OLD.moderation_hidden) THEN
            UPDATE c_comments.read_post_versions
                SET visibility_revision = visibility_revision + 1 WHERE post_id = NEW.post_id;
            IF NOT FOUND THEN RAISE EXCEPTION 'missing prepared post version' USING ERRCODE = '23514'; END IF;
        END IF;
    ELSIF source = 'public.community_identities' THEN
        IF TG_OP = 'INSERT' THEN
            INSERT INTO c_posts.read_identity_versions VALUES (NEW.identity_id, 1);
        ELSIF (NEW.nickname, NEW.avatar, NEW.deleted_at, NEW.account_id)
            IS DISTINCT FROM (OLD.nickname, OLD.avatar, OLD.deleted_at, OLD.account_id) THEN
            UPDATE c_posts.read_identity_versions SET projection_revision = projection_revision + 1
                WHERE identity_id = NEW.identity_id;
            IF NOT FOUND THEN RAISE EXCEPTION 'missing prepared identity version' USING ERRCODE = '23514'; END IF;
        END IF;
    ELSIF source = 'c_auth.accounts' THEN
        IF TG_OP = 'INSERT' THEN
            INSERT INTO c_posts.read_account_versions VALUES (NEW.account_id, 1, 1);
        ELSIF NEW.state IS DISTINCT FROM OLD.state THEN
            UPDATE c_posts.read_account_versions SET projection_revision = projection_revision + 1
                WHERE account_id = NEW.account_id;
            IF NOT FOUND THEN RAISE EXCEPTION 'missing prepared account version' USING ERRCODE = '23514'; END IF;
        END IF;
    ELSIF source = 'c_posts.blocked_authors' THEN
        -- The predicate is symmetric, including closure's bulk DELETE path.
        FOR endpoint IN SELECT x FROM unnest(ARRAY[
            CASE WHEN TG_OP = 'DELETE' THEN OLD.account_id ELSE NEW.account_id END,
            CASE WHEN TG_OP = 'DELETE' THEN OLD.author_account_id ELSE NEW.author_account_id END
        ]) x ORDER BY x LOOP
            UPDATE c_posts.read_account_versions SET visibility_revision = visibility_revision + 1
                WHERE account_id = endpoint;
            IF NOT FOUND THEN RAISE EXCEPTION 'missing prepared account visibility version' USING ERRCODE = '23514'; END IF;
        END LOOP;
    ELSIF source = 'c_comments.likes' THEN
        IF TG_OP = 'DELETE' THEN
            SELECT * INTO STRICT t FROM c_comments.tasks WHERE comment_id = OLD.comment_id;
            delta := -1;
        ELSE
            SELECT * INTO STRICT t FROM c_comments.tasks WHERE comment_id = NEW.comment_id;
            delta := 1;
        END IF;
        UPDATE c_comments.read_comment_versions SET like_count = like_count + delta
            WHERE comment_id = t.comment_id;
        IF NOT FOUND THEN
            RAISE EXCEPTION 'missing prepared comment count' USING ERRCODE = '23514';
        END IF;
        order_changed := true;
    ELSIF source = 'c_comments.tasks' THEN
        t := NEW;
        IF TG_OP = 'INSERT' THEN
            INSERT INTO c_comments.read_comment_versions VALUES (t.comment_id, 1, 0);
            IF t.root_comment_id IS NULL THEN
                INSERT INTO c_comments.read_scope_versions VALUES (t.post_id, t.comment_id, 1, 1);
            END IF;
        ELSE
            projection_changed := (NEW.deleted_at, NEW.moderation_hidden)
                IS DISTINCT FROM (OLD.deleted_at, OLD.moderation_hidden);
            member_changed := projection_changed AND EXISTS (
                SELECT 1 FROM c_comments.attempts a WHERE a.task_id = t.task_id AND a.state = 'PUBLISHED');
        END IF;
    ELSIF source = 'c_comments.attempts' THEN
        SELECT * INTO STRICT t FROM c_comments.tasks WHERE task_id = NEW.task_id;
        IF TG_OP = 'INSERT' THEN
            member_changed := NEW.state = 'PUBLISHED';
            projection_changed := member_changed;
        ELSE
            member_changed := (NEW.state = 'PUBLISHED') IS DISTINCT FROM (OLD.state = 'PUBLISHED');
            projection_changed := member_changed OR (NEW.state = 'PUBLISHED' AND
                (NEW.body, NEW.terminal_at, NEW.image) IS DISTINCT FROM (OLD.body, OLD.terminal_at, OLD.image));
            order_changed := NEW.state = 'PUBLISHED' AND NEW.terminal_at IS DISTINCT FROM OLD.terminal_at;
        END IF;
    ELSIF source = 'c_comments.images' THEN
        SELECT * INTO STRICT t FROM c_comments.tasks WHERE task_id = NEW.task_id;
        projection_changed := EXISTS (SELECT 1 FROM c_comments.attempts a
            WHERE a.task_id = NEW.task_id AND a.version = NEW.version AND a.state = 'PUBLISHED')
            AND (TG_OP = 'INSERT' OR
                (NEW.payload IS NOT NULL, NEW.metadata, NEW.content_version_id, NEW.erased_at)
                IS DISTINCT FROM (OLD.payload IS NOT NULL, OLD.metadata, OLD.content_version_id, OLD.erased_at));
    END IF;

    IF projection_changed THEN
        UPDATE c_comments.read_comment_versions SET projection_revision = projection_revision + 1
            WHERE comment_id = t.comment_id;
        IF NOT FOUND THEN RAISE EXCEPTION 'missing prepared comment version' USING ERRCODE = '23514'; END IF;
    END IF;
    IF member_changed THEN
        UPDATE c_comments.read_post_versions SET count_revision = count_revision + 1
            WHERE post_id = t.post_id;
        IF NOT FOUND THEN RAISE EXCEPTION 'missing prepared post count version' USING ERRCODE = '23514'; END IF;
        IF t.root_comment_id IS NULL THEN
            -- Hiding a root also hides its complete reply floor.
            UPDATE c_comments.read_scope_versions SET membership_revision = membership_revision + 1
                WHERE post_id = t.post_id AND root_id = t.comment_id;
            IF NOT FOUND THEN RAISE EXCEPTION 'missing prepared reply version' USING ERRCODE = '23514'; END IF;
        END IF;
    END IF;
    IF member_changed OR order_changed THEN
        UPDATE c_comments.read_scope_versions
            SET membership_revision = membership_revision + member_changed::integer,
                order_revision = order_revision + 1
            WHERE post_id = t.post_id AND root_id = COALESCE(t.root_comment_id, '00000000-0000-0000-0000-000000000000'::uuid);
        IF NOT FOUND THEN RAISE EXCEPTION 'missing prepared scope version' USING ERRCODE = '23514'; END IF;
    END IF;
    RETURN NULL;
END
$$;

CREATE TRIGGER prepared_read_post AFTER INSERT OR UPDATE OF visibility, moderation_hidden
    ON c_posts.posts FOR EACH ROW EXECUTE FUNCTION c_comments.advance_read_versions();
CREATE TRIGGER prepared_read_task AFTER INSERT OR UPDATE OF deleted_at, moderation_hidden
    ON c_comments.tasks FOR EACH ROW EXECUTE FUNCTION c_comments.advance_read_versions();
CREATE TRIGGER prepared_read_attempt AFTER INSERT OR UPDATE OF state, body, terminal_at, image
    ON c_comments.attempts FOR EACH ROW EXECUTE FUNCTION c_comments.advance_read_versions();
CREATE TRIGGER prepared_read_image AFTER INSERT OR UPDATE OF payload, metadata, content_version_id, erased_at
    ON c_comments.images FOR EACH ROW EXECUTE FUNCTION c_comments.advance_read_versions();
CREATE TRIGGER prepared_read_like AFTER INSERT OR DELETE
    ON c_comments.likes FOR EACH ROW EXECUTE FUNCTION c_comments.advance_read_versions();
CREATE TRIGGER prepared_read_identity AFTER INSERT OR UPDATE OF nickname, avatar, deleted_at, account_id
    ON public.community_identities FOR EACH ROW EXECUTE FUNCTION c_comments.advance_read_versions();
CREATE TRIGGER prepared_read_account AFTER INSERT OR UPDATE OF state
    ON c_auth.accounts FOR EACH ROW EXECUTE FUNCTION c_comments.advance_read_versions();
CREATE TRIGGER prepared_read_block AFTER INSERT OR DELETE
    ON c_posts.blocked_authors FOR EACH ROW EXECUTE FUNCTION c_comments.advance_read_versions();

REVOKE ALL ON c_comments.read_scope_versions, c_comments.read_post_versions,
    c_comments.read_comment_versions, c_posts.read_identity_versions, c_posts.read_account_versions FROM PUBLIC;
REVOKE ALL ON FUNCTION c_comments.advance_read_versions() FROM PUBLIC;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Prepared read versions require freeze and forward repair'; END $$;
-- +goose StatementEnd
