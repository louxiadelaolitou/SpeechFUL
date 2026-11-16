-- Operator-only reapply for migration 0047 after a rollback.
--
-- Keep source writes quiesced while this runs: the migration uses the same
-- transaction-scoped lock and plain CREATE INDEX, because the repository
-- migration chain applies every Up inside one transaction.
-- Run with `psql -v ON_ERROR_STOP=1 -f <this file>`. The explicit transaction
-- matters because psql sends statements one at a time; LOCK TABLE is only
-- valid inside a transaction block.
-- preparedDmDirectoryReads can be enabled again once both indexes report valid
-- and ready.
BEGIN;
LOCK TABLE c_dm.conversations IN SHARE ROW EXCLUSIVE MODE;
CREATE INDEX IF NOT EXISTS dm_conversations_low_cursor
    ON c_dm.conversations (low_identity, conversation_id);
CREATE INDEX IF NOT EXISTS dm_conversations_high_cursor
    ON c_dm.conversations (high_identity, conversation_id);
COMMIT;
