package authprivacy

import (
	"bytes"
	"crypto/ed25519"
	"encoding/binary"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestAuthorizationCheckpointSlotBufferOwnership(t *testing.T) {
	key := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{29}, ed25519.SeedSize))
	public := key.Public().(ed25519.PublicKey)
	// Exercise both an ordinary payload and the complete accepted payload bound.
	for _, size := range []int{2, authorizationCheckpointJSONLimit} {
		raw := append([]byte("{}"), bytes.Repeat([]byte(" "), size-2)...)
		record, e := checkpointSlotRecord(raw, 7, key)
		if e != nil {
			t.Fatal(e)
		}
		// The signed wire message must remain byte-identical to the V2 framing.
		message := append([]byte(authorizationCheckpointWriteMagic), record[:len(record)-ed25519.SignatureSize]...)
		if binary.BigEndian.Uint64(record[:8]) != 7 || int(binary.BigEndian.Uint32(record[8:12])) != size || !bytes.Equal(record[12:12+size], raw) || !ed25519.Verify(public, message, record[len(record)-ed25519.SignatureSize:]) {
			t.Fatal("V2 record changed")
		}
		original := bytes.Clone(raw)
		raw[0] ^= 1
		if !bytes.Equal(record[12:12+size], original) {
			t.Fatal("record retained caller payload")
		}
		path := filepath.Join(t.TempDir(), "anchor")
		if e = writeCheckpointSlotsAtomic(path, record); e != nil {
			t.Fatal(e)
		}
		f, e := os.Open(path)
		if e != nil {
			t.Fatal(e)
		}
		memo := &authorizationSignatureMemo{}
		first, e := decodeCheckpointSlots(f, public, memo)
		if e != nil {
			t.Fatal(e)
		}
		if e = f.Close(); e != nil {
			t.Fatal(e)
		}
		for n := 0; n < 12; n++ {
			next, e := checkpointSlotRecord([]byte(`{"changed":true}`), uint64(8+n), key)
			if e != nil {
				t.Fatal(e)
			}
			if e = writeCheckpointSlotsAtomic(path, next); e != nil {
				t.Fatal(e)
			}
			f, e = os.Open(path)
			if e != nil {
				t.Fatal(e)
			}
			later, e := decodeCheckpointSlots(f, public, memo)
			if e != nil {
				t.Fatal(e)
			}
			if e = f.Close(); e != nil {
				t.Fatal(e)
			}
			later.raw[0][0] ^= 1
			runtime.GC()
			if !bytes.Equal(first.raw[0], original) || !bytes.Equal(first.raw[1], original) {
				t.Fatal("returned JSON borrowed reusable scratch")
			}
		}
		first.raw[0][0] ^= 1
		if !bytes.Equal(first.raw[1], original) {
			t.Fatal("two returned payloads alias each other")
		}
	}
}

func BenchmarkAuthorizationCheckpointBuffers(b *testing.B) {
	key := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{29}, ed25519.SeedSize))
	store, e := NewFileAuthorizationAnchorStore(filepath.Join(b.TempDir(), "anchor"), key)
	if e != nil {
		b.Fatal(e)
	}
	a := authorizationAnchor{Domain: "allocation-diagnostic", Generation: 1, Version: 1, Highwater: time.Now().UTC(), CheckpointID: "11111111-1111-1111-1111-111111111111"}
	if e = store.writeLocked(a); e != nil {
		b.Fatal(e)
	}
	f, e := os.Open(store.Path)
	if e != nil {
		b.Fatal(e)
	}
	defer f.Close()
	// Warm pure mathematical memo; every iteration still rereads and validates bytes.
	if _, e = decodeCheckpointSlots(f, store.public, store.signatures); e != nil {
		b.Fatal(e)
	}
	b.Run("decode", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			slots, e := decodeCheckpointSlots(f, store.public, store.signatures)
			if e != nil || len(slots.raw[slots.latest]) == 0 {
				b.Fatal(e)
			}
		}
	})
	b.Run("record", func(b *testing.B) {
		raw := []byte(`{"diagnostic":"fixed-size-owned-payload"}`)
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, e = checkpointSlotRecord(raw, uint64(i+1), key); e != nil {
				b.Fatal(e)
			}
		}
	})
	b.Run("read_anchor", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, _, e = store.readLocked(); e != nil {
				b.Fatal(e)
			}
		}
	})
}
