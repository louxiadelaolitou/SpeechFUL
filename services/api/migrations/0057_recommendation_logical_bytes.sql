-- +goose Up
-- Apply atomically through Goose during an explicitly
-- authorized maintenance window; no application/worker may run concurrently.
-- Logical quota units: binary field encoding + 32 bytes per row for additional
-- accounting overhead. Independent of tuple TOAST/compression/header layout.
-- NOT heap/index/WAL/bloat/disk bytes. Existing file watermark remains separate.
-- Binary encoding includes field headers. A full six-column history row costs
-- 4 + 6*8 + 2*16 + 4*8 + 32 = 148 bytes, below its 1024-byte future reservation.
SET LOCAL lock_timeout='5s';
-- Match the control-before-state business lock order. NOWAIT also rejects a
-- direct DML writer which already holds a state-table lock and is waiting on
-- its accounting trigger's control lock; never form a maintenance deadlock.
LOCK TABLE c_recommendation.control,c_recommendation.accounts,
 c_recommendation.rounds,c_recommendation.pages,c_recommendation.intents,
 c_recommendation.history IN ACCESS EXCLUSIVE MODE NOWAIT;
-- +goose StatementBegin
DO $$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM c_recommendation.control WHERE singleton_id=1
  AND NOT enabled AND NOT admission_enabled AND NOT feedback_enabled
  AND NOT like_time_enabled AND profile_hash IS NULL) THEN
  RAISE EXCEPTION '0029 requires explicitly closed, unconfigured recommendation control';
 END IF;
END $$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE FUNCTION c_recommendation.logical_row_bytes(encoded bytea) RETURNS bigint
LANGUAGE sql IMMUTABLE STRICT SET search_path=pg_catalog AS $$
 SELECT 32::bigint + octet_length(encoded)::bigint
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION c_recommendation.logical_row_bytes(bytea) FROM PUBLIC;
-- Reapply the existing grant-community.sql under the deployment's role mapping
-- before starting runtime. It grants EXECUTE on all recommendation functions.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION c_recommendation.account_change() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE n bigint:=0; o bigint:=0; nb bigint:=0; ob bigint:=0;
 nr bigint:=0; orows bigint:=0; ne bigint:=0; oe bigint:=0; nw bigint:=0; ow bigint:=0; nu bigint:=0; ou bigint:=0;
BEGIN
 IF TG_OP<>'DELETE' THEN
  SELECT count(*),COALESCE(sum(c_recommendation.logical_row_bytes(pg_catalog.record_send(r))),0) INTO n,nb FROM new_rows r;
  nu:=n*4;
  IF TG_TABLE_NAME='history' THEN SELECT COALESCE(sum(4+CASE WHEN delivery_until IS NOT NULL AND exposure_until IS NOT NULL THEN 4 ELSE 0 END),0) INTO nu FROM new_rows; END IF;
  IF TG_TABLE_NAME='accounts' THEN SELECT COALESCE(sum(CASE WHEN disabled THEN 2 ELSE 4 END),0) INTO nu FROM new_rows; END IF;
  IF TG_TABLE_NAME='rounds' THEN
   SELECT COALESCE(sum(cardinality(future_rows)),0),COALESCE(sum(cardinality(future_exposures)),0),COALESCE(sum(future_writes),0) INTO nr,ne,nw FROM new_rows;
  END IF;
 END IF;
 IF TG_OP<>'INSERT' THEN
  SELECT count(*),COALESCE(sum(c_recommendation.logical_row_bytes(pg_catalog.record_send(r))),0) INTO o,ob FROM old_rows r;
  ou:=o*4;
  IF TG_TABLE_NAME='history' THEN SELECT COALESCE(sum(4+CASE WHEN delivery_until IS NOT NULL AND exposure_until IS NOT NULL THEN 4 ELSE 0 END),0) INTO ou FROM old_rows; END IF;
  IF TG_TABLE_NAME='accounts' THEN SELECT COALESCE(sum(CASE WHEN disabled THEN 2 ELSE 4 END),0) INTO ou FROM old_rows; END IF;
  IF TG_TABLE_NAME='rounds' THEN
   SELECT COALESCE(sum(cardinality(future_rows)),0),COALESCE(sum(cardinality(future_exposures)),0),COALESCE(sum(future_writes),0) INTO orows,oe,ow FROM old_rows;
  END IF;
 END IF;
 IF n+o=0 THEN RETURN NULL; END IF;
 UPDATE c_recommendation.control SET live_rows=live_rows+n-o,live_bytes=live_bytes+nb-ob,live_units=live_units+nu-ou,
  reserved_rows=reserved_rows+nr-orows+ne-oe,reserved_bytes=reserved_bytes+(nr-orows+ne-oe)*1024,
  reserved_units=reserved_units+(nr-orows)*8+(ne-oe)*4,reserved_writes=reserved_writes+nw-ow,
  -- Saturating successful-mutation telemetry only. It is never a file quota.
  write_debt_bytes=write_debt_bytes+least((n+o)*growth_bytes,9223372036854775807-write_debt_bytes) WHERE singleton_id=1;
 IF NOT FOUND THEN RAISE EXCEPTION 'recommendation accounting unavailable'; END IF;
 RETURN NULL;
END $$;
-- +goose StatementEnd
-- Derive the new balance from ACTUAL retained rows while all six tables remain
-- locked. Never subtract an alleged phantom charge or write a fixed zero.
-- Existing quota constraints abort the entire migration on an excessive sum.
UPDATE c_recommendation.control SET live_bytes=(
 SELECT COALESCE(sum(n),0) FROM (
  SELECT c_recommendation.logical_row_bytes(pg_catalog.record_send(r)) n FROM c_recommendation.accounts r
  UNION ALL SELECT c_recommendation.logical_row_bytes(pg_catalog.record_send(r)) FROM c_recommendation.rounds r
  UNION ALL SELECT c_recommendation.logical_row_bytes(pg_catalog.record_send(r)) FROM c_recommendation.pages r
  UNION ALL SELECT c_recommendation.logical_row_bytes(pg_catalog.record_send(r)) FROM c_recommendation.intents r
  UNION ALL SELECT c_recommendation.logical_row_bytes(pg_catalog.record_send(r)) FROM c_recommendation.history r
 ) actual_rows
) WHERE singleton_id=1;
-- No modification to debt, reservations, units, auth, content, clocks or Gate.
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 RAISE EXCEPTION '0029 is forward-only: restoring physical-size accounting is unsafe';
END $$;
-- +goose StatementEnd
