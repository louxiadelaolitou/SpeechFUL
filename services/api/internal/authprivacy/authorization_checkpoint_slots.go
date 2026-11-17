package authprivacy

import (
	"bytes"
	"crypto/ed25519"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"os"
)

const authorizationCheckpointHeaderSize = 4096
const authorizationCheckpointSlotsSize = authorizationCheckpointHeaderSize + 2*authorizationCheckpointRecordSize

type authorizationCheckpointSlots struct {
	raw      [2][]byte
	sequence [2]uint64
	latest   int
}

func checkpointSlotRecord(raw []byte, sequence uint64, key ed25519.PrivateKey) ([]byte, error) {
	if len(raw) == 0 || len(raw) > authorizationCheckpointJSONLimit || sequence == 0 {
		return nil, errors.New("invalid independent checkpoint slot")
	}
	record := make([]byte, authorizationCheckpointRecordSize)
	binary.BigEndian.PutUint64(record[:8], sequence)
	binary.BigEndian.PutUint32(record[8:12], uint32(len(raw)))
	copy(record[12:], raw)
	end := len(record) - ed25519.SignatureSize
	// Preserve the exact V2 signed bytes without a second heap-sized record.
	var signed [len(authorizationCheckpointWriteMagic) + authorizationCheckpointRecordSize - ed25519.SignatureSize]byte
	copy(signed[:], authorizationCheckpointWriteMagic)
	copy(signed[len(authorizationCheckpointWriteMagic):], record[:end])
	message := signed[:]
	copy(record[end:], ed25519.Sign(key, message))
	return record, nil
}

// Count examines every byte using the standard library's optimized primitive.
// Empty padding is valid; one nonzero byte anywhere must still be rejected.
func checkpointZeroPadding(p []byte) bool {
	return bytes.Count(p, []byte{0}) == len(p)
}

func decodeCheckpointSlots(f *os.File, public ed25519.PublicKey, memo *authorizationSignatureMemo) (authorizationCheckpointSlots, error) {
	var slots authorizationCheckpointSlots
	info, err := f.Stat()
	if err != nil {
		return slots, err
	}
	if !info.Mode().IsRegular() || info.Size() != authorizationCheckpointSlotsSize {
		return slots, errors.New("invalid independent checkpoint slots length")
	}
	// The complete fresh container is scratch only. Returned JSON owns a copy,
	// so no caller retains the padded container or borrows this stack storage.
	var scratch [authorizationCheckpointSlotsSize]byte
	raw := scratch[:]
	if _, err = f.ReadAt(raw, 0); err != nil {
		return slots, err
	}
	if string(raw[:len(authorizationCheckpointWriteMagic)]) != authorizationCheckpointWriteMagic {
		return slots, errors.New("invalid independent checkpoint slots header")
	}
	if !checkpointZeroPadding(raw[len(authorizationCheckpointWriteMagic):authorizationCheckpointHeaderSize]) {
		return slots, errors.New("invalid independent checkpoint header padding")
	}
	for n := 0; n < 2; n++ {
		start := authorizationCheckpointHeaderSize + n*authorizationCheckpointRecordSize
		record := raw[start : start+authorizationCheckpointRecordSize]
		sequence := binary.BigEndian.Uint64(record[:8])
		size := int(binary.BigEndian.Uint32(record[8:12]))
		end := len(record) - ed25519.SignatureSize
		if sequence == 0 || size <= 0 || size > authorizationCheckpointJSONLimit {
			return slots, errors.New("invalid independent checkpoint slot metadata")
		}
		if !checkpointZeroPadding(record[12+size : end]) {
			return slots, errors.New("invalid independent checkpoint slot padding")
		}
		if !memo.verifyWithPrefix(public, authorizationCheckpointWriteMagic, record[:end], record[end:]) {
			return slots, errors.New("invalid independent checkpoint slot signature")
		}
		slots.sequence[n], slots.raw[n] = sequence, bytes.Clone(record[12:12+size])
	}
	if slots.sequence[1] > slots.sequence[0] {
		slots.latest = 1
	}
	gap := slots.sequence[slots.latest] - slots.sequence[1-slots.latest]
	if gap > 1 || gap == 0 && !bytes.Equal(slots.raw[0], slots.raw[1]) {
		return slots, errors.New("inconsistent independent checkpoint slots")
	}
	return slots, nil
}

func readAuthorizationCheckpointSlots(path string, public ed25519.PublicKey, memo *authorizationSignatureMemo) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	header := make([]byte, len(authorizationCheckpointWriteMagic))
	_, err = f.ReadAt(header, 0)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if string(header) != authorizationCheckpointWriteMagic {
		return readAuthorizationCheckpoint(path)
	}
	slots, err := decodeCheckpointSlots(f, public, memo)
	if err != nil {
		return nil, err
	}
	return slots.raw[slots.latest], nil
}

func writeCheckpointSlotsAtomic(path string, record []byte) error {
	raw := make([]byte, authorizationCheckpointSlotsSize)
	copy(raw, authorizationCheckpointWriteMagic)
	copy(raw[authorizationCheckpointHeaderSize:], record)
	copy(raw[authorizationCheckpointHeaderSize+authorizationCheckpointRecordSize:], record)
	return writeFileAtomic(path, raw)
}

func writeAuthorizationCheckpointSlots(path string, raw []byte, key ed25519.PrivateKey, business bool, memo *authorizationSignatureMemo) error {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if errors.Is(err, os.ErrNotExist) {
		record, err := checkpointSlotRecord(raw, 1, key)
		if err != nil {
			return err
		}
		return writeCheckpointSlotsAtomic(path, record)
	}
	if err != nil {
		return err
	}
	defer f.Close()
	header := make([]byte, len(authorizationCheckpointWriteMagic))
	_, err = f.ReadAt(header, 0)
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if string(header) != authorizationCheckpointWriteMagic {
		// withLocked has verified the legacy JSON/V1 checkpoint before entering
		// this writer. Upgrade publishes the complete allocated container once.
		record, err := checkpointSlotRecord(raw, 1, key)
		if err != nil {
			return err
		}
		return writeCheckpointSlotsAtomic(path, record)
	}
	slots, err := decodeCheckpointSlots(f, key.Public().(ed25519.PublicKey), memo)
	if err != nil {
		return err
	}
	if slots.sequence[slots.latest] == math.MaxUint64 {
		return errors.New("independent checkpoint sequence exhausted")
	}
	record, err := checkpointSlotRecord(raw, slots.sequence[slots.latest]+1, key)
	if err != nil {
		return err
	}
	if !business {
		return writeCheckpointSlotsAtomic(path, record)
	}
	// Only the older slot is replaced. A partial write leaves the latest
	// complete signed material intact, but normal readers reject either bad
	// slot: they must never fall back to old authority after an uncertain write.
	// The immutable header, allocated blocks and size were already fsynced by
	// creation/upgrade. Data sync still persists all metadata required to read
	// the new record; inode timestamps are not part of the authority proof.
	offset := int64(authorizationCheckpointHeaderSize + (1-slots.latest)*authorizationCheckpointRecordSize)
	if _, err = f.Seek(offset, io.SeekStart); err != nil {
		return err
	}
	return publishAuthorizationCheckpointRecord(authorizationCheckpointDataFile{f}, record)
}

type authorizationCheckpointDataFile struct{ *os.File }

func (f authorizationCheckpointDataFile) Sync() error {
	return syncAuthorizationCheckpointData(f.File)
}
