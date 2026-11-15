-- +goose Up
-- +goose StatementBegin
-- Comment projections count likes per row via
-- c_comments.likes.comment_id. The (account_id,comment_id) primary key cannot
-- serve that second-column filter, so each projected comment scans the likes
-- table once. Reply counts filter both post_id and root_comment_id
-- (community_comments_read.go), which the existing comment_task_thread index
-- serves directly. Install with source writes quiesced for the same
-- transactional migration chain as 0041/0042, where CREATE INDEX CONCURRENTLY
-- is not permitted.
LOCK TABLE c_comments.likes IN SHARE ROW EXCLUSIVE MODE;
CREATE INDEX comment_likes_comment ON c_comments.likes(comment_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Forward-only like 0041/0042/0043: dropping this index would silently
-- restore the per-comment scans. Abandon the access path with an explicit
-- forward repair; the ledger stays at 0044.
DO $$ BEGIN RAISE EXCEPTION 'comment likes index requires forward repair; Down is forbidden' END $$;
-- +goose StatementEnd
