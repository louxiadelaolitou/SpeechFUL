package authprivacy

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/posts"
)

// Exercises real SQL, the final Gate, account isolation, cursor invalidation,
// and the write fence for undo versus a delayed original hide.
func TestFeedHiddenPostLifecyclePostgres(t *testing.T) {
	l := newPostLab(t)
	ctx := context.Background()
	owner := sessionTestAccount(t, l, "hidden_post_owner")
	reader := sessionTestAccount(t, l, "hidden_post_reader")
	other := sessionTestAccount(t, l, "hidden_post_other")
	identity := identityChange(t, l.c, owner.SessionToken, "CREATE", uuid.Nil, "Author").IdentityID
	intent := postTestIntent(t, l, identity)
	intent.Title = "searchable"
	first := postTestCommand(t, l, owner.SessionToken, postTestKey(t), intent)
	second := postTestCommand(t, l, owner.SessionToken, postTestKey(t), intent)
	if first.State != "ACCEPTED" || second.State != "ACCEPTED" {
		t.Fatalf("post fixtures rejected: %#v %#v", first, second)
	}
	postTestPublish(t, l)
	for _, kind := range []posts.InteractionKind{posts.PostLike, posts.PostFavorite} {
		interactionTestSet(t, l, reader.SessionToken, first.PostID, kind, true)
	}
	feedBefore := postTestRead(t, l, reader.SessionToken, PostQuery{Kind: PostFeed, Limit: 1}).(posts.CrossChannelFeedPage)
	if feedBefore.NextCursor == nil {
		t.Fatal("missing cursor fixture")
	}
	initial, err := l.c.ReadHiddenPost(ctx, reader.SessionToken, first.PostID)
	if err != nil || initial.Value.(posts.HiddenPostState).Hidden || !posts.ValidInteractionVersion(initial.InteractionVersion) {
		t.Fatal("initial hidden state", err)
	}
	hidden, err := l.c.SetHiddenPost(ctx, reader.SessionToken, first.PostID, true, initial.InteractionVersion)
	if err != nil || !hidden.Value.(posts.HiddenPostState).Hidden || hidden.InteractionVersion == initial.InteractionVersion {
		t.Fatal("hide", err)
	}
	if _, err = l.c.ReadPosts(ctx, reader.SessionToken, PostQuery{Kind: PostFeed, Limit: 1, Cursor: *feedBefore.NextCursor}); !errors.Is(err, posts.ErrCursorInvalid) {
		t.Fatal("old cursor accepted after hiding", err)
	}
	readerFeed := postTestRead(t, l, reader.SessionToken, PostQuery{Kind: PostFeed}).(posts.CrossChannelFeedPage)
	if len(readerFeed.Items) != 1 || readerFeed.Items[0].PostID != second.PostID {
		t.Fatal("hidden post leaked in feed", readerFeed)
	}
	channelFeed := postTestRead(t, l, reader.SessionToken, PostQuery{Kind: PostChannelFeed, ID: intent.ChannelID}).(posts.FeedPage)
	if len(channelFeed.Items) != 1 || channelFeed.Items[0].PostID != second.PostID {
		t.Fatal("hidden post leaked in channel", channelFeed)
	}
	for _, kind := range []PostReadKind{PostLikedList, PostFavoriteList} {
		page := postTestRead(t, l, reader.SessionToken, PostQuery{Kind: kind}).(posts.FeedPage)
		if len(page.Items) != 0 {
			t.Fatal("hidden post leaked in collection", kind, page)
		}
	}
	search, err := l.c.SearchPosts(ctx, reader.SessionToken, posts.SearchQuery{Keyword: "searchable"})
	if err != nil {
		t.Fatal("search failed", err)
	}
	page := search.Value.(posts.FeedPage)
	if len(page.Items) != 1 || page.Items[0].PostID != second.PostID {
		t.Fatal("hidden post leaked in search", page)
	}
	// The account preference does not change another account or direct links.
	if len(postTestRead(t, l, other.SessionToken, PostQuery{Kind: PostFeed}).(posts.CrossChannelFeedPage).Items) != 2 {
		t.Fatal("preference crossed account")
	}
	postTestRead(t, l, reader.SessionToken, PostQuery{Kind: PostDetail, ID: first.PostID})
	if count(t, l.cp, `SELECT count(*) FROM c_posts.post_likes WHERE account_id=$1 AND post_id=$2`, reader.AccountID, first.PostID) != 1 ||
		count(t, l.cp, `SELECT count(*) FROM c_posts.post_favorites WHERE account_id=$1 AND post_id=$2`, reader.AccountID, first.PostID) != 1 {
		t.Fatal("hiding modified like or favorite")
	}
	restored, err := l.c.SetHiddenPost(ctx, reader.SessionToken, first.PostID, false, hidden.InteractionVersion)
	if err != nil || restored.Value.(posts.HiddenPostState).Hidden {
		t.Fatal("undo failed", err)
	}
	_, err = l.c.SetHiddenPost(ctx, reader.SessionToken, first.PostID, true, initial.InteractionVersion)
	postTestError(t, err, "INTERACTION_VERSION_CONFLICT", 412)
	if len(postTestRead(t, l, reader.SessionToken, PostQuery{Kind: PostFeed}).(posts.CrossChannelFeedPage).Items) != 2 {
		t.Fatal("late hide defeated undo")
	}
	if _, err = l.c.ReadHiddenPost(ctx, [32]byte{}, first.PostID); !errors.Is(err, ErrSessionInvalid) {
		t.Fatal("missing auth accepted", err)
	}
	if err = l.gate.Freeze(ctx, "hidden post gate test"); err != nil {
		t.Fatal(err)
	}
	if _, err = l.c.SetHiddenPost(ctx, reader.SessionToken, first.PostID, true, restored.InteractionVersion); err == nil {
		t.Fatal("frozen gate accepted hidden write")
	}
}
