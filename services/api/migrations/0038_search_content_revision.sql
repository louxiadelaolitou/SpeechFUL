-- +goose Up
-- +goose StatementBegin
CREATE TABLE c_posts.search_content_revision (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    revision bigint NOT NULL CHECK (revision > 0)
);
INSERT INTO c_posts.search_content_revision VALUES (true, 1);
CREATE FUNCTION c_posts.advance_search_content_revision() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.visibility = 'PUBLISHED' AND (NEW.edited_title, NEW.edited_body, NEW.tags) IS DISTINCT FROM
       (OLD.edited_title, OLD.edited_body, OLD.tags) THEN
        UPDATE c_posts.search_content_revision SET revision = revision + 1 WHERE singleton;
    END IF;
    RETURN NEW;
END
$$;
CREATE TRIGGER advance_search_content_revision AFTER UPDATE OF edited_title, edited_body, tags
    ON c_posts.posts FOR EACH ROW EXECUTE FUNCTION c_posts.advance_search_content_revision();
REVOKE ALL ON c_posts.search_content_revision FROM PUBLIC;
REVOKE ALL ON FUNCTION c_posts.advance_search_content_revision() FROM PUBLIC;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Search cursor revision requires freeze and forward repair'; END $$;
-- +goose StatementEnd
