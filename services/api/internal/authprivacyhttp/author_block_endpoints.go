package authprivacyhttp

import (
	"context"
	"net/http"

	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/authprivacy"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/posts"
)

type AuthorBlockBackend interface {
	BlockPostAuthor(context.Context, [32]byte, string) (authprivacy.PostResponse, error)
	AuthorBlocks(context.Context, [32]byte) (authprivacy.PostResponse, error)
	UnblockPostAuthor(context.Context, [32]byte, string) (authprivacy.PostResponse, error)
}

func (b *boundary) communityAuthorBlocks(w http.ResponseWriter, r *http.Request, backend PostBackend, bearer [32]byte, route postRoute) {
	blocks, ok := backend.(AuthorBlockBackend)
	if !ok {
		b.postFail(w, 503, "SERVICE_UNAVAILABLE", 0)
		return
	}
	if noBodyOrQuery(r) != nil {
		b.postBad(w, errMalformed)
		return
	}
	var response authprivacy.PostResponse
	var err error
	switch route.block {
	case "set":
		response, err = blocks.BlockPostAuthor(r.Context(), bearer, route.id)
	case "list":
		response, err = blocks.AuthorBlocks(r.Context(), bearer)
	case "remove":
		response, err = blocks.UnblockPostAuthor(r.Context(), bearer, route.id)
	}
	if err != nil {
		b.postFailure(w, err)
		return
	}
	b.writePostResponse(w, response, route, false)
}

func strictAuthorBlockValue(value any, route postRoute) (any, error) {
	switch route.block {
	case "set":
		result, ok := value.(posts.AuthorBlockReceipt)
		if p, yes := value.(*posts.AuthorBlockReceipt); yes && p != nil {
			result, ok = *p, true
		}
		if !ok || result.State != "BLOCKED" || !postValidID(result.BlockID) || !validPostTime(result.BlockedAt) || !validPublicAuthor(result.Author) {
			return nil, errMalformed
		}
		return result, nil
	case "list":
		result, ok := value.(posts.BlockedAuthorPage)
		if p, yes := value.(*posts.BlockedAuthorPage); yes && p != nil {
			result, ok = *p, true
		}
		if !ok || result.Items == nil {
			return nil, errMalformed
		}
		seen := make(map[string]bool, len(result.Items))
		for _, item := range result.Items {
			if !postValidID(item.BlockID) || seen[item.BlockID] || !validPostTime(item.BlockedAt) || !validPublicAuthor(item.Author) {
				return nil, errMalformed
			}
			seen[item.BlockID] = true
		}
		return result, nil
	case "remove":
		result, ok := value.(posts.AuthorBlockRemoval)
		if p, yes := value.(*posts.AuthorBlockRemoval); yes && p != nil {
			result, ok = *p, true
		}
		if !ok || result.BlockID != route.id || !postValidID(result.BlockID) || (result.State != "REMOVED" && result.State != "ABSENT") {
			return nil, errMalformed
		}
		return result, nil
	}
	return nil, errMalformed
}
