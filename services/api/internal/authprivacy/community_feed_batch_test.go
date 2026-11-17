package authprivacy

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/posts"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/readtiming"
)

type listCountingTx struct {
	pgx.Tx
	queries int
}

func TestFeedBatchNineImagesAndEditedOrderPostgres(t *testing.T) {
	l := preparedFeedLab(t)
	enableImageLab(t, l)
	owner := sessionTestAccount(t, l, "batch_images")
	identity := identityChange(t, l.c, owner.SessionToken, "CREATE", uuid.Nil, "图片作者").IdentityID
	intent := postTestIntent(t, l, identity)
	payloads := [][]byte{}
	for n := 0; n < 3; n++ {
		selection, data := imageFixture(t, []string{"png", "jpeg", "webp"}[n%3])
		intent.Images = append(intent.Images, selection)
		payloads = append(payloads, data)
	}
	accepted := postTestCommand(t, l, owner.SessionToken, postTestKey(t), intent)
	for n, s := range intent.Images {
		grant := imageGrant(t, l, owner.SessionToken, accepted, s.MediaID)
		imageUpload(t, l, accepted, s, grant, payloads[n])
	}
	postTestPublish(t, l)
	compare := func() {
		tx, e := l.cp.Begin(context.Background())
		if e != nil {
			t.Fatal(e)
		}
		defer tx.Rollback(context.Background())
		q := PostQuery{Kind: PostFeed, Limit: 20, ImageSizes: true}
		a, e := l.c.listPublicPostsLegacyCaptured(context.Background(), tx, owner.AccountID, q, time.Now().UTC(), nil)
		if e != nil {
			t.Fatal(e)
		}
		counter := &listCountingTx{Tx: tx}
		b, e := l.c.listPublicPostsBatchCaptured(context.Background(), counter, owner.AccountID, q, time.Now().UTC(), nil)
		if e != nil {
			t.Fatal(e)
		}
		if !reflect.DeepEqual(comparableList(t, a), comparableList(t, b)) || counter.queries > 4 {
			t.Fatal("image geometry mismatch or query growth", counter.queries)
		}
		if e = tx.Rollback(context.Background()); e != nil {
			t.Fatal(e)
		}
		native := checkNativePreparedProjection(t, l, owner.SessionToken, 20, counter.queries)
		if !reflect.DeepEqual(comparableList(t, a), comparableList(t, native)) {
			t.Fatal("native prepared geometry changed")
		}
	}
	compare()
	var imageIDs []string
	rows, e := l.cp.Query(context.Background(), `SELECT content_version_id::text FROM c_posts.images WHERE task_id=$1 ORDER BY position DESC`, accepted.TaskID)
	if e != nil {
		t.Fatal(e)
	}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			t.Fatal(e)
		}
		imageIDs = append(imageIDs, id)
	}
	rows.Close()
	// Current creation is capped at three. Reconstruct a retained nine-image
	// edited post with six fixture media rows; runtime creation remains capped.
	for n := 0; n < 6; n++ {
		id := uuid.NewString()
		mustExec(t, l.cp, `INSERT INTO c_posts.post_edit_media(post_id,content_version_id,staged_at,payload,digest,mime_type,byte_length,width,height,display_width,display_height,frames)
 SELECT $1,$2,p.published_at,m.payload,m.digest,m.mime_type,m.byte_length,m.width,m.height,m.display_width,m.display_height,m.frames FROM c_posts.posts p JOIN c_posts.publication_tasks t USING(post_id) JOIN c_posts.images m USING(task_id) WHERE p.post_id=$1 ORDER BY position LIMIT 1`, accepted.PostID, id)
		imageIDs = append(imageIDs, id)
	}
	mustExec(t, l.cp, `UPDATE c_posts.posts SET edit_revision=gen_random_uuid(),edited_at=published_at,edited_title='重排',edited_body='图片顺序',edited_image_ids=$2::uuid[] WHERE post_id=$1`, accepted.PostID, imageIDs)
	compare()
	mustExec(t, l.cp, `UPDATE c_posts.posts SET edit_revision=gen_random_uuid(),edited_image_ids='{}' WHERE post_id=$1`, accepted.PostID)
	compare()
}

func TestFeedBatchTimingDiagnosticPostgres(t *testing.T) {
	if os.Getenv("REQ002_PROFILE") != "1" {
		t.Skip("optional diagnostic, not endpoint acceptance")
	}
	l := preparedFeedLab(t)
	owner := sessionTestAccount(t, l, "profile_owner")
	reader := sessionTestAccount(t, l, "profile_reader")
	identity := identityChange(t, l.c, owner.SessionToken, "CREATE", uuid.Nil, "诊断作者").IdentityID
	for n := 0; n < 20; n++ {
		postTestCommand(t, l, owner.SessionToken, postTestKey(t), postTestIntent(t, l, identity))
	}
	postTestPublish(t, l)
	for _, batch := range []bool{false, true} {
		l.c.posts.batchListReads = batch
		totals := map[string]int64{}
		var elapsed int64
		for n := 0; n < 20; n++ {
			ctx, trace := readtiming.Start(context.Background())
			started := time.Now()
			if _, e := l.c.ReadPosts(ctx, reader.SessionToken, PostQuery{Kind: PostFeed, Limit: 5, ImageSizes: true}); e != nil {
				t.Fatal(e)
			}
			elapsed += time.Since(started).Nanoseconds()
			for k, v := range trace.Snapshot().Stages {
				totals[k] += v.Nanoseconds
			}
		}
		t.Logf("batch=%v mean total %.3fms", batch, float64(elapsed)/20e6)
		for k, v := range totals {
			t.Logf("batch=%v %s mean %.3fms", batch, k, float64(v)/20e6)
		}
	}
}

func TestFeedIncrementalRefreshInterleavingPostgres(t *testing.T) {
	for _, mode := range []string{"lookahead", "insert", "filters", "twice"} {
		t.Run(mode, func(t *testing.T) {
			l := preparedFeedLab(t)
			owner := sessionTestAccount(t, l, "refresh_race_owner")
			reader := sessionTestAccount(t, l, "refresh_race_reader")
			identity := identityChange(t, l.c, owner.SessionToken, "CREATE", uuid.Nil, "刷新作者").IdentityID
			preparedFeedPublish(t, l, owner.SessionToken, identity, "old")
			first := postTestRead(t, l, reader.SessionToken, PostQuery{Kind: PostFeed, Incremental: true, Limit: 10}).(posts.IncrementalFeedPage)
			newer := []string{}
			for n := 0; n < 7; n++ {
				newer = append(newer, postTestCommand(t, l, owner.SessionToken, postTestKey(t), postTestIntent(t, l, identity)).PostID)
			}
			postTestPublish(t, l)
			// Worker order is by task, so find the actual lookahead in the frozen order.
			var lookahead string
			if e := l.cp.QueryRow(context.Background(), `SELECT post_id FROM c_posts.posts ORDER BY published_at DESC,publication_ordinal DESC OFFSET 5 LIMIT 1`).Scan(&lookahead); e != nil {
				t.Fatal(e)
			}
			original := *l.c
			calls := 0
			inserted := ""
			preparedReadPool(t, l, func(n int) {
				calls = n
				if n > 1 && mode != "twice" {
					return
				}
				switch mode {
				case "lookahead":
					postTestCommand(t, &lab{c: &original, cp: l.cp}, owner.SessionToken, postTestKey(t), posts.Intent{Operation: posts.DeletePost, PostID: lookahead})
				case "insert":
					before := count(t, l.cp, `SELECT sum(publication_revision) FROM c_posts.read_feed_scope_versions`)
					inserted = preparedFeedPublish(t, &lab{c: &original, cp: l.cp}, owner.SessionToken, identity, "new race")
					after := count(t, l.cp, `SELECT sum(publication_revision) FROM c_posts.read_feed_scope_versions`)
					var visibility string
					if e := l.cp.QueryRow(context.Background(), `SELECT visibility FROM c_posts.posts WHERE post_id=$1`, inserted).Scan(&visibility); e != nil {
						t.Fatal(e)
					}
					t.Logf("insert publication revision %d -> %d; visibility=%s", before, after, visibility)
				case "filters":
					var ch string
					l.cp.QueryRow(context.Background(), `SELECT channel_id FROM c_posts.posts LIMIT 1`).Scan(&ch)
					if _, e := original.ReplaceFeedPreferences(context.Background(), reader.SessionToken, posts.FeedPreferences{BlockedChannelIDs: []string{ch}}); e != nil {
						t.Fatal(e)
					}
				case "twice":
					mustExec(t, l.cp, `UPDATE c_posts.posts SET moderation_hidden=NOT moderation_hidden WHERE post_id=$1`, newer[6])
				}
			})
			r, e := l.c.ReadPosts(context.Background(), reader.SessionToken, PostQuery{Kind: PostFeed, Incremental: true, RefreshCursor: *first.RefreshCursor, Limit: 5})
			if mode == "filters" {
				if e == nil || r.Value != nil {
					t.Fatal("filter-bound cursor leaked")
				}
				return
			}
			if mode == "twice" {
				postTestError(t, e, "SERVICE_UNAVAILABLE", 503)
				if calls != 2 || r.Value != nil {
					t.Fatal("retry bound or stale release")
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			page := r.Value.(posts.IncrementalFeedPage)
			if calls != 2 || len(page.Items) != 5 {
				t.Fatal("lost page or missing bounded rebuild", calls)
			}
			if mode == "insert" && page.Items[0].PostID != inserted {
				t.Fatal("newest insertion lost")
			}
		})
	}
}

func (t *listCountingTx) SendBatch(ctx context.Context, batch *pgx.Batch) pgx.BatchResults {
	// Count every SQL statement, not every network exchange.
	t.queries += len(batch.QueuedQueries)
	return t.Tx.SendBatch(ctx, batch)
}

func (t *listCountingTx) Query(ctx context.Context, s string, args ...any) (pgx.Rows, error) {
	t.queries++
	return t.Tx.Query(ctx, s, args...)
}
func (t *listCountingTx) QueryRow(ctx context.Context, s string, args ...any) pgx.Row {
	t.queries++
	return t.Tx.QueryRow(ctx, s, args...)
}

func comparableList(t *testing.T, v any) any {
	t.Helper()
	switch p := v.(type) {
	case posts.CrossChannelFeedPage:
		if p.NextCursor != nil {
			s := "more"
			p.NextCursor = &s
		}
		return p
	case posts.FeedPage:
		if p.NextCursor != nil {
			s := "more"
			p.NextCursor = &s
		}
		return p
	case posts.OwnPostPage:
		if p.NextCursor != nil {
			s := "more"
			p.NextCursor = &s
		}
		return p
	case posts.AuthorProfilePage:
		if p.NextCursor != nil {
			s := "more"
			p.NextCursor = &s
		}
		return p
	default:
		t.Fatalf("unknown page %T", v)
		return nil
	}
}

func TestFeedBatchSixListsPostgres(t *testing.T) {
	control := newGateControl(t)
	l := control.lab
	cfg := enablePostLab(t, l)
	cfg.PreparedFeedReads = true
	var err error
	l.c, err = l.c.WithPosts(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	owner := sessionTestAccount(t, l, "batch_owner")
	identity := identityChange(t, l.c, owner.SessionToken, "CREATE", uuid.Nil, "批量作者").IdentityID
	ids := []string{}
	for n := 0; n < 51; n++ {
		intent := postTestIntent(t, l, identity)
		intent.Title = fmt.Sprintf("post-%02d", n)
		intent.Tags = []string{"测试"}
		ids = append(ids, postTestCommand(t, l, owner.SessionToken, postTestKey(t), intent).PostID)
	}
	postTestPublish(t, l)
	// A frozen tie group exercises the private ordinal tie-break, not UUID order.
	if count(t, l.cp, `SELECT count(DISTINCT published_at) FROM c_posts.posts`) != 1 {
		t.Fatal("fixed Gate clock did not create a tie group")
	}
	mustExec(t, l.cp, `INSERT INTO c_posts.post_likes(post_id,account_id) SELECT post_id,$1 FROM c_posts.posts`, owner.AccountID)
	mustExec(t, l.cp, `INSERT INTO c_posts.post_favorites(post_id,account_id,saved_at) SELECT post_id,$1,published_at FROM c_posts.posts`, owner.AccountID)
	ctx := context.Background()
	tx, e := l.cp.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	for _, size := range []int{1, 5, 10, 20, 50} {
		for _, kind := range []PostReadKind{PostFeed, PostChannelFeed, PostAuthor, PostOwnList, PostLikedList, PostFavoriteList} {
			q := PostQuery{Kind: kind, Limit: size, ImageSizes: true}
			if kind == PostChannelFeed {
				q.ID = feedTestChannels(t, l)[0]
				if e := tx.QueryRow(ctx, `SELECT channel_id FROM c_posts.posts LIMIT 1`).Scan(&q.ID); e != nil {
					t.Fatal(e)
				}
			}
			if kind == PostAuthor {
				q.IdentityID = identity.String()
			}
			for page := 0; page < 55; page++ {
				legacy, e := l.c.listPublicPostsLegacyCaptured(ctx, tx, owner.AccountID, q, time.Now().UTC(), nil)
				if e != nil {
					t.Fatalf("legacy %s %d: %v", kind, size, e)
				}
				counter := &listCountingTx{Tx: tx}
				batch, e := l.c.listPublicPostsBatchCaptured(ctx, counter, owner.AccountID, q, time.Now().UTC(), nil)
				if e != nil {
					t.Fatalf("batch %s %d: %v", kind, size, e)
				}
				a, _ := json.Marshal(comparableList(t, legacy))
				b, _ := json.Marshal(comparableList(t, batch))
				if !reflect.DeepEqual(a, b) {
					t.Fatalf("%s size%d page%d mismatch\n%s\n%s", kind, size, page, a, b)
				}
				if counter.queries > 6 {
					t.Fatalf("unbounded SQL %s size%d: %d", kind, size, counter.queries)
				}
				if page == 0 && (size == 20 || size == 50) {
					t.Logf("SQL %s size%d: %d subject statements (metadata+candidates+projection+geometry/author); authority/version proof measured separately", kind, size, counter.queries)
				}
				var next *string
				switch p := batch.(type) {
				case posts.CrossChannelFeedPage:
					next = p.NextCursor
				case posts.FeedPage:
					next = p.NextCursor
				case posts.OwnPostPage:
					next = p.NextCursor
				case posts.AuthorProfilePage:
					next = p.NextCursor
				}
				if next == nil {
					break
				}
				q.Cursor = *next
			}
			t.Logf("PASS frozen equivalence %s page size %d", kind, size)
		}
	}
	if e = tx.Rollback(ctx); e != nil {
		t.Fatal(e)
	}
	for _, size := range []int{20, 50} {
		page := checkNativePreparedProjection(t, l, owner.SessionToken, size, 3).(posts.CrossChannelFeedPage)
		if len(page.Items) != size || page.NextCursor == nil {
			t.Fatal("native prepared page/lookahead changed", size)
		}
	}
}

func TestFeedIncrementalTenFiveFivePostgres(t *testing.T) {
	l := preparedFeedLab(t)
	owner := sessionTestAccount(t, l, "window_owner")
	reader := sessionTestAccount(t, l, "window_reader")
	identity := identityChange(t, l.c, owner.SessionToken, "CREATE", uuid.Nil, "增量作者").IdentityID
	all := []string{}
	publish := func(n int) {
		for k := 0; k < n; k++ {
			all = append(all, postTestCommand(t, l, owner.SessionToken, postTestKey(t), postTestIntent(t, l, identity)).PostID)
		}
		postTestPublish(t, l)
	}
	read := func(q PostQuery) posts.IncrementalFeedPage {
		q.Kind = PostFeed
		q.Incremental = true
		r, e := l.c.ReadPosts(context.Background(), reader.SessionToken, q)
		if e != nil {
			t.Fatal(e)
		}
		return r.Value.(posts.IncrementalFeedPage)
	}
	empty := read(PostQuery{Limit: 10})
	if len(empty.Items) != 0 || empty.RefreshCursor == nil {
		t.Fatal("empty initial", empty)
	}
	publish(16)
	first := read(PostQuery{Limit: 10})
	if len(first.Items) != 10 || first.NextCursor == nil || first.RefreshCursor == nil {
		t.Fatal("initial", first)
	}
	older := read(PostQuery{Limit: 5, Cursor: *first.NextCursor})
	if len(older.Items) != 5 || older.RefreshCursor != nil || older.Items[0].PostID != all[5] {
		t.Fatal("10 to 5 continuation", older)
	}
	publish(12)
	refresh := read(PostQuery{Limit: 5, RefreshCursor: *first.RefreshCursor})
	if len(refresh.Items) != 5 || refresh.Items[0].PostID != all[27] || refresh.InsertAfter != nil || refresh.NextCursor != nil {
		t.Fatal("newest five", refresh)
	}
	merged := append(append([]posts.FeedPost{}, refresh.Items...), first.Items...)
	token := refresh.RefreshCursor
	for _, expected := range []int{5, 2, 0} {
		p := read(PostQuery{Limit: 5, RefreshCursor: *token})
		if len(p.Items) != expected {
			t.Fatal("missing backlog", expected, p)
		}
		if expected > 0 {
			if p.InsertAfter == nil {
				t.Fatal("missing insertion anchor")
			}
			j := 0
			for j < len(merged) && merged[j].PostID != *p.InsertAfter {
				j++
			}
			if j == len(merged) {
				t.Fatal("anchor missing")
			}
			j++
			merged = append(merged[:j], append(p.Items, merged[j:]...)...)
		}
		token = p.RefreshCursor
	}
	if len(merged) != 22 {
		t.Fatal("retained list", len(merged))
	}
	for n, p := range merged {
		if p.PostID != all[27-n] {
			t.Fatal("gap, duplicate or ordering", n, p.PostID)
		}
	}
	// Old traversal keeps its original ceiling even after new posts and refresh.
	end := read(PostQuery{Limit: 5, Cursor: *older.NextCursor})
	if len(end.Items) != 1 || end.Items[0].PostID != all[0] || end.NextCursor != nil {
		t.Fatal("old traversal moved", end)
	}
	publish(1)
	one := read(PostQuery{Limit: 5, RefreshCursor: *token})
	if len(one.Items) != 1 || one.Items[0].PostID != all[28] {
		t.Fatal("single new post", one)
	}
	_, e := l.c.ReadPosts(context.Background(), reader.SessionToken, PostQuery{Kind: PostFeed, Incremental: true, Limit: 5, RefreshCursor: *first.NextCursor})
	if e == nil {
		t.Fatal("old cursor accepted for refresh")
	}
}
