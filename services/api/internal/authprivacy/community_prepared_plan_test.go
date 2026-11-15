package authprivacy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/readtiming"
)

func TestPreparedReadFlagsRejectMissingSchemaPostgres(t *testing.T) {
	l := newLab(t)
	config := enablePostLab(t, l)
	removePreparedReadFixture(t, l)
	if _, err := l.c.WithPosts(context.Background(), config); err != nil {
		t.Fatal("disabled optional reads raised the minimum legacy schema", err)
	}
	for _, media := range []bool{false, true} {
		config.PreparedFeedReads, config.PreparedMediaReads = !media, media
		if _, err := l.c.WithPosts(context.Background(), config); err == nil {
			t.Fatal("prepared reads enabled without their required schema")
		}
	}
}

func TestPreparedReadPlanLifecyclePostgres(t *testing.T) {
	for _, mode := range []string{"success", "prepare_error", "cpu_error", "stale_prepare_error", "stale_cpu_error", "conflicts", "cancel", "panic", "session_generation", "commit_ack_loss"} {
		t.Run(mode, func(t *testing.T) {
			control := newGateControl(t)
			l := control.lab
			enablePostLab(t, l)
			reader := sessionTestAccount(t, l, "generic_plan_reader")
			token := sha256.Sum256(reader.SessionToken[:])
			mustExec(t, l.cp, `UPDATE c_auth.sessions SET expires_at=last_activity_at+interval '1 hour' WHERE token_digest=$1`, token[:])
			before := preparedReadState(t, l, token)
			advanceClosureClock(t, control, control.clock.now().Add(time.Minute))
			var proxy *commitACKProxy
			if mode == "commit_ack_loss" {
				l.c.pool, proxy = newCommitACKProxyPool(t, l.cp)
			}
			ctx, trace := readtiming.Start(context.Background())
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()
			prepares, afters, revalidations, completions, releases := 0, 0, 0, 0, 0
			preparedErr := postError("MEDIA_NOT_FOUND", 404)
			cpuErr := errors.New("private transform failed")
			panicErr := errors.New("private transform panicked")
			conflictErr := postError("SERVICE_UNAVAILABLE", 503)
			var result PostResponse
			var err error
			var panicked any
			func() {
				defer func() { panicked = recover() }()
				result, err = l.c.withPreparedReadPlan(ctx, reader.SessionToken, conflictErr,
					func(prepareCtx context.Context, ro pgx.Tx, subject readSubject) (*preparedReadPlan, error) {
						prepares++
						attempt := prepares
						var isolation, access string
						if e := ro.QueryRow(prepareCtx, `SELECT current_setting('transaction_isolation'),current_setting('transaction_read_only')`).Scan(&isolation, &access); e != nil || isolation != "repeatable read" || access != "on" {
							t.Fatal("preparation is not a read-only consistent snapshot", e)
						}
						if subject.account != reader.AccountID || subject.token != token {
							t.Fatal("preparation rebound its original subject")
						}
						data := []byte("private image")
						plan := &preparedReadPlan{
							AfterPrepare: func(ctx context.Context) error {
								afters++
								if e := ro.Rollback(ctx); !errors.Is(e, pgx.ErrTxClosed) || l.c.pool.Stat().AcquiredConns() != 0 {
									t.Fatal("CPU started before the RO transaction/connection was released", e)
								}
								// A separate writer can take both caller and Gate locks
								// during CPU work; fail promptly if either leaked.
								probe, e := l.cp.Begin(context.Background())
								if e != nil {
									t.Fatal(e)
								}
								defer probe.Rollback(context.Background())
								if _, e = probe.Exec(context.Background(), `SET LOCAL lock_timeout='100ms'`); e != nil {
									t.Fatal(e)
								}
								if _, e = probe.Exec(context.Background(), `SELECT 1 FROM c_auth.accounts WHERE account_id=$1 FOR UPDATE`, reader.AccountID); e != nil {
									t.Fatal(e)
								}
								if _, e = probe.Exec(context.Background(), `SELECT 1 FROM c_auth.authorization_gate FOR UPDATE`); e != nil {
									t.Fatal("CPU work held caller or Gate write locks", e)
								}
								switch mode {
								case "cpu_error":
									return cpuErr
								case "stale_cpu_error":
									if attempt == 1 {
										return cpuErr
									}
								case "cancel":
									cancel()
								case "panic":
									panic(panicErr)
								case "session_generation":
									// Release the probe before this explicit competing writer.
									_ = probe.Rollback(context.Background())
									mustExec(t, l.cp, `UPDATE c_auth.accounts SET session_generation=session_generation+1 WHERE account_id=$1`, reader.AccountID)
								case "commit_ack_loss":
									proxy.armed.Store(true)
								}
								return nil
							},
							Revalidate: func(pgx.Tx, uuid.UUID, AuthorizationDecision, restriction) error {
								revalidations++
								if mode == "conflicts" || attempt == 1 && (mode == "stale_prepare_error" || mode == "stale_cpu_error") {
									return errReadConflict
								}
								return nil
							},
							Complete: func(pgx.Tx, uuid.UUID, AuthorizationDecision, restriction) (any, error) {
								completions++
								return data, nil
							},
							Release: func() { releases++; data = nil },
						}
						if mode == "prepare_error" || mode == "stale_prepare_error" && attempt == 1 {
							return plan, preparedErr
						}
						return plan, nil
					})
			}()
			wantAttempts := 1
			if mode == "conflicts" || mode == "stale_prepare_error" || mode == "stale_cpu_error" {
				wantAttempts = 2
			}
			if prepares != wantAttempts || releases != prepares {
				t.Fatalf("plan ownership: prepare=%d release=%d", prepares, releases)
			}
			success := mode == "success" || mode == "stale_prepare_error" || mode == "stale_cpu_error"
			if success {
				data, ok := result.Value.([]byte)
				if err != nil || !ok || !bytes.Equal(data, []byte("private image")) || result.ExpiresAt.IsZero() || completions != 1 {
					t.Fatal("successful response was lost or cleared by Release", err)
				}
			} else if result.Value != nil || !result.ExpiresAt.IsZero() {
				t.Fatal("failed finalization released protected data or expiry")
			}
			switch mode {
			case "prepare_error":
				if !errors.Is(err, preparedErr) || afters != 0 || revalidations != 1 {
					t.Fatal("prepare error skipped final proof", err)
				}
			case "cpu_error":
				if !errors.Is(err, cpuErr) || revalidations != 1 {
					t.Fatal("CPU error skipped final proof", err)
				}
			case "conflicts":
				if !errors.Is(err, conflictErr) || completions != 0 {
					t.Fatal("conflict exhausted with wrong result", err)
				}
			case "cancel":
				if err == nil || revalidations != 0 {
					t.Fatal("cancellation authorized a plan", err)
				}
			case "panic":
				if panicked != panicErr || revalidations != 0 {
					t.Fatal("panic did not unwind private ownership", panicked)
				}
			case "session_generation":
				if !errors.Is(err, ErrSessionInvalid) || revalidations != 0 {
					t.Fatal("CPU plan survived session generation change", err)
				}
			case "commit_ack_loss":
				if !errors.Is(err, ErrAuthorizationUnavailable) || completions != 1 {
					t.Fatal("unknown commit released a result", err)
				}
				select {
				case <-proxy.dropped:
				default:
					t.Fatal("real COMMIT acknowledgement was not dropped")
				}
			}
			if mode != "panic" && panicked != nil {
				t.Fatal("unexpected panic", panicked)
			}
			stages := trace.Snapshot().Stages
			wantCommits := int64(0)
			if success || mode == "commit_ack_loss" {
				wantCommits = 1
			}
			if stages["anchor_write"].Calls != wantCommits || stages["sql_commit"].Calls != wantCommits {
				t.Fatal("incorrect final checkpoint count", stages)
			}
			after := preparedReadState(t, l, token)
			if wantCommits == 0 && (!after.highwater.Equal(before.highwater) || !after.activity.Equal(before.activity) || !after.expiry.Equal(before.expiry)) {
				t.Fatal("failed plan renewed the session or checkpoint")
			}
			assertPreparedReadAnchorMatches(t, l, after)
		})
	}
}
