package authprivacy

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/binary"
	"encoding/json"
	"errors"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func checkpointSignedTestRaw(t *testing.T, s *FileAuthorizationAnchorStore, a authorizationAnchor) []byte {
	t.Helper()
	message, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(signedAuthorizationAnchor{Anchor: a, Signature: ed25519.Sign(s.signer, message)})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func checkpointLegacyTestContainer(records ...[]byte) []byte {
	result := []byte(authorizationCheckpointMagic)
	for _, raw := range records {
		record := make([]byte, authorizationCheckpointRecordSize)
		binary.BigEndian.PutUint32(record[:4], uint32(len(raw)))
		copy(record[4:], raw)
		result = append(result, record...)
	}
	return result
}

func checkpointSlotsTestRead(t *testing.T, s *FileAuthorizationAnchorStore) (authorizationCheckpointSlots, []byte) {
	t.Helper()
	f, err := os.Open(s.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	slots, err := decodeCheckpointSlots(f, s.public, nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(s.Path)
	if err != nil {
		t.Fatal(err)
	}
	return slots, raw
}

func TestAuthorizationCheckpointSlotsBusinessReuseAndAtomicControl(t *testing.T) {
	s := checkpointTestStore(t)
	a := authorizationAnchor{Domain: "slot-test", Generation: 1, Version: 1, Highwater: time.Now().UTC()}
	if err := s.withLocked(func(*authorizationAnchor, bool) error { return s.writeLocked(a) }); err != nil {
		t.Fatal(err)
	}
	initial, _ := os.Stat(s.Path)
	for n := 0; n < 130; n++ {
		before, raw := checkpointSlotsTestRead(t, s)
		a.Highwater = a.Highwater.Add(time.Microsecond)
		a.CheckpointID = uuid.NewString()
		if err := s.withLocked(func(*authorizationAnchor, bool) error { return s.writeBusinessLocked(a) }); err != nil {
			t.Fatal(err)
		}
		after, updated := checkpointSlotsTestRead(t, s)
		info, _ := os.Stat(s.Path)
		start := authorizationCheckpointHeaderSize + before.latest*authorizationCheckpointRecordSize
		if !os.SameFile(initial, info) || len(updated) != authorizationCheckpointSlotsSize || !bytes.Equal(raw[start:start+authorizationCheckpointRecordSize], updated[start:start+authorizationCheckpointRecordSize]) || after.sequence[after.latest] != before.sequence[before.latest]+1 {
			t.Fatal("business publication changed allocated file or destroyed previous complete record")
		}
	}
	if err := s.withLocked(func(current *authorizationAnchor, _ bool) error {
		if current.CheckpointID != a.CheckpointID {
			t.Fatal("latest signed checkpoint lost")
		}
		return s.writeLocked(a)
	}); err != nil {
		t.Fatal(err)
	}
	atomic, _ := os.Stat(s.Path)
	if os.SameFile(initial, atomic) {
		t.Fatal("non-business publication lost atomic replacement")
	}
}

func TestAuthorizationCheckpointSlotsV1Upgrade(t *testing.T) {
	s := checkpointTestStore(t)
	a := authorizationAnchor{Domain: "slot-test", Generation: 1, Version: 1, Highwater: time.Now().UTC(), CheckpointID: uuid.NewString()}
	raw := checkpointSignedTestRaw(t, s, a)
	record := make([]byte, authorizationCheckpointRecordSize)
	binary.BigEndian.PutUint32(record[:4], uint32(len(raw)))
	copy(record[4:], raw)
	legacy := append([]byte(authorizationCheckpointMagic), record...)
	legacy = append(legacy, record...)
	if err := writeFileAtomic(s.Path, legacy); err != nil {
		t.Fatal(err)
	}
	if err := s.withLocked(func(current *authorizationAnchor, exists bool) error {
		if !exists || current.CheckpointID != a.CheckpointID {
			t.Fatal("V1 checkpoint was not verified")
		}
		return s.writeBusinessLocked(a)
	}); err != nil {
		t.Fatal(err)
	}
	slots, _ := checkpointSlotsTestRead(t, s)
	if !bytes.Equal(slots.raw[slots.latest], raw) {
		t.Fatal("V1 upgrade changed signed authority")
	}
}

func TestAuthorizationCheckpointSlotsRejectDamagedV1Upgrade(t *testing.T) {
	for _, fault := range []string{"short", "extra", "header", "signature", "length", "padding"} {
		t.Run(fault, func(t *testing.T) {
			s := checkpointTestStore(t)
			a := authorizationAnchor{Domain: "slot-test", Generation: 1, Version: 1, Highwater: time.Now().UTC()}
			signed := checkpointSignedTestRaw(t, s, a)
			raw := checkpointLegacyTestContainer(signed, signed)
			start := len(raw) - authorizationCheckpointRecordSize
			switch fault {
			case "short":
				raw = raw[:len(raw)-1]
			case "extra":
				raw = append(raw, 0)
			case "header":
				raw[0] ^= 1
			case "signature":
				raw[start+4+len(signed)-4] ^= 1
			case "length":
				raw[start] = 255
			case "padding":
				raw[len(raw)-1] = 1
			}
			if err := os.WriteFile(s.Path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			called := false
			err := s.withLocked(func(*authorizationAnchor, bool) error { called = true; return s.writeBusinessLocked(a) })
			if err == nil || called {
				t.Fatal("damaged V1 tail upgraded by falling back")
			}
		})
	}
}

func TestAuthorizationCheckpointSlotsRejectEitherDamagedSlot(t *testing.T) {
	for _, fault := range []string{"header", "older-signature", "newer-signature", "sequence", "length", "padding", "signed-gap", "signed-conflict", "exhausted"} {
		t.Run(fault, func(t *testing.T) {
			s := checkpointTestStore(t)
			a := authorizationAnchor{Domain: "slot-test", Generation: 1, Version: 1, Highwater: time.Now().UTC()}
			if err := s.withLocked(func(*authorizationAnchor, bool) error { return s.writeLocked(a) }); err != nil {
				t.Fatal(err)
			}
			a.Highwater = a.Highwater.Add(time.Second)
			if err := s.withLocked(func(*authorizationAnchor, bool) error { return s.writeBusinessLocked(a) }); err != nil {
				t.Fatal(err)
			}
			slots, raw := checkpointSlotsTestRead(t, s)
			older := authorizationCheckpointHeaderSize + (1-slots.latest)*authorizationCheckpointRecordSize
			newer := authorizationCheckpointHeaderSize + slots.latest*authorizationCheckpointRecordSize
			switch fault {
			case "header":
				raw[authorizationCheckpointHeaderSize-1] = 1
			case "older-signature":
				raw[older+authorizationCheckpointRecordSize-1] ^= 1
			case "newer-signature":
				raw[newer+authorizationCheckpointRecordSize-1] ^= 1
			case "sequence":
				raw[older+7] ^= 1
			case "length":
				raw[older+8] = 255
			case "padding":
				raw[older+authorizationCheckpointRecordSize-65] = 1
			case "signed-gap", "signed-conflict", "exhausted":
				seq := uint64(10)
				if fault == "signed-conflict" {
					seq = slots.sequence[slots.latest]
				}
				if fault == "exhausted" {
					seq = math.MaxUint64
				}
				record, err := checkpointSlotRecord(slots.raw[1-slots.latest], seq, s.signer)
				if err != nil {
					t.Fatal(err)
				}
				copy(raw[older:], record)
				if fault == "exhausted" {
					copy(raw[newer:], record)
				}
			}
			if err := os.WriteFile(s.Path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			called := false
			err := s.withLocked(func(*authorizationAnchor, bool) error { called = true; return s.writeBusinessLocked(a) })
			if err == nil || fault != "exhausted" && called {
				t.Fatal("invalid slot or exhausted sequence accepted", err)
			}
		})
	}
}

type checkpointPartialCrashConfig struct {
	Path, Record, DSN, Schema string
	Offset                    int64
	Atomic                    bool
}

func TestAuthorizationCheckpointPartialCrashChild(t *testing.T) {
	path := os.Getenv("REQ002_CHECKPOINT_CRASH_CONFIG")
	if path == "" {
		t.Skip("private child process only")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("child config unavailable")
	}
	var cfg checkpointPartialCrashConfig
	if json.Unmarshal(raw, &cfg) != nil || cfg.Schema != "c_auth" && cfg.Schema != "v_auth" {
		t.Fatal("invalid child config")
	}
	pool, err := pgxpool.New(context.Background(), cfg.DSN)
	if err != nil {
		t.Fatal("child database unavailable")
	}
	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(context.Background(), `SELECT singleton_id FROM `+cfg.Schema+`.authorization_gate WHERE singleton_id=1 FOR UPDATE`); err != nil {
		t.Fatal(err)
	}
	if !cfg.Atomic {
		if _, err = tx.Exec(context.Background(), `UPDATE public.gate_cancellation_probe SET value=1`); err != nil {
			t.Fatal(err)
		}
	}
	lock, err := os.OpenFile(cfg.Path+".lock", os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	record, err := os.ReadFile(cfg.Record)
	if err != nil {
		t.Fatal(err)
	}
	target := cfg.Path
	if cfg.Atomic {
		target += ".partial-temporary"
	}
	f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.WriteAt(record[:64], cfg.Offset); err != nil {
		t.Fatal(err)
	}
	// Exit without close/rollback/sync: exercise real process interruption and
	// SQL connection loss after only the record prefix reached the page cache.
	os.Exit(79)
}

func TestAuthorizationCheckpointPartialPublicationPostgres(t *testing.T) {
	for _, atomic := range []bool{true, false} {
		name := "business-slot"
		if atomic {
			name = "snapshot-atomic"
		}
		t.Run(name, func(t *testing.T) {
			gateCancellationParties(t, func(t *testing.T, c *gateControl) {
				ctx := context.Background()
				gateCancellationProbe(t, c)
				var session SignupResult
				if c.gate.schema == "c_auth" {
					session = sessionTestAccount(t, c.lab, "slot_crash_user")
				}
				before := checkpointState(t, c)
				slots, original := checkpointSlotsTestRead(t, c.gate.anchor)
				var a authorizationAnchor
				if err := c.gate.anchor.withLocked(func(current *authorizationAnchor, _ bool) error { a = *current; return nil }); err != nil {
					t.Fatal(err)
				}
				a.CheckpointID = uuid.NewString()
				a.Highwater = a.Highwater.Add(time.Microsecond)
				a.ReadAdvance = nil
				record, err := checkpointSlotRecord(checkpointSignedTestRaw(t, c.gate.anchor, a), slots.sequence[slots.latest]+1, c.gate.anchor.signer)
				if err != nil {
					t.Fatal(err)
				}
				dir := t.TempDir()
				recordPath, configPath := filepath.Join(dir, "record"), filepath.Join(dir, "config.json")
				if err = os.WriteFile(recordPath, record, 0600); err != nil {
					t.Fatal(err)
				}
				cfg := checkpointPartialCrashConfig{Path: c.gate.anchor.Path, Record: recordPath, DSN: c.gate.pool.Config().ConnString(), Schema: c.gate.schema, Atomic: atomic, Offset: int64(authorizationCheckpointHeaderSize + (1-slots.latest)*authorizationCheckpointRecordSize)}
				data, _ := json.Marshal(cfg)
				if err = os.WriteFile(configPath, data, 0600); err != nil {
					t.Fatal(err)
				}
				cmd := exec.Command(os.Args[0], "-test.run=^TestAuthorizationCheckpointPartialCrashChild$")
				cmd.Env = append(os.Environ(), "REQ002_CHECKPOINT_CRASH_CONFIG="+configPath)
				err = cmd.Run()
				var exited *exec.ExitError
				if !errors.As(err, &exited) || exited.ExitCode() != 79 {
					t.Fatal("child did not reach partial write boundary", err)
				}
				var value int
				if err = c.gate.pool.QueryRow(ctx, `SELECT value FROM public.gate_cancellation_probe`).Scan(&value); err != nil || value != 0 {
					t.Fatal("interrupted business transaction committed", err)
				}
				after, err := os.ReadFile(c.gate.anchor.Path)
				if err != nil {
					t.Fatal(err)
				}
				retained := authorizationCheckpointHeaderSize + slots.latest*authorizationCheckpointRecordSize
				if !bytes.Equal(original[retained:retained+authorizationCheckpointRecordSize], after[retained:retained+authorizationCheckpointRecordSize]) {
					t.Fatal("partial publication destroyed last complete signed material")
				}
				_, err = c.gate.Snapshot(ctx)
				if atomic {
					if err != nil || checkpointState(t, c).generation != before.generation {
						t.Fatal("atomic read interruption required recovery", err)
					}
					if c.gate.schema == "c_auth" {
						if _, err = c.lab.c.GetCurrentSession(ctx, session.SessionToken); err != nil {
							t.Fatal("read interruption revoked the existing login", err)
						}
					}
				} else {
					if !errors.Is(err, ErrAuthorizationUnavailable) || checkpointState(t, c).state != "FROZEN" || checkpointState(t, c).generation != before.generation {
						t.Fatal("bad business slot fell back or changed generation", err)
					}
					proof := c.signed(t, c.evidence, uint64(before.generation+1), uint64(before.version+1), c.clock.now(), c.clock.now().Add(5*time.Minute))
					if err = c.provider.Store(ctx, proof); err != nil {
						t.Fatal(err)
					}
					request, err := SignAuthorizationRecovery(c.recovery, AuthorizationRecoveryRequest{Role: "authorization-recovery", Actor: "slot-test", Reason: "corrupt slot must not be silently repaired", OperationID: uuid.NewString(), Mode: AuthorizationRecoveryNormal, Evidence: proof})
					if err != nil {
						t.Fatal(err)
					}
					if err = c.gate.Recover(ctx, request); !errors.Is(err, ErrAuthorizationUnavailable) {
						t.Fatal("signed recover bypassed damaged storage", err)
					}
					if checkpointState(t, c).generation != before.generation {
						t.Fatal("corrupt store caused automatic credential revocation")
					}
					if c.gate.schema == "c_auth" {
						var intact bool
						if err = c.gate.pool.QueryRow(ctx, `SELECT bool_and(revoked_at IS NULL AND authorization_generation=$2) FROM c_auth.sessions WHERE account_id=$1`, session.AccountID, before.generation).Scan(&intact); err != nil || !intact {
							t.Fatal("damaged storage revoked stored sessions", err)
						}
					}
				}
			})
		})
	}
}

func TestAuthorizationCheckpointZeroPaddingEveryByte(t *testing.T) {
	for _, size := range []int{0, 1, 31, 32, 33, authorizationCheckpointHeaderSize, authorizationCheckpointRecordSize} {
		b := make([]byte, size)
		if !checkpointZeroPadding(b) {
			t.Fatal("zero padding rejected", size)
		}
		for i := range b {
			for _, value := range []byte{1, 128, 255} {
				b[i] = value
				if checkpointZeroPadding(b) {
					t.Fatal("nonzero padding accepted", size, i, value)
				}
			}
			b[i] = 0
		}
	}
}
