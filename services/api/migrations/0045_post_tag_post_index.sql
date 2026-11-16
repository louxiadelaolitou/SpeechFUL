-- +goose NO TRANSACTION
-- +goose Up
CREATE INDEX CONCURRENTLY post_tag_terms_post_idx ON c_posts.post_tag_terms (post_id);

-- +goose Down
DROP INDEX CONCURRENTLY c_posts.post_tag_terms_post_idx;
