package authprivacy

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/posts"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/readtiming"
)

func preparedFeedLab(t *testing.T) *lab {
	t.Helper()
	l := newLab(t)
	cfg := enablePostLab(t, l)
	cfg.PreparedFeedReads = true
	var e error
	l.c, e = l.c.WithPosts(context.Background(), cfg)
	if e != nil {
		t.Fatal("prepared feed schema must pass its real enablement check", e)
	}
	return l
}

func preparedFeedPublish(t *testing.T, l *lab, b [32]byte, i uuid.UUID, title string) string {
	t.Helper()
	intent := postTestIntent(t, l, i)
	intent.Title = title
	id := postTestCommand(t, l, b, postTestKey(t), intent).PostID
	postTestPublish(t, l)
	return id
}

func TestFeedPreparedTimelinePostgres(t *testing.T) {
	l := preparedFeedLab(t)
	owner := sessionTestAccount(t, l, "pf_owner")
	reader := sessionTestAccount(t, l, "pf_reader")
	i := identityChange(t, l.c, owner.SessionToken, "CREATE", uuid.Nil, "时间线作者").IdentityID
	old := preparedFeedPublish(t, l, owner.SessionToken, i, "first")
	latest := preparedFeedPublish(t, l, owner.SessionToken, i, "latest")
	ctx, trace := readtiming.Start(context.Background())
	r, e := l.c.ReadPosts(ctx, reader.SessionToken, PostQuery{Kind: PostFeed, Limit: 1})
	if e != nil {
		t.Fatal(e)
	}
	page := r.Value.(posts.CrossChannelFeedPage)
	if len(page.Items) != 1 || page.Items[0].PostID != latest || page.NextCursor == nil {
		t.Fatal("prepared first page", page)
	}
	if trace.Snapshot().Stages["feed.anchor_write"].Calls != 1 {
		t.Fatal("read did not have one final checkpoint")
	}
	original := *l.c
	calls := 0
	preparedReadPool(t, l, func(n int) {
		calls = n
		if n == 1 {
			preparedFeedPublish(t, &lab{c: &original, cp: l.cp}, owner.SessionToken, i, "after ceiling")
		}
	})
	r, e = l.c.ReadPosts(context.Background(), reader.SessionToken, PostQuery{Kind: PostFeed, Limit: 1, Cursor: *page.NextCursor})
	if e != nil {
		t.Fatal(e)
	}
	last := r.Value.(posts.CrossChannelFeedPage)
	if calls != 1 || len(last.Items) != 1 || last.Items[0].PostID != old || last.NextCursor != nil {
		t.Fatal("fixed ceiling changed or publication forced a rebuild", last, calls)
	}
	l.c.posts.preparedFeedReads = false
	if _, e = l.c.ReadPosts(context.Background(), reader.SessionToken, PostQuery{Kind: PostFeed, Limit: 1, Cursor: *page.NextCursor}); e != nil {
		t.Fatal("old path could not resume prepared cursor", e)
	}
}

func TestFeedPreparedCandidateInterleavingPostgres(t *testing.T) {
	for _, mode := range []string{"lookahead", "new", "hide", "block", "filters", "profile", "twice", "missing_scope", "missing_projection", "missing_identity", "missing_owner", "missing_viewer"} {
		t.Run(mode, func(t *testing.T) {
			l := preparedFeedLab(t)
			owner := sessionTestAccount(t, l, "pf_race_owner")
			reader := sessionTestAccount(t, l, "pf_race_reader")
			i := identityChange(t, l.c, owner.SessionToken, "CREATE", uuid.Nil, "交错作者").IdentityID
			old := preparedFeedPublish(t, l, owner.SessionToken, i, "lookahead")
			latest := preparedFeedPublish(t, l, owner.SessionToken, i, "selected")
			original := *l.c
			token := sha256.Sum256(reader.SessionToken[:])
			mustExec(t, l.cp, `UPDATE c_auth.sessions SET expires_at=last_activity_at+interval '1 hour' WHERE token_digest=$1`, token[:])
			before := preparedReadState(t, l, token)
			calls := 0
			newID := ""
			preparedReadPool(t, l, func(n int) {
				calls = n
				if n > 1 && mode != "twice" {
					return
				}
				switch mode {
				case "lookahead":
					postTestCommand(t, &lab{c: &original, cp: l.cp}, owner.SessionToken, postTestKey(t), posts.Intent{Operation: posts.DeletePost, PostID: old})
				case "new":
					newID = preparedFeedPublish(t, &lab{c: &original, cp: l.cp}, owner.SessionToken, i, "new candidate")
				case "hide":
					state, e := original.ReadHiddenPost(context.Background(), reader.SessionToken, latest)
					if e != nil {
						t.Fatal(e)
					}
					if _, e = original.SetHiddenPost(context.Background(), reader.SessionToken, latest, true, state.InteractionVersion); e != nil {
						t.Fatal(e)
					}
				case "block":
					if _, e := original.BlockPostAuthor(context.Background(), reader.SessionToken, latest); e != nil {
						t.Fatal(e)
					}
				case "filters":
					var ch string
					if e := l.cp.QueryRow(context.Background(), `SELECT channel_id::text FROM c_posts.posts WHERE post_id=$1`, latest).Scan(&ch); e != nil {
						t.Fatal(e)
					}
					if _, e := original.ReplaceFeedPreferences(context.Background(), reader.SessionToken, posts.FeedPreferences{BlockedChannelIDs: []string{ch}}); e != nil {
						t.Fatal(e)
					}
				case "profile":
					identityChange(t, &original, owner.SessionToken, "RENAME", i, "更新作者")
				case "twice":
					mustExec(t, l.cp, `UPDATE c_posts.posts SET moderation_hidden=NOT moderation_hidden WHERE post_id=$1`, latest)
				case "missing_scope":
					mustExec(t, l.cp, `DELETE FROM c_posts.read_feed_scope_versions WHERE channel_id=(SELECT channel_id FROM c_posts.posts WHERE post_id=$1)`, latest)
				case "missing_projection":
					mustExec(t, l.cp, `DELETE FROM c_posts.read_feed_post_versions WHERE post_id=$1`, latest)
				case "missing_identity":
					mustExec(t, l.cp, `DELETE FROM c_posts.read_identity_versions WHERE identity_id=$1`, i)
				case "missing_owner":
					mustExec(t, l.cp, `DELETE FROM c_posts.read_account_versions WHERE account_id=$1`, owner.AccountID)
				case "missing_viewer":
					mustExec(t, l.cp, `DELETE FROM c_posts.read_account_versions WHERE account_id=$1`, reader.AccountID)
				}
			})
			r, e := l.c.ReadPosts(context.Background(), reader.SessionToken, PostQuery{Kind: PostFeed, Limit: 1})
			if mode == "twice" {
				postTestError(t, e, "SERVICE_UNAVAILABLE", 503)
				after := preparedReadState(t, l, token)
				if calls != 2 || !after.activity.Equal(before.activity) || !after.expiry.Equal(before.expiry) {
					t.Fatal("normal conflicts retried or renewed")
				}
				assertPreparedReadAnchorMatches(t, l, after)
				return
			}
			if mode == "missing_scope" || mode == "missing_projection" || mode == "missing_identity" || mode == "missing_owner" || mode == "missing_viewer" {
				if !errors.Is(e, ErrAuthorizationUnavailable) || r.Value != nil {
					t.Fatal("missing facts failed open", e)
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			page := r.Value.(posts.CrossChannelFeedPage)
			if calls != 2 {
				t.Fatal("candidate change did not rebuild", calls)
			}
			switch mode {
			case "lookahead":
				if len(page.Items) != 1 || page.Items[0].PostID != latest || page.NextCursor != nil {
					t.Fatal("removed lookahead left a false next page")
				}
			case "new":
				if page.Items[0].PostID != newID {
					t.Fatal("new candidate lost")
				}
			case "hide":
				if page.Items[0].PostID != old {
					t.Fatal("hidden candidate escaped")
				}
			case "block", "filters":
				if len(page.Items) != 0 || page.NextCursor != nil {
					t.Fatal("viewer filtering escaped")
				}
			case "profile":
				if page.Items[0].Author.Nickname != "更新作者" {
					t.Fatal("old author escaped")
				}
			}
		})
	}
}

func TestFeedPreparedLifecyclePostgres(t *testing.T) {
	for _, mode := range []string{"cancel", "generation", "revoke", "commit_ack_loss"} {
		t.Run(mode, func(t *testing.T) {
			l := preparedFeedLab(t)
			reader := sessionTestAccount(t, l, "pf_lifecycle")
			i := identityChange(t, l.c, reader.SessionToken, "CREATE", uuid.Nil, "原生读取").IdentityID
			preparedFeedPublish(t, l, reader.SessionToken, i, "private first")
			preparedFeedPublish(t, l, reader.SessionToken, i, "private next")
			token := sha256.Sum256(reader.SessionToken[:])
			mustExec(t, l.cp, `UPDATE c_auth.sessions SET expires_at=last_activity_at+interval '1 hour' WHERE token_digest=$1`, token[:])
			before := preparedReadState(t, l, token)
			original := *l.c
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "commit_ack_loss" {
				pool, proxy := newCommitACKProxyPool(t, l.cp)
				l.c.pool = pool
				proxy.armed.Store(true)
			} else {
				preparedReadPool(t, l, func(n int) {
					if n != 1 {
						return
					}
					switch mode {
					case "cancel":
						cancel()
					case "generation":
						mustExec(t, l.cp, `UPDATE c_auth.accounts SET session_generation=session_generation+1 WHERE account_id=$1`, reader.AccountID)
					case "revoke":
						if e := original.RevokeSession(context.Background(), reader.RevokeSecret); e != nil {
							t.Fatal(e)
						}
					}
				})
			}
			r, e := l.c.ReadPosts(ctx, reader.SessionToken, PostQuery{Kind: PostFeed, Limit: 1})
			if e == nil || r.Value != nil || !r.ExpiresAt.IsZero() {
				t.Fatal("denied or unknown read released a page", e)
			}
			after := preparedReadState(t, l, token)
			assertPreparedReadAnchorMatches(t, l, after)
			if mode == "commit_ack_loss" {
				if !after.highwater.After(before.highwater) || !after.expiry.After(before.expiry) {
					t.Fatal("fixture did not lose a durable commit ACK")
				}
			} else if !after.expiry.Equal(before.expiry) || !after.activity.Equal(before.activity) {
				t.Fatal("denied read renewed")
			}
		})
	}
}
