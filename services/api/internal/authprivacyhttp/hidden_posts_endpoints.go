package authprivacyhttp

import (
	"context"
	"net/http"

	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/authprivacy"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/posts"
)

type HiddenPostBackend interface {
	ReadHiddenPost(context.Context, [32]byte, string) (authprivacy.PostResponse, error)
	SetHiddenPost(context.Context, [32]byte, string, bool, string) (authprivacy.PostResponse, error)
}

func isHiddenPostKind(kind authprivacy.PostReadKind) bool {
	return kind == authprivacy.PostHiddenState || kind == authprivacy.PostHiddenSet
}

func (b *boundary) communityHiddenPost(w http.ResponseWriter, r *http.Request, backend PostBackend, bearer [32]byte, route postRoute) {
	service, ok := backend.(HiddenPostBackend)
	if !ok {
		b.postFail(w, 503, "SERVICE_UNAVAILABLE", 0)
		return
	}
	if hasHeader(r.Header, "If-Match") {
		b.postBad(w, errMalformed)
		return
	}
	var response authprivacy.PostResponse
	var err error
	if r.Method == http.MethodGet {
		if hasHeader(r.Header, "If-Interaction-Version") || noBodyOrQuery(r) != nil {
			b.postBad(w, errMalformed)
			return
		}
		response, err = service.ReadHiddenPost(r.Context(), bearer, route.id)
	} else {
		expected, e := singleHeader(r.Header, "If-Interaction-Version")
		if e != nil || !posts.ValidInteractionVersion(expected) {
			b.postBad(w, errMalformed)
			return
		}
		object, e := readRequestObject(r, 1024, []string{"hidden"}, nil)
		if e != nil {
			b.postBad(w, e)
			return
		}
		desired, ok := object["hidden"].(bool)
		if !ok {
			b.postBad(w, errMalformed)
			return
		}
		route.enabled = &desired
		response, err = service.SetHiddenPost(r.Context(), bearer, route.id, desired, expected)
	}
	if err != nil {
		b.postFailure(w, err)
		return
	}
	if !posts.ValidInteractionVersion(response.InteractionVersion) {
		b.postFail(w, 503, "SERVICE_UNAVAILABLE", 0)
		return
	}
	b.writePostResponse(w, response, route, false)
}
