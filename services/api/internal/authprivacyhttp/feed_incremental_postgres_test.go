package authprivacyhttp

import (
	"context"
	"fmt"
	"testing"

	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/authprivacy"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/posts"
)

func TestRealHTTPSFeedIncrementalPostgres(t *testing.T) {
	s := e2eNewServices(t, func(c *authprivacy.Community) *authprivacy.Community { return preparedFeedHTTP(t, c) })
	owner, _ := e2ePrivacySignup(t, s, "window_http_owner")
	reader, _ := e2ePrivacySignup(t, s, "window_http_reader")
	channel := postHTTPSChannel(t, s, owner)
	identity := postHTTPSIdentity(t, s, owner, "分页作者", 91)
	publish := func(start, n int) {
		for k := start; k < start+n; k++ {
			postHTTPSJSON(t, s, "PUT", "/api/v1/post-commands/"+httpTestEncoding(16, byte(k+10)), owner, "", posts.CreatePostInput{ChannelID: channel, IdentityID: identity, Title: fmt.Sprintf("Post %02d", k), Body: "Published text"}, 202)
		}
		if _, e := s.c.PublishAcceptedPosts(context.Background(), 100); e != nil {
			t.Fatal(e)
		}
	}
	get := func(query string) map[string]any {
		return postHTTPSJSON(t, s, "GET", "/api/v1/feed?paging=incremental-v1"+query, reader, "", nil, 200)
	}
	publish(0, 16)
	first := get("")
	postHTTPSFields(t, first, "items", "nextCursor", "refreshCursor", "insertAfter")
	if len(first["items"].([]any)) != 10 {
		t.Fatal("initial size")
	}
	older := get("&cursor=" + first["nextCursor"].(string))
	if len(older["items"].([]any)) != 5 || older["refreshCursor"] != nil {
		t.Fatal("continuation")
	}
	publish(16, 12)
	token := first["refreshCursor"].(string)
	for _, expected := range []int{5, 5, 2, 0} {
		p := get("&refreshCursor=" + token)
		if len(p["items"].([]any)) != expected || p["nextCursor"] != nil {
			t.Fatal("refresh batch or old cursor overwritten")
		}
		token = p["refreshCursor"].(string)
	}
	postHTTPSJSON(t, s, "GET", "/api/v1/feed?paging=incremental-v1&refreshCursor="+token, owner, "", nil, 400)
	postHTTPSJSON(t, s, "GET", "/api/v1/feed?cursor="+first["nextCursor"].(string)+"&limit=5", reader, "", nil, 400)
	if e := s.gate.Freeze(context.Background(), "incremental final authority denial"); e != nil {
		t.Fatal(e)
	}
	postHTTPSJSON(t, s, "GET", "/api/v1/feed?paging=incremental-v1&refreshCursor="+token, reader, "", nil, 503)
}
