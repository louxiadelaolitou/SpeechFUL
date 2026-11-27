-- +goose Up
-- Private, finite recommendation state. No feature or collection is enabled.
CREATE SCHEMA c_recommendation;
CREATE TABLE c_recommendation.control (
 singleton_id integer PRIMARY KEY CHECK(singleton_id=1),
 version integer NOT NULL CHECK(version=1),
 enabled boolean NOT NULL DEFAULT false,
 admission_enabled boolean NOT NULL DEFAULT false,
 feedback_enabled boolean NOT NULL DEFAULT false,
 like_time_enabled boolean NOT NULL DEFAULT false,
 security_epoch bigint NOT NULL DEFAULT 0 CHECK(security_epoch>=0),
 profile_hash bytea CHECK(profile_hash IS NULL OR octet_length(profile_hash)=32),
 max_rows bigint NOT NULL DEFAULT 8192 CHECK(max_rows BETWEEN 1 AND 65536),
 max_bytes bigint NOT NULL DEFAULT 16777216 CHECK(max_bytes BETWEEN 4096 AND 268435456),
 max_units bigint NOT NULL DEFAULT 16384 CHECK(max_units BETWEEN 1 AND 262144),
 max_file_bytes bigint NOT NULL DEFAULT 1073741824 CHECK(max_file_bytes BETWEEN 1048576 AND 1073741824),
 growth_bytes bigint NOT NULL DEFAULT 65536 CHECK(growth_bytes BETWEEN 8192 AND 1048576),
 baseline_file_bytes bigint NOT NULL DEFAULT 0 CHECK(baseline_file_bytes>=0),
 write_debt_bytes bigint NOT NULL DEFAULT 0 CHECK(write_debt_bytes>=0),
 reserved_writes bigint NOT NULL DEFAULT 0 CHECK(reserved_writes>=0),
 live_rows bigint NOT NULL DEFAULT 0 CHECK(live_rows>=0),
 live_bytes bigint NOT NULL DEFAULT 0 CHECK(live_bytes>=0),
 live_units bigint NOT NULL DEFAULT 0 CHECK(live_units>=0),
 reserved_rows bigint NOT NULL DEFAULT 0 CHECK(reserved_rows>=0),
 reserved_bytes bigint NOT NULL DEFAULT 0 CHECK(reserved_bytes>=0),
 reserved_units bigint NOT NULL DEFAULT 0 CHECK(reserved_units>=0),
 CHECK(live_rows+reserved_rows<=max_rows),
 CHECK(live_bytes+reserved_bytes<=max_bytes),
 CHECK(live_units+reserved_units<=max_units)
);
INSERT INTO c_recommendation.control(singleton_id,version) VALUES(1,1);

-- +goose StatementBegin
CREATE FUNCTION c_recommendation.unique_ids(ids uuid[]) RETURNS boolean
LANGUAGE sql IMMUTABLE STRICT SET search_path=pg_catalog AS $$
 SELECT cardinality(ids)<=100 AND NOT EXISTS(SELECT 1 FROM unnest(ids) x WHERE x IS NULL OR x='00000000-0000-0000-0000-000000000000'::uuid)
 AND cardinality(ids)=(SELECT count(DISTINCT x) FROM unnest(ids) x)
$$;
-- +goose StatementEnd

CREATE TABLE c_recommendation.accounts (
 account_id uuid PRIMARY KEY REFERENCES c_auth.accounts(account_id),
 epoch bigint NOT NULL DEFAULT 0 CHECK(epoch>=0),
 disabled boolean NOT NULL DEFAULT false,
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
 last_at timestamptz NOT NULL
);
-- Only never-revoked active accounts may be reclaimed without a durable epoch.
CREATE INDEX recommendation_account_reclaim ON c_recommendation.accounts(last_at,account_id) WHERE NOT disabled AND epoch=0;
CREATE TABLE c_recommendation.rounds (
 round_id uuid PRIMARY KEY,
 account_id uuid NOT NULL REFERENCES c_recommendation.accounts(account_id) ON DELETE CASCADE,
 binding bytea NOT NULL CHECK(octet_length(binding) BETWEEN 1 AND 1024),
 policy bytea NOT NULL CHECK(octet_length(policy) BETWEEN 1 AND 512),
 config_hash bytea NOT NULL CHECK(octet_length(config_hash)=32),
 seed bytea NOT NULL CHECK(octet_length(seed)=8),
 ids uuid[] NOT NULL CHECK(c_recommendation.unique_ids(ids)),
 future_rows uuid[] NOT NULL CHECK(c_recommendation.unique_ids(future_rows) AND future_rows<@ids),
 future_exposures uuid[] NOT NULL CHECK(c_recommendation.unique_ids(future_exposures) AND future_exposures<@ids),
 future_writes integer NOT NULL CHECK(future_writes BETWEEN 0 AND 2048),
 revision bigint NOT NULL CHECK(revision>0),
 created_at timestamptz NOT NULL,
 last_at timestamptz NOT NULL CHECK(last_at>=created_at),
 expires_at timestamptz NOT NULL CHECK(expires_at>created_at AND expires_at<=created_at+interval '30 minutes'),
 feedback_until timestamptz NOT NULL CHECK(feedback_until>expires_at AND feedback_until<=expires_at+interval '10 minutes'),
 CHECK(last_at<=feedback_until)
);
CREATE INDEX recommendation_round_owner ON c_recommendation.rounds(account_id,round_id);
CREATE INDEX recommendation_round_expiry ON c_recommendation.rounds(feedback_until,round_id);
CREATE TABLE c_recommendation.pages (
 round_id uuid NOT NULL REFERENCES c_recommendation.rounds(round_id) ON DELETE CASCADE,
 page_index integer NOT NULL CHECK(page_index BETWEEN 0 AND 9),
 first_at timestamptz,
 delivered integer NOT NULL DEFAULT 0 CHECK(delivered BETWEEN 0 AND 1023),
 omitted integer NOT NULL DEFAULT 0 CHECK(omitted BETWEEN 0 AND 1023),
 consumed integer NOT NULL DEFAULT 0 CHECK(consumed BETWEEN 0 AND 1023 AND (consumed & ~delivered)=0),
 PRIMARY KEY(round_id,page_index),
 CHECK(first_at IS NOT NULL OR (delivered=0 AND omitted=0 AND consumed=0))
);
CREATE TABLE c_recommendation.intents (
 account_id uuid NOT NULL REFERENCES c_recommendation.accounts(account_id) ON DELETE CASCADE,
 key_digest bytea NOT NULL CHECK(octet_length(key_digest)=32),
 round_id uuid NOT NULL,
 accept_until timestamptz NOT NULL,
 retain_until timestamptz NOT NULL CHECK(retain_until>accept_until AND retain_until<=accept_until+interval '5 minutes'),
 PRIMARY KEY(account_id,key_digest)
);
CREATE INDEX recommendation_intent_expiry ON c_recommendation.intents(retain_until,account_id,key_digest);
CREATE TABLE c_recommendation.history (
 account_id uuid NOT NULL REFERENCES c_recommendation.accounts(account_id) ON DELETE CASCADE,
 post_id uuid NOT NULL,
 delivery_at timestamptz,
 delivery_until timestamptz,
 exposure_until timestamptz,
 due_at timestamptz GENERATED ALWAYS AS (least(delivery_until,exposure_until)) STORED,
 PRIMARY KEY(account_id,post_id),
 CHECK((delivery_at IS NULL)=(delivery_until IS NULL)),
 CHECK(delivery_until IS NULL OR (delivery_until>delivery_at AND delivery_until<=delivery_at+interval '5 minutes')),
 CHECK(delivery_until IS NOT NULL OR exposure_until IS NOT NULL)
);
CREATE INDEX recommendation_history_expiry ON c_recommendation.history(due_at,account_id,post_id);

-- No default/backfill: old and same-value like writes remain time-unknown.
ALTER TABLE c_posts.post_likes ADD COLUMN liked_at timestamptz;
CREATE INDEX recommendation_like_time ON c_posts.post_likes(liked_at DESC,post_id,account_id) WHERE liked_at IS NOT NULL;
CREATE INDEX recommendation_like_post_time ON c_posts.post_likes(post_id,liked_at DESC,account_id) WHERE liked_at IS NOT NULL;
CREATE INDEX recommendation_favorite_time ON c_posts.post_favorites(saved_at DESC,post_id,account_id);
CREATE INDEX recommendation_favorite_post_time ON c_posts.post_favorites(post_id,saved_at DESC,account_id);
CREATE INDEX recommendation_comment_time ON c_comments.attempts(terminal_at DESC,task_id) WHERE state='PUBLISHED';

-- All accounting is invoker-only and transactional, including cascades. Actual
-- tuple bytes are measured, future history bytes are conservatively reserved.
-- Index/bloat/file allocation uses an operational fresh-admission watermark;
-- neither tuple bytes nor successful-mutation telemetry is a disk hard bound.
-- +goose StatementBegin
CREATE FUNCTION c_recommendation.account_change() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE n bigint:=0; o bigint:=0; nb bigint:=0; ob bigint:=0;
 nr bigint:=0; orows bigint:=0; ne bigint:=0; oe bigint:=0; nw bigint:=0; ow bigint:=0; nu bigint:=0; ou bigint:=0;
BEGIN
 IF TG_OP<>'DELETE' THEN
  SELECT count(*),COALESCE(sum(pg_column_size(r)),0) INTO n,nb FROM new_rows r;
  nu:=n*4;
  IF TG_TABLE_NAME='history' THEN SELECT COALESCE(sum(4+CASE WHEN delivery_until IS NOT NULL AND exposure_until IS NOT NULL THEN 4 ELSE 0 END),0) INTO nu FROM new_rows; END IF;
  IF TG_TABLE_NAME='accounts' THEN SELECT COALESCE(sum(CASE WHEN disabled THEN 2 ELSE 4 END),0) INTO nu FROM new_rows; END IF;
  IF TG_TABLE_NAME='rounds' THEN
   SELECT COALESCE(sum(cardinality(future_rows)),0),COALESCE(sum(cardinality(future_exposures)),0),COALESCE(sum(future_writes),0) INTO nr,ne,nw FROM new_rows;
  END IF;
 END IF;
 IF TG_OP<>'INSERT' THEN
  SELECT count(*),COALESCE(sum(pg_column_size(r)),0) INTO o,ob FROM old_rows r;
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
-- +goose StatementBegin
DO $$ DECLARE t text; BEGIN
 FOREACH t IN ARRAY ARRAY['accounts','rounds','pages','intents','history'] LOOP
  EXECUTE format('CREATE TRIGGER recommendation_count_insert AFTER INSERT ON c_recommendation.%I REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION c_recommendation.account_change()',t);
  EXECUTE format('CREATE TRIGGER recommendation_count_update AFTER UPDATE ON c_recommendation.%I REFERENCING OLD TABLE AS old_rows NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION c_recommendation.account_change()',t);
  EXECUTE format('CREATE TRIGGER recommendation_count_delete AFTER DELETE ON c_recommendation.%I REFERENCING OLD TABLE AS old_rows FOR EACH STATEMENT EXECUTE FUNCTION c_recommendation.account_change()',t);
 END LOOP;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION c_recommendation.guard_state() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF TG_OP='UPDATE' THEN
  IF TG_TABLE_NAME='control' THEN
   IF NEW.security_epoch<OLD.security_epoch THEN RAISE EXCEPTION 'recommendation security epoch moved backwards'; END IF;
   -- Ordinary profile replacement is stop-new, drain, replace, resume. All old
   -- processes keep their matching profile throughout the drain. First install
   -- may set a profile and admit together, but only with no pending promises.
   IF NEW.profile_hash IS DISTINCT FROM OLD.profile_hash THEN
    IF (OLD.profile_hash IS NOT NULL AND (OLD.admission_enabled OR NEW.admission_enabled))
     OR EXISTS(SELECT 1 FROM c_recommendation.rounds)
     OR EXISTS(SELECT 1 FROM c_recommendation.intents)
     THEN RAISE EXCEPTION 'pause admissions and drain recommendation promises before profile replacement'; END IF;
   END IF;
  ELSIF TG_TABLE_NAME='rounds' THEN
   IF ((NEW.round_id,NEW.account_id,NEW.binding,NEW.policy,NEW.config_hash,NEW.seed,NEW.ids,NEW.created_at,NEW.expires_at,NEW.feedback_until)
    IS DISTINCT FROM (OLD.round_id,OLD.account_id,OLD.binding,OLD.policy,OLD.config_hash,OLD.seed,OLD.ids,OLD.created_at,OLD.expires_at,OLD.feedback_until)
    OR NEW.revision<=OLD.revision OR NEW.last_at<OLD.last_at OR NOT NEW.future_rows<@OLD.future_rows OR NOT NEW.future_exposures<@OLD.future_exposures OR NEW.future_writes>OLD.future_writes)
    THEN RAISE EXCEPTION 'immutable recommendation round changed'; END IF;
  ELSIF TG_TABLE_NAME='pages' THEN
   IF ((NEW.round_id,NEW.page_index) IS DISTINCT FROM (OLD.round_id,OLD.page_index)
    OR (OLD.first_at IS NOT NULL AND (NEW.first_at,NEW.delivered) IS DISTINCT FROM (OLD.first_at,OLD.delivered))
    OR (NEW.omitted | OLD.omitted)<>NEW.omitted OR (NEW.consumed | OLD.consumed)<>NEW.consumed)
    THEN RAISE EXCEPTION 'nonmonotone recommendation page'; END IF;
  ELSIF TG_TABLE_NAME='accounts' THEN
   IF (NEW.account_id<>OLD.account_id OR NEW.epoch<OLD.epoch OR NEW.revision<=OLD.revision OR NEW.last_at<OLD.last_at)
    THEN RAISE EXCEPTION 'nonmonotone recommendation account'; END IF;
  ELSIF TG_TABLE_NAME='intents' THEN RAISE EXCEPTION 'immutable recommendation receipt';
  ELSIF TG_TABLE_NAME='history' THEN
   IF ((NEW.account_id,NEW.post_id) IS DISTINCT FROM (OLD.account_id,OLD.post_id)
    OR (NEW.exposure_until IS NOT NULL AND OLD.exposure_until IS NOT NULL AND NEW.exposure_until<OLD.exposure_until)
    OR (NEW.delivery_at IS NOT NULL AND OLD.delivery_at IS NOT NULL AND NEW.delivery_at<OLD.delivery_at)
    OR (NEW.delivery_until IS NOT NULL AND OLD.delivery_until IS NOT NULL AND NEW.delivery_until<OLD.delivery_until))
    THEN RAISE EXCEPTION 'recommendation history moved backwards'; END IF;
  END IF;
 END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER recommendation_control_guard BEFORE UPDATE ON c_recommendation.control
 FOR EACH ROW EXECUTE FUNCTION c_recommendation.guard_state();
-- +goose StatementBegin
DO $$ DECLARE t text; BEGIN
 FOREACH t IN ARRAY ARRAY['accounts','rounds','pages','intents','history'] LOOP
  EXECUTE format('CREATE TRIGGER recommendation_guard BEFORE UPDATE ON c_recommendation.%I FOR EACH ROW EXECUTE FUNCTION c_recommendation.guard_state()',t);
 END LOOP;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION c_recommendation.clear_closed_account() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF NEW.state='CLOSED' AND OLD.state IS DISTINCT FROM NEW.state THEN
  DELETE FROM c_recommendation.accounts WHERE account_id=NEW.account_id;
 END IF;
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER recommendation_account_closed AFTER UPDATE OF state ON c_auth.accounts
 FOR EACH ROW EXECUTE FUNCTION c_recommendation.clear_closed_account();
REVOKE ALL ON SCHEMA c_recommendation FROM PUBLIC;
REVOKE ALL ON ALL TABLES IN SCHEMA c_recommendation FROM PUBLIC;
REVOKE ALL ON ALL FUNCTIONS IN SCHEMA c_recommendation FROM PUBLIC;

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION '0028 contains private recommendation receipts; disable and repair forward'; END $$;
-- +goose StatementEnd
