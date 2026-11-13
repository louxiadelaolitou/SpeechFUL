package authprivacy

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/comments"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/posts"
)

// An account block covers every identity of both accounts: content becomes
// mutually invisible, DM sending pauses, and the private block row keeps only
// the public author snapshot.
func TestAuthorBlockPostgres(t *testing.T) {
	l := commentTextLab(t)
	ctx := context.Background()
	blocker := sessionTestAccount(t, l, "author_block_a")
	target := sessionTestAccount(t, l, "author_block_b")
	stranger := sessionTestAccount(t, l, "author_block_c")
	blockerIdentity := identityChange(t, l.c, blocker.SessionToken, "CREATE", uuid.Nil, "拉黑者").IdentityID
	targetIdentity := identityChange(t, l.c, target.SessionToken, "CREATE", uuid.Nil, "被拉黑者").IdentityID

	blockerPost := commentTextPost(t, l, blocker.SessionToken, blockerIdentity)
	targetPost := commentTextPost(t, l, target.SessionToken, targetIdentity)

	comment := comments.Intent{Operation: comments.Create, TaskID: uuid.NewString(), PostID: blockerPost, IdentityID: targetIdentity.String(), Body: "被拉黑账号的评论"}
	commentTextSend(t, l, target.SessionToken, postTestKey(t), comment)
	commentTextPublish(t, l)

	dm, err := l.c.WithDirectMessages(ctx)
	if err != nil {
		t.Fatal(err)
	}
	sendText := func(token [32]byte, in DMTextIntent) DMReceipt {
		t.Helper()
		response, sendErr := dm.SendDMText(ctx, token, uuid.NewString(), in)
		if sendErr != nil {
			t.Fatal(sendErr)
		}
		return response.Value.(DMReceipt)
	}
	first := sendText(blocker.SessionToken, DMTextIntent{IdentityID: blockerIdentity.String(), PeerID: targetIdentity.String(), SourcePostID: targetPost, Text: "拉黑前", Attempt: 1})
	if first.State != "SENT" || first.ConversationID == "" {
		t.Fatal("conversation fixture failed", first)
	}
	reply := sendText(target.SessionToken, DMTextIntent{IdentityID: targetIdentity.String(), PeerID: blockerIdentity.String(), ConversationID: first.ConversationID, Text: "收到", Attempt: 1})
	if reply.State != "SENT" {
		t.Fatal("conversation reply failed", reply)
	}

	// Baseline: both accounts see each other and the comment.
	postTestRead(t, l, blocker.SessionToken, PostQuery{Kind: PostDetail, ID: targetPost})
	before := postTestRead(t, l, blocker.SessionToken, PostQuery{Kind: PostFeed}).(posts.CrossChannelFeedPage)
	if !feedContains(before.Items, targetPost) {
		t.Fatal("feed fixture missing before block")
	}
	commentsBefore, err := l.c.ReadComments(ctx, blocker.SessionToken, CommentQuery{ID: blockerPost})
	if err != nil || len(commentsBefore.Value.(TextCommentPage).Comments) != 1 {
		t.Fatalf("comment fixture failed: %v", err)
	}

	// Self blocks and missing anchors are rejected without touching state.
	_, err = l.c.BlockPostAuthor(ctx, blocker.SessionToken, blockerPost)
	postTestError(t, err, "SELF_BLOCK_REJECTED", 409)
	_, err = l.c.BlockPostAuthor(ctx, blocker.SessionToken, uuid.NewString())
	postTestError(t, err, "POST_NOT_FOUND", 404)

	response, err := l.c.BlockPostAuthor(ctx, blocker.SessionToken, targetPost)
	if err != nil {
		t.Fatal(err)
	}
	receipt := response.Value.(posts.AuthorBlockReceipt)
	if receipt.State != "BLOCKED" || receipt.Author.Nickname != "被拉黑者" || receipt.Author.State != "ACTIVE" || receipt.BlockID == "" || receipt.BlockedAt.IsZero() {
		t.Fatalf("wrong block receipt: %#v", receipt)
	}
	encoded, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"identityId", "accountId", "email", "slot"} {
		if strings.Contains(string(encoded), `"`+secret+`"`) {
			t.Fatalf("block receipt leaked %s", secret)
		}
	}

	// Repeating the call after the author became invisible is idempotent and
	// returns the original row instead of probing the hidden post.
	repeated, err := l.c.BlockPostAuthor(ctx, blocker.SessionToken, targetPost)
	if err != nil {
		t.Fatal(err)
	}
	if repeated.Value.(posts.AuthorBlockReceipt).BlockID != receipt.BlockID {
		t.Fatal("retry created another block row")
	}

	list, err := l.c.AuthorBlocks(ctx, blocker.SessionToken)
	if err != nil {
		t.Fatal(err)
	}
	items := list.Value.(posts.BlockedAuthorPage).Items
	if len(items) != 1 || items[0].BlockID != receipt.BlockID || items[0].Author.Nickname != "被拉黑者" {
		t.Fatalf("wrong blocked author list: %#v", items)
	}
	encoded, err = json.Marshal(items)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"identityId", "accountId", "email", "slot"} {
		if strings.Contains(string(encoded), `"`+secret+`"`) {
			t.Fatalf("blocked author list leaked %s", secret)
		}
	}
	for _, other := range []SignupResult{target, stranger} {
		page, listErr := l.c.AuthorBlocks(ctx, other.SessionToken)
		if listErr != nil || len(page.Value.(posts.BlockedAuthorPage).Items) != 0 {
			t.Fatalf("private block list crossed accounts: %v", listErr)
		}
	}

	// Every content surface is symmetric for both accounts.
	for _, probe := range []struct {
		token [32]byte
		post  string
	}{
		{blocker.SessionToken, targetPost},
		{target.SessionToken, blockerPost},
	} {
		_, err = l.c.ReadPosts(ctx, probe.token, PostQuery{Kind: PostDetail, ID: probe.post})
		postTestError(t, err, "POST_NOT_FOUND", 404)
		_, err = l.c.ReadPosts(ctx, probe.token, PostQuery{Kind: PostAuthor, ID: probe.post})
		postTestError(t, err, "POST_NOT_FOUND", 404)
	}
	blockerFeed := postTestRead(t, l, blocker.SessionToken, PostQuery{Kind: PostFeed}).(posts.CrossChannelFeedPage)
	if feedContains(blockerFeed.Items, targetPost) || !feedContains(blockerFeed.Items, blockerPost) {
		t.Fatalf("blocker feed not filtered: %#v", blockerFeed.Items)
	}
	targetFeed := postTestRead(t, l, target.SessionToken, PostQuery{Kind: PostFeed}).(posts.CrossChannelFeedPage)
	if feedContains(targetFeed.Items, blockerPost) || !feedContains(targetFeed.Items, targetPost) {
		t.Fatalf("target feed not filtered: %#v", targetFeed.Items)
	}
	commentsAfter, err := l.c.ReadComments(ctx, blocker.SessionToken, CommentQuery{ID: blockerPost})
	if err != nil || len(commentsAfter.Value.(TextCommentPage).Comments) != 0 {
		t.Fatalf("blocked author comment leaked: %v", err)
	}
	strangerFeed := postTestRead(t, l, stranger.SessionToken, PostQuery{Kind: PostFeed}).(posts.CrossChannelFeedPage)
	if !feedContains(strangerFeed.Items, blockerPost) || !feedContains(strangerFeed.Items, targetPost) {
		t.Fatal("block crossed unrelated accounts")
	}
	strangerComments, err := l.c.ReadComments(ctx, stranger.SessionToken, CommentQuery{ID: blockerPost})
	if err != nil || len(strangerComments.Value.(TextCommentPage).Comments) != 1 {
		t.Fatalf("block crossed unrelated comments: %v", err)
	}

	// Both feed cursors are invalidated immediately.
	revisions := func(account uuid.UUID) int64 {
		t.Helper()
		var revision int64
		if queryErr := l.cp.QueryRow(ctx, `SELECT revision FROM c_posts.feed_preferences WHERE account_id=$1`, account).Scan(&revision); queryErr != nil {
			t.Fatal(queryErr)
		}
		return revision
	}
	blockerRevision := revisions(blocker.AccountID)
	targetRevision := revisions(target.AccountID)
	if blockerRevision < 1 || targetRevision < 1 {
		t.Fatal("block did not invalidate both feed cursors", blockerRevision, targetRevision)
	}

	// DM sending pauses in both directions while history stays readable.
	blockedSend := sendText(blocker.SessionToken, DMTextIntent{IdentityID: blockerIdentity.String(), PeerID: targetIdentity.String(), ConversationID: first.ConversationID, Text: "拉黑后", Attempt: 1})
	if blockedSend.State != "FAILED" || blockedSend.ErrorCode != "CONTACT_UNAVAILABLE" {
		t.Fatalf("blocker send was not rejected: %#v", blockedSend)
	}
	blockedReply := sendText(target.SessionToken, DMTextIntent{IdentityID: targetIdentity.String(), PeerID: blockerIdentity.String(), ConversationID: first.ConversationID, Text: "反向", Attempt: 1})
	if blockedReply.State != "FAILED" || blockedReply.ErrorCode != "CONTACT_UNAVAILABLE" {
		t.Fatalf("target send was not rejected: %#v", blockedReply)
	}
	pending, err := dm.PullDMChat(ctx, target.SessionToken)
	if err != nil {
		t.Fatal(err)
	}
	history := pending.Value.(DMChatPending).Items
	if len(history) == 0 || history[0].MessageID != first.MessageID {
		t.Fatalf("dm history disappeared after block: %#v", history)
	}
	directory, err := dm.ReadDMChatDirectory(ctx, target.SessionToken, targetIdentity.String(), "")
	if err != nil {
		t.Fatal(err)
	}
	conversations := directory.Value.(DMChatDirectory).Items
	if len(conversations) != 1 || conversations[0].CanSend {
		t.Fatalf("blocked conversation is still writable: %#v", conversations)
	}

	// A record owned by another account is indistinguishable from a missing
	// one, and unblocking restores content in both directions.
	foreign, err := l.c.UnblockPostAuthor(ctx, stranger.SessionToken, receipt.BlockID)
	if err != nil {
		t.Fatal(err)
	}
	if removal := foreign.Value.(posts.AuthorBlockRemoval); removal.State != "ABSENT" || removal.BlockID != receipt.BlockID {
		t.Fatalf("foreign unblock leaked ownership: %#v", removal)
	}
	removed, err := l.c.UnblockPostAuthor(ctx, blocker.SessionToken, receipt.BlockID)
	if err != nil {
		t.Fatal(err)
	}
	if removal := removed.Value.(posts.AuthorBlockRemoval); removal.State != "REMOVED" || removal.BlockID != receipt.BlockID {
		t.Fatalf("wrong unblock receipt: %#v", removal)
	}
	absent, err := l.c.UnblockPostAuthor(ctx, blocker.SessionToken, receipt.BlockID)
	if err != nil {
		t.Fatal(err)
	}
	if removal := absent.Value.(posts.AuthorBlockRemoval); removal.State != "ABSENT" {
		t.Fatalf("repeated unblock is not idempotent: %#v", removal)
	}
	if revisions(blocker.AccountID) <= blockerRevision || revisions(target.AccountID) <= targetRevision {
		t.Fatal("unblock did not invalidate both feed cursors")
	}
	postTestRead(t, l, blocker.SessionToken, PostQuery{Kind: PostDetail, ID: targetPost})
	postTestRead(t, l, target.SessionToken, PostQuery{Kind: PostDetail, ID: blockerPost})
	commentsRestored, err := l.c.ReadComments(ctx, blocker.SessionToken, CommentQuery{ID: blockerPost})
	if err != nil || len(commentsRestored.Value.(TextCommentPage).Comments) != 1 {
		t.Fatalf("comment did not return after unblock: %v", err)
	}
	restored := sendText(blocker.SessionToken, DMTextIntent{IdentityID: blockerIdentity.String(), PeerID: targetIdentity.String(), ConversationID: first.ConversationID, Text: "解除后", Attempt: 1})
	if restored.State != "SENT" {
		t.Fatalf("dm send did not resume: %#v", restored)
	}
}

// A target account's own request locks its account row before waiting for the
// Gate. The block transaction must take the target account lock before the
// Gate row as well; otherwise the two transactions form a lock cycle and
// PostgreSQL aborts one of them with SQLSTATE 40P01.
func TestAuthorBlockDoesNotDeadlockWithTargetRequestPostgres(t *testing.T) {
	l := commentTextLab(t)
	ctx := context.Background()
	blocker := sessionTestAccount(t, l, "author_block_lock_a")
	target := sessionTestAccount(t, l, "author_block_lock_b")
	targetIdentity := identityChange(t, l.c, target.SessionToken, "CREATE", uuid.Nil, "锁目标").IdentityID
	targetPost := commentTextPost(t, l, target.SessionToken, targetIdentity)

	holding := make(chan struct{})
	targetDone := make(chan error, 1)
	go func() {
		tx, err := l.cp.Begin(ctx)
		if err != nil {
			targetDone <- err
			return
		}
		defer tx.Rollback(ctx)
		if _, err := tx.Exec(ctx, `SELECT 1 FROM c_auth.accounts WHERE account_id=$1 FOR UPDATE`, target.AccountID); err != nil {
			targetDone <- err
			return
		}
		close(holding)
		// Give the block request time to reach the target account row.
		time.Sleep(500 * time.Millisecond)
		// The target's final request then waits on the Gate exactly like
		// withPostSessionAt does after validating the session.
		if _, err := tx.Exec(ctx, `SELECT 1 FROM c_auth.authorization_gate WHERE singleton_id=1 FOR UPDATE`); err != nil {
			targetDone <- err
			return
		}
		targetDone <- tx.Commit(ctx)
	}()

	<-holding
	response, err := l.c.BlockPostAuthor(ctx, blocker.SessionToken, targetPost)
	if err != nil {
		t.Fatalf("block failed while the target account was active: %v", err)
	}
	if err := <-targetDone; err != nil {
		t.Fatalf("target account request failed during block: %v", err)
	}
	if receipt := response.Value.(posts.AuthorBlockReceipt); receipt.State != "BLOCKED" {
		t.Fatalf("unexpected block receipt: %#v", receipt)
	}
}

// The floor's reply count must use the same block-filtered projection as the
// reply page; otherwise it advertises replies that expanding returns zero.
func TestAuthorBlockReplyCountPostgres(t *testing.T) {
	l := commentTextLab(t)
	ctx := context.Background()
	author := sessionTestAccount(t, l, "reply_count_author")
	blocked := sessionTestAccount(t, l, "reply_count_blocked")
	authorIdentity := identityChange(t, l.c, author.SessionToken, "CREATE", uuid.Nil, "楼层作者").IdentityID
	blockedIdentity := identityChange(t, l.c, blocked.SessionToken, "CREATE", uuid.Nil, "回复者").IdentityID
	post := commentTextPost(t, l, author.SessionToken, authorIdentity)
	blockedPost := commentTextPost(t, l, blocked.SessionToken, blockedIdentity)

	root := comments.Intent{Operation: comments.Create, TaskID: uuid.NewString(), PostID: post, IdentityID: authorIdentity.String(), Body: "根评论"}
	commentTextSend(t, l, author.SessionToken, postTestKey(t), root)
	commentTextPublish(t, l)
	published := commentTextOwn(t, l, author.SessionToken, root.TaskID)
	if published.PublishedCommentID == nil {
		t.Fatal("root comment was not published")
	}
	reply := comments.Intent{Operation: comments.Create, TaskID: uuid.NewString(), PostID: post, IdentityID: blockedIdentity.String(), RootCommentID: *published.PublishedCommentID, TargetCommentID: *published.PublishedCommentID, Body: "将被隐藏的回复"}
	commentTextSend(t, l, blocked.SessionToken, postTestKey(t), reply)
	commentTextPublish(t, l)

	read := func(token [32]byte) TextCommentPage {
		t.Helper()
		response, err := l.c.ReadComments(ctx, token, CommentQuery{ID: post})
		if err != nil {
			t.Fatal(err)
		}
		return response.Value.(TextCommentPage)
	}
	before := read(author.SessionToken)
	if len(before.Comments) != 1 || before.Comments[0].ReplyCount != 1 {
		t.Fatalf("baseline reply count: %#v", before.Comments)
	}
	if _, err := l.c.BlockPostAuthor(ctx, author.SessionToken, blockedPost); err != nil {
		t.Fatal(err)
	}
	after := read(author.SessionToken)
	if len(after.Comments) != 1 || after.Comments[0].ReplyCount != 0 {
		t.Fatalf("blocked reply still counted: %#v", after.Comments)
	}
	if after.TotalCount == nil || *after.TotalCount != 1 {
		t.Fatalf("blocked reply still counted in the post total: %#v", after.TotalCount)
	}
	expanded, err := l.c.ReadComments(ctx, author.SessionToken, CommentQuery{ID: *published.PublishedCommentID, Replies: true})
	if err != nil {
		t.Fatal(err)
	}
	if items := expanded.Value.(TextCommentPage).Comments; len(items) != 0 {
		t.Fatalf("blocked reply still expandable: %#v", items)
	}
}

// A third party's reply to a blocked author keeps its content: the block hides
// the target author projection, not the reply. The contracted response keeps
// the floor and exact target id and only omits replyTo.
func TestAuthorBlockKeepsReplyWithHiddenTargetPostgres(t *testing.T) {
	l := commentTextLab(t)
	ctx := context.Background()
	viewer := sessionTestAccount(t, l, "hidden_target_viewer")
	blocked := sessionTestAccount(t, l, "hidden_target_blocked")
	third := sessionTestAccount(t, l, "hidden_target_third")
	viewerIdentity := identityChange(t, l.c, viewer.SessionToken, "CREATE", uuid.Nil, "楼主").IdentityID
	blockedIdentity := identityChange(t, l.c, blocked.SessionToken, "CREATE", uuid.Nil, "被拉黑").IdentityID
	thirdIdentity := identityChange(t, l.c, third.SessionToken, "CREATE", uuid.Nil, "第三方").IdentityID
	post := commentTextPost(t, l, viewer.SessionToken, viewerIdentity)
	blockedPost := commentTextPost(t, l, blocked.SessionToken, blockedIdentity)

	root := comments.Intent{Operation: comments.Create, TaskID: uuid.NewString(), PostID: post, IdentityID: viewerIdentity.String(), Body: "根评论"}
	commentTextSend(t, l, viewer.SessionToken, postTestKey(t), root)
	commentTextPublish(t, l)
	publishedRoot := commentTextOwn(t, l, viewer.SessionToken, root.TaskID)
	if publishedRoot.PublishedCommentID == nil {
		t.Fatal("root comment was not published")
	}
	blockedReply := comments.Intent{Operation: comments.Create, TaskID: uuid.NewString(), PostID: post, IdentityID: blockedIdentity.String(), RootCommentID: *publishedRoot.PublishedCommentID, TargetCommentID: *publishedRoot.PublishedCommentID, Body: "将被隐藏的回复"}
	commentTextSend(t, l, blocked.SessionToken, postTestKey(t), blockedReply)
	commentTextPublish(t, l)
	publishedBlocked := commentTextOwn(t, l, blocked.SessionToken, blockedReply.TaskID)
	if publishedBlocked.PublishedCommentID == nil {
		t.Fatal("blocked reply was not published")
	}
	thirdReply := comments.Intent{Operation: comments.Create, TaskID: uuid.NewString(), PostID: post, IdentityID: thirdIdentity.String(), RootCommentID: *publishedRoot.PublishedCommentID, TargetCommentID: *publishedBlocked.PublishedCommentID, Body: "第三方的回复"}
	commentTextSend(t, l, third.SessionToken, postTestKey(t), thirdReply)
	commentTextPublish(t, l)

	if _, err := l.c.BlockPostAuthor(ctx, viewer.SessionToken, blockedPost); err != nil {
		t.Fatal(err)
	}
	response, err := l.c.ReadComments(ctx, viewer.SessionToken, CommentQuery{ID: *publishedRoot.PublishedCommentID, Replies: true})
	if err != nil {
		t.Fatal(err)
	}
	items := response.Value.(TextCommentPage).Comments
	if len(items) != 1 || items[0].Body != "第三方的回复" {
		t.Fatalf("third-party reply did not survive its blocked target: %#v", items)
	}
	reply := items[0]
	if reply.RootCommentID == nil || *reply.RootCommentID != *publishedRoot.PublishedCommentID {
		t.Fatalf("reply floor changed: %#v", reply.RootCommentID)
	}
	if reply.TargetCommentID == nil || *reply.TargetCommentID != *publishedBlocked.PublishedCommentID {
		t.Fatalf("reply target changed: %#v", reply.TargetCommentID)
	}
	if reply.ReplyTo != nil {
		t.Fatalf("blocked target author leaked: %#v", reply.ReplyTo)
	}
	if reply.ReplyCount != 0 {
		t.Fatalf("nested reply owns a reply count: %d", reply.ReplyCount)
	}
}

// BlockPostAuthor resolves and pre-locks the anchor before the final session
// check. That early work must not return a business outcome: a revoked token
// has to receive the same session error for its own post, an unknown post and
// another account's post, otherwise it can probe ownership.
func TestAuthorBlockRevokedSessionCannotProbePostOwnershipPostgres(t *testing.T) {
	l := commentTextLab(t)
	ctx := context.Background()
	owner := sessionTestAccount(t, l, "block_probe_owner")
	other := sessionTestAccount(t, l, "block_probe_other")
	ownerIdentity := identityChange(t, l.c, owner.SessionToken, "CREATE", uuid.Nil, "探测本人").IdentityID
	otherIdentity := identityChange(t, l.c, other.SessionToken, "CREATE", uuid.Nil, "探测他人").IdentityID
	ownPost := commentTextPost(t, l, owner.SessionToken, ownerIdentity)
	otherPost := commentTextPost(t, l, other.SessionToken, otherIdentity)

	mustExec(t, l.cp, `UPDATE c_auth.sessions SET revoked_at=clock_timestamp(),revocation_reason='LOGOUT' WHERE account_id=$1`, owner.AccountID)

	for _, probe := range []struct {
		name string
		id   string
	}{
		{"own post", ownPost},
		{"unknown post", uuid.NewString()},
		{"foreign post", otherPost},
	} {
		_, err := l.c.BlockPostAuthor(ctx, owner.SessionToken, probe.id)
		if !errors.Is(err, ErrSessionInvalid) {
			t.Fatalf("revoked session probed %s: %v", probe.name, err)
		}
	}
	if n := count(t, l.cp, `SELECT count(*) FROM c_posts.blocked_authors WHERE account_id=$1`, owner.AccountID); n != 0 {
		t.Fatalf("revoked session wrote %d block rows", n)
	}

	// Live sessions keep the deferred business outcomes.
	_, err := l.c.BlockPostAuthor(ctx, other.SessionToken, otherPost)
	postTestError(t, err, "SELF_BLOCK_REJECTED", 409)
	_, err = l.c.BlockPostAuthor(ctx, other.SessionToken, uuid.NewString())
	postTestError(t, err, "POST_NOT_FOUND", 404)
}

func feedContains(items []posts.FeedPost, id string) bool {
	for _, item := range items {
		if item.PostID == id {
			return true
		}
	}
	return false
}
