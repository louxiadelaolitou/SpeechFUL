-- +goose Up
-- Digests cover the signed proof and C operation key only. C never stores an
-- email address, an address hash, or an OTP.
ALTER TABLE c_auth.reset_intents ADD COLUMN proof_kind text NOT NULL DEFAULT 'RECOVERY_CODE'
    CHECK (proof_kind IN ('RECOVERY_CODE','EMAIL'));
ALTER TABLE c_auth.reset_intents ADD COLUMN proof_digest bytea
    CHECK (proof_digest IS NULL OR octet_length(proof_digest)=32);
ALTER TABLE c_auth.reset_intents ADD COLUMN operation_digest bytea
    CHECK (operation_digest IS NULL OR octet_length(operation_digest)=32);
CREATE UNIQUE INDEX email_reset_proof_once ON c_auth.reset_intents(proof_digest)
    WHERE proof_digest IS NOT NULL;
CREATE UNIQUE INDEX email_reset_operation_once ON c_auth.reset_intents(operation_digest)
    WHERE operation_digest IS NOT NULL;
ALTER TABLE c_auth.reset_intents ADD CONSTRAINT email_reset_proof_shape CHECK (
    (proof_kind='EMAIL') = (proof_digest IS NOT NULL) AND
    (proof_kind='EMAIL') = (operation_digest IS NOT NULL));

-- +goose StatementBegin
CREATE FUNCTION c_auth.guard_reset_proof_kind() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF (NEW.proof_kind,NEW.proof_digest,NEW.operation_digest) IS DISTINCT FROM (OLD.proof_kind,OLD.proof_digest,OLD.operation_digest) THEN
        RAISE EXCEPTION 'reset proof purpose and binding are immutable' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END
$$;
CREATE TRIGGER reset_proof_kind_immutable BEFORE UPDATE ON c_auth.reset_intents
    FOR EACH ROW EXECUTE FUNCTION c_auth.guard_reset_proof_kind();
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION 'Email password reset requires freeze and forward repair'; END $$;
-- +goose StatementEnd
