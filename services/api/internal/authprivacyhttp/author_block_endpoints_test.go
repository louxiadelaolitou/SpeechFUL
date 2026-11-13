package authprivacyhttp

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/authprivacy"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/posts"
)

const postHTTPBlock = "44444444-4444-4444-8444-444444444444"

func (f *postHTTPBackend) BlockPostAuthor(_ context.Context, bearer [32]byte, id string) (authprivacy.PostResponse, error) {
	f.calls++
	f.bearer, f.method, f.command = bearer, "block", id
	return f.response(posts.AuthorBlockReceipt{
		BlockID:   postHTTPBlock,
		State:     "BLOCKED",
		Author:    postHTTPCard().Author,
		BlockedAt: postHTTPTime,
	})
}

func (f *postHTTPBackend) AuthorBlocks(_ context.Context, bearer [32]byte) (authprivacy.PostResponse, error) {
	f.calls++
	f.bearer, f.method = bearer, "blocks"
	return f.response(posts.BlockedAuthorPage{Items: []posts.BlockedAuthor{}})
}

func (f *postHTTPBackend) UnblockPostAuthor(_ context.Context, bearer [32]byte, id string) (authprivacy.PostResponse, error) {
	f.calls++
	f.bearer, f.method, f.command = bearer, "unblock", id
	return f.response(posts.AuthorBlockRemoval{BlockID: id, State: "REMOVED"})
}

func TestAuthorBlockStrictHTTP(t *testing.T) {
	backend, handler := newPostHTTP(t, httpTestLimits())
	setPath := "/api/v1/posts/" + postHTTPPost + "/author/block"
	listPath := "/api/v1/me/blocked-authors"
	removePath := listPath + "/" + postHTTPBlock
	headers := postHTTPHeaders(false)

	// Every author-block route forbids a command key and any payload.
	withKey := postHTTPHeaders(true)
	before := backend.calls
	if w := postHTTPRequest(handler, "PUT", setPath, "", withKey); w.Code != 400 || backend.calls != before {
		t.Fatal("block accepted idempotency key", w.Code)
	}
	if w := postHTTPRequest(handler, "PUT", setPath, `{}`, headers); w.Code != 400 || backend.calls != before {
		t.Fatal("block accepted body", w.Code)
	}
	if w := postHTTPRequest(handler, "PUT", setPath+"?accountId="+postHTTPPost, "", headers); w.Code != 400 || backend.calls != before {
		t.Fatal("block accepted query", w.Code)
	}
	if w := postHTTPRequest(handler, "GET", setPath, "", headers); w.Code != 405 || backend.calls != before {
		t.Fatal("block method", w.Code)
	}

	w := postHTTPRequest(handler, "PUT", setPath, "", headers)
	if w.Code != 200 {
		t.Fatal("block", w.Code, w.Body.String())
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"blockId", "state", "author", "blockedAt"} {
		if _, ok := body[field]; !ok {
			t.Fatalf("block response misses %s", field)
		}
	}
	if backend.method != "block" || backend.command != postHTTPPost {
		t.Fatal("block reached the wrong backend call", backend.method, backend.command)
	}

	if w := postHTTPRequest(handler, "GET", listPath, "", headers); w.Code != 200 || !strings.Contains(w.Body.String(), `"items":[]`) {
		t.Fatal("list", w.Code, w.Body.String())
	}
	if w := postHTTPRequest(handler, "GET", listPath+"?cursor=next", "", headers); w.Code != 400 {
		t.Fatal("list accepted query", w.Code)
	}
	if w := postHTTPRequest(handler, "DELETE", listPath, "", headers); w.Code != 405 {
		t.Fatal("list method", w.Code)
	}
	if w := postHTTPRequest(handler, "GET", listPath, "", http.Header{}); w.Code != 401 {
		t.Fatal("list unauthenticated", w.Code)
	}

	if w := postHTTPRequest(handler, "DELETE", removePath, "", headers); w.Code != 200 || !strings.Contains(w.Body.String(), `"state":"REMOVED"`) {
		t.Fatal("unblock", w.Code, w.Body.String())
	}

	// Structured errors keep the status and never renew a session.
	backend.err = &authprivacy.PostError{Code: "SELF_BLOCK_REJECTED", Status: 409}
	if w := postHTTPRequest(handler, "PUT", setPath, "", headers); w.Code != 409 || w.Header().Get("Session-Expires-At") != "" {
		t.Fatal("self block rejection", w.Code)
	}
	backend.err = nil

	// A backend value that violates the closed DTO shape fails closed.
	backend.value = posts.AuthorBlockReceipt{BlockID: postHTTPBlock, State: "BLOCKED", Author: posts.PublicAuthor{State: "ACTIVE", Nickname: "x", Avatar: "default-v1"}, BlockedAt: postHTTPTime}
	if w := postHTTPRequest(handler, "PUT", setPath, "", headers); w.Code != 200 {
		t.Fatal("valid receipt", w.Code, w.Body.String())
	}
	backend.value = posts.AuthorBlockReceipt{BlockID: "not-an-id", State: "BLOCKED", Author: postHTTPCard().Author, BlockedAt: postHTTPTime}
	if w := postHTTPRequest(handler, "PUT", setPath, "", headers); w.Code != 503 {
		t.Fatal("malformed receipt accepted", w.Code)
	}
	backend.value = posts.BlockedAuthorPage{Items: nil}
	if w := postHTTPRequest(handler, "GET", listPath, "", headers); w.Code != 503 {
		t.Fatal("nil list accepted", w.Code)
	}
	backend.value = posts.BlockedAuthorPage{Items: []posts.BlockedAuthor{
		{BlockID: postHTTPBlock, Author: postHTTPCard().Author, BlockedAt: postHTTPTime},
		{BlockID: postHTTPBlock, Author: postHTTPCard().Author, BlockedAt: postHTTPTime},
	}}
	if w := postHTTPRequest(handler, "GET", listPath, "", headers); w.Code != 503 {
		t.Fatal("duplicate block ids accepted", w.Code)
	}
	backend.value = posts.AuthorBlockRemoval{BlockID: postHTTPPost, State: "REMOVED"}
	if w := postHTTPRequest(handler, "DELETE", removePath, "", headers); w.Code != 503 {
		t.Fatal("mismatched unblock accepted", w.Code)
	}
	backend.value = posts.AuthorBlockRemoval{BlockID: postHTTPBlock, State: "BLOCKED"}
	if w := postHTTPRequest(handler, "DELETE", removePath, "", headers); w.Code != 503 {
		t.Fatal("unknown unblock state accepted", w.Code)
	}
}
