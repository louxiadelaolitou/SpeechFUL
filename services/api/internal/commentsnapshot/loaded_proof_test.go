package commentsnapshot

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestLoadedProofDetectsChanges(t *testing.T) {
	s, _ := realStore(t, 10)
	ctx := context.Background()
	snapshot, identity := sample(s, [32]byte{1})
	p, e := s.Prepare(ctx, snapshot, identity, LegacyUsage{})
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Publish(ctx, p); e != nil {
		t.Fatal(e)
	}
	loaded, e := s.Load(ctx, p.Snapshot.Scope, p.Snapshot.Ref, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	proof, e := s.ProveLoaded(loaded)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.ValidateLoaded(ctx, proof, time.Now()); e != nil {
		t.Fatal(e)
	}
	data, e := s.client.Get(ctx, s.key(loaded.Ref.ID)).Bytes()
	if e != nil {
		t.Fatal(e)
	}
	data[payloadHeader] ^= 1
	if e = s.client.Set(ctx, s.key(loaded.Ref.ID), data, time.Minute).Err(); e != nil {
		t.Fatal(e)
	}
	if e = s.ValidateLoaded(ctx, proof, time.Now()); !errors.Is(e, ErrIntegrity) {
		t.Fatal("same-size payload corruption escaped", e)
	}
	if e = s.client.Del(ctx, s.key(loaded.Ref.ID)).Err(); e != nil {
		t.Fatal(e)
	}
	if e = s.ValidateLoaded(ctx, proof, time.Now()); !errors.Is(e, ErrInvalid) {
		t.Fatal("deleted payload survived", e)
	}
}
