package authprivacy

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Installed state remains a closure dependency even with every feature off.
func checkRecommendationSchema(ctx context.Context, pool *pgxpool.Pool, required bool) error {
	var present, valid, owner bool
	if err := pool.QueryRow(ctx, `SELECT to_regnamespace('c_recommendation') IS NOT NULL OR EXISTS(SELECT 1 FROM pg_attribute WHERE attrelid=to_regclass('c_posts.post_likes') AND attname='liked_at' AND NOT attisdropped)`).Scan(&present); err != nil {
		return errPostSchema
	}
	if !present && !required {
		var tracker bool
		if pool.QueryRow(ctx, `SELECT to_regclass('public.goose_db_version') IS NOT NULL`).Scan(&tracker) != nil {
			return errPostSchema
		}
		if !tracker {
			return nil
		}
		if pool.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM public.goose_db_version WHERE version_id>=56 AND is_applied)`).Scan(&valid) != nil || !valid {
			return errPostSchema
		}
		return nil
	}
	tables := []string{"control", "accounts", "rounds", "pages", "intents", "history"}
	if pool.QueryRow(ctx, `SELECT count(*)=6 AND bool_and(c.relkind='r' AND c.relpersistence='p') FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='c_recommendation' AND c.relname=ANY($1::text[])`, tables).Scan(&valid) != nil || !valid {
		return errPostSchema
	}
	if pool.QueryRow(ctx, `SELECT count(*)=1 AND bool_and(singleton_id=1 AND version=1) FROM c_recommendation.control`).Scan(&valid) != nil || !valid {
		return errPostSchema
	}
	fields := map[string][]string{
		"control":  {"singleton_id", "version", "profile_hash", "max_rows", "max_bytes", "max_units", "max_file_bytes", "growth_bytes", "baseline_file_bytes", "write_debt_bytes", "reserved_writes", "security_epoch", "enabled", "admission_enabled", "feedback_enabled", "like_time_enabled", "live_rows", "live_bytes", "live_units", "reserved_rows", "reserved_bytes", "reserved_units"},
		"accounts": {"account_id", "epoch", "disabled", "revision", "last_at"},
		"rounds":   {"round_id", "account_id", "binding", "policy", "config_hash", "seed", "ids", "future_rows", "future_exposures", "future_writes", "revision", "created_at", "last_at", "expires_at", "feedback_until"},
		"pages":    {"round_id", "page_index", "first_at", "delivered", "omitted", "consumed"},
		"intents":  {"account_id", "key_digest", "round_id", "accept_until", "retain_until"},
		"history":  {"account_id", "post_id", "delivery_at", "delivery_until", "exposure_until", "due_at"},
	}
	for table, names := range fields {
		types := make([]string, len(names))
		for i, name := range names {
			typ := "bigint"
			switch {
			case name == "account_id" || name == "round_id" || name == "post_id":
				typ = "uuid"
			case name == "ids" || name == "future_rows" || name == "future_exposures":
				typ = "uuid[]"
			case name == "binding" || name == "policy" || name == "config_hash" || name == "seed" || name == "profile_hash" || name == "key_digest":
				typ = "bytea"
			case strings.HasSuffix(name, "_at") || strings.HasSuffix(name, "_until"):
				typ = "timestamptz"
			case name == "enabled" || name == "admission_enabled" || name == "feedback_enabled" || name == "like_time_enabled" || name == "disabled":
				typ = "boolean"
			case name == "singleton_id" || name == "version" || name == "future_writes" || name == "page_index" || name == "delivered" || name == "omitted" || name == "consumed":
				typ = "integer"
			}
			types[i] = typ
		}
		nullable := map[string][]string{"control": {"profile_hash"}, "pages": {"first_at"}, "history": {"delivery_at", "delivery_until", "exposure_until", "due_at"}}[table]
		if nullable == nil {
			nullable = []string{}
		}
		if pool.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM unnest($2::text[],$3::text[]) n(name,typ) WHERE NOT EXISTS(SELECT 1 FROM pg_attribute a WHERE a.attrelid=to_regclass('c_recommendation.'||$1) AND a.attname=n.name AND a.atttypid=to_regtype(n.typ) AND a.attnum>0 AND NOT a.attisdropped AND a.attnotnull=(NOT n.name=ANY($4::text[]))))`, table, names, types, nullable).Scan(&valid) != nil || !valid {
			return errPostSchema
		}
	}
	if pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_attribute WHERE attrelid='c_posts.post_likes'::regclass AND attname='liked_at' AND atttypid='timestamptz'::regtype AND NOT attnotnull AND NOT atthasdef AND NOT attisdropped)
 AND EXISTS(SELECT 1 FROM pg_trigger t JOIN pg_proc p ON p.oid=t.tgfoid WHERE t.tgrelid='c_auth.accounts'::regclass AND t.tgname='recommendation_account_closed' AND t.tgenabled='O' AND NOT t.tgisinternal AND p.oid='c_recommendation.clear_closed_account()'::regprocedure AND NOT p.prosecdef)
 AND NOT EXISTS(SELECT 1 FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname='c_recommendation' AND p.prosecdef)`).Scan(&valid) != nil || !valid {
		return errPostSchema
	}
	for _, table := range tables[1:] {
		if pool.QueryRow(ctx, `SELECT count(*)=4 AND bool_and(tgenabled='O' AND NOT tgisinternal AND tgfoid=CASE WHEN tgname='recommendation_guard' THEN 'c_recommendation.guard_state()'::regprocedure ELSE 'c_recommendation.account_change()'::regprocedure END) FROM pg_trigger WHERE tgrelid=to_regclass('c_recommendation.'||$1) AND tgname IN ('recommendation_count_insert','recommendation_count_update','recommendation_count_delete','recommendation_guard')`, table).Scan(&valid) != nil || !valid {
			return errPostSchema
		}
		if pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid=to_regclass('c_recommendation.'||$1) AND contype='p' AND convalidated AND NOT condeferrable) AND NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid=to_regclass('c_recommendation.'||$1) AND NOT convalidated)`, table).Scan(&valid) != nil || !valid {
			return errPostSchema
		}
		keys := map[string][]string{"accounts": {"account_id"}, "rounds": {"round_id"}, "pages": {"round_id", "page_index"}, "intents": {"account_id", "key_digest"}, "history": {"account_id", "post_id"}}
		if pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_constraint k WHERE k.conrelid=to_regclass('c_recommendation.'||$1) AND k.contype='p' AND NOT k.condeferrable AND k.convalidated AND k.conkey=ARRAY(SELECT a.attnum FROM unnest($2::text[]) WITH ORDINALITY n(name,pos) JOIN pg_attribute a ON a.attrelid=k.conrelid AND a.attname=n.name ORDER BY n.pos))`, table, keys[table]).Scan(&valid) != nil || !valid {
			return errPostSchema
		}
	}
	if err := checkRecommendationStructure(ctx, pool); err != nil {
		return err
	}
	// Owner fixtures prove structure only. Every other role, including randomized
	// isolated runtime roles, must pass the actual minimal-column privilege audit.
	if pool.QueryRow(ctx, `SELECT pg_get_userbyid(relowner)=current_user FROM pg_class WHERE oid='c_recommendation.control'::regclass`).Scan(&owner) != nil {
		return errPostSchema
	}
	if owner {
		return nil
	}
	columns := map[string][]string{
		"control":  {"live_rows", "live_bytes", "live_units", "reserved_rows", "reserved_bytes", "reserved_units", "write_debt_bytes", "reserved_writes"},
		"accounts": {"epoch", "disabled", "revision", "last_at"},
		"rounds":   {"future_rows", "future_exposures", "future_writes", "revision", "last_at"},
		"pages":    {"first_at", "delivered", "omitted", "consumed"},
		"intents":  {}, "history": {"delivery_at", "delivery_until", "exposure_until"},
	}
	for table, allowed := range columns {
		if pool.QueryRow(ctx, `SELECT has_table_privilege(current_user,'c_recommendation.'||$1,'SELECT')
 AND NOT has_table_privilege(current_user,'c_recommendation.'||$1,'UPDATE,TRUNCATE,TRIGGER,REFERENCES')
 AND NOT EXISTS(SELECT 1 FROM pg_attribute a WHERE a.attrelid=to_regclass('c_recommendation.'||$1) AND a.attnum>0 AND NOT a.attisdropped AND has_column_privilege(current_user,a.attrelid,a.attname,'UPDATE')<> (a.attname=ANY($2::text[])))`, table, allowed).Scan(&valid) != nil || !valid {
			return errPostSchema
		}
		if table != "control" {
			if pool.QueryRow(ctx, `SELECT has_table_privilege(current_user,'c_recommendation.'||$1,'INSERT') AND has_table_privilege(current_user,'c_recommendation.'||$1,'DELETE')`, table).Scan(&valid) != nil || !valid {
				return errPostSchema
			}
		}
	}
	if pool.QueryRow(ctx, `SELECT has_schema_privilege(current_user,'c_recommendation','USAGE') AND NOT has_schema_privilege(current_user,'c_recommendation','CREATE')
 AND NOT has_table_privilege(current_user,'c_recommendation.control','INSERT,DELETE')
 AND NOT EXISTS(SELECT 1 FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname='c_recommendation' AND NOT has_function_privilege(current_user,p.oid,'EXECUTE'))`).Scan(&valid) != nil || !valid {
		return errPostSchema
	}
	return nil
}
