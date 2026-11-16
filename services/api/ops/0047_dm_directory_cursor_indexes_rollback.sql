-- Operator-only rollback for migration 0047.
--
-- 0047's goose Down is forward-only by policy: the migration ledger stays at
-- 0047 so a later `up` cannot silently replay unrelated history. Disable
-- preparedDmDirectoryReads on every API instance before running this file;
-- the startup schema gate then refuses to start any instance that still asks
-- for the prepared path without its indexes.
DROP INDEX IF EXISTS c_dm.dm_conversations_low_cursor;
DROP INDEX IF EXISTS c_dm.dm_conversations_high_cursor;
