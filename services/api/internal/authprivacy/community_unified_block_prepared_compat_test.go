package authprivacy

import (
	"bytes"
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/postmedia"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/posts"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/readtiming"
)

// The main branch's unified DM API must advance both endpoints of the new
// prepared-read fence. All business changes below go through their real API;
// SQL is used only to inspect the owned fixture and private revision evidence.
func TestUnifiedDMBlockPreparedReadCompatibilityPostgres(t *testing.T) {
	f := newPreparedMediaFixture(t)
	l, ctx := f.l, context.Background()
	f.config.PreparedFeedReads = true
	var err error
	l.c, err = l.c.WithPosts(ctx, f.config)
	if err != nil {
		t.Fatal("full prepared post flags rejected complete schema", err)
	}
	commentCfg := CommentConfig{PreparedReads: true}
	if commentCfg.KeyKey, err = random32(); err != nil {
		t.Fatal(err)
	}
	if commentCfg.FingerprintKey, err = random32(); err != nil {
		t.Fatal(err)
	}
	l.c, err = l.c.WithComments(ctx, commentCfg)
	if err != nil {
		t.Fatal("prepared comments rejected complete schema", err)
	}
	l.c, err = l.c.WithDirectMessages(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !l.c.nativePreparedReads() {
		t.Fatal("compatibility test selected a custom Gate fallback")
	}
	var ownerIdentity uuid.UUID
	if err = l.cp.QueryRow(ctx, `SELECT identity_id FROM c_posts.publication_tasks WHERE post_id=$1`, f.post).Scan(&ownerIdentity); err != nil {
		t.Fatal(err)
	}
	viewerIdentity := identityChange(t, l.c, f.viewer.SessionToken, "CREATE", uuid.Nil, "统一拉黑读者").IdentityID
	intent := postTestIntent(t, l, viewerIdentity)
	selection, viewerData := imageFixture(t, "png")
	intent.Images = []postmedia.Selection{selection}
	viewerPost := postTestCommand(t, l, f.viewer.SessionToken, postTestKey(t), intent)
	imageUpload(t, l, viewerPost, selection, imageGrant(t, l, f.viewer.SessionToken, viewerPost, selection.MediaID), viewerData)
	postTestPublish(t, l)
	manifest, err := l.c.ReadPostImages(ctx, f.owner.SessionToken, viewerPost.PostID, "")
	if err != nil {
		t.Fatal(err)
	}
	viewerContent := manifest.Value.(postmedia.Group).Images[0].ContentVersionID
	preparedComment(t, l, f.owner.SessionToken, viewerPost.PostID, ownerIdentity, "")
	preparedComment(t, l, f.viewer.SessionToken, f.post, viewerIdentity, "")
	contact, err := l.c.SendDMText(ctx, f.viewer.SessionToken, uuid.NewString(), DMTextIntent{
		IdentityID: viewerIdentity.String(), PeerID: ownerIdentity.String(), SourcePostID: f.post, Text: "open unified block fixture", Attempt: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	conversation := contact.Value.(DMReceipt)
	if conversation.State != "SENT" || conversation.ConversationID == "" {
		t.Fatal("DM conversation fixture failed", conversation)
	}

	visibility := func(account uuid.UUID) int {
		return count(t, l.cp, `SELECT visibility_revision FROM c_posts.read_account_versions WHERE account_id=$1`, account)
	}
	checkMutualReads := func(hidden bool) {
		t.Helper()
		for _, side := range []struct {
			bearer                      [32]byte
			otherPost, ownPost, content string
			data                        []byte
		}{
			{f.viewer.SessionToken, f.post, viewerPost.PostID, f.content, f.data},
			{f.owner.SessionToken, viewerPost.PostID, f.post, viewerContent, viewerData},
		} {
			readCtx, trace := readtiming.Start(ctx)
			response, e := l.c.ReadPosts(readCtx, side.bearer, PostQuery{Kind: PostFeed, Limit: 20})
			if e != nil {
				t.Fatal(e)
			}
			if feedContains(response.Value.(posts.CrossChannelFeedPage).Items, side.otherPost) == hidden || trace.Snapshot().Stages["feed.prepare_read"].Calls != 1 {
				t.Fatal("prepared feed lost symmetric block visibility or selected legacy route")
			}
			readCtx, trace = readtiming.Start(ctx)
			response, e = l.c.ReadComments(readCtx, side.bearer, CommentQuery{ID: side.ownPost})
			if e != nil {
				t.Fatal(e)
			}
			want := 1
			if hidden {
				want = 0
			}
			if len(response.Value.(TextCommentPage).Comments) != want || trace.Snapshot().Stages["comments.prepare_read"].Calls != 1 {
				t.Fatal("prepared comments lost symmetric author filtering or selected legacy route")
			}
			readCtx, trace = readtiming.Start(ctx)
			response, e = l.c.ReadPostImages(readCtx, side.bearer, side.otherPost, side.content)
			if hidden {
				postTestError(t, e, "MEDIA_NOT_FOUND", 404)
				if response.Value != nil || !response.ExpiresAt.IsZero() {
					t.Fatal("blocked original released bytes or renewal")
				}
			} else if e != nil || !bytes.Equal(response.Value.(ImageBytes).Bytes, side.data) {
				t.Fatal("unified unblock did not restore original media", e)
			}
			if preparedMediaCalls(trace.Snapshot(), "prepare_read") != 1 {
				t.Fatal("media compatibility test selected a legacy route")
			}
		}
	}
	checkMutualReads(false)
	original := *l.c
	for _, blocked := range []bool{true, false} {
		beforeViewer, beforeOwner := visibility(f.viewer.AccountID), visibility(f.owner.AccountID)
		operation := uuid.NewString()
		input := DMBlockInput{IdentityID: viewerIdentity.String(), ConversationID: conversation.ConversationID, Blocked: blocked}
		var changed DMBlockResult
		finals := 0
		preparedReadPool(t, l, func(n int) {
			finals = n
			if n != 1 {
				return
			}
			// The real block command commits after the feed's RO preparation
			// but before its final caller/Gate locks, on the original pool.
			writeCtx, writeTrace := readtiming.Start(ctx)
			response, e := original.ChangeDMBlock(writeCtx, f.viewer.SessionToken, operation, input)
			if e != nil {
				t.Fatal(e)
			}
			changed = response.Value.(DMBlockResult)
			if changed.State != "APPLIED" || changed.BlockID == "" || response.ServerTime.IsZero() || !response.ExpiresAt.After(response.ServerTime) ||
				writeTrace.Snapshot().Stages["anchor_write"].Calls != 1 || writeTrace.Snapshot().Stages["sql_commit"].Calls != 1 {
				t.Fatal("unified block did not use exactly one final C checkpoint", changed, writeTrace.Snapshot())
			}
		})
		readCtx, trace := readtiming.Start(ctx)
		response, e := l.c.ReadPosts(readCtx, f.viewer.SessionToken, PostQuery{Kind: PostFeed, Limit: 20})
		if e != nil {
			t.Fatal(e)
		}
		stages := trace.Snapshot().Stages
		if finals != 2 || stages["feed.prepare_read"].Calls != 2 || stages["feed.prepared_read_rebuild"].Calls != 1 || stages["feed.anchor_write"].Calls != 1 ||
			feedContains(response.Value.(posts.CrossChannelFeedPage).Items, f.post) == blocked {
			t.Fatal("unified block/unblock failed to invalidate the in-flight prepared feed", blocked, finals, stages)
		}
		if visibility(f.viewer.AccountID) != beforeViewer+1 || visibility(f.owner.AccountID) != beforeOwner+1 {
			t.Fatal("unified block did not advance both private visibility fences")
		}
		wantRows := 0
		if blocked {
			wantRows = 1
		}
		if count(t, l.cp, `SELECT count(*) FROM c_posts.blocked_authors`) != wantRows || count(t, l.cp, `SELECT count(*) FROM c_dm.blocks`) != 0 {
			t.Fatal("unified API left a second or legacy block record")
		}
		replay, e := original.ChangeDMBlock(ctx, f.viewer.SessionToken, operation, input)
		if e != nil || replay.Value.(DMBlockResult).BlockID != changed.BlockID || visibility(f.viewer.AccountID) != beforeViewer+1 || visibility(f.owner.AccountID) != beforeOwner+1 {
			t.Fatal("unified replay advanced the visibility fence twice", e)
		}
		checkMutualReads(blocked)
	}
}
