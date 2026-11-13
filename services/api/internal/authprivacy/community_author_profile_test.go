package authprivacy

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/posts"
)

// 作者主页只以来源帖子解析身份；游标绑定作者、查看者与分页位置。
func TestPostAuthorProfilePostgres(t *testing.T) {
	l := newPostLab(t)
	ctx := context.Background()
	owner := sessionTestAccount(t, l, "author_profile_owner")
	reader := sessionTestAccount(t, l, "author_profile_reader")
	stranger := sessionTestAccount(t, l, "author_profile_stranger")
	identity := identityChange(t, l.c, owner.SessionToken, "CREATE", uuid.Nil, "主页作者").IdentityID
	otherIdentity := identityChange(t, l.c, stranger.SessionToken, "CREATE", uuid.Nil, "其他作者").IdentityID
	rows, err := l.cp.Query(ctx, `SELECT id FROM public.channels ORDER BY display_order LIMIT 2`)
	if err != nil {
		t.Fatal(err)
	}
	var channels []string
	for rows.Next() {
		var channel string
		if err = rows.Scan(&channel); err != nil {
			t.Fatal(err)
		}
		channels = append(channels, channel)
	}
	rows.Close()
	if err = rows.Err(); err != nil || len(channels) != 2 {
		t.Fatalf("need two channels: %v %v", channels, err)
	}
	create := func(bearer [32]byte, id uuid.UUID, channel, title string) posts.CommandResult {
		intent := postTestIntent(t, l, id)
		intent.ChannelID = channel
		intent.Title = title
		return postTestCommand(t, l, bearer, postTestKey(t), intent)
	}
	first := create(owner.SessionToken, identity, channels[0], "第一帖")
	second := create(owner.SessionToken, identity, channels[1], "第二帖")
	foreign := create(stranger.SessionToken, otherIdentity, channels[0], "别人的帖子")
	postTestPublish(t, l)

	page := postTestRead(t, l, reader.SessionToken, PostQuery{Kind: PostAuthor, ID: first.PostID, Limit: 1}).(posts.AuthorProfilePage)
	if page.Author.State != "ACTIVE" || page.Author.Nickname != "主页作者" || page.Author.Avatar != "default-v1" {
		t.Fatalf("wrong public author projection: %#v", page.Author)
	}
	if len(page.Items) != 1 || page.Items[0].PostID != second.PostID || page.NextCursor == nil {
		t.Fatalf("author posts are not newest-first across channels: %#v", page)
	}
	encoded, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"identityId", "accountId", "email", "slot", "isOriginal"} {
		if strings.Contains(string(encoded), `"`+secret+`"`) {
			t.Fatalf("author profile leaked %s", secret)
		}
	}
	next := postTestRead(t, l, reader.SessionToken, PostQuery{Kind: PostAuthor, ID: first.PostID, Limit: 1, Cursor: *page.NextCursor}).(posts.AuthorProfilePage)
	if len(next.Items) != 1 || next.Items[0].PostID != first.PostID || next.NextCursor != nil {
		t.Fatalf("author cursor repeated or skipped posts: %#v", next)
	}
	if _, err = l.c.ReadPosts(ctx, reader.SessionToken, PostQuery{Kind: PostAuthor, ID: foreign.PostID, Limit: 1, Cursor: *page.NextCursor}); !errors.Is(err, posts.ErrCursorInvalid) {
		t.Fatalf("cursor accepted for another author: %v", err)
	}
	if _, err = l.c.ReadPosts(ctx, reader.SessionToken, PostQuery{Kind: PostAuthor, ID: uuid.NewString()}); err == nil {
		t.Fatal("missing anchor post returned an author profile")
	} else {
		postTestError(t, err, "POST_NOT_FOUND", 404)
	}

	initial, err := l.c.ReadHiddenPost(ctx, reader.SessionToken, second.PostID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = l.c.SetHiddenPost(ctx, reader.SessionToken, second.PostID, true, initial.InteractionVersion); err != nil {
		t.Fatal(err)
	}
	readerPage := postTestRead(t, l, reader.SessionToken, PostQuery{Kind: PostAuthor, ID: first.PostID}).(posts.AuthorProfilePage)
	if len(readerPage.Items) != 1 || readerPage.Items[0].PostID != first.PostID {
		t.Fatal("viewer-hidden post leaked on author profile", readerPage)
	}
	strangerPage := postTestRead(t, l, stranger.SessionToken, PostQuery{Kind: PostAuthor, ID: first.PostID}).(posts.AuthorProfilePage)
	if len(strangerPage.Items) != 2 {
		t.Fatal("viewer preference crossed accounts", strangerPage)
	}

	identityChange(t, l.c, owner.SessionToken, "CREATE", uuid.Nil, "备用身份")
	identityChange(t, l.c, owner.SessionToken, "DELETE", identity, "")
	deleted := postTestRead(t, l, reader.SessionToken, PostQuery{Kind: PostAuthor, ID: first.PostID}).(posts.AuthorProfilePage)
	if deleted.Author.State != "DELETED" || deleted.Author.Avatar != "inactive-v1" || deleted.Author.ShortCode == "" {
		t.Fatalf("deleted identity projection regressed: %#v", deleted.Author)
	}
}
