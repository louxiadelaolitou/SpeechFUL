package authprivacy

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/posts"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/readtiming"
)

func TestFeedContinuationPreviewsPostgres(t *testing.T) {
	f := newPreparedMediaFixture(t)
	ctx := context.Background()
	f.config.PreparedFeedReads = true
	var err error
	f.l.c, err = f.l.c.WithPosts(ctx, f.config)
	if err != nil {
		t.Fatal(err)
	}
	identity := identityChange(t, f.l.c, f.owner.SessionToken, "CREATE", uuid.Nil, "续页作者").IdentityID
	textPost := preparedFeedPublish(t, f.l, f.owner.SessionToken, identity, "续页文字")
	for range 10 {
		preparedFeedPublish(t, f.l, f.owner.SessionToken, identity, "首屏文字")
	}
	first := postTestRead(t, f.l, f.viewer.SessionToken, PostQuery{Kind: PostFeed, Incremental: true}).(posts.IncrementalFeedPage)
	if first.NextCursor == nil {
		t.Fatal("fixture must have a continuation")
	}
	query := PostQuery{Kind: PostFeed, Incremental: true, Cursor: *first.NextCursor, WindowPreviews: true, ImageSizes: true}
	read := func() (FeedWindowPreviews, map[string]readtiming.Stage) {
		t.Helper()
		timed, trace := readtiming.Start(ctx)
		response, err := f.l.c.ReadPosts(timed, f.viewer.SessionToken, query)
		if err != nil {
			t.Fatal(err)
		}
		window, ok := response.Value.(FeedWindowPreviews)
		if !ok || window.RefreshCursor != nil || window.InsertAfter != nil {
			t.Fatal("expected a continuation with previews", response.Value)
		}
		stages := trace.Snapshot().Stages
		if stages["feed.anchor_write"].Calls != 1 || stages["feed.sql_commit"].Calls != 1 {
			t.Fatal("expected one final checkpoint and one commit", stages)
		}
		return window, stages
	}
	window, stages := read()
	if stages["feed.finalize_read"].Calls != 1 ||
		len(window.Items) != 2 || window.Items[0].PostID != textPost || window.Items[1].PostID != f.post ||
		len(window.Previews.Items) != 2 || window.Previews.Items[0].PostID != textPost || window.Previews.Items[1].PostID != f.post {
		t.Fatal("continuation previews did not match the page order", window)
	}
	groups := window.Previews.ImageManifests
	if len(groups) != 1 || groups[0].PostID != f.post || len(groups[0].Images) != 1 || groups[0].Images[0].ContentVersionID != f.content {
		t.Fatal("continuation did not return the current image selection", groups)
	}

	preparedReadPool(t, f.l, func(attempt int) {
		if attempt == 1 {
			mustExec(t, f.l.cp, "UPDATE c_posts.posts SET moderation_hidden=true WHERE post_id=$1", f.post)
		}
	})
	rebuilt, stages := read()
	if stages["feed.prepared_read_conflict"].Calls != 1 || stages["feed.finalize_read"].Calls != 2 ||
		len(rebuilt.Items) != 1 || rebuilt.Items[0].PostID != textPost ||
		len(rebuilt.Previews.Items) != 1 || rebuilt.Previews.Items[0].PostID != textPost || len(rebuilt.Previews.ImageManifests) != 0 {
		t.Fatal("visibility changed during preparation, but the continuation did not rebuild cleanly", rebuilt, stages)
	}
}
