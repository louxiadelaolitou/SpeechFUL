package authprivacy

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// A finite structural contract for the constraints/queues/lifecycle used by
// this implementation. This is not a hash of the whole schema or function body.
func checkRecommendationStructure(ctx context.Context, pool *pgxpool.Pool) error {
	var valid bool
	// C29 changes the byte-quota unit. Reject a missing, altered, definer-rights
	// or physical-tuple-size accounting function even while the feature is off.
	// Hash only normalized function bodies; signatures and execution properties
	// are checked separately. A function change requires an explicit new pin.
	if pool.QueryRow(ctx, `SELECT count(*)=2 AND COALESCE(bool_and(
 NOT p.prosecdef AND p.prokind='f' AND p.prorettype=to_regtype(e.result)
 AND l.lanname=e.language AND p.provolatile::text=e.volatility
 AND p.proisstrict=e.strict AND p.proconfig=ARRAY['search_path=pg_catalog']
 AND encode(sha256(convert_to(regexp_replace(p.prosrc,'[[:space:]]','','g'),'UTF8')),'hex')=e.digest),false)
 FROM (VALUES
 ('c_recommendation.logical_row_bytes(bytea)','bigint','sql','i',true,'e5242a2450303e59a31f5e6d46e2a1fadcf9c53893a80537e7549f34557624fb'),
 ('c_recommendation.account_change()','trigger','plpgsql','v',false,'edb4bfdcad6045aaf1ed78f8b197cc065c19ff5b437f8f9dd45e252c019ebc5c')
 ) e(signature,result,language,volatility,strict,digest)
 JOIN pg_proc p ON p.oid=to_regprocedure(e.signature) JOIN pg_language l ON l.oid=p.prolang`).Scan(&valid) != nil || !valid {
		return errPostSchema
	}
	for table, checks := range recommendationRequiredChecks {
		// C30 only expands operator ceilings. Accept either complete, exact
		// set so the new binary can drain a retained C29 before migration.
		// Mixed or altered constraints remain a schema failure.
		if table == "control" {
			var expanded bool
			if pool.QueryRow(ctx, `SELECT regexp_replace(pg_get_constraintdef(oid),'[[:space:]]','','g')=regexp_replace('CHECK (((max_units >= 1) AND (max_units <= 4194304)))','[[:space:]]','','g') FROM pg_constraint WHERE conrelid='c_recommendation.control'::regclass AND conname='control_max_units_check'`).Scan(&expanded) != nil {
				return errPostSchema
			}
			if expanded {
				copy := make(map[string]string, len(checks))
				for name, definition := range checks {
					copy[name] = definition
				}
				copy["control_max_rows_check"] = "CHECK (((max_rows >= 1) AND (max_rows <= 1048576)))"
				copy["control_max_bytes_check"] = "CHECK (((max_bytes >= 4096) AND (max_bytes <= 1073741824)))"
				copy["control_max_units_check"] = "CHECK (((max_units >= 1) AND (max_units <= 4194304)))"
				copy["control_max_file_bytes_check"] = "CHECK (((max_file_bytes >= 1048576) AND (max_file_bytes <= '8589934592'::bigint)))"
				checks = copy
			}
		}
		if table == "pages" {
			// pg_dump prints this CHECK without the redundant inner parentheses
			// and pg_restore parses that text into a flat AND list. PG16 keeps
			// the parse-time shape, so an untouched schema would otherwise be
			// rejected after backup/restore. Accept exactly that spelling; an
			// altered bound still fails the exact-set comparison below.
			flat := "CHECK (((consumed >= 0) AND (consumed <= 1023) AND ((consumed & (~ delivered)) = 0)))"
			var restored bool
			if pool.QueryRow(ctx, `SELECT COALESCE((SELECT regexp_replace(pg_get_constraintdef(oid),'[[:space:]]','','g')=regexp_replace($1,'[[:space:]]','','g') FROM pg_constraint WHERE conrelid='c_recommendation.pages'::regclass AND conname='pages_check'),false)`, flat).Scan(&restored) != nil {
				return errPostSchema
			}
			if restored {
				copy := make(map[string]string, len(checks))
				for name, definition := range checks {
					copy[name] = definition
				}
				copy["pages_check"] = flat
				checks = copy
			}
		}
		names, definitions := []string{}, []string{}
		for name, definition := range checks {
			names = append(names, name)
			definitions = append(definitions, definition)
		}
		if pool.QueryRow(ctx, `SELECT count(*)=$4 AND COALESCE(bool_and(k.convalidated AND NOT k.condeferrable AND k.contype='c'
 AND regexp_replace(pg_get_constraintdef(k.oid),'[[:space:]]','','g')=regexp_replace(n.definition,'[[:space:]]','','g')),false)
 FROM unnest($2::text[],$3::text[]) n(name,definition) JOIN pg_constraint k ON k.conrelid=to_regclass('c_recommendation.'||$1) AND k.conname=n.name`, table, names, definitions, len(names)).Scan(&valid) != nil || !valid {
			return errPostSchema
		}
	}
	for _, fk := range [][4]string{
		{"accounts", "c_auth.accounts", "account_id", "a"},
		{"rounds", "c_recommendation.accounts", "account_id", "c"},
		{"intents", "c_recommendation.accounts", "account_id", "c"},
		{"history", "c_recommendation.accounts", "account_id", "c"},
		{"pages", "c_recommendation.rounds", "round_id", "c"},
	} {
		if pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_constraint k
 JOIN pg_attribute a ON a.attrelid=k.conrelid AND a.attname=$3
 JOIN pg_attribute p ON p.attrelid=k.confrelid AND p.attname=$3
 WHERE k.conrelid=to_regclass('c_recommendation.'||$1) AND k.confrelid=to_regclass($2) AND k.contype='f'
 AND k.conkey=ARRAY[a.attnum] AND k.confkey=ARRAY[p.attnum] AND k.confdeltype::text=$4 AND k.confupdtype='a'
 AND k.convalidated AND NOT k.condeferrable)`, fk[0], fk[1], fk[2], fk[3]).Scan(&valid) != nil || !valid {
			return errPostSchema
		}
	}
	for _, index := range []struct {
		name, table, predicate string
		columns                []string
	}{
		{"recommendation_history_expiry", "history", "", []string{"due_at", "account_id", "post_id"}},
		{"recommendation_round_expiry", "rounds", "", []string{"feedback_until", "round_id"}},
		{"recommendation_intent_expiry", "intents", "", []string{"retain_until", "account_id", "key_digest"}},
		{"recommendation_round_owner", "rounds", "", []string{"account_id", "round_id"}},
		{"recommendation_account_reclaim", "accounts", "((NOT disabled) AND (epoch = 0))", []string{"last_at", "account_id"}},
	} {
		if pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_index i JOIN pg_class x ON x.oid=i.indexrelid JOIN pg_am am ON am.oid=x.relam
 WHERE i.indexrelid=to_regclass('c_recommendation.'||$1) AND i.indrelid=to_regclass('c_recommendation.'||$2)
 AND i.indisvalid AND i.indisready AND i.indislive AND NOT i.indisunique AND am.amname='btree'
 AND i.indnatts=cardinality($3::text[]) AND i.indnkeyatts=i.indnatts AND i.indexprs IS NULL
 AND ARRAY(SELECT k FROM unnest(i.indkey) WITH ORDINALITY t(k,n) ORDER BY n)=ARRAY(SELECT a.attnum FROM unnest($3::text[]) WITH ORDINALITY n(name,pos) JOIN pg_attribute a ON a.attrelid=i.indrelid AND a.attname=n.name ORDER BY n.pos)
 AND NOT EXISTS(SELECT 1 FROM unnest(i.indoption) o WHERE o<>0)
 AND regexp_replace(COALESCE(pg_get_expr(i.indpred,i.indrelid),''),'[[:space:]]','','g')=regexp_replace($4,'[[:space:]]','','g'))`, index.name, index.table, index.columns, index.predicate).Scan(&valid) != nil || !valid {
			return errPostSchema
		}
	}
	if pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_attribute a JOIN pg_attrdef d ON d.adrelid=a.attrelid AND d.adnum=a.attnum
 WHERE a.attrelid='c_recommendation.history'::regclass AND a.attname='due_at' AND a.attgenerated='s'
 AND pg_get_expr(d.adbin,d.adrelid)='LEAST(delivery_until, exposure_until)')`).Scan(&valid) != nil || !valid {
		return errPostSchema
	}
	for _, table := range []string{"accounts", "rounds", "pages", "intents", "history"} {
		for _, trigger := range []struct {
			name, function, oldTable, newTable string
			kind                               int
		}{
			{"recommendation_count_insert", "account_change", "", "new_rows", 4},
			{"recommendation_count_update", "account_change", "old_rows", "new_rows", 16},
			{"recommendation_count_delete", "account_change", "old_rows", "", 8},
			{"recommendation_guard", "guard_state", "", "", 19},
		} {
			if !recommendationTriggerMatches(ctx, pool, "c_recommendation."+table, trigger.name, trigger.function, trigger.kind, "", trigger.oldTable, trigger.newTable) {
				return errPostSchema
			}
		}
	}
	if !recommendationTriggerMatches(ctx, pool, "c_recommendation.control", "recommendation_control_guard", "guard_state", 19, "", "", "") {
		return errPostSchema
	}
	var stateColumn string
	if pool.QueryRow(ctx, `SELECT attnum::text FROM pg_attribute WHERE attrelid='c_auth.accounts'::regclass AND attname='state' AND NOT attisdropped`).Scan(&stateColumn) != nil ||
		!recommendationTriggerMatches(ctx, pool, "c_auth.accounts", "recommendation_account_closed", "clear_closed_account", 17, stateColumn, "", "") {
		return errPostSchema
	}
	return nil
}

func recommendationTriggerMatches(ctx context.Context, pool *pgxpool.Pool, table, name, function string, kind int, columns, oldTable, newTable string) bool {
	var valid bool
	err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_trigger t WHERE t.tgrelid=to_regclass($1) AND t.tgname=$2
 AND t.tgfoid=to_regprocedure('c_recommendation.'||$3||'()') AND t.tgtype=$4 AND t.tgenabled='O' AND NOT t.tgisinternal
 AND t.tgattr::text=$5 AND COALESCE(t.tgoldtable::text,'')=$6 AND COALESCE(t.tgnewtable::text,'')=$7
 AND t.tgqual IS NULL AND t.tgnargs=0 AND t.tgconstraint=0 AND NOT t.tgdeferrable AND NOT t.tginitdeferred)`, table, name, function, kind, columns, oldTable, newTable).Scan(&valid)
	return err == nil && valid
}
