// Package transport defines the finite public recommendation protocol. Opaque
// references are authenticated ciphertext, separate from chronological cursors.
package transport

import (
	"errors"
	"time"

	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/posts"
	rm "github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/recommendation/roundmodel"
)

const (
	Version       = 1
	MaxContext    = 2048
	MaxReference  = 1024
	MaxBody       = 16384
	ContextWindow = 10 * time.Minute
	MaxLifetime   = 40 * time.Minute
	ContextKind   = "context"
	CursorKind    = "cursor"
	ReceiptKind   = "receipt"
)

var (
	ErrInvalid     = errors.New("invalid recommendation protocol input")
	ErrExpired     = errors.New("recommendation reference expired")
	ErrScope       = errors.New("recommendation scope changed")
	ErrUnavailable = errors.New("recommendation capability unavailable")
)

type Start struct {
	Version   int       `json:"version"`
	Nonce     string    `json:"nonce"`
	CreatedAt time.Time `json:"createdAt"`
	ChannelID string    `json:"channelId,omitempty"`
	Context   string    `json:"context"`
}
type Feedback struct {
	Receipt string   `json:"receipt"`
	PostIDs []string `json:"postIds"`
}
type Page struct {
	Version         int              `json:"version"`
	Items           []posts.FeedPost `json:"items"`
	NextCursor      *string          `json:"nextCursor"`
	FeedbackReceipt *string          `json:"feedbackReceipt"`
	ExpiresAt       time.Time        `json:"expiresAt"`
	FeedbackUntil   time.Time        `json:"feedbackUntil"`
}
type ACK struct {
	Accepted bool `json:"accepted"`
}

// Capability is sent in normal authorized feed response HEADERS. The existing
// strict mobile JSON parser continues to see exactly items/nextCursor.
type Capability struct {
	Available    bool
	Context      string
	IssuedAt     time.Time
	CreateBefore time.Time
}
type Scope struct {
	Binding   rm.Binding
	ChannelID string
}
type Reference struct {
	Scope              Scope
	RoundID            rm.ID
	Offset             int
	FirstAt, ExpiresAt time.Time
}

func ValidTime(t time.Time) bool {
	return !t.IsZero() && t.Location() == time.UTC && t.Nanosecond()%1000 == 0
}
func ValidID(id string) bool { return rm.ID(id).Valid() }
func ValidFeedback(input Feedback) bool {
	if input.PostIDs == nil || len(input.PostIDs) > 10 || !ValidToken(input.Receipt, MaxReference) {
		return false
	}
	seen := map[string]bool{}
	for _, id := range input.PostIDs {
		if !ValidID(id) || seen[id] {
			return false
		}
		seen[id] = true
	}
	return true
}
