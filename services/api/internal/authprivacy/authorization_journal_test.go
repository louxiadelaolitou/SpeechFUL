package authprivacy

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type checkpointFaultAppender struct {
	written                           int
	writeError, syncError, closeError error
	synced, closed                    bool
}

func (f *checkpointFaultAppender) Write([]byte) (int, error) { return f.written, f.writeError }
func (f *checkpointFaultAppender) Sync() error               { f.synced = true; return f.syncError }
func (f *checkpointFaultAppender) Close() error              { f.closed = true; return f.closeError }

func TestAuthorizationCheckpointAppendFailures(t *testing.T) {
	fault := errors.New("injected checkpoint I/O failure")
	for _, f := range []*checkpointFaultAppender{
		{written: 3}, {written: 3, writeError: fault}, {written: 4, syncError: fault}, {written: 4, closeError: fault},
	} {
		if err := publishAuthorizationCheckpointRecord(f, []byte("test")); err == nil || !f.closed {
			t.Fatal("checkpoint failure acknowledged", f, err)
		}
		if f.written != 4 && f.synced {
			t.Fatal("short write attempted publication")
		}
	}
}

func checkpointTestStore(t *testing.T) *FileAuthorizationAnchorStore {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewFileAuthorizationAnchorStore(filepath.Join(t.TempDir(), "anchor"), key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestAuthorizationCheckpointLegacyUpgradeAndAtomicPublication(t *testing.T) {
	s := checkpointTestStore(t)
	a := authorizationAnchor{Domain: "checkpoint-test", Generation: 1, Version: 1, Highwater: time.Now().UTC()}
	message, _ := json.Marshal(a)
	legacy, _ := json.Marshal(signedAuthorizationAnchor{Anchor: a, Signature: ed25519.Sign(s.signer, message)})
	if err := writeFileAtomic(s.Path, legacy); err != nil {
		t.Fatal(err)
	}
	for n := 0; n < authorizationCheckpointMaxRecords+3; n++ {
		if err := s.withLocked(func(current *authorizationAnchor, exists bool) error {
			if !exists || !current.Highwater.Equal(a.Highwater) {
				t.Fatal("lost checkpoint")
			}
			a.Highwater = a.Highwater.Add(time.Microsecond)
			return s.writeLocked(a)
		}); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(s.Path)
		if err != nil || info.Size() > int64(len(authorizationCheckpointMagic)+authorizationCheckpointRecordSize*authorizationCheckpointMaxRecords) || info.Mode().Perm() != 0600 {
			t.Fatal("invalid bounded checkpoint", info, err)
		}
	}
	if err := s.withLocked(func(current *authorizationAnchor, _ bool) error {
		if !current.Highwater.Equal(a.Highwater) {
			t.Fatal("atomic publication changed latest checkpoint")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAuthorizationCheckpointRejectsDamagedTail(t *testing.T) {
	for _, kind := range []string{"short", "extra", "length", "signature", "outer-signature", "slot0-padding", "slot1-padding", "header-padding", "header", "oversize"} {
		t.Run(kind, func(t *testing.T) {
			s := checkpointTestStore(t)
			a := authorizationAnchor{Domain: "checkpoint-test", Generation: 1, Version: 1, Highwater: time.Now().UTC()}
			for n := 0; n < 2; n++ {
				a.Highwater = a.Highwater.Add(time.Microsecond)
				if err := s.withLocked(func(*authorizationAnchor, bool) error { return s.writeLocked(a) }); err != nil {
					t.Fatal(err)
				}
			}
			raw, err := os.ReadFile(s.Path)
			if err != nil {
				t.Fatal(err)
			}
			before := append([]byte(nil), raw...)
			start := len(raw) - authorizationCheckpointRecordSize
			resign := func(offset int) {
				end := offset + authorizationCheckpointRecordSize - ed25519.SignatureSize
				message := append([]byte(authorizationCheckpointWriteMagic), raw[offset:end]...)
				copy(raw[end:end+ed25519.SignatureSize], ed25519.Sign(s.signer, message))
			}
			switch kind {
			case "short":
				raw = raw[:len(raw)-1]
			case "extra":
				raw = append(raw, 0)
			case "length":
				raw[start+8] = 255
			case "signature":
				i := bytes.Index(raw[start:], []byte(`"signature":"`))
				if i < 0 {
					t.Fatal("missing signature")
				}
				index := start + i + len(`"signature":"`)
				if raw[index] == 'A' {
					raw[index] = 'B'
				} else {
					raw[index] = 'A'
				}
				resign(start)                                              // Valid base64 and outer signature, invalid inner signature.
				copy(raw[authorizationCheckpointHeaderSize:], raw[start:]) // Equal sequence requires identical records.
			case "outer-signature":
				raw[len(raw)-1] ^= 1
			case "slot0-padding", "slot1-padding":
				offset := authorizationCheckpointHeaderSize
				if kind == "slot1-padding" {
					offset += authorizationCheckpointRecordSize
				}
				index := offset + authorizationCheckpointRecordSize - ed25519.SignatureSize - 1
				if raw[index] != 0 {
					t.Fatal("fixture index is not zero padding")
				}
				raw[index] = 1
				resign(offset) // Even correctly signed nonzero padding is invalid.
			case "header-padding":
				index := authorizationCheckpointHeaderSize - 1
				if raw[index] != 0 {
					t.Fatal("fixture index is not header padding")
				}
				raw[index] = 1
			case "header":
				raw[0] ^= 1
			case "oversize":
				raw = append(raw, make([]byte, authorizationCheckpointRecordSize*authorizationCheckpointMaxRecords)...)
			}
			if bytes.Equal(before, raw) {
				t.Fatal("fault injection did not change checkpoint bytes")
			}
			if err := os.WriteFile(s.Path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			called := false
			if err := s.withLocked(func(*authorizationAnchor, bool) error { called = true; return nil }); err == nil || called {
				t.Fatal("damaged tail fell back to previous checkpoint")
			}
		})
	}
}

func TestAuthorizationCheckpointInstancesSerialize(t *testing.T) {
	s := checkpointTestStore(t)
	a := authorizationAnchor{Domain: "checkpoint-test", Generation: 1, Version: 1, Highwater: time.Now().UTC()}
	if err := s.withLocked(func(*authorizationAnchor, bool) error { return s.writeLocked(a) }); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errors := make(chan error, 4)
	for n := 0; n < 4; n++ {
		other, err := NewFileAuthorizationAnchorStore(s.Path, s.signer)
		if err != nil {
			t.Fatal(err)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 0; n < 40; n++ {
				if err := other.withLocked(func(current *authorizationAnchor, _ bool) error {
					current.Version++
					return other.writeLocked(*current)
				}); err != nil {
					errors <- err
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}
	if err := s.withLocked(func(current *authorizationAnchor, _ bool) error {
		if current.Version != 161 {
			t.Fatal("lost concurrent checkpoint", current.Version)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAuthorizationCheckpointProcessesSerialize(t *testing.T) {
	if os.Getenv("REQ002_CHECKPOINT_CHILD") == "1" {
		key, err := hex.DecodeString(os.Getenv("REQ002_CHECKPOINT_TEST_KEY"))
		if err != nil {
			t.Fatal(err)
		}
		s, err := NewFileAuthorizationAnchorStore(os.Getenv("REQ002_CHECKPOINT_TEST_PATH"), key)
		if err != nil {
			t.Fatal(err)
		}
		for n := 0; n < 40; n++ {
			if err = s.withLocked(func(current *authorizationAnchor, _ bool) error {
				current.Version++
				return s.writeLocked(*current)
			}); err != nil {
				t.Fatal(err)
			}
		}
		return
	}
	s := checkpointTestStore(t)
	a := authorizationAnchor{Domain: "checkpoint-process-test", Generation: 1, Version: 1, Highwater: time.Now().UTC()}
	if err := s.withLocked(func(*authorizationAnchor, bool) error { return s.writeLocked(a) }); err != nil {
		t.Fatal(err)
	}
	commands := []*exec.Cmd{}
	for n := 0; n < 4; n++ {
		cmd := exec.Command(os.Args[0], "-test.run=^TestAuthorizationCheckpointProcessesSerialize$")
		cmd.Env = append(os.Environ(), "REQ002_CHECKPOINT_CHILD=1", "REQ002_CHECKPOINT_TEST_KEY="+hex.EncodeToString(s.signer), "REQ002_CHECKPOINT_TEST_PATH="+s.Path)
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		commands = append(commands, cmd)
	}
	for _, cmd := range commands {
		if err := cmd.Wait(); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.withLocked(func(current *authorizationAnchor, _ bool) error {
		if current.Version != 161 {
			t.Fatal("lost cross-process checkpoint", current.Version)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// Truncation at an exact record boundary leaves a valid older signature. The
// independent SQL/checkpoint equality check, never a signature alone, must stop
// release and persist a freeze in that case.
func TestAuthorizationCheckpointRecordTruncationFreezesPostgres(t *testing.T) {
	c := newGateControl(t)
	previous, err := readAuthorizationCheckpointSlots(c.gate.anchor.Path, c.gate.anchor.public, nil)
	if err != nil {
		t.Fatal(err)
	}
	c.clock.set(c.clock.now().Add(time.Second))
	if _, err := c.gate.Snapshot(context.Background()); err != nil {
		t.Fatal(err)
	}
	current, err := readAuthorizationCheckpointSlots(c.gate.anchor.Path, c.gate.anchor.public, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = writeFileAtomic(c.gate.anchor.Path, checkpointLegacyTestContainer(previous, current)); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(c.gate.anchor.Path)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Truncate(c.gate.anchor.Path, info.Size()-authorizationCheckpointRecordSize); err != nil {
		t.Fatal(err)
	}
	if _, err = c.gate.Snapshot(context.Background()); err == nil {
		t.Fatal("accepted truncated checkpoint")
	}
	var state string
	if err = c.lab.cp.QueryRow(context.Background(), `SELECT gate_state FROM c_auth.authorization_gate WHERE singleton_id=1`).Scan(&state); err != nil || state != "FROZEN" {
		t.Fatal("freeze not persistent", state, err)
	}
}
