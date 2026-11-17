package authprivacy

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// AuthorizationEvidence is a signed, independently held time/recovery sample.
// File providers and signers are test infrastructure, not production time or
// independent operator services. Domain binds a sample to one party's auth domain.
type AuthorizationEvidence struct {
	Domain     string    `json:"domain"`
	Version    uint64    `json:"version"`
	Generation uint64    `json:"generation"`
	IssuedAt   time.Time `json:"issuedAt"`
	ValidUntil time.Time `json:"validUntil"`
	TrustedAt  time.Time `json:"trustedAt"`
}

type SignedAuthorizationEvidence struct {
	Evidence  AuthorizationEvidence `json:"evidence"`
	Signature []byte                `json:"signature"`
}

func SignAuthorizationEvidence(key ed25519.PrivateKey, e AuthorizationEvidence) (SignedAuthorizationEvidence, error) {
	if len(key) != ed25519.PrivateKeySize {
		return SignedAuthorizationEvidence{}, errors.New("invalid evidence signer")
	}
	e.IssuedAt, e.ValidUntil, e.TrustedAt = e.IssuedAt.UTC(), e.ValidUntil.UTC(), e.TrustedAt.UTC()
	message, err := json.Marshal(e)
	if err != nil {
		return SignedAuthorizationEvidence{}, err
	}
	return SignedAuthorizationEvidence{Evidence: e, Signature: ed25519.Sign(key, message)}, nil
}

type AuthorizationEvidenceProvider interface {
	Current(context.Context) (SignedAuthorizationEvidence, error)
}

// FileAuthorizationEvidenceProvider is a controllable DB-external lab source.
// The verifier never trusts its file contents without checking the signature.
type FileAuthorizationEvidenceProvider struct{ Path string }

func NewFileAuthorizationEvidenceProvider(path string) *FileAuthorizationEvidenceProvider {
	return &FileAuthorizationEvidenceProvider{Path: path}
}

func (p *FileAuthorizationEvidenceProvider) Current(ctx context.Context) (SignedAuthorizationEvidence, error) {
	if err := ctx.Err(); err != nil {
		return SignedAuthorizationEvidence{}, err
	}
	raw, err := readAuthorizationFile(p.Path, 4096)
	if err != nil {
		return SignedAuthorizationEvidence{}, err
	}
	var signed SignedAuthorizationEvidence
	if err = json.Unmarshal(raw, &signed); err != nil {
		return SignedAuthorizationEvidence{}, err
	}
	return signed, nil
}

func (p *FileAuthorizationEvidenceProvider) Store(ctx context.Context, signed SignedAuthorizationEvidence) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	raw, err := json.Marshal(signed)
	if err != nil {
		return err
	}
	return writeFileAtomic(p.Path, raw)
}

// authorizationAnchor remains outside database snapshots. It is monotonically
// advanced before a database authorization commit; an interrupted commit may
// require explicit recovery, but must never grant authority from a stale DB.
type authorizationAnchor struct {
	Domain          string                       `json:"domain"`
	Generation      uint64                       `json:"generation"`
	Version         uint64                       `json:"version"`
	Highwater       time.Time                    `json:"highwater"`
	Frozen          bool                         `json:"frozen"`
	OfflineEvidence *SignedAuthorizationEvidence `json:"offlineEvidence,omitempty"`
	CheckpointID    string                       `json:"checkpointId,omitempty"`
	ReadAdvance     *authorizationReadAdvance    `json:"readAdvance,omitempty"`
}

// A Snapshot transaction changes only the Gate checkpoint. Its signed receipt
// permits replaying that exact change after an interrupted SQL commit, provided
// SQL still has the uniquely identified predecessor. Business commits never
// carry this receipt: an uncertain business outcome must remain fail closed.
type authorizationReadAdvance struct {
	PreviousID        string    `json:"previousId"`
	PreviousVersion   uint64    `json:"previousVersion"`
	PreviousHighwater time.Time `json:"previousHighwater"`
	IssuedAt          time.Time `json:"issuedAt"`
	ValidUntil        time.Time `json:"validUntil"`
}

type signedAuthorizationAnchor struct {
	Anchor    authorizationAnchor `json:"anchor"`
	Signature []byte              `json:"signature"`
}

type FileAuthorizationAnchorStore struct {
	Path       string
	signer     ed25519.PrivateKey
	public     ed25519.PublicKey
	signatures *authorizationSignatureMemo
}

func NewFileAuthorizationAnchorStore(path string, key ed25519.PrivateKey) (*FileAuthorizationAnchorStore, error) {
	if path == "" || len(key) != ed25519.PrivateKeySize {
		return nil, errors.New("invalid independent authorization anchor")
	}
	private := append(ed25519.PrivateKey(nil), key...)
	return &FileAuthorizationAnchorStore{Path: path, signer: private, public: private.Public().(ed25519.PublicKey), signatures: &authorizationSignatureMemo{}}, nil
}

// withLocked serializes all gate instances sharing this anchor, including
// instances in other processes. Callers lock the database gate row first.
func (s *FileAuthorizationAnchorStore) withLocked(fn func(*authorizationAnchor, bool) error) error {
	lockFile, err := os.OpenFile(s.Path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lockFile.Close()
	if err = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
	a, exists, err := s.readLocked()
	if err != nil {
		return err
	}
	return fn(&a, exists)
}

func (s *FileAuthorizationAnchorStore) readLocked() (authorizationAnchor, bool, error) {
	raw, err := readAuthorizationCheckpointSlots(s.Path, s.public, s.signatures)
	if errors.Is(err, os.ErrNotExist) {
		return authorizationAnchor{}, false, nil
	}
	if err != nil {
		return authorizationAnchor{}, false, err
	}
	var signed signedAuthorizationAnchor
	if err = json.Unmarshal(raw, &signed); err != nil {
		return authorizationAnchor{}, false, err
	}
	message, err := json.Marshal(signed.Anchor)
	if err != nil || !s.signatures.verify(s.public, message, signed.Signature) {
		return authorizationAnchor{}, false, errors.New("invalid independent anchor signature")
	}
	return signed.Anchor, true, nil
}

func readAuthorizationFile(path string, max int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > max {
		return nil, errors.New("authorization evidence file too large")
	}
	return raw, nil
}

func (s *FileAuthorizationAnchorStore) writeLocked(a authorizationAnchor) error {
	return s.writeCheckpointLocked(a, false)
}

// Only a final business commit uses the fixed-slot fast publication path.
// Snapshot, explicit freeze and recovery retain whole-file atomic publication.
func (s *FileAuthorizationAnchorStore) writeBusinessLocked(a authorizationAnchor) error {
	return s.writeCheckpointLocked(a, true)
}

func (s *FileAuthorizationAnchorStore) writeCheckpointLocked(a authorizationAnchor, business bool) error {
	a.Highwater = a.Highwater.UTC()
	message, err := json.Marshal(a)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(signedAuthorizationAnchor{Anchor: a, Signature: ed25519.Sign(s.signer, message)})
	if err != nil {
		return err
	}
	return writeAuthorizationCheckpointSlots(s.Path, raw, s.signer, business, s.signatures)
}

// Deployment preflight compares the writer format independently of the signed
// Gate schema. V2 readers also accept legacy JSON and V1 append containers.
const authorizationCheckpointWriteMagic = "HNUHOLE-AUTH-CHECKPOINT-V2\n"

// The V1 reader is retained for one-time atomic upgrade. It never skips a bad
// tail or chooses a previous record, including when the last record is damaged.
const authorizationCheckpointMagic = "HNUHOLE-AUTH-CHECKPOINT-V1\n"
const authorizationCheckpointRecordSize = 16 * 1024
const authorizationCheckpointMaxRecords = 128
const authorizationCheckpointJSONLimit = 8192

func readAuthorizationCheckpoint(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("invalid independent anchor file")
	}
	header := make([]byte, len(authorizationCheckpointMagic))
	n, err := f.ReadAt(header, 0)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if n != len(header) || string(header) != authorizationCheckpointMagic {
		// Existing signed JSON checkpoints remain readable. An unknown journal
		// header is not interpreted as an older journal or a previous record.
		if info.Size() > authorizationCheckpointJSONLimit {
			return nil, errors.New("authorization evidence file too large")
		}
		raw := make([]byte, info.Size())
		_, err := f.ReadAt(raw, 0)
		return raw, err
	}
	length := info.Size() - int64(len(header))
	if length <= 0 || length%authorizationCheckpointRecordSize != 0 || length > authorizationCheckpointRecordSize*authorizationCheckpointMaxRecords {
		return nil, errors.New("invalid independent anchor journal length")
	}
	record := make([]byte, authorizationCheckpointRecordSize)
	if _, err = f.ReadAt(record, info.Size()-authorizationCheckpointRecordSize); err != nil {
		return nil, err
	}
	size := int(binary.BigEndian.Uint32(record[:4]))
	if size <= 0 || size > authorizationCheckpointJSONLimit {
		return nil, errors.New("invalid independent anchor journal record")
	}
	for _, value := range record[4+size:] {
		if value != 0 {
			return nil, errors.New("invalid independent anchor journal padding")
		}
	}
	return record[4 : 4+size], nil
}

type authorizationCheckpointWriter interface {
	Write([]byte) (int, error)
	Sync() error
	Close() error
}

func publishAuthorizationCheckpointRecord(f authorizationCheckpointWriter, record []byte) error {
	n, err := f.Write(record)
	if err == nil && n != len(record) {
		err = io.ErrShortWrite
	}
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	return err
}

func writeFileAtomic(path string, raw []byte) error {
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, ".authgate-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if err = temp.Chmod(0600); err == nil {
		_, err = temp.Write(raw)
	}
	if err == nil {
		err = temp.Sync()
	}
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(temp.Name(), path); err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	if err = d.Sync(); err != nil {
		return fmt.Errorf("sync independent anchor directory: %w", err)
	}
	return nil
}

func equalSignedAuthorizationEvidence(a, b SignedAuthorizationEvidence) bool {
	x, errX := json.Marshal(a.Evidence)
	y, errY := json.Marshal(b.Evidence)
	return errX == nil && errY == nil && bytes.Equal(x, y) && bytes.Equal(a.Signature, b.Signature)
}
