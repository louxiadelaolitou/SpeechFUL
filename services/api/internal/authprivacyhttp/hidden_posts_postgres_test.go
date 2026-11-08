package authprivacyhttp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/posts"
)

// Exercise the actual TLS boundary, private write fence and database together.
func TestRealHTTPSFeedHiddenPostPostgres(t *testing.T) {
	s := e2eNewServices(t)
	owner, _ := e2ePrivacySignup(t, s, "hidden_https_owner")
	reader, _ := e2ePrivacySignup(t, s, "hidden_https_reader")
	channel := postHTTPSChannel(t, s, owner)
	identity := postHTTPSIdentity(t, s, owner, "Author", 171)
	created := postHTTPSJSON(t, s, "PUT", "/api/v1/post-commands/"+httpTestEncoding(16, 172), owner, "", posts.CreatePostInput{ChannelID: channel, IdentityID: identity, Title: "Hidden", Body: "Body"}, 202)
	if _, err := s.c.PublishAcceptedPosts(context.Background(), 100); err != nil {
		t.Fatal(err)
	}
	post := created["postId"].(string)
	path := "/api/v1/posts/" + post + "/hidden"
	call := func(method, body, version string, status int) (string, map[string]any) {
		t.Helper()
		req, err := http.NewRequest(method, s.cPublic.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+reader)
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		if version != "" {
			req.Header.Set("If-Interaction-Version", version)
		}
		response, err := s.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		raw, err := io.ReadAll(response.Body)
		if err != nil || response.StatusCode != status {
			t.Fatalf("hidden TLS status %d, want %d: %v", response.StatusCode, status, err)
		}
		var result map[string]any
		if err := json.Unmarshal(raw, &result); err != nil {
			t.Fatal(err)
		}
		version = response.Header.Get("Interaction-Version")
		if status == 200 {
			if !posts.ValidInteractionVersion(version) || response.Header.Get("Session-Expires-At") == "" {
				t.Fatal("missing successful response authority")
			}
			postHTTPSFields(t, result, "postId", "hidden")
		} else if version != "" || response.Header.Get("Session-Expires-At") != "" {
			t.Fatal("failed request renewed authority")
		}
		return version, result
	}
	initial, state := call("GET", "", "", 200)
	if state["hidden"] != false {
		t.Fatal("initial state hidden")
	}
	hidden, state := call("PUT", `{"hidden":true}`, initial, 200)
	if state["hidden"] != true || hidden == initial {
		t.Fatal("hide did not rotate version")
	}
	if items := postHTTPSJSON(t, s, "GET", "/api/v1/feed", reader, "", nil, 200)["items"].([]any); len(items) != 0 {
		t.Fatal("hidden post leaked into feed")
	}
	if items := postHTTPSJSON(t, s, "GET", "/api/v1/feed", owner, "", nil, 200)["items"].([]any); len(items) != 1 {
		t.Fatal("hidden preference crossed account")
	}
	postHTTPSJSON(t, s, "GET", "/api/v1/posts/"+post, reader, "", nil, 200)
	restored, state := call("PUT", `{"hidden":false}`, hidden, 200)
	if state["hidden"] != false || restored == hidden {
		t.Fatal("restore did not rotate version")
	}
	call("PUT", `{"hidden":true}`, initial, 412)
	stable, state := call("GET", "", "", 200)
	if state["hidden"] != false || stable != restored {
		t.Fatal("late hide overwrote undo")
	}
}
