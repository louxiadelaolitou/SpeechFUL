package roundmodel

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"testing"
	"time"
)

func testKey() StorageKey { var k StorageKey; k.ID = 7; k.Secret[0] = 42; return k }

func TestStorageCodecCanonicalAuthenticatedAndBounded(t *testing.T) {
	r := fixture(t, 20)
	r, _ = mustProject(t, r, 0, 0, 1, 7)
	encoded, e := Encode(r, testKey())
	if e != nil {
		t.Fatal(e)
	}
	decoded, e := Decode(encoded, testKey())
	if e != nil || !reflect.DeepEqual(decoded, r) {
		t.Fatal("roundtrip", e)
	}
	again, e := Encode(decoded, testKey())
	if e != nil || !bytes.Equal(encoded, again) {
		t.Fatal("canonical encoding")
	}
	for _, data := range [][]byte{nil, encoded[:10], append(append([]byte{}, encoded...), 0), make([]byte, MaxStorageBytes+1)} {
		if _, e := Decode(data, testKey()); !errors.Is(e, ErrCodec) {
			t.Fatal("size or trailing", e)
		}
	}
	for _, index := range []int{0, 4, 8, 12, len(encoded) - 1} {
		data := append([]byte{}, encoded...)
		data[index] ^= 1
		if _, e := Decode(data, testKey()); !errors.Is(e, ErrCodec) {
			t.Fatal("tamper", index, e)
		}
	}
	wrong := testKey()
	wrong.Secret[1] = 1
	if _, e := Decode(encoded, wrong); !errors.Is(e, ErrCodec) {
		t.Fatal("wrong key")
	}
	wrong = testKey()
	wrong.ID++
	if _, e := Decode(encoded, wrong); !errors.Is(e, ErrCodec) {
		t.Fatal("unknown key")
	}
	if _, e := Encode(r, StorageKey{}); !errors.Is(e, ErrCodec) {
		t.Fatal("missing key")
	}
	data := append([]byte{}, encoded...)
	binary.BigEndian.PutUint32(data[8:12], math.MaxUint32)
	if _, e := Decode(data, testKey()); !errors.Is(e, ErrCodec) {
		t.Fatal("length overflow")
	}
}

func TestStorageCodecRejectsAuthenticatedMalformedSchema(t *testing.T) {
	r := fixture(t, 1)
	payload, e := json.Marshal(r)
	if e != nil {
		t.Fatal(e)
	}
	cases := [][]byte{
		append(append([]byte{}, payload...), []byte(" {}")...),
		append([]byte(" "), payload...),
		bytes.Replace(payload, []byte(`"Version":1`), []byte(`"Version":1,"Version":1`), 1),
		bytes.Replace(payload, []byte(`"Version":1`), []byte(`"Version":1,"Unknown":0`), 1),
		bytes.Replace(payload, []byte(`"Version":1,`), nil, 1),
		bytes.Replace(payload, []byte(`"Deliveries":[]`), []byte(`"Deliveries":null`), 1),
		bytes.Replace(payload, []byte(`2026-10-25T08:00:00Z`), []byte(`2026-10-25T08:00:00+00:00`), 1),
		[]byte(`{"State":`),
	}
	for i, b := range cases {
		if _, e := Decode(sealPayload(b, testKey()), testKey()); !errors.Is(e, ErrCodec) {
			t.Fatal("authenticated malformed payload", i, e)
		}
	}
}

func TestModelStructuralValidation(t *testing.T) {
	cases := []struct {
		name   string
		change func(*Record)
	}{
		{"nilIDs", func(r *Record) { r.State.Manifest.IDs = nil }},
		{"duplicateIDs", func(r *Record) { r.State.Manifest.IDs[1] = r.State.Manifest.IDs[0] }},
		{"invalidID", func(r *Record) { r.State.Manifest.IDs[0] = "00000000-0000-0000-0000-000000000000" }},
		{"upperID", func(r *Record) { r.State.Manifest.IDs[0] = "AAAAAAAA-1111-4111-8111-111111111111" }},
		{"tooManyIDs", func(r *Record) {
			for i := 3; i < 103; i++ {
				r.State.Manifest.IDs = append(r.State.Manifest.IDs, testID(i))
			}
		}},
		{"badRoundID", func(r *Record) { r.State.Manifest.RoundID = "bad" }},
		{"badAccount", func(r *Record) { r.State.Manifest.Binding.Account = "bad" }},
		{"zeroSession", func(r *Record) { r.State.Manifest.Binding.Session = Digest{} }},
		{"zeroGeneration", func(r *Record) { r.State.Manifest.Binding.SessionGeneration = 0 }},
		{"zeroScope", func(r *Record) { r.State.Manifest.Binding.Scope = Digest{} }},
		{"zeroConfig", func(r *Record) { r.State.Manifest.ConfigHash = Digest{} }},
		{"badVersion", func(r *Record) { r.State.Version = 2 }},
		{"zeroRevision", func(r *Record) { r.State.Revision = 0 }},
		{"overflowRevision", func(r *Record) { r.State.Revision = math.MaxUint64 }},
		{"badLastTime", func(r *Record) { r.State.LastAt = testTime().Add(-time.Second) }},
		{"futureLastTime", func(r *Record) { r.State.LastAt = testTime().Add(time.Hour) }},
		{"badPrecision", func(r *Record) { r.State.Manifest.CreatedAt = testTime().Add(time.Nanosecond) }},
		{"badPages", func(r *Record) { r.State.Pages = nil }},
		{"overflowBits", func(r *Record) { r.State.Pages[0].Omitted = 1 << 10 }},
		{"undeliveredConsumed", func(r *Record) { r.State.Pages[0].Consumed = 1 }},
		{"noFirstAt", func(r *Record) { r.State.Pages[0].Delivered = 1 }},
		{"incompleteFirstMask", func(r *Record) { r.State.Pages[0].FirstAt = testTime() }},
		{"expiredFirstAt", func(r *Record) {
			r.State.LastAt = testTime().Add(30 * time.Minute)
			r.State.Pages[0] = Page{FirstAt: r.State.LastAt, Delivered: 3}
		}},
		{"wrongHistoryAccount", func(r *Record) { r.History.Account = testID(1001) }},
		{"nilHistory", func(r *Record) { r.History.Deliveries = nil }},
		{"zeroHistoryRevision", func(r *Record) { r.History.Revision = 0 }},
		{"tooLongDelivery", func(r *Record) { r.History.Deliveries = []Delivery{{testID(4), testTime(), testTime().Add(time.Hour)}} }},
		{"duplicateHistory", func(r *Record) { r.History.Exposures = []Exposure{{testID(4), testTime()}, {testID(4), testTime()}} }},
		{"tooLongExposure", func(r *Record) { r.History.Exposures = []Exposure{{testID(4), testTime().Add(73 * time.Hour)}} }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := fixture(t, 2)
			c.change(&r)
			if r.Validate() == nil {
				t.Fatal("accepted invalid record")
			}
			if _, e := Encode(r, testKey()); e == nil {
				t.Fatal("encoded invalid record")
			}
		})
	}
	for _, change := range []func(*Policy){func(p *Policy) { p.RoundTTL = 0 }, func(p *Policy) { p.FeedbackGrace = 11 * time.Minute }, func(p *Policy) { p.DeliveryWindow = 6 * time.Minute }, func(p *Policy) { p.ExposureWindow = time.Minute }, func(p *Policy) { p.ExposureWindow = 73 * time.Hour }, func(p *Policy) { p.DeliveryLimit = 1025 }, func(p *Policy) { p.ExposureLimit = 0 }, func(p *Policy) { p.RoundTTL += time.Nanosecond }} {
		p := ExperimentalPolicy()
		change(&p)
		if p.Validate() == nil {
			t.Fatal("accepted invalid policy")
		}
	}
}

func TestMaxHistoryStorageRoundTrip(t *testing.T) {
	r := fixture(t, 100)
	for i := 0; i < MaxDeliveryHistory; i++ {
		r.History.Deliveries = append(r.History.Deliveries, Delivery{testID(i + 1), testTime(), testTime().Add(time.Minute)})
	}
	for i := 0; i < MaxExposureHistory; i++ {
		r.History.Exposures = append(r.History.Exposures, Exposure{testID(i + 1), testTime().Add(72 * time.Hour)})
	}
	b, e := Encode(r, testKey())
	if e != nil || len(b) > MaxStorageBytes {
		t.Fatal("max legitimate record", len(b), e)
	}
	decoded, e := Decode(b, testKey())
	if e != nil || !reflect.DeepEqual(decoded, r) {
		t.Fatal(e)
	}
}

func FuzzStorageDecode(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte("RMS1"))
	f.Add(make([]byte, 44))
	s, e := New(Manifest{testID(2000), testBinding(), testDigest(3), 42, testTime(), ExperimentalPolicy(), []ID{}})
	if e != nil {
		f.Fatal(e)
	}
	valid, e := Encode(Record{s, History{testBinding().Account, 1, testTime(), []Delivery{}, []Exposure{}}}, testKey())
	if e != nil {
		f.Fatal(e)
	}
	f.Add(valid)
	f.Fuzz(func(t *testing.T, b []byte) {
		r, e := Decode(b, testKey())
		if e == nil {
			encoded, e := Encode(r, testKey())
			if e != nil || !bytes.Equal(encoded, b) {
				t.Fatal("accepted noncanonical bytes")
			}
		}
	})
}
