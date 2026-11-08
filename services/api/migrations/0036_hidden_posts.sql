-- +goose Up
-- Account-private per-post preference and replay fence. A false row retains the
-- write version so a delayed earlier request cannot re-hide an undone post.
CREATE TABLE c_posts.hidden_posts (
    account_id uuid NOT NULL REFERENCES c_auth.accounts(account_id),
    post_id uuid NOT NULL REFERENCES c_posts.posts(post_id),
    hidden boolean NOT NULL,
    version uuid NOT NULL CHECK (version <> '00000000-0000-0000-0000-000000000000'),
    PRIMARY KEY (account_id, post_id)
);

-- +goose StatementBegin
CREATE FUNCTION c_posts.clear_closed_account_hidden_posts() RETURNS trigger
LANGUAGE plpgsql SET search_path = pg_catalog AS $$
BEGIN
    IF NEW.state = 'CLOSED' AND OLD.state IS DISTINCT FROM NEW.state THEN
        DELETE FROM c_posts.hidden_posts WHERE account_id = NEW.account_id;
    END IF;
    RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER hidden_posts_account_closed AFTER UPDATE OF state ON c_auth.accounts
    FOR EACH ROW EXECUTE FUNCTION c_posts.clear_closed_account_hidden_posts();
REVOKE ALL ON c_posts.hidden_posts FROM PUBLIC;
REVOKE ALL ON FUNCTION c_posts.clear_closed_account_hidden_posts() FROM PUBLIC;

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
    RAISE EXCEPTION '0036 stores private preference and write-fence facts; repair forward';
END $$;
-- +goose StatementEnd
