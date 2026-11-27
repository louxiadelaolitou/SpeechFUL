package authprivacy

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestRecommendationCapacityCeilingMigrationPostgres(t *testing.T) {
	p := newRecommendationPG(t, 20)
	ctx := context.Background()
	a := sessionTestAccount(t, p.l, "capacity_ceiling")
	p.start(t, a, p.request(t, a))
	raw, err := os.ReadFile("../../migrations/0058_recommendation_capacity_ceiling.sql")
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.SplitN(string(raw), "-- +goose Down", 2)
	exec := func(sql string, success bool) {
		t.Helper()
		tx, e := p.l.cp.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		defer tx.Rollback(ctx)
		_, e = tx.Exec(ctx, sql)
		if (e == nil) != success {
			t.Fatalf("capacity migration expected success=%v: %v", success, e)
		}
		if success {
			if e = tx.Commit(ctx); e != nil {
				t.Fatal(e)
			}
		}
	}
	snapshot := func() string {
		var value string
		if e := p.l.cp.QueryRow(ctx, `SELECT md5(string_agg(x::text,E'\n' ORDER BY x::text)) FROM (SELECT to_jsonb(t) x FROM c_recommendation.control t UNION ALL SELECT to_jsonb(t) FROM c_recommendation.accounts t UNION ALL SELECT to_jsonb(t) FROM c_recommendation.rounds t UNION ALL SELECT to_jsonb(t) FROM c_recommendation.pages t UNION ALL SELECT to_jsonb(t) FROM c_recommendation.intents t UNION ALL SELECT to_jsonb(t) FROM c_recommendation.history t) q`).Scan(&value); e != nil {
			t.Fatal(e)
		}
		return value
	}
	before := snapshot()
	exec(parts[0], false)
	if snapshot() != before {
		t.Fatal("rejected migration changed live state")
	}
	if _, err = p.l.cp.Exec(ctx, `UPDATE c_recommendation.control SET admission_enabled=false`); err != nil {
		t.Fatal(err)
	}
	before = snapshot()
	exec(parts[0], true)
	if snapshot() != before || checkRecommendationSchema(ctx, p.runtime, true) != nil {
		t.Fatal("ceiling migration changed quotas/promises or failed contract")
	}
	if _, err = p.l.cp.Exec(ctx, `UPDATE c_recommendation.control SET max_units=375600`); err != nil {
		t.Fatal("schema ceiling cannot represent 300 full-round peaks", err)
	}
	before = snapshot()
	exec(parts[1], false)
	if snapshot() != before {
		t.Fatal("unsafe downgrade changed quotas or state")
	}
	if _, err = p.l.cp.Exec(ctx, `UPDATE c_recommendation.control SET max_units=16384`); err != nil {
		t.Fatal(err)
	}
	before = snapshot()
	exec(parts[1], false)
	if snapshot() != before || checkRecommendationSchema(ctx, p.runtime, true) != nil {
		t.Fatal("forward-only downgrade changed quotas or state")
	}
}
