package authprivacyhttp

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/authprivacy"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/posts"
)

func (f *postHTTPBackend) ReadHiddenPost(_ context.Context, bearer [32]byte, id string) (authprivacy.PostResponse, error) {
	f.calls++
	f.bearer = bearer
	r, e := f.response(posts.HiddenPostState{PostID: id})
	r.InteractionVersion = `"` + strings.Repeat("b", 64) + `"`
	return r, e
}
func (f *postHTTPBackend) SetHiddenPost(_ context.Context, bearer [32]byte, id string, hidden bool, version string) (authprivacy.PostResponse, error) {
	f.calls++
	f.bearer = bearer
	f.command = version
	r, e := f.response(posts.HiddenPostState{PostID: id, Hidden: hidden})
	r.InteractionVersion = `"` + strings.Repeat("b", 64) + `"`
	return r, e
}

func TestFeedHiddenPostStrictHTTP(t *testing.T) {
	backend, handler := newPostHTTP(t, httpTestLimits())
	path := "/api/v1/posts/" + postHTTPPost + "/hidden"
	headers := postHTTPHeaders(false)
	if w := postHTTPRequest(handler, "PUT", path, `{"hidden":true}`, headers); w.Code != 400 {
		t.Fatal("missing version", w.Code)
	}
	headers.Set("If-Interaction-Version", `"`+strings.Repeat("a", 64)+`"`)
	for _, body := range []string{`{"hidden":"true"}`, `{"hidden":true,"accountId":"x"}`, `{"hidden":true,"hidden":false}`} {
		before := backend.calls
		if w := postHTTPRequest(handler, "PUT", path, body, headers); w.Code != 400 || backend.calls != before {
			t.Fatal("invalid request reached backend", w.Code)
		}
	}
	if w := postHTTPRequest(handler, "PUT", path, `{"hidden":true}`, headers); w.Code != 200 || !posts.ValidInteractionVersion(w.Header().Get("Interaction-Version")) {
		t.Fatal("write", w.Code, w.Body.String())
	}
	if w := postHTTPRequest(handler, "GET", path, "", postHTTPHeaders(false)); w.Code != 200 || !strings.Contains(w.Body.String(), `"hidden":false`) {
		t.Fatal("read", w.Code, w.Body.String())
	}
	if w := postHTTPRequest(handler, "GET", path+"?accountId="+postHTTPPost, "", postHTTPHeaders(false)); w.Code != 400 {
		t.Fatal("unexpected query accepted", w.Code)
	}
	if w := postHTTPRequest(handler, "GET", path, "", http.Header{}); w.Code != 401 {
		t.Fatal("unauthenticated", w.Code)
	}
	backend.err = &authprivacy.PostError{Code: "INTERACTION_VERSION_CONFLICT", Status: 412}
	if w := postHTTPRequest(handler, "PUT", path, `{"hidden":false}`, headers); w.Code != 412 || w.Header().Get("Session-Expires-At") != "" {
		t.Fatal("conflict metadata", w.Code)
	}
}
