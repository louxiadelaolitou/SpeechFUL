package posts

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"time"
)

// A refresh freezes one new range. Its newest five are released first; the
// remainder is drained before a subsequent new range is opened. This bounds
// cursor size even if publishers keep adding posts while the reader refreshes.
type FeedRefreshPosition struct {
	AccountID     string    `json:"a"`
	ChannelID     string    `json:"c"`
	Revision      int64     `json:"r"`
	ExpiresAt     time.Time `json:"x"`
	HeadAt        time.Time `json:"h"`
	HeadOrdinal   int64     `json:"o"`
	HeadID        string    `json:"i,omitempty"`
	FloorAt       time.Time `json:"f,omitempty"`
	FloorOrdinal  int64     `json:"n,omitempty"`
	BeforeAt      time.Time `json:"b,omitempty"`
	BeforeOrdinal int64     `json:"q,omitempty"`
	InsertAfter   string    `json:"s,omitempty"`
}

func validFeedRefresh(p FeedRefreshPosition) bool {
	if !validID(p.AccountID) || p.ChannelID != "" && !validID(p.ChannelID) || p.Revision < 0 || p.ExpiresAt.IsZero() || p.HeadAt.IsZero() || p.HeadOrdinal < 0 {
		return false
	}
	if p.HeadOrdinal == 0 && p.HeadID != "" || p.HeadOrdinal > 0 && !validID(p.HeadID) {
		return false
	}
	if p.InsertAfter == "" {
		return p.BeforeOrdinal == 0 && p.FloorOrdinal == 0 && p.BeforeAt.IsZero() && p.FloorAt.IsZero()
	}
	return validID(p.InsertAfter) && p.BeforeOrdinal > 0 && p.FloorOrdinal >= 0 && !p.BeforeAt.IsZero() && !p.FloorAt.IsZero() &&
		tupleAfter(p.BeforeAt, p.BeforeOrdinal, p.FloorAt, p.FloorOrdinal) && !tupleAfter(p.BeforeAt, p.BeforeOrdinal, p.HeadAt, p.HeadOrdinal)
}

func tupleAfter(a time.Time, ao int64, b time.Time, bo int64) bool {
	return a.After(b) || a.Equal(b) && ao > bo
}

func (c *CursorCodec) EncodeFeedRefresh(p FeedRefreshPosition) (string, error) {
	if !validFeedRefresh(p) {
		return "", ErrCursorInvalid
	}
	raw := make([]byte, 1+c.aead.NonceSize())
	raw[0] = 1
	if _, e := io.ReadFull(rand.Reader, raw[1:]); e != nil {
		return "", e
	}
	data, e := json.Marshal(p)
	if e != nil {
		return "", e
	}
	raw = c.aead.Seal(raw, raw[1:], data, append(append([]byte{}, c.aad...), []byte("/feed-refresh-v1")...))
	token := base64.RawURLEncoding.EncodeToString(raw)
	if len(token) > 1024 {
		return "", ErrCursorInvalid
	}
	return token, nil
}

func (c *CursorCodec) DecodeFeedRefresh(token string, expected FeedRefreshPosition, at time.Time) (FeedRefreshPosition, error) {
	var p FeedRefreshPosition
	if len(token) < 1 || len(token) > 1024 || at.IsZero() {
		return p, ErrCursorInvalid
	}
	raw, e := base64.RawURLEncoding.Strict().DecodeString(token)
	n := c.aead.NonceSize()
	if e != nil || len(raw) < 1+n+c.aead.Overhead() || raw[0] != 1 || base64.RawURLEncoding.EncodeToString(raw) != token {
		return p, ErrCursorInvalid
	}
	data, e := c.aead.Open(nil, raw[1:1+n], raw[1+n:], append(append([]byte{}, c.aad...), []byte("/feed-refresh-v1")...))
	if e != nil || json.Unmarshal(data, &p) != nil || !validFeedRefresh(p) || p.AccountID != expected.AccountID || p.ChannelID != expected.ChannelID || p.Revision != expected.Revision || !at.Before(p.ExpiresAt) || p.ExpiresAt.After(at.Add(CursorTTL)) {
		return FeedRefreshPosition{}, ErrCursorInvalid
	}
	return p, nil
}
