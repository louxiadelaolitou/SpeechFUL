package authcaptcha

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type memoryStore struct {
	mu      sync.Mutex
	records map[[32]byte]Record
	proofs  map[[32]byte][32]byte
}

func (s *memoryStore) Issue(_ context.Context, id [32]byte, r Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records[id] = r
	return nil
}
func (s *memoryStore) Take(_ context.Context, id [32]byte) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.records[id]
	delete(s.records, id)
	if !ok {
		return r, ErrInvalid
	}
	return r, nil
}
func (s *memoryStore) Grant(_ context.Context, id, b [32]byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.proofs[id] = b
	return nil
}
func (s *memoryStore) Consume(_ context.Context, id, b [32]byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.proofs[id]
	delete(s.proofs, id)
	if !ok || value != b {
		return ErrInvalid
	}
	return nil
}

func TestChallengeOneAttemptAndProofBinding(t *testing.T) {
	ctx := context.Background()
	store := &memoryStore{records: map[[32]byte]Record{}, proofs: map[[32]byte][32]byte{}}
	service := New(store)
	binding := Binding("login", "installation", "key", "private_user")
	for _, mode := range []string{"success", "wrong", "expired", "wrong-binding"} {
		t.Run(mode, func(t *testing.T) {
			challenge, err := service.Create(ctx, binding)
			if err != nil {
				t.Fatal(err)
			}
			if directory := os.Getenv("AUTH_CAPTCHA_QA_DIR"); directory != "" && mode == "success" {
				raw, err := json.Marshal(challenge)
				if err != nil {
					t.Fatal(err)
				}
				if err = os.MkdirAll(directory, 0700); err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(filepath.Join(directory, "challenge.json"), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if challenge.Width != 320 || challenge.Height != 160 || challenge.Background == "" || challenge.Piece == "" {
				t.Fatal("missing rendered puzzle")
			}
			id, _ := digest(challenge.ID)
			r := store.records[id]
			x := float64(r.X)
			if mode == "wrong" {
				x += 20
			}
			if mode == "expired" {
				r.Expires = time.Now().Add(-time.Second)
				store.records[id] = r
			}
			token, err := service.Solve(ctx, challenge.ID, x)
			if mode == "wrong" || mode == "expired" {
				if err != ErrInvalid {
					t.Fatal("wrong/expired puzzle accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err = service.Solve(ctx, challenge.ID, x); err != ErrInvalid {
				t.Fatal("puzzle reused")
			}
			target := binding
			if mode == "wrong-binding" {
				target = Binding("reset", "installation", "key", "private_user")
			}
			err = service.Verify(ctx, token, target)
			if mode == "wrong-binding" {
				if err != ErrInvalid {
					t.Fatal("proof crossed purpose")
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if err = service.Verify(ctx, token, binding); err != ErrInvalid {
				t.Fatal("proof reused")
			}
		})
	}
}
