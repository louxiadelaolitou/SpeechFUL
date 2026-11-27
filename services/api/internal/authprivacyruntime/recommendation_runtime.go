package authprivacyruntime

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/authprivacyhttp"
	"time"

	"github.com/google/uuid"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/authprivacy"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/recommendation"
	rm "github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/recommendation/roundmodel"
	rw "github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/recommendation/transport"
)

// Omission and the closed compatibility shape keep recommendation off. An
// explicit C-only activation needs a current measured capacity profile and a
// separate private transport key. No profile or key bytes live in JSON.
type RecommendationConfig struct {
	Enabled                     bool      `json:"enabled"`
	Feedback                    bool      `json:"feedback"`
	LikeTime                    bool      `json:"likeTime"`
	ProfileHash                 string    `json:"profileHash,omitempty"`
	MeasuredAt                  time.Time `json:"measuredAt,omitempty"`
	ValidUntil                  time.Time `json:"validUntil,omitempty"`
	MinUnitsPerSecond           int64     `json:"minUnitsPerSecond,omitempty"`
	MaxPauseMicros              int64     `json:"maxPauseMicros,omitempty"`
	WorstGrowthBytesPerMutation int64     `json:"worstGrowthBytesPerMutation,omitempty"`
	TransportKeyFile            string    `json:"transportKeyFile,omitempty"`
	TransportKeyID              uint8     `json:"transportKeyId,omitempty"`
	KeyIssueFrom                time.Time `json:"keyIssueFrom,omitempty"`
	KeyIssueUntil               time.Time `json:"keyIssueUntil,omitempty"`
	KeyVerifyUntil              time.Time `json:"keyVerifyUntil,omitempty"`
}

func (r RecommendationConfig) capacity() (rm.ConservedCapacity, error) {
	var profile rm.Digest
	if len(r.ProfileHash) != 64 {
		return rm.ConservedCapacity{}, errors.New("recommendation requires measured profile hash")
	}
	b, err := hex.DecodeString(r.ProfileHash)
	if err != nil || hex.EncodeToString(b) != r.ProfileHash {
		return rm.ConservedCapacity{}, errors.New("invalid recommendation profile hash")
	}
	copy(profile[:], b)
	return rm.ConservedCapacity{Profile: profile, MeasuredAt: r.MeasuredAt, ValidUntil: r.ValidUntil, MinUnitsPerSecond: r.MinUnitsPerSecond, MaxPause: time.Duration(r.MaxPauseMicros) * time.Microsecond, WorstGrowthBytesPerMutation: r.WorstGrowthBytesPerMutation}, nil
}

func (c Config) validateRecommendation(service authprivacyhttp.Service) error {
	r := c.Recommendation
	if r == nil {
		return nil
	}
	if service != authprivacyhttp.CommunityService {
		return errors.New("recommendation belongs only to C")
	}
	if !r.Enabled {
		if r.Feedback || r.LikeTime || r.ProfileHash != "" || !r.MeasuredAt.IsZero() || !r.ValidUntil.IsZero() || r.MinUnitsPerSecond != 0 || r.MaxPauseMicros != 0 || r.WorstGrowthBytesPerMutation != 0 || r.TransportKeyFile != "" || r.TransportKeyID != 0 || !r.KeyIssueFrom.IsZero() || !r.KeyIssueUntil.IsZero() || !r.KeyVerifyUntil.IsZero() {
			return errors.New("recommendation activation requires enabled and complete real profile")
		}
		return nil
	}
	if c.Environment != "dev" || c.Posts == nil || r.TransportKeyFile == "" || r.TransportKeyID == 0 || r.MaxPauseMicros < 0 || r.MaxPauseMicros >= int64(10*time.Minute/time.Microsecond) {
		return errors.New("recommendation requires explicit C posts, key and bounded profile")
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	if now.Before(r.KeyIssueFrom) || !now.Before(r.KeyIssueUntil) {
		return errors.New("recommendation transport key is outside issue window")
	}
	p, err := r.capacity()
	if err != nil || p.Validate(now, p.Profile, 1, p.WorstGrowthBytesPerMutation) != nil {
		return errors.New("recommendation requires current reviewed real capacity profile")
	}
	if _, err = c.recommendationCodec(); err != nil {
		return errors.New("invalid recommendation transport key or lifetime")
	}
	return nil
}

func (c Config) recommendationCodec() (*rw.Codec, error) {
	r := c.Recommendation
	if r == nil || !r.Enabled {
		return nil, errors.New("recommendation disabled")
	}
	secret, err := c.Key32(r.TransportKeyFile)
	if err != nil {
		return nil, err
	}
	return rw.NewCodec(c.Environment, r.TransportKeyID, []rw.Key{{ID: r.TransportKeyID, Secret: secret, IssueFrom: r.KeyIssueFrom, IssueUntil: r.KeyIssueUntil, VerifyUntil: r.KeyVerifyUntil}})
}

func (c Config) installRecommendation(ctx context.Context, community *authprivacy.Community) (*authprivacy.Community, error) {
	if c.Recommendation == nil || !c.Recommendation.Enabled {
		return community.WithRecommendation(ctx, authprivacy.RecommendationConfig{}, nil)
	}
	cfg, codec, err := c.buildRecommendationRuntime()
	if err != nil {
		return nil, err
	}
	return community.WithRecommendationRuntime(ctx, cfg, codec)
}

func (c Config) buildRecommendationRuntime() (authprivacy.RecommendationConfig, *rw.Codec, error) {
	codec, err := c.recommendationCodec()
	if err != nil {
		return authprivacy.RecommendationConfig{}, nil, err
	}
	capacity, err := c.Recommendation.capacity()
	if err != nil {
		return authprivacy.RecommendationConfig{}, nil, err
	}
	cfg := authprivacy.RecommendationConfig{
		Enabled: true, FeedbackEnabled: c.Recommendation.Feedback, LikeTimeEnabled: c.Recommendation.LikeTime,
		Selector: recommendation.ExperimentalConfig(), Policy: rm.ExperimentalPolicy(), Capacity: capacity,
		NewRound: func() (uuid.UUID, uint64, error) {
			id, err := uuid.NewRandom()
			if err != nil {
				return uuid.Nil, 0, err
			}
			var seed [8]byte
			if _, err = rand.Read(seed[:]); err != nil {
				return uuid.Nil, 0, err
			}
			return id, binary.BigEndian.Uint64(seed[:]), nil
		},
	}
	return cfg, codec, nil
}
