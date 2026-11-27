package authprivacy

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/channels"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/posts"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/recommendation"
	rm "github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/recommendation/roundmodel"
	rw "github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/recommendation/transport"
)

// This is an internal final-transaction seam, NOT a public route/DTO. B4 must
// supply A's accepted scope/projection adapter. B3b tests use a named test port.
type RecommendationScope struct {
	Binding   rm.Binding
	At        time.Time
	Channels  []string
	ChannelID string
	Feedback  bool
}
type RecommendationProjection struct {
	Unavailable []int
	Items       []posts.FeedPost
}

var errRecommendationProjection = errors.New("invalid internal recommendation projection")

func checkRecommendationUnavailable(indices []int, size int) error {
	seen := map[int]bool{}
	for _, i := range indices {
		if i < 0 || i >= size || seen[i] {
			return errRecommendationProjection
		}
		seen[i] = true
	}
	return nil
}

type RecommendationProjectionPort interface {
	ProjectRecommendation(context.Context, pgx.Tx, RecommendationScope, []rm.ID) (RecommendationProjection, error)
}
type RecommendationConfig struct {
	Enabled, FeedbackEnabled, LikeTimeEnabled bool
	Selector                                  recommendation.Config
	Policy                                    rm.Policy
	Capacity                                  rm.ConservedCapacity
	NewRound                                  func() (uuid.UUID, uint64, error)
	syntheticLab                              bool      // only same-package isolated fixtures; never a runtime JSON field
	realRuntime                               bool      // set only by WithRecommendationRuntime after explicit runtime validation
	transport                                 *rw.Codec // runtime-validated or same-package test codec
}
type recommendationServices struct {
	config     RecommendationConfig
	projection RecommendationProjectionPort
	installed  bool
}
type RecommendationPage struct {
	View                     rm.View
	RoundID                  rm.ID
	Items                    []posts.FeedPost
	Revisions                rm.Revisions
	ExpiresAt, FeedbackUntil time.Time
}
type RecommendationStart struct {
	Tuple   rm.Tuple
	Channel string
}

func (c *Community) WithRecommendation(ctx context.Context, cfg RecommendationConfig, port RecommendationProjectionPort) (*Community, error) {
	if c == nil || c.pool == nil {
		return nil, ErrIntentInvalid
	}
	if err := checkRecommendationSchema(ctx, c.pool, cfg.Enabled || cfg.FeedbackEnabled || cfg.LikeTimeEnabled); err != nil {
		return nil, err
	}
	if cfg.Enabled || cfg.FeedbackEnabled || cfg.LikeTimeEnabled {
		if (!cfg.syntheticLab && !cfg.realRuntime) || port == nil || cfg.NewRound == nil || cfg.Selector.Validate() != nil || cfg.Policy.Validate() != nil || (cfg.realRuntime && (cfg.transport == nil || cfg.Capacity.Profile == (rm.Digest{}))) {
			return nil, ErrAuthorizationUnavailable
		}
	}
	var installed bool
	if err := c.pool.QueryRow(ctx, `SELECT to_regclass('c_recommendation.control') IS NOT NULL`).Scan(&installed); err != nil {
		return nil, err
	}
	copy := *c
	copy.recommendation = &recommendationServices{cfg, port, installed}
	return &copy, nil
}

// WithRecommendationRuntime is the explicit C-process bridge. The runtime
// validates the private key and measured profile before calling it; the normal
// final transaction still enforces the database control and capacity gates.
func (c *Community) WithRecommendationRuntime(ctx context.Context, cfg RecommendationConfig, codec *rw.Codec) (*Community, error) {
	if !cfg.Enabled || codec == nil || cfg.Capacity.Profile == (rm.Digest{}) || cfg.Capacity.Validate(time.Now().UTC().Truncate(time.Microsecond), cfg.Capacity.Profile, 1, cfg.Capacity.WorstGrowthBytesPerMutation) != nil {
		return nil, ErrAuthorizationUnavailable
	}
	cfg.realRuntime = true
	cfg.transport = codec
	return c.WithRecommendation(ctx, cfg, recommendationFeedProjection{})
}

func recommendationSession(bearer [32]byte) rm.Digest {
	return rm.Digest(sha256.Sum256(append([]byte("HNUHOLE/RECO/AUTHENTICATED-SESSION/V1\x00"), bearer[:]...)))
}

func (c *Community) recommendationScope(ctx context.Context, tx pgx.Tx, account uuid.UUID, bearer [32]byte, final AuthorizationDecision, channel string, control recommendationControl) (RecommendationScope, error) {
	s := RecommendationScope{At: final.TrustedAt.UTC().Truncate(time.Microsecond), ChannelID: channel, Feedback: control.feedback && c.recommendation.config.FeedbackEnabled}
	var generation, epoch int64
	var disabled bool
	if err := tx.QueryRow(ctx, `SELECT session_generation FROM c_auth.accounts WHERE account_id=$1`, account).Scan(&generation); err != nil {
		return s, err
	}
	err := tx.QueryRow(ctx, `SELECT epoch,disabled FROM c_recommendation.accounts WHERE account_id=$1`, account).Scan(&epoch, &disabled)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return s, err
	}
	if disabled {
		return s, rm.ErrScope
	}
	prefs, revision, err := readFeedPreferences(ctx, tx, account)
	if err != nil {
		return s, err
	}
	if prefs.BlockedChannelIDs == nil || len(prefs.BlockedChannelIDs) > channels.CatalogSize || revision < 0 {
		return s, errPostSchema
	}
	blocked := map[string]bool{}
	for _, value := range prefs.BlockedChannelIDs {
		parsed, e := uuid.Parse(value)
		if e != nil || parsed == uuid.Nil || parsed.String() != value || blocked[value] {
			return s, errPostSchema
		}
		blocked[value] = true
	}
	if channel != "" {
		if id, e := uuid.Parse(channel); e != nil || id == uuid.Nil || id.String() != channel {
			return s, rm.ErrInvalid
		}
	}
	rows, err := tx.Query(ctx, `SELECT id::text FROM public.channels WHERE ($1::text='' OR id::text=$1) AND NOT(id=ANY($2::uuid[])) ORDER BY id`, channel, prefs.BlockedChannelIDs)
	if err != nil {
		return s, err
	}
	defer rows.Close()
	s.Channels = []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return s, err
		}
		s.Channels = append(s.Channels, id)
	}
	if err = rows.Err(); err != nil {
		return s, err
	}
	sort.Strings(prefs.BlockedChannelIDs)
	canonical, _ := json.Marshal(struct {
		Channel string
		Blocked []string
	}{channel, prefs.BlockedChannelIDs})
	s.Binding = rm.Binding{Account: rm.ID(account.String()), Session: recommendationSession(bearer), SessionGeneration: uint64(generation), AccountEpoch: uint64(epoch), SecurityEpoch: uint64(control.epoch), FilterRevision: uint64(revision), Scope: rm.Digest(sha256.Sum256(canonical))}
	return s, nil
}

// RecommendationContext is an INTERNAL bootstrap seam. B4 may piggyback the
// nonprivate subset on an existing authorized bootstrap response; do not expose
// Account or SessionGeneration. A retry never refills tuple fields from here.
func (c *Community) RecommendationContext(ctx context.Context, bearer [32]byte, channel string) (PostResponse, error) {
	if c.recommendation == nil || !c.recommendation.config.Enabled {
		return PostResponse{}, ErrAuthorizationUnavailable
	}
	return c.withPostSession(ctx, bearer, func(tx pgx.Tx, a uuid.UUID, final AuthorizationDecision, _ restriction) (any, error) {
		ctl, e := c.recommendationControl(ctx, tx, final.TrustedAt, false)
		if e != nil {
			return nil, e
		}
		return c.recommendationScope(ctx, tx, a, bearer, final, channel, ctl)
	})
}

func (c *Community) StartRecommendation(ctx context.Context, bearer [32]byte, request RecommendationStart) (PostResponse, error) {
	if c.recommendation == nil || !c.recommendation.config.Enabled {
		return PostResponse{}, ErrAuthorizationUnavailable
	}
	return c.withPostSession(ctx, bearer, func(tx pgx.Tx, a uuid.UUID, final AuthorizationDecision, _ restriction) (any, error) {
		ctl, e := c.recommendationControl(ctx, tx, final.TrustedAt, false)
		if e != nil {
			return nil, e
		}
		scope, e := c.recommendationScope(ctx, tx, a, bearer, final, request.Channel, ctl)
		if e != nil {
			return nil, e
		}
		identity, e := (rm.TupleValidator{Policy: rm.ExperimentalIntentPolicy()}).ValidateTuple(scope.At, scope.Binding, request.Tuple)
		if e != nil {
			return nil, e
		}
		return c.startRecommendation(ctx, tx, scope, identity)
	})
}

func (c *Community) ReadRecommendation(ctx context.Context, bearer [32]byte, channel string, id rm.ID, offset int) (PostResponse, error) {
	return c.withRecommendationPage(ctx, bearer, channel, id, func(tx pgx.Tx, scope RecommendationScope, record rm.Record) (any, error) {
		return c.projectRecommendation(ctx, tx, scope, record, offset)
	})
}
func (c *Community) FeedbackRecommendation(ctx context.Context, bearer [32]byte, channel string, id rm.ID, offset int, slots []int) (PostResponse, error) {
	if c.recommendation == nil || !c.recommendation.config.FeedbackEnabled {
		return PostResponse{}, ErrAuthorizationUnavailable
	}
	return c.withRecommendationPage(ctx, bearer, channel, id, func(tx pgx.Tx, scope RecommendationScope, record rm.Record) (any, error) {
		return c.feedbackRecommendation(ctx, tx, scope, record, offset, slots)
	})
}

func (c *Community) feedbackRecommendation(ctx context.Context, tx pgx.Tx, scope RecommendationScope, record rm.Record, offset int, slots []int) (rm.Revisions, error) {
	// Scope was derived from the control row already held FOR UPDATE in
	// this final authorized business transaction. It includes both the DB and
	// runtime feedback switches; another read cannot observe a newer setting.
	if !scope.Feedback {
		return rm.Revisions{}, ErrAuthorizationUnavailable
	}
	if len(slots) > 10 {
		return rm.Revisions{}, rm.ErrInvalid
	}
	page := offset / 10
	if offset < 0 || offset%10 != 0 || page >= len(record.State.Pages) {
		return rm.Revisions{}, rm.ErrInvalid
	}
	ids := []rm.ID{}
	for _, slot := range slots {
		if slot < 0 || slot >= 10 || offset+slot >= len(record.State.Manifest.IDs) {
			return rm.Revisions{}, rm.ErrInvalid
		}
		ids = append(ids, record.State.Manifest.IDs[offset+slot])
	}
	p, e := c.recommendation.projection.ProjectRecommendation(ctx, tx, scope, ids)
	if e != nil {
		return rm.Revisions{}, e
	}
	missing := []int{}
	if e = checkRecommendationUnavailable(p.Unavailable, len(slots)); e != nil {
		return rm.Revisions{}, e
	}
	for _, index := range p.Unavailable {
		missing = append(missing, slots[index])
	}
	next, e := rm.Feedback(record, rm.Context{Now: scope.At, Binding: scope.Binding}, recommendationRevisions(record), offset, slots, missing)
	if e != nil {
		return rm.Revisions{}, e
	}
	if e = c.saveRecommendation(ctx, tx, record, next); e != nil {
		return rm.Revisions{}, e
	}
	return recommendationRevisions(next), nil
}

func (c *Community) withRecommendationPage(ctx context.Context, bearer [32]byte, channel string, id rm.ID, fn func(pgx.Tx, RecommendationScope, rm.Record) (any, error)) (PostResponse, error) {
	if c.recommendation == nil || !c.recommendation.config.Enabled || !id.Valid() {
		return PostResponse{}, ErrAuthorizationUnavailable
	}
	return c.withPostSession(ctx, bearer, func(tx pgx.Tx, a uuid.UUID, final AuthorizationDecision, _ restriction) (any, error) {
		ctl, e := c.recommendationControl(ctx, tx, final.TrustedAt, false)
		if e != nil {
			return nil, e
		}
		scope, e := c.recommendationScope(ctx, tx, a, bearer, final, channel, ctl)
		if e != nil {
			return nil, e
		}
		record, e := loadRecommendation(ctx, tx, a, id)
		if e != nil {
			return nil, e
		}
		if record.State.Manifest.Binding != scope.Binding {
			return nil, rm.ErrScope
		}
		return fn(tx, scope, record)
	})
}
func recommendationRevisions(r rm.Record) rm.Revisions {
	return rm.Revisions{Round: r.State.Revision, History: r.History.Revision}
}
