-- +goose Up
-- RESET uses the existing address/device budgets and immutable OTP-purpose
-- trigger, but cannot be confirmed as LOGIN or REGISTER. Confirmation receipts
-- share the already bounded store with distinct request/key MAC domains.
ALTER TABLE v_auth.otp_flows DROP CONSTRAINT otp_flows_purpose_check;
ALTER TABLE v_auth.otp_flows ADD CONSTRAINT otp_flows_purpose_check
    CHECK (purpose IN ('REGISTER', 'LOGIN', 'RESET'));

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Email reset requires freeze and forward repair'; END $$;
-- +goose StatementEnd
