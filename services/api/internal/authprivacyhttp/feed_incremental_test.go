package authprivacyhttp

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/authprivacy"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/posts"
)

func TestFeedHTTPIncrementalInputsAndResponses(t *testing.T) {
	for _, tc := range []struct {
		query string
		limit int
		ok    bool
	}{
		{"paging=incremental-v1", 10, true}, {"paging=incremental-v1&limit=10", 10, true},
		{"paging=incremental-v1&cursor=YQ", 5, true}, {"paging=incremental-v1&refreshCursor=YQ&limit=5", 5, true},
		{"paging=incremental-v1&limit=5", 0, false}, {"paging=incremental-v1&cursor=YQ&limit=10", 0, false},
		{"paging=incremental-v1&cursor=YQ&refreshCursor=YQ", 0, false}, {"refreshCursor=YQ", 0, false},
		{"paging=other", 0, false}, {"paging=incremental-v1&paging=incremental-v1", 0, false},
	} {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/feed?"+tc.query, nil)
		q, e := postReadQuery(r, postRoute{kind: authprivacy.PostFeed})
		if (e == nil) != tc.ok || tc.ok && (q.Limit != tc.limit || !q.Incremental) {
			t.Fatalf("query %s limit%d err%v", tc.query, q.Limit, e)
		}
	}
	backend, handler := newPostHTTP(t, httpTestLimits())
	token := "YQ"
	backend.value = posts.IncrementalFeedPage{Items: []posts.FeedPost{}, RefreshCursor: &token}
	w := postHTTPRequest(handler, "GET", "/api/v1/feed?paging=incremental-v1", "", postHTTPHeaders(false))
	if w.Code != 200 {
		t.Fatal("new protocol", w.Code, w.Body.String())
	}
	w = postHTTPRequest(handler, "GET", "/api/v1/feed", "", postHTTPHeaders(false))
	if w.Code != 503 {
		t.Fatal("new shape escaped to strict legacy client")
	}
	backend.value = posts.IncrementalFeedPage{Items: []posts.FeedPost{}}
	w = postHTTPRequest(handler, "GET", "/api/v1/feed?paging=incremental-v1", "", postHTTPHeaders(false))
	if w.Code != 503 {
		t.Fatal("missing refresh cursor accepted")
	}
}
