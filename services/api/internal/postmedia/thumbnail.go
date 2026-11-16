package postmedia

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"os/exec"
)

const ThumbnailVariant = "thumbnail-v1-640"

// ThumbnailRecord is a validated static preview, never an original or a read
// capability. It is immutable while shared by concurrent preparation jobs.
type ThumbnailRecord struct {
	Bytes  []byte `json:"-"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	SHA256 string `json:"sha256"`
}

func (r ThumbnailRecord) Valid() bool {
	if len(r.Bytes) < 12 || len(r.Bytes) > MaxThumbnailBytes || r.Width < 1 || r.Width > 640 || r.Height < 1 || r.Height > 640 ||
		string(r.Bytes[:4]) != "RIFF" || string(r.Bytes[8:12]) != "WEBP" {
		return false
	}
	h := sha256.Sum256(r.Bytes)
	return r.SHA256 == hex.EncodeToString(h[:])
}

// Reuse requires a versioned encoder and validation of its actual output.
// Original-only and legacy Thumbnailer implementations remain usable.
type ReusableThumbnailer interface {
	Thumbnailer
	ThumbnailEncoderVersion(context.Context) (string, error)
	ThumbnailRecord(context.Context, []byte) (ThumbnailRecord, error)
}

func (v *ProcessValidator) ThumbnailEncoderVersion(ctx context.Context) (string, error) {
	source, err := os.ReadFile(v.script)
	if err != nil || sha256.Sum256(source) != v.scriptHash {
		return "", ErrUnavailable
	}
	child, cancel := context.WithTimeout(ctx, thumbnailTimeout)
	defer cancel()
	cmd := exec.CommandContext(child, v.python, "-I", "-B", v.script, "--thumbnail-encoder")
	cmd.Env = []string{"PATH=/usr/bin:/bin", "LANG=C.UTF-8"}
	var out limitedOutput
	cmd.Stdout = &out
	cmd.Stderr = io.Discard
	if cmd.Run() != nil || out.Len() < 1 || out.Len() > 256 {
		return "", ErrUnavailable
	}
	version := sha256.Sum256(append(append([]byte("postmedia-thumbnail-record/v1\x00"), v.scriptHash[:]...), out.Bytes()...))
	return hex.EncodeToString(version[:]), nil
}

func (v *ProcessValidator) ThumbnailRecord(ctx context.Context, data []byte) (ThumbnailRecord, error) {
	raw, err := v.runThumbnail(ctx, data, true)
	if err != nil {
		return ThumbnailRecord{}, err
	}
	line, payload, ok := bytes.Cut(raw, []byte{'\n'})
	if !ok || len(line) > 512 {
		return ThumbnailRecord{}, ErrUnavailable
	}
	var r ThumbnailRecord
	d := json.NewDecoder(bytes.NewReader(line))
	d.DisallowUnknownFields()
	if d.Decode(&r) != nil || d.Decode(new(any)) != io.EOF {
		return ThumbnailRecord{}, ErrUnavailable
	}
	r.Bytes = payload
	if !r.Valid() {
		return ThumbnailRecord{}, ErrUnavailable
	}
	return r, nil
}
