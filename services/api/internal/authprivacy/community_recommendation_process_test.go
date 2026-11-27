package authprivacy

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/authprivacy/protocol"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/recommendation"
	rm "github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/recommendation/roundmodel"
)

type recommendationChildFixture struct {
	Domain, EvidencePath, AnchorPath                          string
	EvidencePublic, RecoveryPublic, GlassPublic, AnchorSigner []byte
	RequestKey                                                [32]byte
	PostConfig                                                PostConfig
	Now                                                       time.Time
	Bearer                                                    [32]byte
	Request                                                   RecommendationStart
	Selector                                                  recommendation.Config
	Policy                                                    rm.Policy
	Capacity                                                  rm.ConservedCapacity
}

// Named outside TestRecommendation so a normal run cannot count an empty child
// helper as a successful feature case. Only the parent executes this exact test.
func TestRecoProcessHelper(t *testing.T) {
	path := os.Getenv("HNUB3B_CHILD_CONFIG")
	if path == "" {
		t.Skip("explicit isolated subprocess only")
	}
	if os.Getenv("FEED_RECOMMENDATION_OWNED") != "1" {
		t.Fatal("missing task marker")
	}
	data, e := os.ReadFile(path)
	if e != nil {
		t.Fatal("private child fixture absent")
	}
	var f recommendationChildFixture
	if json.Unmarshal(data, &f) != nil {
		t.Fatal("invalid child fixture")
	}
	ctx := context.Background()
	pool, e := pgxpool.New(ctx, os.Getenv("AUTHLAB_POSTS_RUNTIME_DSN"))
	if e != nil {
		t.Fatal("child pool")
	}
	defer pool.Close()
	anchor, e := NewFileAuthorizationAnchorStore(f.AnchorPath, f.AnchorSigner)
	if e != nil {
		t.Fatal("child anchor")
	}
	gate, e := NewPostgresAuthorizationGate(AuthorizationGateConfig{Pool: pool, Domain: f.Domain, Evidence: NewFileAuthorizationEvidenceProvider(f.EvidencePath), EvidencePublicKey: f.EvidencePublic, RecoveryPublicKey: f.RecoveryPublic, BreakGlassPublicKey: f.GlassPublic, Anchor: anchor, Clock: func() time.Time { return f.Now }})
	if e != nil {
		t.Fatal("child gate")
	}
	var key protocol.PublicKey
	copy(key[:], f.EvidencePublic)
	verifier, e := protocol.NewVerifier("lab", []protocol.TrustedKey{{Environment: "lab", Purpose: protocol.PurposeRegister, Epoch: 1, PublicKey: key}})
	if e != nil {
		t.Fatal("child verifier")
	}
	c, e := NewCommunity(pool, verifier, 1, f.RequestKey, gate)
	if e != nil {
		t.Fatal(e)
	}
	c, e = c.WithPosts(ctx, f.PostConfig)
	if e != nil {
		t.Fatal("child post schema/key boundary", e)
	}
	c, e = c.WithRecommendation(ctx, RecommendationConfig{Enabled: true, FeedbackEnabled: true, LikeTimeEnabled: true, Selector: f.Selector, Policy: f.Policy, Capacity: f.Capacity, NewRound: func() (uuid.UUID, uint64, error) { id, e := uuid.NewRandom(); return id, 20261025, e }, syntheticLab: true}, &recommendationTestPort{})
	if e != nil {
		t.Fatal("child recommendation setup")
	}
	response, e := c.StartRecommendation(ctx, f.Bearer, f.Request)
	if e != nil {
		t.Fatal("child final transaction", e)
	}
	result, e := json.Marshal(response.Value.(RecommendationPage))
	if e != nil {
		t.Fatal("child result")
	}
	if e = os.WriteFile(os.Getenv("HNUB3B_CHILD_RESULT"), result, 0600); e != nil {
		t.Fatal("child result write")
	}
}

func TestRecommendationIndependentProcessesSameIntentPostgres(t *testing.T) {
	p := newRecommendationPG(t, 12)
	a := sessionTestAccount(t, p.l, "reco_process")
	request := p.request(t, a)
	f := recommendationChildFixture{Domain: p.control.config.Domain, EvidencePath: p.control.provider.Path, AnchorPath: p.control.gate.anchor.Path, EvidencePublic: p.control.config.EvidencePublicKey, RecoveryPublic: p.control.config.RecoveryPublicKey, GlassPublic: p.control.config.BreakGlassPublicKey, AnchorSigner: p.control.gate.anchor.signer, RequestKey: p.l.c.requestKey, PostConfig: p.postConfig, Now: p.control.clock.now(), Bearer: a.SessionToken, Request: request, Selector: p.config.Selector, Policy: p.config.Policy, Capacity: p.config.Capacity}
	dir := t.TempDir()
	config := filepath.Join(dir, "child.json")
	data, e := json.Marshal(f)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(config, data, 0600); e != nil {
		t.Fatal(e)
	}
	binary, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	commands := []*exec.Cmd{}
	for i := 0; i < 2; i++ {
		name := filepath.Join(dir, []string{"first.json", "second.json"}[i])
		cmd := exec.Command(binary, "-test.run=^TestRecoProcessHelper$", "-test.count=1")
		cmd.Env = append(os.Environ(), "HNUB3B_CHILD_CONFIG="+config, "HNUB3B_CHILD_RESULT="+name)
		if e = cmd.Start(); e != nil {
			t.Fatal(e)
		}
		commands = append(commands, cmd)
	}
	for _, cmd := range commands {
		if e = cmd.Wait(); e != nil {
			t.Fatal("independent child failed", e)
		}
	}
	var results [2]RecommendationPage
	for i, name := range []string{"first.json", "second.json"} {
		b, e := os.ReadFile(filepath.Join(dir, name))
		if e != nil || json.Unmarshal(b, &results[i]) != nil {
			t.Fatal("missing child commit result")
		}
	}
	if results[0].RoundID != results[1].RoundID || !reflect.DeepEqual(results[0].View, results[1].View) || count(t, p.l.cp, `SELECT count(*) FROM c_recommendation.intents`) != 1 || count(t, p.l.cp, `SELECT count(*) FROM c_recommendation.rounds`) != 1 {
		t.Fatal("cross-process intent not unique")
	}
	// Discard both subprocess responses and recover from the unchanged tuple.
	retry := p.start(t, a, request)
	if retry.RoundID != results[0].RoundID || !reflect.DeepEqual(retry.View, results[0].View) {
		t.Fatal("lost subprocess response recreated round")
	}
}
