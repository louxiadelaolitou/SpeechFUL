package authprivacyruntime

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/authprivacyhttp"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/recommendation"
)

func testRecommendationConfig(t *testing.T) Config {
	t.Helper()
	path := filepath.Join(t.TempDir(), "recommendation.key")
	if err := os.WriteFile(path, []byte("0123456789abcdef0123456789abcdef"), 0600); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	digest := sha256.Sum256([]byte("test-only-measured-profile"))
	return Config{Environment: "dev", Posts: &PostsConfig{}, Recommendation: &RecommendationConfig{
		Enabled: true, ProfileHash: hex.EncodeToString(digest[:]), MeasuredAt: now.Add(-time.Minute), ValidUntil: now.Add(30 * time.Minute),
		MinUnitsPerSecond: 100000, MaxPauseMicros: int64(time.Second / time.Microsecond), WorstGrowthBytesPerMutation: 65536,
		TransportKeyFile: path, TransportKeyID: 1, KeyIssueFrom: now.Add(-time.Minute), KeyIssueUntil: now.Add(time.Hour), KeyVerifyUntil: now.Add(time.Hour + 40*time.Minute),
	}}
}

func TestRecommendationClosedAndExplicitRuntimeMapping(t *testing.T) {
	for _, r := range []*RecommendationConfig{nil, {}} {
		c := Config{Recommendation: r}
		if err := c.validateRecommendation(authprivacyhttp.CommunityService); err != nil {
			t.Fatal(err)
		}
	}
	c := testRecommendationConfig(t)
	if err := c.validateRecommendation(authprivacyhttp.CommunityService); err != nil {
		t.Fatal(err)
	}
	cfg, codec, err := c.buildRecommendationRuntime()
	if err != nil || codec == nil || !cfg.Enabled || cfg.FeedbackEnabled || cfg.LikeTimeEnabled || cfg.NewRound == nil || cfg.Selector != recommendation.ExperimentalConfig() || cfg.Capacity.Profile == ([32]byte{}) {
		t.Fatal("explicit runtime config did not reach recommendation engine", err)
	}
	id, _, err := cfg.NewRound()
	if err != nil || id.String() == "00000000-0000-0000-0000-000000000000" {
		t.Fatal("missing random round identity", err)
	}
}

func TestRecommendationRuntimeRejectsIllegalCombinations(t *testing.T) {
	for _, r := range []RecommendationConfig{{Feedback: true}, {LikeTime: true}, {Enabled: true}} {
		c := Config{Environment: "dev", Recommendation: &r}
		if err := c.validateRecommendation(authprivacyhttp.CommunityService); err == nil {
			t.Fatal("incomplete activation accepted", r)
		}
	}
	base := testRecommendationConfig(t)
	mutate := []func(*Config){
		func(c *Config) { c.Recommendation.ProfileHash = "00" },
		func(c *Config) { c.Recommendation.ValidUntil = time.Now().UTC().Add(-time.Minute) },
		func(c *Config) { c.Recommendation.TransportKeyFile = "" },
		func(c *Config) { c.Posts = nil },
		func(c *Config) { c.Recommendation.Feedback = true; c.Recommendation.Enabled = false },
	}
	for i, change := range mutate {
		c := base
		r := *base.Recommendation
		c.Recommendation = &r
		change(&c)
		if err := c.validateRecommendation(authprivacyhttp.CommunityService); err == nil {
			t.Fatalf("illegal combination %d accepted", i)
		}
	}
	if err := base.validateRecommendation(authprivacyhttp.VerifierService); err == nil {
		t.Fatal("V accepted C recommendation")
	}
}
