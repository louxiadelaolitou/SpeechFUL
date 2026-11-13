-- +goose Up
-- Account-level author blocks. A row is private to the blocker; public reads
-- only use the derived symmetric predicate and never expose either account ID.
CREATE TABLE c_posts.blocked_authors (
    block_id uuid PRIMARY KEY,
    account_id uuid NOT NULL REFERENCES c_auth.accounts(account_id) ON DELETE CASCADE,
    author_account_id uuid NOT NULL REFERENCES c_auth.accounts(account_id) ON DELETE CASCADE,
    author_state text NOT NULL CHECK (author_state IN ('ACTIVE', 'DELETED', 'ACCOUNT_CLOSED')),
    nickname text NOT NULL,
    avatar text NOT NULL,
    short_code text NOT NULL,
    created_at timestamptz NOT NULL,
    CHECK (account_id <> author_account_id),
    UNIQUE (account_id, author_account_id)
);

-- The reversed pair supports the symmetric read predicate without scanning.
CREATE INDEX blocked_authors_author_idx ON c_posts.blocked_authors(author_account_id, account_id);

-- One predicate for every content read: a block hides both directions.
-- +goose StatementBegin
CREATE FUNCTION c_posts.accounts_blocked(viewer uuid, author uuid) RETURNS boolean
LANGUAGE sql STABLE PARALLEL SAFE SET search_path = pg_catalog AS $$
    SELECT EXISTS (SELECT 1 FROM c_posts.blocked_authors b
                   WHERE b.account_id = viewer AND b.author_account_id = author)
        OR EXISTS (SELECT 1 FROM c_posts.blocked_authors b
                   WHERE b.account_id = author AND b.author_account_id = viewer)
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION c_posts.clear_closed_account_author_blocks() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
    IF NEW.state = 'CLOSED' AND OLD.state IS DISTINCT FROM NEW.state THEN
        DELETE FROM c_posts.blocked_authors
            WHERE account_id = NEW.account_id OR author_account_id = NEW.account_id;
    END IF;
    RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER blocked_authors_account_closed AFTER UPDATE OF state ON c_auth.accounts
    FOR EACH ROW EXECUTE FUNCTION c_posts.clear_closed_account_author_blocks();
REVOKE ALL ON c_posts.blocked_authors FROM PUBLIC;
REVOKE ALL ON FUNCTION c_posts.accounts_blocked(uuid, uuid) FROM PUBLIC;
REVOKE ALL ON FUNCTION c_posts.clear_closed_account_author_blocks() FROM PUBLIC;

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
    RAISE EXCEPTION '0039 stores private account blocks; repair forward';
END $$;
-- +goose StatementEnd
