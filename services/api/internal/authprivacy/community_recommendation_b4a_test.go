package authprivacy

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/posts"
	rm "github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/recommendation/roundmodel"
)

func recommendationDrain(t *testing.T, p *recommendationPG) int {
	t.Helper()
	total := 0
	for i := 0; i < 32; i++ {
		n, err := p.service.CleanRecommendation(context.Background(), 16)
		if err != nil {
			t.Fatal("bounded cleanup", err)
		}
		total += n
		if n == 0 {
			return total
		}
	}
	t.Fatal("cleanup did not finish its finite fixture")
	return total
}

func recommendationEmptyRequest(t *testing.T, p *recommendationPG, a SignupResult) RecommendationStart {
	t.Helper()
	ctx := context.Background()
	rows, err := p.l.cp.Query(ctx, `SELECT id::text FROM public.channels ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.service.ReplaceFeedPreferences(ctx, a.SessionToken, posts.FeedPreferences{BlockedChannelIDs: ids}); err != nil {
		t.Fatal(err)
	}
	return p.request(t, a)
}

func TestRecommendationB4aPromisedExposureAfterDeliveryDeletePostgres(t *testing.T) {
	for _, budget := range []string{"rows", "bytes"} {
		t.Run(budget, func(t *testing.T) {
			postCount := 4
			if budget == "bytes" {
				postCount = 20
			}
			p := newRecommendationPG(t, postCount)
			ctx := context.Background()
			a := sessionTestAccount(t, p.l, "b4a_credit_a")
			b := sessionTestAccount(t, p.l, "b4a_credit_b")
			page := p.start(t, a, p.request(t, a))
			if len(page.View.IDs) == 0 {
				t.Fatal("fixture has no delivered item")
			}
			if budget == "rows" {
				if _, err := p.l.cp.Exec(ctx, `UPDATE c_recommendation.control SET max_rows=live_rows+reserved_rows`); err != nil {
					t.Fatal(err)
				}
			}
			advanceClosureClock(t, p.control, p.control.clock.now().Add(6*time.Minute))
			recommendationDrain(t, p)
			if count(t, p.l.cp, `SELECT count(*) FROM c_recommendation.history`) != 0 {
				t.Fatal("delivery row not deleted")
			}
			p.start(t, b, recommendationEmptyRequest(t, p, b))
			if budget == "rows" {
				if count(t, p.l.cp, `SELECT max_rows-live_rows-reserved_rows FROM c_recommendation.control`) != 0 {
					t.Fatal("row budget not full")
				}
			} else {
				if _, err := p.l.cp.Exec(ctx, `UPDATE c_recommendation.control SET max_bytes=live_bytes+reserved_bytes`); err != nil {
					t.Fatal(err)
				}
			}
			advanceClosureClock(t, p.control, p.control.clock.now().Add(time.Minute))
			beforeCredit := count(t, p.l.cp, `SELECT reserved_rows FROM c_recommendation.control`)
			// Force a failure after round/page credit changes but before history insert.
			if _, err := p.l.cp.Exec(ctx, `CREATE FUNCTION c_recommendation.b4a_reject_feedback() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'B4a injected feedback failure'; END $$; CREATE TRIGGER b4a_reject_feedback BEFORE INSERT ON c_recommendation.history FOR EACH ROW EXECUTE FUNCTION c_recommendation.b4a_reject_feedback()`); err != nil {
				t.Fatal(err)
			}
			if _, err := p.service.FeedbackRecommendation(ctx, a.SessionToken, "", page.RoundID, 0, []int{0}); err == nil {
				t.Fatal("injected failure accepted")
			}
			if count(t, p.l.cp, `SELECT reserved_rows FROM c_recommendation.control`) != beforeCredit || count(t, p.l.cp, `SELECT sum(consumed) FROM c_recommendation.pages`) != 0 {
				t.Fatal("rollback consumed promise")
			}
			if _, err := p.l.cp.Exec(ctx, `DROP TRIGGER b4a_reject_feedback ON c_recommendation.history; DROP FUNCTION c_recommendation.b4a_reject_feedback()`); err != nil {
				t.Fatal(err)
			}
			if _, err := p.service.FeedbackRecommendation(ctx, a.SessionToken, "", page.RoundID, 0, []int{0}); err != nil {
				t.Fatal("promised feedback lost to new admission", err)
			}
			after := count(t, p.l.cp, `SELECT reserved_rows FROM c_recommendation.control`)
			if _, err := p.service.FeedbackRecommendation(ctx, a.SessionToken, "", page.RoundID, 0, []int{0}); err != nil {
				t.Fatal(err)
			}
			if count(t, p.l.cp, `SELECT reserved_rows FROM c_recommendation.control`) != after || count(t, p.l.cp, `SELECT count(*) FROM c_recommendation.history WHERE exposure_until IS NOT NULL`) != 1 {
				t.Fatal("duplicate feedback consumed twice")
			}
			advanceClosureClock(t, p.control, p.control.clock.now().Add(41*time.Minute))
			recommendationDrain(t, p)
			if count(t, p.l.cp, `SELECT reserved_rows+reserved_bytes+reserved_units+reserved_writes FROM c_recommendation.control`) != 0 {
				t.Fatal("terminated rounds retained future credits")
			}
		})
	}
}

func recommendationRefreshSyntheticTime(p *recommendationPG) {
	now := p.control.clock.now()
	services := *p.service.recommendation
	services.config.Capacity.MeasuredAt, services.config.Capacity.ValidUntil = now, now.Add(30*time.Minute)
	copy := *p.service
	copy.recommendation = &services
	p.service = &copy
}

func TestRecommendationB4aRepeatedLifecycleRecoversAdmissionPostgres(t *testing.T) {
	p := newRecommendationPG(t, 0)
	ctx := context.Background()
	a := sessionTestAccount(t, p.l, "b4a_cycle")
	p.start(t, a, p.request(t, a))
	advanceClosureClock(t, p.control, p.control.clock.now().Add(41*time.Minute))
	recommendationDrain(t, p)
	// Model accumulated successful accounting near the former hard ceiling.
	if _, err := p.l.cp.Exec(ctx, `UPDATE c_recommendation.control SET write_debt_bytes=max_file_bytes-baseline_file_bytes-(live_units+reserved_units+reserved_writes+16)*growth_bytes,max_rows=4`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		recommendationRefreshSyntheticTime(p)
		p.start(t, a, p.request(t, a))
		advanceClosureClock(t, p.control, p.control.clock.now().Add(41*time.Minute))
		recommendationDrain(t, p)
		if count(t, p.l.cp, `SELECT count(*) FROM c_recommendation.rounds`) != 0 {
			t.Fatal("cycle retained rounds")
		}
	}
	if _, err := p.l.cp.Exec(ctx, `UPDATE c_recommendation.control SET write_debt_bytes=9223372036854775806`); err != nil {
		t.Fatal(err)
	}
	recommendationRefreshSyntheticTime(p)
	p.start(t, a, p.request(t, a))
	var saturated int64
	if err := p.l.cp.QueryRow(ctx, `SELECT write_debt_bytes FROM c_recommendation.control`).Scan(&saturated); err != nil || saturated != 9223372036854775807 {
		t.Fatal("telemetry overflow or blocked admission", saturated, err)
	}
	advanceClosureClock(t, p.control, p.control.clock.now().Add(41*time.Minute))
	recommendationDrain(t, p)
}

func TestRecommendationB4aCleanupBatchAndCancellationPostgres(t *testing.T) {
	p := newRecommendationPG(t, 20)
	ctx := context.Background()
	a := sessionTestAccount(t, p.l, "b4a_batch")
	p.start(t, a, p.request(t, a))
	advanceClosureClock(t, p.control, p.control.clock.now().Add(6*time.Minute))
	before := count(t, p.l.cp, `SELECT count(*) FROM c_recommendation.history`)
	if before != 10 {
		t.Fatal("fixture page size", before)
	}
	rows, err := p.l.cp.Query(ctx, `SELECT post_id::text FROM c_recommendation.history WHERE account_id=$1 ORDER BY post_id`, a.AccountID)
	if err != nil {
		t.Fatal(err)
	}
	posts, err := recommendationCleanupIDs(rows)
	if err != nil || len(posts) != before {
		t.Fatal("fixture post ids", len(posts), err)
	}
	// A candidate can disappear between the queue read and the account lock.
	d, err := p.l.gate.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	stale, err := p.service.nextRecommendationCleanup(ctx, d.TrustedAt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.l.cp.Exec(ctx, `DELETE FROM c_recommendation.history WHERE account_id=$1 AND post_id=$2`, stale.account, stale.id); err != nil {
		t.Fatal(err)
	}
	// Grouped cleanup re-reads the due group instead of trusting the stale
	// object; the other nine expired deliveries still make bounded progress.
	if n, used, err := p.service.cleanRecommendationObject(ctx, d, stale, 16); err != nil || n != 9 || used != 10 {
		t.Fatal("stale candidate did not converge on the due group", n, used, err)
	}
	// Restore the same expired page, not a new expiry or longer retention.
	for _, post := range posts {
		if _, err = p.l.cp.Exec(ctx, `INSERT INTO c_recommendation.history(account_id,post_id,delivery_at,delivery_until) VALUES($1,$2,$3,$4)`, a.AccountID, post, d.TrustedAt.Add(-6*time.Minute), d.TrustedAt.Add(-time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	// A locked oldest account must not be silently skipped or partially changed.
	blocker, err := p.runtime.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = blocker.Exec(ctx, `SELECT account_id FROM c_auth.accounts WHERE account_id=$1 FOR UPDATE`, a.AccountID); err != nil {
		t.Fatal(err)
	}
	wait, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	n, err := p.service.CleanRecommendation(wait, 16)
	cancel()
	blocker.Rollback(ctx)
	if err == nil || n != 0 || count(t, p.l.cp, `SELECT count(*) FROM c_recommendation.history`) != before {
		t.Fatal("locked-oldest cleanup boundary", n, err)
	}
	if n, err = p.service.CleanRecommendation(ctx, 16); err != nil || n != 10 {
		t.Fatal("maxRows did not drive the due page batch", n, err)
	}
	if n, err = p.service.CleanRecommendation(ctx, 16); err != nil || n != 0 {
		t.Fatal("grouped cleanup left a second batch", n, err)
	}
	cancelled, stop := context.WithCancel(ctx)
	stop()
	if n, err = p.service.CleanRecommendation(cancelled, 16); n != 0 || !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled cleanup", n, err)
	}
	advanceClosureClock(t, p.control, p.control.clock.now().Add(41*time.Minute))
	d, err = p.l.gate.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Explicitly select this round to verify the child+revision row budget.
	var roundID string
	if err = p.l.cp.QueryRow(ctx, `SELECT round_id::text FROM c_recommendation.rounds WHERE account_id=$1`, a.AccountID).Scan(&roundID); err != nil {
		t.Fatal(err)
	}
	object := recommendationCleanupObject{account: a.AccountID.String(), id: roundID, kind: 1}
	// Grouped cleanup reserves the worst-case round shape (round + ten pages +
	// revision) before selecting an object, so 11 units cannot admit it.
	if n, used, err := p.service.cleanRecommendationObject(ctx, d, object, 11); err != nil || n != 0 || used != 0 {
		t.Fatal("cascade reservation exceeded remainder", n, used, err)
	}
	if n, used, err := p.service.cleanRecommendationObject(ctx, d, object, 12); err != nil || n != 3 || used != 4 {
		t.Fatal("cascade/revision budget", n, used, err)
	}
}

func TestRecommendationB4aIdleAccountAndFullNewOptOutPostgres(t *testing.T) {
	p := newRecommendationPG(t, 0)
	ctx := context.Background()
	a := sessionTestAccount(t, p.l, "b4a_idle")
	b := sessionTestAccount(t, p.l, "b4a_disabled")
	c := sessionTestAccount(t, p.l, "b4a_epoch_fence")
	request := p.request(t, a)
	p.start(t, a, request)
	if _, err := p.l.cp.Exec(ctx, `UPDATE c_recommendation.control SET max_rows=live_rows+reserved_rows`); err != nil {
		t.Fatal(err)
	}
	if _, err := p.service.RevokeRecommendation(ctx, b.SessionToken); err == nil {
		t.Fatal("new full-budget opt-out falsely acknowledged")
	}
	if count(t, p.l.cp, `SELECT count(*) FROM c_recommendation.accounts WHERE account_id=$1`, b.AccountID) != 0 {
		t.Fatal("failed opt-out partially persisted")
	}
	if _, err := p.l.cp.Exec(ctx, `UPDATE c_recommendation.control SET max_rows=16`); err != nil {
		t.Fatal(err)
	}
	if _, err := p.service.RevokeRecommendation(ctx, b.SessionToken); err != nil {
		t.Fatal(err)
	}
	advanceClosureClock(t, p.control, p.control.clock.now().Add(41*time.Minute))
	recommendationDrain(t, p)
	advanceClosureClock(t, p.control, p.control.clock.now().Add(18*time.Minute))
	recommendationDrain(t, p)
	if count(t, p.l.cp, `SELECT count(*) FROM c_recommendation.accounts WHERE account_id=$1`, a.AccountID) != 0 {
		t.Fatal("idle active account never reclaimed")
	}
	if count(t, p.l.cp, `SELECT count(*) FROM c_recommendation.accounts WHERE account_id=$1 AND disabled`, b.AccountID) != 1 {
		t.Fatal("disabled fence was automatically removed")
	}
	if _, err := p.service.StartRecommendation(ctx, a.SessionToken, request); !errors.Is(err, rm.ErrExpired) {
		t.Fatal("old tuple revived after reclaim", err)
	}
	if _, err := p.l.cp.Exec(ctx, `INSERT INTO c_recommendation.accounts(account_id,epoch,last_at) VALUES($1,7,$2)`, c.AccountID, p.control.clock.now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	recommendationDrain(t, p)
	if count(t, p.l.cp, `SELECT epoch FROM c_recommendation.accounts WHERE account_id=$1`, c.AccountID) != 7 {
		t.Fatal("nonzero epoch fence reclaimed")
	}
	staleEpoch := p.request(t, c)
	staleEpoch.Tuple.AccountEpoch = 0
	if _, err := p.service.StartRecommendation(ctx, c.SessionToken, staleEpoch); !errors.Is(err, rm.ErrScope) {
		t.Fatal("pre-revocation epoch revived", err)
	}
}

func TestRecommendationB4aMissingAndWrongStructuresPostgres(t *testing.T) {
	p := newRecommendationPG(t, 0)
	ctx := context.Background()
	var checkName, checkDef, fkDef, indexDef, insertDef, closureDef string
	for _, q := range []struct {
		sql string
		dst []any
	}{
		{`SELECT conname,pg_get_constraintdef(oid) FROM pg_constraint WHERE conrelid='c_recommendation.control'::regclass AND contype='c' AND pg_get_constraintdef(oid) LIKE '%live_rows%reserved_rows%'`, []any{&checkName, &checkDef}},
		{`SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conrelid='c_recommendation.rounds'::regclass AND conname='rounds_account_id_fkey'`, []any{&fkDef}},
		{`SELECT pg_get_indexdef('c_recommendation.recommendation_history_expiry'::regclass)`, []any{&indexDef}},
		{`SELECT pg_get_triggerdef(oid) FROM pg_trigger WHERE tgrelid='c_recommendation.history'::regclass AND tgname='recommendation_count_insert'`, []any{&insertDef}},
		{`SELECT pg_get_triggerdef(oid) FROM pg_trigger WHERE tgrelid='c_auth.accounts'::regclass AND tgname='recommendation_account_closed'`, []any{&closureDef}},
	} {
		if err := p.l.cp.QueryRow(ctx, q.sql).Scan(q.dst...); err != nil {
			t.Fatal(err)
		}
	}
	checkID := pgx.Identifier{checkName}.Sanitize()
	cases := []struct{ name, damage, repair string }{
		{"missing_budget", `ALTER TABLE c_recommendation.control DROP CONSTRAINT ` + checkID, `ALTER TABLE c_recommendation.control ADD CONSTRAINT ` + checkID + ` ` + checkDef},
		{"wrong_budget", `ALTER TABLE c_recommendation.control DROP CONSTRAINT ` + checkID + `; ALTER TABLE c_recommendation.control ADD CONSTRAINT ` + checkID + ` CHECK(live_rows<=max_rows)`, `ALTER TABLE c_recommendation.control DROP CONSTRAINT ` + checkID + `; ALTER TABLE c_recommendation.control ADD CONSTRAINT ` + checkID + ` ` + checkDef},
		{"nullable_ledger", `ALTER TABLE c_recommendation.control ALTER COLUMN live_rows DROP NOT NULL`, `ALTER TABLE c_recommendation.control ALTER COLUMN live_rows SET NOT NULL`},
		{"missing_cascade", `ALTER TABLE c_recommendation.rounds DROP CONSTRAINT rounds_account_id_fkey`, `ALTER TABLE c_recommendation.rounds ADD CONSTRAINT rounds_account_id_fkey ` + fkDef},
		{"missing_expiry_index", `DROP INDEX c_recommendation.recommendation_history_expiry`, indexDef},
		{"wrong_named_index", `DROP INDEX c_recommendation.recommendation_history_expiry; CREATE INDEX recommendation_history_expiry ON c_recommendation.history(account_id,post_id)`, `DROP INDEX c_recommendation.recommendation_history_expiry; ` + indexDef},
		{"wrong_trigger_event", `DROP TRIGGER recommendation_count_insert ON c_recommendation.history; CREATE TRIGGER recommendation_count_insert AFTER DELETE ON c_recommendation.history REFERENCING OLD TABLE AS old_rows FOR EACH STATEMENT EXECUTE FUNCTION c_recommendation.account_change()`, `DROP TRIGGER recommendation_count_insert ON c_recommendation.history; ` + insertDef},
		{"wrong_transition_name", `DROP TRIGGER recommendation_count_insert ON c_recommendation.history; CREATE TRIGGER recommendation_count_insert AFTER INSERT ON c_recommendation.history REFERENCING NEW TABLE AS wrong_rows FOR EACH STATEMENT EXECUTE FUNCTION c_recommendation.account_change()`, `DROP TRIGGER recommendation_count_insert ON c_recommendation.history; ` + insertDef},
		{"wrong_closure_timing", `DROP TRIGGER recommendation_account_closed ON c_auth.accounts; CREATE TRIGGER recommendation_account_closed BEFORE UPDATE OF state ON c_auth.accounts FOR EACH ROW EXECUTE FUNCTION c_recommendation.clear_closed_account()`, `DROP TRIGGER recommendation_account_closed ON c_auth.accounts; ` + closureDef},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := p.l.cp.Exec(ctx, tc.damage); err != nil {
				t.Fatal(err)
			}
			if err := checkRecommendationSchema(ctx, p.runtime, false); err == nil {
				t.Error("damaged M1 accepted while disabled")
			}
			if _, err := p.l.cp.Exec(ctx, tc.repair); err != nil {
				t.Fatal(err)
			}
			if err := checkRecommendationSchema(ctx, p.runtime, false); err != nil {
				t.Fatal("repaired M1 rejected", err)
			}
		})
	}
}

func TestRecommendationPagesCheckAcceptsDumpRestoreSpellingPostgres(t *testing.T) {
	p := newRecommendationPG(t, 0)
	ctx := context.Background()
	if err := checkRecommendationSchema(ctx, p.runtime, true); err != nil {
		t.Fatal("fresh migration schema rejected", err)
	}
	// pg_dump drops the redundant inner parentheses and pg_restore keeps the
	// flattened AND list. A restored cluster is semantically identical, so the
	// contract must accept that spelling while still rejecting altered bounds.
	flat := `CHECK (((consumed >= 0) AND (consumed <= 1023) AND ((consumed & (~ delivered)) = 0)))`
	if _, err := p.l.cp.Exec(ctx, `ALTER TABLE c_recommendation.pages DROP CONSTRAINT pages_check; ALTER TABLE c_recommendation.pages ADD CONSTRAINT pages_check `+flat); err != nil {
		t.Fatal(err)
	}
	if err := checkRecommendationSchema(ctx, p.runtime, true); err != nil {
		t.Fatal("dump/restore spelling rejected", err)
	}
	if _, err := p.l.cp.Exec(ctx, `ALTER TABLE c_recommendation.pages DROP CONSTRAINT pages_check; ALTER TABLE c_recommendation.pages ADD CONSTRAINT pages_check CHECK ((consumed >= 0) AND (consumed <= 2047) AND ((consumed & (~ delivered)) = 0))`); err != nil {
		t.Fatal(err)
	}
	if err := checkRecommendationSchema(ctx, p.runtime, true); err == nil {
		t.Fatal("altered pages budget accepted")
	}
}

func TestRecommendationB4aProfileChangeCannotStrandPromisesPostgres(t *testing.T) {
	p := newRecommendationPG(t, 2)
	ctx := context.Background()
	a := sessionTestAccount(t, p.l, "b4a_profile")
	request := p.request(t, a)
	page := p.start(t, a, request)
	nextProfile := sha256.Sum256([]byte("B4A/SYNTHETIC/NEW-PROFILE/NOT-A-MEASUREMENT"))
	if _, err := p.l.cp.Exec(ctx, `UPDATE c_recommendation.control SET profile_hash=$1`, nextProfile[:]); err == nil {
		t.Fatal("ordinary profile update stranded committed promises")
	}
	if _, err := p.l.cp.Exec(ctx, `UPDATE c_recommendation.control SET admission_enabled=false,max_rows=64`); err != nil {
		t.Fatal(err)
	}
	if retry := p.start(t, a, request); retry.RoundID != page.RoundID {
		t.Fatal("pause replaced existing intent")
	}
	fresh := p.request(t, a)
	if _, err := p.service.StartRecommendation(ctx, a.SessionToken, fresh); !errors.Is(err, rm.ErrCapacity) {
		t.Fatal("pause admitted a new tuple", err)
	}
	if _, err := p.l.cp.Exec(ctx, `UPDATE c_recommendation.control SET max_rows=live_rows+reserved_rows-1`); err == nil {
		t.Fatal("shrink evicted promises")
	}
	if _, err := p.l.cp.Exec(ctx, `UPDATE c_recommendation.control SET profile_hash=$1`, nextProfile[:]); err == nil {
		t.Fatal("paused but undrained profile replacement accepted")
	}
	if _, err := p.service.FeedbackRecommendation(ctx, a.SessionToken, "", page.RoundID, 0, []int{0}); err != nil {
		t.Fatal("old process lost accepted feedback", err)
	}
	oldProcess := p.service
	nextConfig := p.config
	nextConfig.Capacity.Profile = rm.Digest(nextProfile)
	newProcess, err := p.service.WithRecommendation(ctx, nextConfig, p.port)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = newProcess.ReadRecommendation(ctx, a.SessionToken, "", page.RoundID, 0); !errors.Is(err, ErrAuthorizationUnavailable) {
		t.Fatal("unmatched fleet treated as compatible", err)
	}
	if _, err = p.l.cp.Exec(ctx, `UPDATE c_recommendation.control SET security_epoch=security_epoch+1`); err != nil {
		t.Fatal(err)
	}
	if _, err = oldProcess.FeedbackRecommendation(ctx, a.SessionToken, "", page.RoundID, 0, []int{1}); !errors.Is(err, rm.ErrScope) {
		t.Fatal("paused fleet ignored safety revocation", err)
	}
	advanceClosureClock(t, p.control, p.control.clock.now().Add(41*time.Minute))
	recommendationDrain(t, p)
	if _, err = p.l.cp.Exec(ctx, `UPDATE c_recommendation.control SET profile_hash=$1`, nextProfile[:]); err != nil {
		t.Fatal("drained profile update", err)
	}
	if _, err = p.l.cp.Exec(ctx, `UPDATE c_recommendation.control SET admission_enabled=true`); err != nil {
		t.Fatal(err)
	}
	now := p.control.clock.now()
	nextConfig.Capacity.MeasuredAt, nextConfig.Capacity.ValidUntil = now, now.Add(30*time.Minute)
	newProcess, err = p.service.WithRecommendation(ctx, nextConfig, p.port)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = oldProcess.StartRecommendation(ctx, a.SessionToken, request); !errors.Is(err, ErrAuthorizationUnavailable) {
		t.Fatal("old fleet admitted after cutover", err)
	}
	p.service = newProcess
	p.start(t, a, p.request(t, a))
	if err = p.l.gate.Freeze(ctx, "B4a explicit gate boundary"); err != nil {
		t.Fatal(err)
	}
	if _, err = newProcess.ReadRecommendation(ctx, a.SessionToken, "", rm.ID(uuid.NewString()), 0); !errors.Is(err, ErrAuthorizationUnavailable) {
		t.Fatal("Gate freeze ignored", err)
	}
}
