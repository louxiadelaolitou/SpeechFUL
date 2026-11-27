-- +goose Up
-- Raise operator-selectable ceilings for the confirmed 300-active workload.
-- Existing installed quotas, measured profile, counters and feature flags are
-- unchanged. Admission still requires a fresh measured service profile.
-- +goose StatementBegin
DO $$ BEGIN
 PERFORM singleton_id FROM c_recommendation.control WHERE singleton_id=1 FOR UPDATE;
 IF EXISTS(SELECT 1 FROM c_recommendation.control WHERE admission_enabled) THEN
  RAISE EXCEPTION 'pause recommendation admissions before capacity migration';
 END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE c_recommendation.control
 DROP CONSTRAINT control_max_rows_check,
 DROP CONSTRAINT control_max_bytes_check,
 DROP CONSTRAINT control_max_units_check,
 DROP CONSTRAINT control_max_file_bytes_check,
 ADD CONSTRAINT control_max_rows_check CHECK(max_rows BETWEEN 1 AND 1048576),
 ADD CONSTRAINT control_max_bytes_check CHECK(max_bytes BETWEEN 4096 AND 1073741824),
 ADD CONSTRAINT control_max_units_check CHECK(max_units BETWEEN 1 AND 4194304),
 ADD CONSTRAINT control_max_file_bytes_check CHECK(max_file_bytes BETWEEN 1048576 AND 8589934592);

-- +goose Down
-- The recommendation chain is forward-only once the upper ceilings are
-- installed; a partial downgrade would leave the chain inconsistent.
-- +goose StatementBegin
DO $$ BEGIN
 RAISE EXCEPTION '0030 capacity ceilings are forward-only; pause admissions and repair forward';
END $$;
-- +goose StatementEnd
