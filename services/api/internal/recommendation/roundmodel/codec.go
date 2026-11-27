package roundmodel

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"io"
)

const MaxStorageBytes = 512 * 1024
const headerBytes = 12

var storageDomain = []byte("HNUHOLE/RECOMMENDATION/PRIVATE-STORAGE/V1\x00")

// StorageKey is explicitly injected. B3a uses synthetic test keys only. This
// codec is not a cursor, an intent token, encryption, or an authorization proof.
type StorageKey struct {
	ID     uint32
	Secret [32]byte
}

func (k StorageKey) valid() bool { return k.ID > 0 && k.Secret != ([32]byte{}) }

func Encode(r Record, key StorageKey) ([]byte, error) {
	if !key.valid() {
		return nil, ErrCodec
	}
	if err := r.Validate(); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(r)
	if err != nil || len(payload) > MaxStorageBytes-headerBytes-sha256.Size {
		return nil, ErrCodec
	}
	return sealPayload(payload, key), nil
}

func sealPayload(payload []byte, key StorageKey) []byte {
	data := make([]byte, headerBytes+len(payload), headerBytes+len(payload)+sha256.Size)
	copy(data, "RMS1")
	binary.BigEndian.PutUint32(data[4:8], key.ID)
	binary.BigEndian.PutUint32(data[8:12], uint32(len(payload)))
	copy(data[12:], payload)
	mac := hmac.New(sha256.New, key.Secret[:])
	mac.Write(storageDomain)
	mac.Write(data)
	return append(data, mac.Sum(nil)...)
}

func Decode(data []byte, key StorageKey) (Record, error) {
	if !key.valid() || len(data) < headerBytes+sha256.Size || len(data) > MaxStorageBytes || string(data[:4]) != "RMS1" || binary.BigEndian.Uint32(data[4:8]) != key.ID {
		return Record{}, ErrCodec
	}
	n := binary.BigEndian.Uint32(data[8:12])
	if uint64(n)+headerBytes+sha256.Size != uint64(len(data)) {
		return Record{}, ErrCodec
	}
	mac := hmac.New(sha256.New, key.Secret[:])
	mac.Write(storageDomain)
	mac.Write(data[:len(data)-sha256.Size])
	if !hmac.Equal(mac.Sum(nil), data[len(data)-sha256.Size:]) {
		return Record{}, ErrCodec
	}
	payload := data[headerBytes : len(data)-sha256.Size]
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var r Record
	if decoder.Decode(&r) != nil {
		return Record{}, ErrCodec
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return Record{}, ErrCodec
	}
	canonical, err := json.Marshal(r)
	// Reject duplicate keys, omitted required fields, alternate time encodings,
	// whitespace, trailing values, and null-vs-empty aliases as noncanonical.
	if err != nil || !bytes.Equal(canonical, payload) || r.Validate() != nil {
		return Record{}, ErrCodec
	}
	return r, nil
}
