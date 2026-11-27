package authprivacy

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type cleanupSnapshotGate struct {
	AuthorizationGate
	snapshots, commits int
	beforeCommit       func(int) error
}

func TestRecommendationCleanupGroupedReceiptsRoundsAndTombstonesPostgres(t *testing.T) {
	for _, freeze := range []bool{false, true} {
		t.Run(map[bool]string{false: "idle_batch", true: "frozen_idle_batch"}[freeze], func(t *testing.T) {
			p := newRecommendationPG(t, 12)
			ctx := context.Background()
			var revoked SignupResult
			for i := 0; i < 6; i++ {
				a := sessionTestAccount(t, p.l, "cleanup_group_"+strconv.Itoa(i))
				p.start(t, a, p.request(t, a))
				if i == 0 {
					revoked = a
				}
			}
			if _, err := p.service.RevokeRecommendation(ctx, revoked.SessionToken); err != nil {
				t.Fatal(err)
			}
			advanceClosureClock(t, p.control, p.control.clock.now().Add(41*time.Minute))
			gate := &cleanupSnapshotGate{AuthorizationGate: p.service.gate}
			service := *p.service
			service.gate = gate
			if _, err := service.CleanRecommendation(ctx, 4096); err != nil {
				t.Fatal(err)
			}
			if count(t, p.l.cp, `SELECT (SELECT count(*) FROM c_recommendation.rounds)+(SELECT count(*) FROM c_recommendation.pages)+(SELECT count(*) FROM c_recommendation.intents)+(SELECT count(*) FROM c_recommendation.history)`) != 0 {
				t.Fatal("grouped round/receipt expiry left details")
			}
			advanceClosureClock(t, p.control, p.control.clock.now().Add(2*time.Hour))
			before := gate.commits
			if freeze {
				gate.beforeCommit = func(int) error { return p.l.gate.Freeze(ctx, "grouped idle final Gate regression") }
			}
			n, err := service.CleanRecommendation(ctx, 16)
			if freeze {
				if n != 0 || !errors.Is(err, ErrAuthorizationUnavailable) || count(t, p.l.cp, `SELECT count(*) FROM c_recommendation.accounts`) != 6 {
					t.Fatal("frozen multi-account cleanup committed", n, err)
				}
			} else if err != nil || n != 5 || gate.commits-before != 1 || count(t, p.l.cp, `SELECT count(*) FROM c_recommendation.accounts`) != 1 {
				t.Fatal("idle accounts did not share one bounded final transaction", n, err)
			}
			if count(t, p.l.cp, `SELECT count(*) FROM c_recommendation.accounts WHERE account_id=$1 AND disabled AND epoch=1`, revoked.AccountID) != 1 {
				t.Fatal("cleanup removed the durable revoked epoch fence")
			}
		})
	}
}

func (g *cleanupSnapshotGate) Snapshot(ctx context.Context) (AuthorizationDecision, error) {
	g.snapshots++
	return g.AuthorizationGate.Snapshot(ctx)
}

func TestRecommendationCleanupIdleEligibilityBoundaryPostgres(t *testing.T) {
	p := newRecommendationPG(t, 0)
	ctx := context.Background()
	at := p.control.clock.now().Truncate(time.Microsecond)
	cutoff := at.Add(-recommendationIdleGrace())
	ids := make([]string, 6)
	for i := range ids {
		a := sessionTestAccount(t, p.l, "cleanup_idle_boundary_"+strconv.Itoa(i))
		ids[i] = a.AccountID.String()
		last := cutoff.Add(-time.Minute)
		if i == 0 {
			last = cutoff.Add(time.Microsecond)
		} else if i == 1 {
			last = cutoff
		}
		mustExec(t, p.l.cp, `INSERT INTO c_recommendation.accounts(account_id,last_at) VALUES($1,$2)`, ids[i], last)
	}
	mustExec(t, p.l.cp, `UPDATE c_recommendation.accounts SET disabled=true,epoch=1,revision=revision+1 WHERE account_id=$1`, ids[3])
	mustExec(t, p.l.cp, `UPDATE c_recommendation.accounts SET epoch=7,revision=revision+1 WHERE account_id=$1`, ids[4])
	mustExec(t, p.l.cp, `INSERT INTO c_recommendation.history(account_id,post_id,exposure_until) VALUES($1,'00000000-0000-4000-8000-000000000001',$2)`, ids[5], at.Add(72*time.Hour))
	for _, expected := range []string{ids[2], ids[1]} {
		object, err := p.service.nextRecommendationCleanup(ctx, at)
		if err != nil || object.kind != 3 || object.account != expected {
			t.Fatal("oldest eligible idle account not selected", object, expected, err)
		}
		mustExec(t, p.l.cp, `DELETE FROM c_recommendation.accounts WHERE account_id=$1`, expected)
	}
	if object, err := p.service.nextRecommendationCleanup(ctx, at); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("fresh account, epoch fence or retained exposure selected for cleanup", object, err)
	}
}

func (g *cleanupSnapshotGate) CommitAuthorized(ctx context.Context, tx pgx.Tx, generation uint64, checks ...func(AuthorizationDecision) error) (AuthorizationDecision, error) {
	g.commits++
	if g.beforeCommit != nil {
		if err := g.beforeCommit(g.commits); err != nil {
			return AuthorizationDecision{}, err
		}
	}
	return g.AuthorizationGate.CommitAuthorized(ctx, tx, generation, checks...)
}

func TestRecommendationCleanupBatchSnapshotPostgres(t *testing.T) {
	for _, freeze := range []bool{false, true} {
		name := "drain"
		if freeze {
			name = "freeze_before_final_batch_commit"
		}
		t.Run(name, func(t *testing.T) {
			p := newRecommendationPG(t, 20)
			ctx := context.Background()
			a := sessionTestAccount(t, p.l, "cleanup_batch_snapshot")
			p.start(t, a, p.request(t, a))
			advanceClosureClock(t, p.control, p.control.clock.now().Add(6*time.Minute))
			gate := &cleanupSnapshotGate{AuthorizationGate: p.service.gate}
			if freeze {
				gate.beforeCommit = func(n int) error {
					if n == 1 {
						return p.l.gate.Freeze(ctx, "cleanup batch freeze regression")
					}
					return nil
				}
			}
			service := *p.service
			service.gate = gate
			n, err := service.CleanRecommendation(ctx, 32)
			if freeze {
				if n != 0 || !errors.Is(err, ErrAuthorizationUnavailable) || count(t, p.l.cp, `SELECT count(*) FROM c_recommendation.history`) != 10 {
					t.Fatalf("batch crossed final Gate freeze: removed=%d err=%v", n, err)
				}
			} else if err != nil || n != 10 || count(t, p.l.cp, `SELECT count(*) FROM c_recommendation.history`) != 0 {
				t.Fatalf("batch did not drain deliveries: removed=%d err=%v", n, err)
			}
			if gate.snapshots != 1 {
				t.Fatalf("one bounded batch published %d preflight checkpoints", gate.snapshots)
			}
		})
	}
}

func TestRecommendationCleanupGroupedBudgetAndExposurePostgres(t *testing.T) {
	p := newRecommendationPG(t, 100)
	ctx := context.Background()
	a := sessionTestAccount(t, p.l, "cleanup_grouped_budget")
	page := p.start(t, a, p.request(t, a))
	for offset := 10; offset < 100; offset += 10 {
		p.read(t, a, page.RoundID, offset)
	}
	advanceClosureClock(t, p.control, p.control.clock.now().Add(6*time.Minute))
	gate := &cleanupSnapshotGate{AuthorizationGate: p.service.gate}
	service := *p.service
	service.gate = gate
	if n, err := service.CleanRecommendation(ctx, 32); err != nil || n != 31 {
		t.Fatal("history plus one revision exceeded 32-row budget", n, err)
	}
	if gate.snapshots != 1 || gate.commits != 1 || count(t, p.l.cp, `SELECT count(*) FROM c_recommendation.history WHERE account_id=$1`, a.AccountID) != 69 {
		t.Fatal("grouped history did not use one final authorized transaction")
	}
	// A retained receipt may still submit exposure after delivery expires.
	if _, err := p.service.FeedbackRecommendation(ctx, a.SessionToken, "", page.RoundID, 0, []int{0}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CleanRecommendation(ctx, 4096); err != nil {
		t.Fatal(err)
	}
	if count(t, p.l.cp, `SELECT count(*) FROM c_recommendation.history WHERE account_id=$1 AND exposure_until>$2 AND delivery_until IS NULL`, a.AccountID, p.control.clock.now()) != 1 {
		t.Fatal("grouped cleanup removed or shortened live exposure")
	}
	if count(t, p.l.cp, `SELECT count(*) FROM c_recommendation.history WHERE account_id=$1`, a.AccountID) != 1 {
		t.Fatal("due deliveries remained after bounded drain")
	}
}

func TestRecommendationCleanupCooperativeDeadlinePostgres(t *testing.T) {
	for _, callerCancelled := range []bool{false, true} {
		t.Run(map[bool]string{false: "finish_current_group", true: "honor_parent_cancellation"}[callerCancelled], func(t *testing.T) {
			p := newRecommendationPG(t, 100)
			a := sessionTestAccount(t, p.l, "cleanup_cooperative")
			page := p.start(t, a, p.request(t, a))
			for offset := 10; offset < 100; offset += 10 {
				p.read(t, a, page.RoundID, offset)
			}
			advanceClosureClock(t, p.control, p.control.clock.now().Add(6*time.Minute))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			gate := &cleanupSnapshotGate{AuthorizationGate: p.service.gate}
			gate.beforeCommit = func(int) error {
				if callerCancelled {
					cancel()
				} else {
					time.Sleep(2100 * time.Millisecond)
				}
				return nil
			}
			service := *p.service
			service.gate = gate
			n, err := service.CleanRecommendation(ctx, 32)
			left := count(t, p.l.cp, `SELECT count(*) FROM c_recommendation.history WHERE account_id=$1`, a.AccountID)
			if callerCancelled {
				if err == nil || n != 0 || left != 100 {
					t.Fatal("parent cancellation committed cleanup", n, left, err)
				}
			} else if err != nil || n != 31 || left != 69 || gate.commits != 1 {
				t.Fatal("pass cutoff interrupted a bounded group or started another", n, left, gate.commits, err)
			}
			if count(t, p.l.cp, `SELECT count(*) FROM c_auth.authorization_gate WHERE gate_state='OPEN'`) != 1 {
				t.Fatal("normal cutoff/cancellation froze a healthy Gate")
			}
		})
	}
}

func TestRecommendationCleanupPreservesPlannerSettingPostgres(t *testing.T) {
	for _, prior := range []string{"on", "off"} {
		t.Run(prior, func(t *testing.T) {
			p := newRecommendationPG(t, 100)
			ctx := context.Background()
			a := sessionTestAccount(t, p.l, "cleanup_planner_setting")
			page := p.start(t, a, p.request(t, a))
			for offset := 10; offset < 100; offset += 10 {
				p.read(t, a, page.RoundID, offset)
			}
			advanceClosureClock(t, p.control, p.control.clock.now().Add(6*time.Minute))
			// Hold every pool connection so the check covers the actual cleanup
			// connection, rather than accidentally observing a different backend.
			connections := func() []*pgxpool.Conn {
				t.Helper()
				var held []*pgxpool.Conn
				for i := int32(0); i < p.runtime.Config().MaxConns; i++ {
					conn, err := p.runtime.Acquire(ctx)
					if err != nil {
						for _, c := range held {
							c.Release()
						}
						t.Fatal(err)
					}
					held = append(held, conn)
				}
				return held
			}
			withConnections := func(check func(*pgxpool.Conn)) {
				held := connections()
				defer func() {
					for _, conn := range held {
						conn.Release()
					}
				}()
				for _, conn := range held {
					check(conn)
				}
			}
			withConnections(func(conn *pgxpool.Conn) {
				_, err := conn.Exec(ctx, `SELECT set_config('enable_sort',$1,false)`, prior)
				if err != nil {
					t.Fatal(err)
				}
			})
			if n, err := p.service.CleanRecommendation(ctx, 32); err != nil || n != 31 {
				t.Fatal("ordered cleanup lost its row budget", n, err)
			}
			if count(t, p.l.cp, `SELECT count(*) FROM c_recommendation.history WHERE account_id=$1`, a.AccountID) != 69 {
				t.Fatal("ordered cleanup removed unbudgeted history")
			}
			withConnections(func(conn *pgxpool.Conn) {
				var got string
				err := conn.QueryRow(ctx, `SELECT current_setting('enable_sort')`).Scan(&got)
				if err != nil || got != prior {
					t.Fatal("cleanup changed the pooled connection's planner setting", got, err)
				}
			})
		})
	}
}
