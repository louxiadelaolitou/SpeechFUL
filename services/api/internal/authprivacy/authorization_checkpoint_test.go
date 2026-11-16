package authprivacy

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type checkpointCrashConfig struct {
	DSN, Schema, Domain, Anchor, Evidence, Boundary string
	AnchorKey, EvidenceKey, RecoveryKey, GlassKey   []byte
	At                                              time.Time
}

// A real process exits after the signed file is durable, before SQL commits.
// This helper deliberately does not construct/reset an authlab database.
func TestGateReadCheckpointCrashChild(t *testing.T) {
	path := os.Getenv("AUTHLAB_CHECKPOINT_CRASH_CONFIG")
	if path == "" {
		return
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var cfg checkpointCrashConfig
	if err = json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	poolConfig, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		t.Fatal("invalid private crash fixture")
	}
	poolConfig.ConnConfig.Tracer = &gateCancellationTracer{match: func(sql string) bool {
		if cfg.Boundary == "commit" {
			return sql == "commit"
		}
		return strings.Contains(sql, "authorization_gate SET trusted_high_watermark=")
	}, onStart: func(ctx context.Context) context.Context { os.Exit(73); return ctx }}
	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	anchor, err := NewFileAuthorizationAnchorStore(cfg.Anchor, ed25519.PrivateKey(cfg.AnchorKey))
	if err != nil {
		t.Fatal(err)
	}
	gate, err := NewPostgresAuthorizationGate(AuthorizationGateConfig{Pool: pool, Schema: cfg.Schema, Domain: cfg.Domain,
		Anchor: anchor, Evidence: NewFileAuthorizationEvidenceProvider(cfg.Evidence), EvidencePublicKey: cfg.EvidenceKey,
		RecoveryPublicKey: cfg.RecoveryKey, BreakGlassPublicKey: cfg.GlassKey, Clock: func() time.Time { return cfg.At }})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = gate.Snapshot(context.Background())
	t.Fatal("crash boundary was not reached")
}

func crashReadCheckpoint(t *testing.T, c *gateControl, boundary string) {
	t.Helper()
	cfg := checkpointCrashConfig{DSN: c.gate.pool.Config().ConnString(), Schema: c.gate.schema, Domain: c.config.Domain,
		Anchor: c.gate.anchor.Path, Evidence: c.provider.Path, AnchorKey: c.gate.anchor.signer,
		EvidenceKey: c.config.EvidencePublicKey, RecoveryKey: c.config.RecoveryPublicKey, GlassKey: c.config.BreakGlassPublicKey,
		At: c.clock.now(), Boundary: boundary}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "crash.private.json")
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestGateReadCheckpointCrashChild$", "-test.count=1")
	cmd.Env = append(os.Environ(), "AUTHLAB_CHECKPOINT_CRASH_CONFIG="+path)
	output, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 73 {
		t.Fatalf("checkpoint crash fixture failed: %v %s", err, output)
	}
}

func checkpointState(t *testing.T, c *gateControl) gateRow {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tx, err := begin(ctx, c.gate.pool)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	row, err := c.gate.readGateRow(ctx, tx)
	if err != nil {
		t.Fatal(err)
	}
	return row
}

func gateRecoveryCount(t *testing.T, c *gateControl) int {
	t.Helper()
	return count(t, c.gate.pool, `SELECT count(*) FROM `+c.gate.schema+`.authorization_gate_audit WHERE event_kind='RECOVER'`)
}

func assertTemporaryEvidenceRecovers(t *testing.T, c *gateControl) {
	t.Helper()
	before, recoveries := checkpointState(t, c), gateRecoveryCount(t, c)
	if _, err := c.gate.Snapshot(context.Background()); !errors.Is(err, ErrAuthorizationUnavailable) {
		t.Fatalf("missing/expired evidence permitted work: %v", err)
	}
	if after := checkpointState(t, c); after != before || after.state != "OPEN" {
		t.Fatalf("temporary evidence failure changed durable authority: before=%+v after=%+v", before, after)
	}
	at := c.clock.now()
	fresh := c.signed(t, c.evidence, uint64(before.generation), uint64(before.version+1), at, at.Add(authorizationEvidenceTTL))
	if err := c.provider.Store(context.Background(), fresh); err != nil {
		t.Fatal(err)
	}
	if _, err := c.gate.Snapshot(context.Background()); err != nil {
		t.Fatalf("fresh same-generation evidence required security recovery: %v", err)
	}
	if after := checkpointState(t, c); after.generation != before.generation || gateRecoveryCount(t, c) != recoveries {
		t.Fatal("temporary outage invalidated existing credentials")
	}
}

func TestGateCheckpointCrashPreservesExistingSessionPostgres(t *testing.T) {
	for _, boundary := range []string{"update", "commit", "repeated"} {
		t.Run(boundary, func(t *testing.T) {
			gateCancellationParties(t, func(t *testing.T, c *gateControl) {
				ctx := context.Background()
				var session SignupResult
				if c.gate.schema == "c_auth" {
					session = sessionTestAccount(t, c.lab, "checkpoint_user")
				}
				if _, err := c.gate.Snapshot(ctx); err != nil {
					t.Fatal(err)
				}
				before, recoveries := checkpointState(t, c), gateRecoveryCount(t, c)
				c.clock.set(c.clock.now().Add(time.Second))
				first := boundary
				if first == "repeated" {
					first = "update"
				}
				crashReadCheckpoint(t, c, first)
				if boundary == "repeated" {
					// Crash again after replay and a second read advance, so both
					// SQL updates roll back together to the original predecessor.
					c.clock.set(c.clock.now().Add(time.Second))
					crashReadCheckpoint(t, c, "commit")
				}
				if row := checkpointState(t, c); row != before {
					t.Fatalf("killed process committed SQL unexpectedly: %+v", row)
				}
				restarted, err := NewPostgresAuthorizationGate(c.config)
				if err != nil {
					t.Fatal(err)
				}
				c.gate = restarted
				if _, err = restarted.Snapshot(ctx); err != nil {
					t.Fatalf("provable read-only interruption did not resume: %v", err)
				}
				assertGateCheckpointOpen(t, c)
				if checkpointState(t, c).generation != before.generation || gateRecoveryCount(t, c) != recoveries {
					t.Fatal("ordinary continuation changed security generation")
				}
				if c.gate.schema == "c_auth" {
					c.lab.c.gate = restarted
					if _, err = c.lab.c.GetCurrentSession(ctx, session.SessionToken); err != nil {
						t.Fatalf("already logged-in account was forced to log in again: %v", err)
					}
				}
			})
		})
	}
}

func TestGateCheckpointCannotHideBusinessRollbackPostgres(t *testing.T) {
	gateCancellationParties(t, func(t *testing.T, c *gateControl) {
		ctx := context.Background()
		decision, err := c.gate.Snapshot(ctx)
		if err != nil {
			t.Fatal(err)
		}
		before := checkpointState(t, c)
		gateCancellationProbe(t, c)
		tx, err := begin(ctx, c.gate.pool)
		if err != nil {
			t.Fatal(err)
		}
		defer c.gate.Abort(ctx, tx)
		if _, err = tx.Exec(ctx, `UPDATE public.gate_cancellation_probe SET value=1`); err != nil {
			t.Fatal(err)
		}
		if _, err = c.gate.CommitAuthorized(ctx, tx, decision.Generation); err != nil {
			t.Fatal(err)
		}
		after := checkpointState(t, c)
		if after.checkpoint == before.checkpoint || !after.highwater.Equal(before.highwater) || after.version != before.version {
			t.Fatal("every business commit needs a distinct ID even when time/evidence do not change")
		}
		// Restore the last SQL checkpoint and business value while keeping the
		// independent committed anchor. An old snapshot must not be accepted.
		if _, err = c.gate.pool.Exec(ctx, `UPDATE `+c.gate.schema+`.authorization_gate SET checkpoint_id=$1`, before.checkpoint); err != nil {
			t.Fatal(err)
		}
		if _, err = c.gate.pool.Exec(ctx, `UPDATE public.gate_cancellation_probe SET value=0`); err != nil {
			t.Fatal(err)
		}
		if _, err = c.gate.Snapshot(ctx); !errors.Is(err, ErrAuthorizationUnavailable) {
			t.Fatal("automatic read continuation accepted lost business data")
		}
		if checkpointState(t, c).state != "FROZEN" {
			t.Fatal("business rollback did not freeze durably")
		}
	})
}

func TestGateReadReceiptRejectsWrongPredecessorAndFrozenPostgres(t *testing.T) {
	for _, fault := range []string{"wrong_predecessor", "explicit_freeze"} {
		t.Run(fault, func(t *testing.T) {
			gateCancellationParties(t, func(t *testing.T, c *gateControl) {
				ctx := context.Background()
				if _, err := c.gate.Snapshot(ctx); err != nil {
					t.Fatal(err)
				}
				c.clock.set(c.clock.now().Add(time.Second))
				crashReadCheckpoint(t, c, "update")
				if fault == "wrong_predecessor" {
					if _, err := c.gate.pool.Exec(ctx, `UPDATE `+c.gate.schema+`.authorization_gate SET checkpoint_id=$1`, uuid.NewString()); err != nil {
						t.Fatal(err)
					}
				} else if err := c.gate.Freeze(ctx, "explicit security event"); err != nil {
					t.Fatal(err)
				}
				if _, err := c.gate.Snapshot(ctx); !errors.Is(err, ErrAuthorizationUnavailable) {
					t.Fatal("read receipt authorized an unproven predecessor or reopened a frozen Gate")
				}
				if checkpointState(t, c).state != "FROZEN" {
					t.Fatal("rejected continuation did not remain frozen")
				}
			})
		})
	}
}

func TestGateTemporaryEvidenceOutagePreservesSessionPostgres(t *testing.T) {
	for _, fault := range []string{"missing", "expired"} {
		t.Run(fault, func(t *testing.T) {
			c := newGateControl(t)
			session := sessionTestAccount(t, c.lab, "outage_login_user")
			if fault == "missing" {
				if err := os.Remove(c.provider.Path); err != nil {
					t.Fatal(err)
				}
			} else {
				c.clock.set(c.clock.now().Add(authorizationEvidenceTTL + time.Second))
			}
			if _, err := c.lab.c.GetCurrentSession(context.Background(), session.SessionToken); !errors.Is(err, ErrAuthorizationUnavailable) {
				t.Fatalf("unavailable evidence did not stop protected reads: %v", err)
			}
			assertTemporaryEvidenceRecovers(t, c)
			if _, err := c.lab.c.GetCurrentSession(context.Background(), session.SessionToken); err != nil {
				t.Fatalf("temporary provider outage invalidated the existing login: %v", err)
			}
		})
	}
}

func TestGateLegacyCheckpointUpgradePreservesGenerationPostgres(t *testing.T) {
	gateCancellationParties(t, func(t *testing.T, c *gateControl) {
		ctx := context.Background()
		before, recoveries := checkpointState(t, c), gateRecoveryCount(t, c)
		// Model the exact legacy material during an additive schema upgrade.
		if _, err := c.gate.pool.Exec(ctx, `UPDATE `+c.gate.schema+`.authorization_gate SET checkpoint_id=NULL`); err != nil {
			t.Fatal(err)
		}
		if err := c.gate.anchor.withLocked(func(a *authorizationAnchor, exists bool) error {
			a.CheckpointID, a.ReadAdvance = "", nil
			return c.gate.anchor.writeLocked(*a)
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := c.gate.Snapshot(ctx); err != nil {
			t.Fatalf("matching legacy material could not upgrade in place: %v", err)
		}
		after := checkpointState(t, c)
		if after.checkpoint == "" || after.generation != before.generation || gateRecoveryCount(t, c) != recoveries {
			t.Fatal("additive checkpoint upgrade required login revocation")
		}
	})
}
