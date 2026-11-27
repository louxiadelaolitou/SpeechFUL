package authprivacy

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	rm "github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/recommendation/roundmodel"
	rw "github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/recommendation/transport"
)

func (c *Community) recommendationPublicReady() bool {
	return c.recommendation != nil && c.recommendation.config.Enabled && c.recommendation.config.transport != nil
}

// Called only INSIDE the normal feed's final authorized transaction. Off/M0
// does not read recommendation state or issue ciphertext; no extra handshake.
func (c *Community) recommendationFeedCapability(ctx context.Context, tx pgx.Tx, a uuid.UUID, bearer [32]byte, final AuthorizationDecision, channel string) (*rw.Capability, error) {
	off := &rw.Capability{}
	if !c.recommendationPublicReady() {
		return off, nil
	}
	ctl, err := c.recommendationControl(ctx, tx, final.TrustedAt, false)
	if errors.Is(err, ErrAuthorizationUnavailable) {
		return off, nil
	}
	if err != nil {
		return nil, err
	}
	if !ctl.admission {
		return off, nil
	}
	scope, err := c.recommendationScope(ctx, tx, a, bearer, final, channel, ctl)
	if errors.Is(err, rm.ErrScope) {
		return off, nil
	}
	if err != nil {
		return nil, err
	}
	capability, err := c.recommendation.config.transport.Context(rw.Scope{Binding: scope.Binding, ChannelID: channel}, scope.At)
	if err != nil {
		return off, nil
	}
	return &capability, nil
}

func (c *Community) StartRecommendationPublic(ctx context.Context, bearer [32]byte, input rw.Start) (PostResponse, error) {
	return c.withPostSession(ctx, bearer, func(tx pgx.Tx, a uuid.UUID, final AuthorizationDecision, _ restriction) (any, error) {
		if !c.recommendationPublicReady() {
			return nil, rw.ErrUnavailable
		}
		ctl, err := c.recommendationControl(ctx, tx, final.TrustedAt, false)
		if err != nil {
			return nil, err
		}
		now := final.TrustedAt.UTC().Truncate(time.Microsecond)
		tuple, frozen, err := c.recommendation.config.transport.OpenStart(input, now)
		if err != nil {
			return nil, err
		}
		scope, err := c.recommendationScope(ctx, tx, a, bearer, final, input.ChannelID, ctl)
		if err != nil {
			return nil, err
		}
		if frozen.Binding != scope.Binding {
			return nil, rw.ErrScope
		}
		identity, err := (rm.TupleValidator{Policy: rm.ExperimentalIntentPolicy()}).ValidateTuple(scope.At, scope.Binding, tuple)
		if err != nil {
			return nil, err
		}
		page, err := c.startRecommendation(ctx, tx, scope, identity)
		if err != nil {
			return nil, err
		}
		return c.recommendationPublicPage(scope, page)
	})
}

func (c *Community) ReadRecommendationPublic(ctx context.Context, bearer [32]byte, cursor string) (PostResponse, error) {
	return c.withRecommendationPublicReference(ctx, bearer, cursor, rw.CursorKind, func(tx pgx.Tx, scope RecommendationScope, record rm.Record, ref rw.Reference) (any, error) {
		page, err := c.projectRecommendation(ctx, tx, scope, record, ref.Offset)
		if err != nil {
			return nil, err
		}
		return c.recommendationPublicPage(scope, page)
	})
}

func (c *Community) FeedbackRecommendationPublic(ctx context.Context, bearer [32]byte, input rw.Feedback) (PostResponse, error) {
	return c.withRecommendationPublicReference(ctx, bearer, input.Receipt, rw.ReceiptKind, func(tx pgx.Tx, scope RecommendationScope, record rm.Record, ref rw.Reference) (any, error) {
		if !scope.Feedback {
			return nil, rw.ErrUnavailable
		}
		if !rw.ValidFeedback(input) {
			return nil, rw.ErrInvalid
		}
		page := record.State.Pages[ref.Offset/10]
		if !page.FirstAt.Equal(ref.FirstAt) {
			return nil, rw.ErrInvalid
		}
		slots := []int{}
		for _, id := range input.PostIDs {
			found := -1
			for slot := 0; slot < 10 && ref.Offset+slot < len(record.State.Manifest.IDs); slot++ {
				if string(record.State.Manifest.IDs[ref.Offset+slot]) == id && page.Delivered&(1<<slot) != 0 {
					found = slot
					break
				}
			}
			if found < 0 {
				return nil, rw.ErrInvalid
			}
			slots = append(slots, found)
		}
		if _, err := c.feedbackRecommendation(ctx, tx, scope, record, ref.Offset, slots); err != nil {
			return nil, err
		}
		return rw.ACK{Accepted: true}, nil
	})
}

func (c *Community) withRecommendationPublicReference(ctx context.Context, bearer [32]byte, token, kind string, fn func(pgx.Tx, RecommendationScope, rm.Record, rw.Reference) (any, error)) (PostResponse, error) {
	return c.withPostSession(ctx, bearer, func(tx pgx.Tx, a uuid.UUID, final AuthorizationDecision, _ restriction) (any, error) {
		if !c.recommendationPublicReady() {
			return nil, rw.ErrUnavailable
		}
		ctl, err := c.recommendationControl(ctx, tx, final.TrustedAt, false)
		if err != nil {
			return nil, err
		}
		ref, err := c.recommendation.config.transport.OpenReference(kind, token, final.TrustedAt.UTC().Truncate(time.Microsecond))
		if err != nil {
			return nil, err
		}
		scope, err := c.recommendationScope(ctx, tx, a, bearer, final, ref.Scope.ChannelID, ctl)
		if err != nil {
			return nil, err
		}
		if ref.Scope.Binding != scope.Binding {
			return nil, rw.ErrScope
		}
		record, err := loadRecommendation(ctx, tx, a, ref.RoundID)
		if err != nil {
			return nil, err
		}
		if record.State.Manifest.Binding != ref.Scope.Binding {
			return nil, rw.ErrScope
		}
		if ref.Offset/10 >= len(record.State.Pages) {
			return nil, rw.ErrInvalid
		}
		return fn(tx, scope, record, ref)
	})
}

func (c *Community) recommendationPublicPage(scope RecommendationScope, page RecommendationPage) (rw.Page, error) {
	if page.Items == nil || len(page.Items) != len(page.View.IDs) {
		return rw.Page{}, errRecommendationProjection
	}
	for i, item := range page.Items {
		if item.PostID != string(page.View.IDs[i]) {
			return rw.Page{}, errRecommendationProjection
		}
	}
	out := rw.Page{Version: rw.Version, Items: page.Items, ExpiresAt: page.ExpiresAt, FeedbackUntil: page.FeedbackUntil}
	ref := rw.Reference{Scope: rw.Scope{Binding: scope.Binding, ChannelID: scope.ChannelID}, RoundID: page.RoundID, ExpiresAt: page.ExpiresAt}
	if page.View.Next != nil {
		ref.Offset = *page.View.Next
		token, err := c.recommendation.config.transport.SealReference(rw.CursorKind, ref, scope.At)
		if err != nil {
			return rw.Page{}, rw.ErrUnavailable
		}
		out.NextCursor = &token
	}
	if scope.Feedback && scope.At.Before(page.FeedbackUntil) {
		ref.Offset, ref.FirstAt, ref.ExpiresAt = page.View.Offset, page.View.FirstAt, page.FeedbackUntil
		token, err := c.recommendation.config.transport.SealReference(rw.ReceiptKind, ref, scope.At)
		if err != nil {
			return rw.Page{}, rw.ErrUnavailable
		}
		out.FeedbackReceipt = &token
	}
	return out, nil
}
