-- +goose Up
-- Nullable only for an existing signed legacy checkpoint. Its first successful
-- Gate transaction installs an ID; a NULL restored behind that ID is rejected.
ALTER TABLE c_auth.authorization_gate ADD COLUMN IF NOT EXISTS checkpoint_id uuid;

-- +goose Down
-- Downgrade requires an explicit, reviewed Gate material migration. Dropping
-- this field while retaining a signed checkpoint would discard rollback proof.
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Gate checkpoint downgrade requires reviewed material migration'; END $$;
-- +goose StatementEnd
