package authprivacy

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/posts"
	rm "github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/recommendation/roundmodel"
)

func TestRecommendationScopeSessionAndGatePostgres(t *testing.T) {
	p := newRecommendationPG(t, 12)
	ctx := context.Background()
	a := sessionTestAccount(t, p.l, "reco_boundaries")
	request := p.request(t, a)
	page := p.start(t, a, request)
	var channel string
	if e := p.l.cp.QueryRow(ctx, `SELECT id::text FROM public.channels ORDER BY id LIMIT 1`).Scan(&channel); e != nil {
		t.Fatal(e)
	}
	if _, e := p.service.ReplaceFeedPreferences(ctx, a.SessionToken, posts.FeedPreferences{BlockedChannelIDs: []string{channel}}); e != nil {
		t.Fatal(e)
	}
	if _, e := p.service.StartRecommendation(ctx, a.SessionToken, request); !errors.Is(e, rm.ErrScope) {
		t.Fatal("stale tuple accepted after scope revision", e)
	}
	if _, e := p.service.ReadRecommendation(ctx, a.SessionToken, "", page.RoundID, 0); !errors.Is(e, rm.ErrScope) {
		t.Fatal("old round accepted after scope revision", e)
	}
	if _, e := p.service.FeedbackRecommendation(ctx, a.SessionToken, "", page.RoundID, 0, []int{0}); !errors.Is(e, rm.ErrScope) {
		t.Fatal("stale feedback accepted", e)
	}
	if count(t, p.l.cp, `SELECT count(*) FROM c_recommendation.rounds`) != 1 || count(t, p.l.cp, `SELECT count(*) FROM c_recommendation.history WHERE exposure_until IS NOT NULL`) != 0 {
		t.Fatal("stale scope partially wrote")
	}
	if _, e := p.l.cp.Exec(ctx, `UPDATE c_auth.accounts SET session_generation=session_generation+1 WHERE account_id=$1`, a.AccountID); e != nil {
		t.Fatal(e)
	}
	if _, e := p.service.StartRecommendation(ctx, a.SessionToken, request); !errors.Is(e, ErrSessionReplaced) && !errors.Is(e, ErrSessionInvalid) {
		t.Fatal("old generation accepted", e)
	}
	b := sessionTestAccount(t, p.l, "reco_gate_reader")
	newRequest := p.request(t, b)
	if e := p.l.gate.Freeze(ctx, "B3b explicit final authorization rejection"); e != nil {
		t.Fatal(e)
	}
	if _, e := p.service.StartRecommendation(ctx, b.SessionToken, newRequest); !errors.Is(e, ErrAuthorizationUnavailable) {
		t.Fatal("frozen gate accepted start", e)
	}
	if count(t, p.l.cp, `SELECT count(*) FROM c_recommendation.rounds`) != 1 {
		t.Fatal("frozen gate partially wrote")
	}
}

// This passing regression establishes a LIMITATION, not a capacity achievement:
// transactional tuple/debt counters cannot prove a bound on aborted MVCC files.
func TestRecommendationAbortedWritePhysicalCounterexamplePostgres(t *testing.T) {
	p := newRecommendationPG(t, 0)
	ctx := context.Background()
	a := sessionTestAccount(t, p.l, "reco_abort")
	p.start(t, a, p.request(t, a))
	beforeDebt := count(t, p.l.cp, `SELECT write_debt_bytes FROM c_recommendation.control`)
	beforeFile := count(t, p.l.cp, `SELECT pg_total_relation_size('c_recommendation.history')`)
	tx, e := p.runtime.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	ids := make([]string, 64)
	for i := range ids {
		ids[i] = uuid.NewString()
	}
	if _, e = tx.Exec(ctx, `INSERT INTO c_recommendation.history(account_id,post_id,exposure_until) SELECT $1,id,$3 FROM unnest($2::uuid[]) id`, a.AccountID, ids, p.control.clock.now().Add(time.Hour)); e != nil {
		t.Fatal(e)
	}
	if e = tx.Rollback(ctx); e != nil {
		t.Fatal(e)
	}
	afterDebt := count(t, p.l.cp, `SELECT write_debt_bytes FROM c_recommendation.control`)
	afterFile := count(t, p.l.cp, `SELECT pg_total_relation_size('c_recommendation.history')`)
	if count(t, p.l.cp, `SELECT count(*) FROM c_recommendation.history`) != 0 || beforeDebt != afterDebt || afterFile <= beforeFile {
		t.Fatal("counterexample not observed", beforeDebt, afterDebt, beforeFile, afterFile)
	}
	evidence := map[string]any{"result": "OBSERVED_LIMITATION_NOT_CAPACITY_PASS", "abortedRows": 64, "committedHistoryRows": 0, "writeDebtBefore": beforeDebt, "writeDebtAfter": afterDebt, "historyFileBefore": beforeFile, "historyFileAfter": afterFile, "scope": "own isolated history heap/index/TOAST only; not WAL, shared files or throughput"}
	b, e := json.MarshalIndent(evidence, "", "  ")
	if e != nil || os.WriteFile(filepath.Join(os.Getenv("HNUB3B_EVIDENCE_DIR"), "B-B3b-physical-counterexample.json"), append(b, '\n'), 0600) != nil {
		t.Fatal("counterexample evidence")
	}
}
