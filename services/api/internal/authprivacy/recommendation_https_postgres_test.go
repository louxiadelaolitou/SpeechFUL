package authprivacy_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	ap "github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/authprivacy"
	ah "github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/authprivacyhttp"
	rw "github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/recommendation/transport"
)

type recoHTTPS struct {
	t      *testing.T
	lab    *ap.RecommendationHTTPLab
	server *httptest.Server
}
type recoHTTPResult struct {
	status  int
	headers http.Header
	body    []byte
	err     error
}

func newRecoHTTPS(t *testing.T, n int) *recoHTTPS {
	t.Helper()
	s := &recoHTTPS{t: t, lab: ap.NewRecommendationHTTPLab(t, n)}
	s.open()
	t.Cleanup(func() { s.server.Close() })
	return s
}
func (s *recoHTTPS) open() {
	s.t.Helper()
	trust := httptest.NewTLSServer(http.NotFoundHandler())
	roots := x509.NewCertPool()
	roots.AddCert(trust.Certificate())
	trust.Close()
	passwords, err := ap.NewPasswordPreparer(1, []string{"a deliberately blocked phrase"})
	if err != nil {
		s.t.Fatal(err)
	}
	limits := ah.NetworkLimits{MutationCapacity: 1000, QueryCapacity: 1000, Window: time.Minute, MaxEntries: 8}
	if _, err = rand.Read(limits.Key[:]); err != nil {
		s.t.Fatal(err)
	}
	endpoints, err := ah.NewCommunityEndpoints(ah.CommunityOptions{Backend: s.lab.Community(), Passwords: passwords, InternalPeer: ah.PeerIdentity{Environment: "lab", Service: ah.VerifierService, Roots: roots}, AllowedOrigins: []string{"https://client.example"}, Limits: limits})
	if err != nil {
		s.t.Fatal(err)
	}
	s.server = httptest.NewTLSServer(endpoints.Public)
}
func (s *recoHTTPS) request(method, path string, bearer [32]byte, body []byte) recoHTTPResult {
	req, err := http.NewRequest(method, s.server.URL+path, bytes.NewReader(body))
	if err != nil {
		return recoHTTPResult{err: err}
	}
	if bearer != ([32]byte{}) {
		req.Header.Set("Authorization", "Bearer "+base64.RawURLEncoding.EncodeToString(bearer[:]))
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Origin", "https://client.example")
	response, err := s.server.Client().Do(req)
	if err != nil {
		return recoHTTPResult{err: err}
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	return recoHTTPResult{response.StatusCode, response.Header, data, err}
}
func (s *recoHTTPS) check(r recoHTTPResult, status int) recoHTTPResult {
	s.t.Helper()
	if r.err != nil || r.status != status {
		s.t.Fatalf("HTTPS status: got %d want %d; error %v; public body %s", r.status, status, r.err, r.body)
	}
	if r.headers.Get("Cache-Control") != "no-store" || r.headers.Get("X-Request-ID") == "" {
		s.t.Fatal("missing privacy headers")
	}
	if status != 200 && (r.headers.Get("Session-Expires-At") != "" || r.headers.Get("Server-Time") != "" || r.headers.Get("Recommendation-Context") != "") {
		s.t.Fatal("failed request leaked success metadata")
	}
	return r
}
func (s *recoHTTPS) bootstrap() rw.Start {
	s.t.Helper()
	return s.bootstrapAs(s.lab.Reader.SessionToken, "")
}
func (s *recoHTTPS) bootstrapAs(bearer [32]byte, channel string) rw.Start {
	s.t.Helper()
	path := "/api/v1/feed"
	if channel != "" {
		path += "?channelId=" + channel
	}
	r := s.check(s.request("GET", path, bearer, nil), 200)
	if r.headers.Get("Recommendation-Available") != "true" || r.headers.Get("Recommendation-Version") != "1" || !strings.Contains(r.headers.Get("Access-Control-Expose-Headers"), "Recommendation-Context") {
		s.t.Fatal("feed did not bootstrap")
	}
	var body map[string]json.RawMessage
	if json.Unmarshal(r.body, &body) != nil || len(body) != 2 || body["items"] == nil || body["nextCursor"] == nil {
		s.t.Fatal("legacy body changed")
	}
	issued, err := time.Parse(time.RFC3339Nano, r.headers.Get("Recommendation-Issued-At"))
	if err != nil {
		s.t.Fatal(err)
	}
	before, err := time.Parse(time.RFC3339Nano, r.headers.Get("Recommendation-Create-Before"))
	if err != nil || before.Sub(issued) != 10*time.Minute {
		s.t.Fatal("context window")
	}
	context := r.headers.Get("Recommendation-Context")
	if !rw.ValidToken(context, rw.MaxContext) || r.headers.Get("Session-Expires-At") == "" || r.headers.Get("Server-Time") == "" {
		s.t.Fatal("bounded feed metadata")
	}
	return rw.Start{Version: 1, Nonce: uuid.NewString(), CreatedAt: issued, ChannelID: channel, Context: context}
}
func jsonBytes(v any) []byte { b, _ := json.Marshal(v); return b }
func (s *recoHTTPS) start(body []byte) rw.Page {
	s.t.Helper()
	return s.startAs(body, s.lab.Reader.SessionToken)
}
func (s *recoHTTPS) startAs(body []byte, bearer [32]byte) rw.Page {
	s.t.Helper()
	return s.page(s.check(s.request("POST", "/api/v1/feed/recommendation/start", bearer, body), 200))
}
func (s *recoHTTPS) page(r recoHTTPResult) rw.Page {
	s.t.Helper()
	var p rw.Page
	if json.Unmarshal(r.body, &p) != nil || p.Version != 1 || p.Items == nil || len(p.Items) > 10 {
		s.t.Fatal("invalid public page")
	}
	var raw map[string]json.RawMessage
	if json.Unmarshal(r.body, &raw) != nil || len(raw) != 6 {
		s.t.Fatal("private fields crossed DTO")
	}
	return p
}
func pageIDs(p rw.Page) []string {
	ids := []string{}
	for _, x := range p.Items {
		ids = append(ids, x.PostID)
	}
	return ids
}
func (s *recoHTTPS) ref(path string, token *string, bearer [32]byte, status int) recoHTTPResult {
	s.t.Helper()
	if token == nil {
		s.t.Fatal("missing reference")
	}
	key := "cursor"
	if path == "feedback" {
		key = "receipt"
	}
	body := map[string]any{key: *token}
	if path == "feedback" {
		body["postIds"] = []string{}
	}
	return s.check(s.request("POST", "/api/v1/feed/recommendation/"+path, bearer, jsonBytes(body)), status)
}

func TestRecommendationHTTPSRealProjectionRetryAndReconstructionPostgres(t *testing.T) {
	s := newRecoHTTPS(t, 25)
	input := s.bootstrap()
	persisted := jsonBytes(input)
	s.lab.BeginTrace()
	first := s.start(persisted)
	trace := s.lab.FinishTrace()
	if len(first.Items) != 10 || first.NextCursor == nil || first.FeedbackReceipt == nil {
		t.Fatal("positive page")
	}
	results := make(chan recoHTTPResult, 4)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- s.request("POST", "/api/v1/feed/recommendation/start", s.lab.Reader.SessionToken, persisted)
		}()
	}
	wg.Wait()
	close(results)
	for result := range results {
		p := s.page(s.check(result, 200))
		if !reflect.DeepEqual(pageIDs(first), pageIDs(p)) || !first.ExpiresAt.Equal(p.ExpiresAt) || !first.FeedbackUntil.Equal(p.FeedbackUntil) {
			t.Fatal("response-loss parallel replay changed page")
		}
	}
	if s.lab.Rounds(t) != 1 {
		t.Fatal("parallel duplicate round")
	}
	s.server.Close()
	s.lab.Rebuild(t)
	s.open()
	again := s.start(persisted)
	if !reflect.DeepEqual(pageIDs(first), pageIDs(again)) || s.lab.Rounds(t) != 1 {
		t.Fatal("reconstructed service lost persisted tuple")
	}
	s.ref("page", first.NextCursor, s.lab.Other.SessionToken, 409)
	s.ref("feedback", first.FeedbackReceipt, s.lab.Other.SessionToken, 409)
	s.check(s.request("POST", "/api/v1/feed/recommendation/start", s.lab.Other.SessionToken, persisted), 409)
	s.ref("page", first.FeedbackReceipt, s.lab.Reader.SessionToken, 400)
	s.ref("feedback", first.NextCursor, s.lab.Reader.SessionToken, 400)
	next := s.page(s.ref("page", first.NextCursor, s.lab.Reader.SessionToken, 200))
	for _, id := range pageIDs(next) {
		for _, old := range pageIDs(first) {
			if id == old {
				t.Fatal("duplicate in next page")
			}
		}
	}
	var counts struct{ Categories map[string]int }
	if json.Unmarshal(trace, &counts) != nil || counts.Categories["A1_content_projection"] != 1 || counts.Categories["test_projection_port"] != 0 {
		t.Fatal("real A1 projection not measured")
	}
	if dir := os.Getenv("HNUB3B_EVIDENCE_DIR"); dir != "" {
		normal := s.check(s.request("GET", "/api/v1/feed", s.lab.Reader.SessionToken, nil), 200)
		if os.WriteFile(filepath.Join(dir, "B-B4b-legacy-feed.json"), normal.body, 0600) != nil || os.WriteFile(filepath.Join(dir, "B-B4b-http-start-sql.json"), trace, 0600) != nil {
			t.Fatal("public evidence write")
		}
	}
}

func TestRecommendationHTTPSPreparedFeedBootstrapPostgres(t *testing.T) {
	s := newRecoHTTPS(t, 25)
	s.lab.UsePreparedFeed()
	input := s.bootstrap()
	persisted := jsonBytes(input)
	first := s.start(persisted)
	if len(first.Items) != 10 || first.NextCursor == nil || first.FeedbackReceipt == nil {
		t.Fatal("prepared feed did not bootstrap a recommendation round")
	}
	again := s.start(persisted)
	if !reflect.DeepEqual(pageIDs(first), pageIDs(again)) || s.lab.Rounds(t) != 1 {
		t.Fatal("prepared feed replay duplicated the round")
	}
}

func TestRecommendationHTTPSChannelsBlockingHideAndSingleScopePostgres(t *testing.T) {
	s := newRecoHTTPS(t, 0)
	channels := s.lab.PublishAllChannels(t)
	if len(channels) != 10 {
		t.Fatalf("mainline catalog has %d channels, want 10", len(channels))
	}
	putPrefs := func(bearer [32]byte, blocked []string) {
		t.Helper()
		s.check(s.request("PUT", "/api/v1/me/feed-preferences", bearer, jsonBytes(map[string]any{"blockedChannelIds": blocked})), 200)
	}

	all := s.start(jsonBytes(s.bootstrap()))
	if len(all.Items) != 10 {
		t.Fatalf("ten-channel round returned %d items", len(all.Items))
	}
	seen := map[string]bool{}
	for _, item := range all.Items {
		seen[item.ChannelID] = true
	}
	for _, channel := range channels {
		if !seen[channel] {
			t.Fatalf("channel %s missing from the ten-channel round", channel)
		}
	}

	one := s.start(jsonBytes(s.bootstrapAs(s.lab.Reader.SessionToken, channels[0])))
	if len(one.Items) != 1 || one.Items[0].ChannelID != channels[0] {
		t.Fatalf("single-channel scope returned %d items, want only %s", len(one.Items), channels[0])
	}

	other := s.lab.Other.SessionToken
	putPrefs(other, channels[:5])
	partial := s.startAs(jsonBytes(s.bootstrapAs(other, "")), other)
	if len(partial.Items) != 5 {
		t.Fatalf("partial blocking returned %d items, want 5", len(partial.Items))
	}
	allowed := map[string]bool{}
	for _, channel := range channels[5:] {
		allowed[channel] = true
	}
	for _, item := range partial.Items {
		if !allowed[item.ChannelID] {
			t.Fatalf("blocked channel %s leaked into the partial round", item.ChannelID)
		}
	}

	putPrefs(other, channels)
	empty := s.startAs(jsonBytes(s.bootstrapAs(other, "")), other)
	if len(empty.Items) != 0 || empty.NextCursor != nil {
		t.Fatal("all-blocked scope produced a nonempty round")
	}

	putPrefs(other, []string{})
	page := s.startAs(jsonBytes(s.bootstrapAs(other, "")), other)
	if len(page.Items) != 10 {
		t.Fatalf("cleared blocking returned %d items", len(page.Items))
	}
	if _, err := s.lab.Community().BlockPostAuthor(context.Background(), other, page.Items[0].PostID); err != nil {
		t.Fatal(err)
	}
	blocked := s.startAs(jsonBytes(s.bootstrapAs(other, "")), other)
	if len(blocked.Items) != 0 {
		t.Fatalf("author block leaked %d items", len(blocked.Items))
	}

	base := s.start(jsonBytes(s.bootstrap()))
	if len(base.Items) != 10 {
		t.Fatalf("hide fixture returned %d items", len(base.Items))
	}
	accountHidden, moderated := base.Items[0].PostID, base.Items[1].PostID
	hiddenState, err := s.lab.Community().ReadHiddenPost(context.Background(), s.lab.Reader.SessionToken, accountHidden)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.lab.Community().SetHiddenPost(context.Background(), s.lab.Reader.SessionToken, accountHidden, true, hiddenState.InteractionVersion); err != nil {
		t.Fatal(err)
	}
	s.lab.Hide(t, []string{moderated}, true)
	after := s.start(jsonBytes(s.bootstrap()))
	if len(after.Items) != 8 {
		t.Fatalf("hidden posts returned %d items, want 8", len(after.Items))
	}
	for _, item := range after.Items {
		if item.PostID == accountHidden || item.PostID == moderated {
			t.Fatal("hidden post resurfaced in a recommendation round")
		}
	}
}

func TestRecommendationHTTPSCreateBeforeAndExpiryPostgres(t *testing.T) {
	s := newRecoHTTPS(t, 12)
	input := s.bootstrap()
	input.CreatedAt = input.CreatedAt.Add(9 * time.Minute)
	persisted := jsonBytes(input)
	s.lab.Advance(t, 9*time.Minute)
	first := s.start(persisted)
	s.lab.Advance(t, 2*time.Minute)
	again := s.start(persisted)
	if !reflect.DeepEqual(pageIDs(first), pageIDs(again)) || s.lab.Rounds(t) != 1 {
		t.Fatal("valid original tuple lost after create-before")
	}
	changed := input
	changed.CreatedAt = changed.CreatedAt.Add(time.Minute)
	s.check(s.request("POST", "/api/v1/feed/recommendation/start", s.lab.Reader.SessionToken, jsonBytes(changed)), 409)
	s.lab.Advance(t, 9*time.Minute)
	s.check(s.request("POST", "/api/v1/feed/recommendation/start", s.lab.Reader.SessionToken, persisted), 409)
	s.ref("feedback", first.FeedbackReceipt, s.lab.Reader.SessionToken, 409)
}

func TestRecommendationHTTPSFeedbackAtomicDeliveredOnlyAndTTLPostgres(t *testing.T) {
	s := newRecoHTTPS(t, 12)
	first := s.start(jsonBytes(s.bootstrap()))
	if first.FeedbackReceipt == nil {
		t.Fatal("receipt")
	}
	ids := pageIDs(first)
	for _, bad := range [][]string{{uuid.NewString()}, {ids[0], ids[0]}, append(append([]string{}, ids...), uuid.NewString())} {
		s.check(s.request("POST", "/api/v1/feed/recommendation/feedback", s.lab.Reader.SessionToken, jsonBytes(rw.Feedback{Receipt: *first.FeedbackReceipt, PostIDs: bad})), 400)
	}
	body := jsonBytes(rw.Feedback{Receipt: *first.FeedbackReceipt, PostIDs: ids[:2]})
	s.lab.FeedbackFault(t, true)
	before := s.lab.Snapshot(t)
	s.check(s.request("POST", "/api/v1/feed/recommendation/feedback", s.lab.Reader.SessionToken, body), 503)
	if before != s.lab.Snapshot(t) {
		t.Fatal("failed feedback committed partial credit/state")
	}
	s.lab.FeedbackFault(t, false)
	s.lab.BeginTrace()
	ack := s.check(s.request("POST", "/api/v1/feed/recommendation/feedback", s.lab.Reader.SessionToken, body), 200)
	trace := s.lab.FinishTrace()
	if strings.TrimSpace(string(ack.body)) != "{\"accepted\":true}" {
		t.Fatal("ACK shape")
	}
	if s.lab.Exposures(t, ids[0]) != 1 || s.lab.Exposures(t, ids[1]) != 1 {
		t.Fatal("feedback did not persist delivered exposures")
	}
	until := s.lab.ExposureUntil(t)
	s.lab.Advance(t, time.Minute)
	s.check(s.request("POST", "/api/v1/feed/recommendation/feedback", s.lab.Reader.SessionToken, body), 200)
	if !until.Equal(s.lab.ExposureUntil(t)) {
		t.Fatal("repeated feedback extended TTL")
	}
	next := s.page(s.ref("page", first.NextCursor, s.lab.Reader.SessionToken, 200))
	s.check(s.request("POST", "/api/v1/feed/recommendation/feedback", s.lab.Reader.SessionToken, jsonBytes(rw.Feedback{Receipt: *first.FeedbackReceipt, PostIDs: pageIDs(next)})), 400)
	s.lab.Hide(t, []string{ids[2]}, true)
	s.lab.DeletePost(t, ids[3])
	s.check(s.request("POST", "/api/v1/feed/recommendation/feedback", s.lab.Reader.SessionToken, jsonBytes(rw.Feedback{Receipt: *first.FeedbackReceipt, PostIDs: ids[2:4]})), 200)
	if s.lab.Exposures(t, ids[2]) != 0 || s.lab.Exposures(t, ids[3]) != 0 {
		t.Fatal("hidden/deleted content credited exposure")
	}
	var counts struct{ Categories map[string]int }
	if json.Unmarshal(trace, &counts) != nil || counts.Categories["A1_content_projection"] != 1 {
		t.Fatal("feedback did not use A1 projection")
	}
}

func TestRecommendationHTTPSAllOmittedAndEmptyProgressPostgres(t *testing.T) {
	t.Run("all_omitted", func(t *testing.T) {
		s := newRecoHTTPS(t, 25)
		body := jsonBytes(s.bootstrap())
		first := s.start(body)
		s.lab.Hide(t, pageIDs(first), true)
		empty := s.start(body)
		if len(empty.Items) != 0 || empty.NextCursor == nil {
			t.Fatal("all omit lost progress or refilled")
		}
		s.lab.Hide(t, pageIDs(first), false)
		again := s.start(body)
		if len(again.Items) != 0 {
			t.Fatal("omission resurrected")
		}
		next := s.page(s.ref("page", empty.NextCursor, s.lab.Reader.SessionToken, 200))
		if len(next.Items) == 0 {
			t.Fatal("next fixed page did not progress")
		}
	})
	t.Run("empty_manifest", func(t *testing.T) {
		s := newRecoHTTPS(t, 0)
		body := jsonBytes(s.bootstrap())
		p := s.start(body)
		if len(p.Items) != 0 || p.NextCursor != nil {
			t.Fatal("empty manifest did not terminate")
		}
		s.start(body)
		if s.lab.Rounds(t) != 1 {
			t.Fatal("empty replay duplicated round")
		}
	})
}

func TestRecommendationHTTPSScopeEpochsAndDisablePostgres(t *testing.T) {
	for _, mode := range []string{"preferences", "account_epoch", "security_epoch", "disable"} {
		t.Run(mode, func(t *testing.T) {
			s := newRecoHTTPS(t, 12)
			body := jsonBytes(s.bootstrap())
			first := s.start(body)
			switch mode {
			case "preferences":
				s.check(s.request("PUT", "/api/v1/me/feed-preferences", s.lab.Reader.SessionToken, jsonBytes(map[string]any{"blockedChannelIds": []string{s.lab.ChannelID}})), 200)
			case "account_epoch":
				s.lab.Epoch(t, true)
			case "security_epoch":
				s.lab.Epoch(t, false)
			case "disable":
				s.lab.Disable(t)
			}
			s.check(s.request("POST", "/api/v1/feed/recommendation/start", s.lab.Reader.SessionToken, body), 409)
			s.ref("page", first.NextCursor, s.lab.Reader.SessionToken, 409)
			s.ref("feedback", first.FeedbackReceipt, s.lab.Reader.SessionToken, 409)
			if mode == "disable" {
				normal := s.check(s.request("GET", "/api/v1/feed", s.lab.Reader.SessionToken, nil), 200)
				if normal.headers.Get("Recommendation-Available") != "false" || normal.headers.Get("Recommendation-Context") != "" {
					t.Fatal("disabled bootstrap")
				}
			}
		})
	}
}

func TestRecommendationHTTPSFinalSessionAndGatePostgres(t *testing.T) {
	for _, mode := range []string{"revoked", "replaced", "closure", "gate"} {
		t.Run(mode, func(t *testing.T) {
			s := newRecoHTTPS(t, 12)
			body := jsonBytes(s.bootstrap())
			first := s.start(body)
			status := 401
			bearer := s.lab.Reader.SessionToken
			switch mode {
			case "revoked":
				s.lab.Revoke(t)
			case "replaced":
				bearer = s.lab.ReplaceSession(t)
				status = 409
			case "closure":
				s.lab.CloseAccount(t)
			case "gate":
				s.lab.Freeze(t)
				status = 503
			}
			s.check(s.request("POST", "/api/v1/feed/recommendation/start", bearer, body), status)
			s.ref("page", first.NextCursor, bearer, status)
			s.ref("feedback", first.FeedbackReceipt, bearer, status)
			if mode == "replaced" {
				s.check(s.request("POST", "/api/v1/feed/recommendation/start", s.lab.Reader.SessionToken, body), 401)
			}
		})
	}
}

func TestRecommendationHTTPSOffM0M1AndStrictBoundaryPostgres(t *testing.T) {
	for _, m0 := range []bool{false, true} {
		name := "M1"
		if m0 {
			name = "M0"
		}
		t.Run(name, func(t *testing.T) {
			s := newRecoHTTPS(t, 1)
			input := s.bootstrap()
			s.server.Close()
			s.lab.Off(t, m0)
			s.open()
			r := s.check(s.request("GET", "/api/v1/feed", s.lab.Reader.SessionToken, nil), 200)
			if r.headers.Get("Recommendation-Available") != "false" || r.headers.Get("Recommendation-Context") != "" {
				t.Fatal("off emits active context")
			}
			body := jsonBytes(input)
			s.check(s.request("POST", "/api/v1/feed/recommendation/start", s.lab.Reader.SessionToken, body), 503)
			s.check(s.request("POST", "/api/v1/feed/recommendation/start", [32]byte{}, body), 401)
		})
	}
	t.Run("strict", func(t *testing.T) {
		s := newRecoHTTPS(t, 12)
		input := s.bootstrap()
		body := jsonBytes(input)
		for _, bad := range [][]byte{[]byte(strings.Replace(string(body), "\"version\":1", "\"version\":1,\"version\":1", 1)), []byte(strings.TrimSuffix(string(body), "}") + ",\"accountId\":\"x\"}"), []byte(`{"version":1}`)} {
			s.check(s.request("POST", "/api/v1/feed/recommendation/start", s.lab.Reader.SessionToken, bad), 400)
		}
		s.check(s.request("POST", "/api/v1/feed/recommendation/start", s.lab.Reader.SessionToken, bytes.Repeat([]byte(" "), rw.MaxBody+1)), 413)
		s.check(s.request("POST", "/api/v1/feed/recommendation/start?context=x", s.lab.Reader.SessionToken, body), 400)
		s.check(s.request("GET", "/api/v1/feed/recommendation/start", s.lab.Reader.SessionToken, nil), 405)
		s.start(body)
		s.lab.ProducerFault()
		s.check(s.request("POST", "/api/v1/feed/recommendation/start", s.lab.Reader.SessionToken, body), 503)
	})
}
