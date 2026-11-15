package authprivacyhttp

import (
	"context"
	"testing"

	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/authprivacy"
)

// e2eNewServices applies decorators before creating accounts or protocol
// history. This real configuration enables the flag through its schema check.
func preparedFeedHTTP(t *testing.T, c *authprivacy.Community) *authprivacy.Community {
	t.Helper()
	clone, e := c.WithPosts(context.Background(), authprivacy.PostConfig{Environment: "prepared-feed-http", KeyKey: e2eKey(t), FingerprintKey: e2eKey(t), CursorKey: e2eKey(t), PreparedFeedReads: true})
	if e != nil {
		t.Fatal(e)
	}
	return clone
}
