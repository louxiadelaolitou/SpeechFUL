-- +goose Up
ALTER TABLE c_auth.reset_intents ADD COLUMN change_session_digest bytea
    CHECK (change_session_digest IS NULL OR
           (proof_kind = 'EMAIL' AND octet_length(change_session_digest) = 32));

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION c_auth.guard_reset_proof_kind() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF (NEW.proof_kind,NEW.proof_digest,NEW.operation_digest,NEW.change_session_digest)
        IS DISTINCT FROM (OLD.proof_kind,OLD.proof_digest,OLD.operation_digest,OLD.change_session_digest) THEN
        RAISE EXCEPTION 'reset proof purpose and binding are immutable' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END
$$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Password change requires freeze and forward repair'; END $$;
-- +goose StatementEnd
