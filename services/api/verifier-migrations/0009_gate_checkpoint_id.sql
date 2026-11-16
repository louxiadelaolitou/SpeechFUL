-- +goose Up
ALTER TABLE v_auth.authorization_gate ADD COLUMN IF NOT EXISTS checkpoint_id uuid;

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Gate checkpoint downgrade requires reviewed material migration'; END $$;
-- +goose StatementEnd
