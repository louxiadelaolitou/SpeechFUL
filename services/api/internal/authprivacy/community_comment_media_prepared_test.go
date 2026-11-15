package authprivacy

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/comments"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/readtiming"
)

type preparedCommentMediaFixture struct {
	preparedMediaFixture
	author        SignupResult
	identity      uuid.UUID
	root, comment string
	image         string
}

func newPreparedCommentMediaFixture(t *testing.T) preparedCommentMediaFixture {
	t.Helper()
	f := newPreparedMediaFixture(t)
	cfg := CommentConfig{}
	cfg.KeyKey, _ = random32()
	cfg.FingerprintKey, _ = random32()
	var err error
	f.l.c, err = f.l.c.WithComments(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	author := sessionTestAccount(t, f.l, "media_comment_author")
	identity := identityChange(t, f.l.c, author.SessionToken, "CREATE", uuid.Nil, "评论媒体作者").IdentityID
	root := preparedComment(t, f.l, author.SessionToken, f.post, identity, "")
	selection, data := imageFixture(t, "png")
	intent := comments.Intent{Operation: comments.Create, TaskID: uuid.NewString(), PostID: f.post, IdentityID: identity.String(), RootCommentID: root, TargetCommentID: root, Image: &selection}
	commentTextSend(t, f.l, author.SessionToken, postTestKey(t), intent)
	if _, err = f.l.c.UploadCommentImage(context.Background(), author.SessionToken, intent.TaskID, 1, data, selection.MIME); err != nil {
		t.Fatal(err)
	}
	commentTextPublish(t, f.l)
	own := commentTextOwn(t, f.l, author.SessionToken, intent.TaskID)
	var content string
	if err = f.l.cp.QueryRow(context.Background(), `SELECT content_version_id::text FROM c_comments.images WHERE task_id=$1 AND version=1`, intent.TaskID).Scan(&content); err != nil {
		t.Fatal(err)
	}
	f.data = data
	return preparedCommentMediaFixture{preparedMediaFixture: f, author: author, identity: identity, root: root, comment: *own.PublishedCommentID, image: content}
}

func TestPreparedCommentMediaOriginalAndFallbackPostgres(t *testing.T) {
	f := newPreparedCommentMediaFixture(t)
	for _, enabled := range []bool{true, false} {
		f.config.PreparedMediaReads = enabled
		var err error
		f.l.c, err = f.l.c.WithPosts(context.Background(), f.config)
		if err != nil {
			t.Fatal(err)
		}
		ctx, trace := readtiming.Start(context.Background())
		r, err := f.l.c.ReadCommentImage(ctx, f.viewer.SessionToken, f.post, f.comment, f.image)
		if err != nil || !bytes.Equal(r.Value.(ImageBytes).Bytes, f.data) || r.Value.(ImageBytes).MIME != "image/png" {
			t.Fatal("protected comment original changed or was cleared", err)
		}
		wantPrepare := int64(0)
		if enabled {
			wantPrepare = 1
		}
		if preparedMediaCalls(trace.Snapshot(), "prepare_read") != wantPrepare || preparedMediaCalls(trace.Snapshot(), "anchor_write") != 1 {
			t.Fatal("comment original used the wrong route/checkpoint count")
		}
	}
}

func TestPreparedCommentMediaRacesPostgres(t *testing.T) {
	for _, mode := range []string{"root_delete", "root_hide", "post_block", "author_block", "reverse_block", "session_generation", "missing_session", "cancel", "commit_ack_loss"} {
		t.Run(mode, func(t *testing.T) {
			f := newPreparedCommentMediaFixture(t)
			ctx, trace := readtiming.Start(context.Background())
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()
			var anchor string
			if mode == "author_block" {
				anchor = commentTextPost(t, f.l, f.author.SessionToken, f.identity)
			} else if mode == "reverse_block" {
				identity := identityChange(t, f.l.c, f.viewer.SessionToken, "CREATE", uuid.Nil, "读者锚点").IdentityID
				anchor = commentTextPost(t, f.l, f.viewer.SessionToken, identity)
			}
			var proxy *commitACKProxy
			if mode == "commit_ack_loss" {
				f.l.c.pool, proxy = newCommitACKProxyPool(t, f.l.cp)
				proxy.armed.Store(true)
			} else {
				preparedReadPool(t, f.l, func(n int) {
					if n != 1 {
						return
					}
					switch mode {
					case "root_delete":
						commentTextSend(t, f.l, f.author.SessionToken, postTestKey(t), comments.Intent{Operation: comments.DeleteComment, CommentID: f.root})
					case "root_hide":
						mustExec(t, f.l.cp, `UPDATE c_comments.tasks SET moderation_hidden=true WHERE comment_id=$1`, f.root)
					case "post_block", "author_block":
						post := f.post
						if anchor != "" {
							post = anchor
						}
						if _, err := f.l.c.BlockPostAuthor(context.Background(), f.viewer.SessionToken, post); err != nil {
							t.Fatal(err)
						}
					case "reverse_block":
						if _, err := f.l.c.BlockPostAuthor(context.Background(), f.author.SessionToken, anchor); err != nil {
							t.Fatal(err)
						}
					case "session_generation", "missing_session":
						mustExec(t, f.l.cp, `UPDATE c_auth.accounts SET session_generation=session_generation+1 WHERE account_id=$1`, f.viewer.AccountID)
					case "cancel":
						cancel()
					}
				})
			}
			post := f.post
			if mode == "missing_session" {
				post = uuid.NewString()
			}
			r, err := f.l.c.ReadCommentImage(ctx, f.viewer.SessionToken, post, f.comment, f.image)
			if err == nil || r.Value != nil || !r.ExpiresAt.IsZero() {
				t.Fatal("stale comment image released bytes or renewal", err)
			}
			switch mode {
			case "session_generation", "missing_session":
				if !errors.Is(err, ErrSessionInvalid) {
					t.Fatal("private image outcome bypassed final session check", err)
				}
			case "commit_ack_loss":
				if !errors.Is(err, ErrAuthorizationUnavailable) {
					t.Fatal(err)
				}
				if preparedMediaCalls(trace.Snapshot(), "prepare_read") != 1 || preparedMediaCalls(trace.Snapshot(), "anchor_write") != 1 || preparedMediaCalls(trace.Snapshot(), "sql_commit") != 1 {
					t.Fatal("ACK loss did not reach the one native final checkpoint")
				}
				select {
				case <-proxy.dropped:
				default:
					t.Fatal("test did not drop a real COMMIT response")
				}
			case "cancel":
			default:
				postTestError(t, err, "MEDIA_NOT_FOUND", 404)
				if preparedMediaCalls(trace.Snapshot(), "prepared_read_rebuild") != 1 {
					t.Fatal("changed comment image did not rebuild its proof once")
				}
			}
		})
	}
}
