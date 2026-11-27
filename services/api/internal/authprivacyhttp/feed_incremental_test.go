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

func TestFeedContinuationPreviewNegotiation(t *testing.T) {
	backend, handler := newPostHTTP(t, httpTestLimits())
	backend.value = posts.IncrementalFeedPage{Items: []posts.FeedPost{}}
	const feed = "/api/v1/feed?paging=incremental-v1"
	for _, tc := range []struct {
		name, path, preview string
		extras              []string
		status              int
	}{
		{"continuation", feed + "&cursor=YQ", "thumbnail-v1", []string{"previews-v1"}, 200},
		{"old client", feed + "&cursor=YQ", "thumbnail-v1", nil, 200},
		{"initial", feed, "thumbnail-v1", []string{"previews-v1"}, 400},
		{"refresh", feed + "&refreshCursor=YQ", "thumbnail-v1", []string{"previews-v1"}, 400},
		{"legacy", "/api/v1/feed?cursor=YQ", "thumbnail-v1", []string{"previews-v1"}, 400},
		{"no thumbnail opt-in", feed + "&cursor=YQ", "", []string{"previews-v1"}, 400},
		{"duplicates", feed + "&cursor=YQ", "thumbnail-v1", []string{"previews-v1", "previews-v1"}, 400},
		{"unsupported value", feed + "&cursor=YQ", "thumbnail-v1", []string{"other"}, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := postHTTPHeaders(false)
			if tc.preview != "" {
				h.Set("X-Post-Preview", tc.preview)
			}
			for _, extra := range tc.extras {
				h.Add("X-Feed-Continuation-Extras", extra)
			}
			calls := backend.calls
			w := postHTTPRequest(handler, http.MethodGet, tc.path, "", h)
			if w.Code != tc.status {
				t.Fatalf("status %d, want %d: %s", w.Code, tc.status, w.Body.String())
			}
			if tc.status == 200 {
				if backend.calls != calls+1 || backend.query.WindowPreviews != (len(tc.extras) == 1) {
					t.Fatal("continuation preview intent was routed incorrectly")
				}
			} else if backend.calls != calls {
				t.Fatal("malformed negotiation reached the backend")
			}
		})
	}
	preflight := http.Header{
		"Origin":                         {"https://client.example"},
		"Access-Control-Request-Method":  {"GET"},
		"Access-Control-Request-Headers": {"authorization, x-post-preview, x-feed-continuation-extras"},
	}
	if w := postHTTPRequest(handler, http.MethodOptions, "/api/v1/feed", "", preflight); w.Code != 204 {
		t.Fatal("continuation preflight rejected", w.Code)
	}
}
