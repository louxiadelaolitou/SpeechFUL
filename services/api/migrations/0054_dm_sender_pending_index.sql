-- +goose Up
-- +goose StatementBegin
-- DM sends count unacknowledged messages per account before accepting a new
-- one (community_direct_messages.go). The existing dm_pending index starts
-- with recipient, so it cannot serve the sender-side count; without this
-- partial index each send scans c_dm.messages, whose rows are retained after
-- ACK. Install with source writes quiesced for the same transactional
-- migration chain as 0041/0042/0047/0048, where CREATE INDEX CONCURRENTLY is
-- not permitted.
LOCK TABLE c_dm.messages IN SHARE ROW EXCLUSIVE MODE;
CREATE INDEX dm_sender_pending ON c_dm.messages(sender) WHERE NOT acknowledged;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Forward-only, like 0041/0042/0047/0048: dropping this index would silently
-- restore the sender-side scan. The ledger stays at 0054.
DO $$ BEGIN RAISE EXCEPTION 'DM sender pending index requires forward repair; Down is forbidden'; END $$;
-- +goose StatementEnd
