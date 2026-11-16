-- +goose Up
-- +goose StatementBegin
-- The DM directory pages by conversation_id for one endpoint. Both endpoint
-- columns need a cursor-ordered entry; the unique identity pair stays the
-- authoritative constraint. Install with source writes quiesced: this keeps
-- the migration in the same transactional chain as 0041/0042, where
-- CREATE INDEX CONCURRENTLY is not permitted.
LOCK TABLE c_dm.conversations IN SHARE ROW EXCLUSIVE MODE;
CREATE INDEX dm_conversations_low_cursor
    ON c_dm.conversations (low_identity, conversation_id);
CREATE INDEX dm_conversations_high_cursor
    ON c_dm.conversations (high_identity, conversation_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Forward-only, like 0041/0042: the prepared directory validates both indexes
-- at startup. To abandon the access path, disable preparedDmDirectoryReads and
-- run services/api/ops/0043_dm_directory_cursor_indexes_rollback.sql; the
-- ledger stays at 0043 and the counterpart _reapply.sql restores the indexes.
DO $$ BEGIN RAISE EXCEPTION 'DM directory cursor indexes require forward repair; Down is forbidden'; END $$;
-- +goose StatementEnd
