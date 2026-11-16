package authprivacy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/readtiming"
)

const (
	dmDirFirst  = "00000000-0000-4000-8000-000000000201"
	dmDirSecond = "00000000-0000-4000-8000-000000000301"
	dmDirThird  = "00000000-0000-4000-8000-000000000401"
	dmDirInsert = "00000000-0000-4000-8000-000000000251"
	dmDirLate   = "00000000-0000-4000-8000-000000000501"
)

type dmDirectoryPeer struct {
	account  SignupResult
	identity uuid.UUID
}

type dmDirectoryFixture struct {
	l                *lab
	viewer           SignupResult
	viewerIdentity   uuid.UUID
	post, hiddenPost string
	first            string
	second           string
	third            string
	peers            []dmDirectoryPeer
	blockID          string
}

// dmDirectoryFixture publishes two source posts (one hidden), creates four
// peers on both sides of the viewer identity, and seeds three conversations
// with delivery/settings/block states. Conversations and messages are inserted
// directly so the test controls unread, acknowledged, hidden and deleted
// boundaries without depending on send quotas.
func newDMDirectoryFixture(t *testing.T) *dmDirectoryFixture {
	t.Helper()
	l := newPostLab(t)
	ctx := context.Background()
	var e error
	l.c, e = l.c.WithDirectMessages(ctx, DirectMessageConfig{PreparedDirectoryReads: true})
	if e != nil {
		t.Fatal("prepared directory rejected the complete schema", e)
	}
	if !l.c.preparedDMDirectoryReads() {
		t.Fatal("fixture did not select the prepared directory")
	}
	f := &dmDirectoryFixture{l: l}
	f.viewer = sessionTestAccount(t, l, "dm_dir_viewer")
	f.viewerIdentity = identityChange(t, l.c, f.viewer.SessionToken, "CREATE", uuid.Nil, "目录读者").IdentityID
	// The deletion case soft-deletes the viewer identity, so the account keeps a
	// second identity to satisfy the lifecycle shape invariant.
	dmDirectorySeedIdentity(t, l, f.viewer.AccountID, uuid.New(), "目录备用", false)
	intent := postTestIntent(t, l, f.viewerIdentity)
	intent.Title, intent.Body = "目录来源", "目录正文"
	f.post = postTestCommand(t, l, f.viewer.SessionToken, postTestKey(t), intent).PostID
	intent = postTestIntent(t, l, f.viewerIdentity)
	intent.Title, intent.Body = "隐藏来源", "隐藏正文"
	f.hiddenPost = postTestCommand(t, l, f.viewer.SessionToken, postTestKey(t), intent).PostID
	postTestPublish(t, l)
	mustExec(t, l.cp, `UPDATE c_posts.posts SET moderation_hidden=true WHERE post_id=$1`, f.hiddenPost)

	low, high := dmDirectorySides(f.viewerIdentity)
	peerIDs := []uuid.UUID{low, high, uuid.New(), uuid.New()}
	for i, id := range peerIDs {
		account := sessionTestAccount(t, l, fmt.Sprintf("dm_dir_peer_%d", i))
		// Seed each identity directly so one peer sorts below and one above the
		// viewer regardless of random UUIDs; the API path is covered by the
		// identity tests.
		dmDirectorySeedIdentity(t, l, account.AccountID, id, fmt.Sprintf("目录作者%d", i), true)
		if i == 1 {
			// The second peer is soft-deleted below; keep a spare identity so
			// the account stays within the lifecycle shape invariant.
			dmDirectorySeedIdentity(t, l, account.AccountID, uuid.New(), "目录备用", false)
		}
		f.peers = append(f.peers, dmDirectoryPeer{account: account, identity: id})
	}

	f.first = dmDirectoryConversation(t, l, dmDirFirst, f.viewerIdentity, f.peers[0].identity, f.post, 2, true, true)
	f.second = dmDirectoryConversation(t, l, dmDirSecond, f.viewerIdentity, f.peers[1].identity, f.hiddenPost, 1, true, true)
	f.third = dmDirectoryConversation(t, l, dmDirThird, f.viewerIdentity, f.peers[2].identity, f.post, 0, false, false)

	// First conversation: one plain unread and one acknowledged-unread.
	dmDirectoryMessage(t, l, f.first, 1, f.peers[0].identity, f.viewerIdentity, "未读一", false, false)
	dmDirectoryMessage(t, l, f.first, 2, f.peers[0].identity, f.viewerIdentity, "", true, false)
	mustExec(t, l.cp, `INSERT INTO c_dm.settings(conversation_id,identity_id,version,pinned,muted,note,hidden_through,deleted_through)
 VALUES($1,$2,3,true,true,'置顶备注',0,0)`, f.first, f.viewerIdentity)
	// Second conversation: read+acknowledged history that was hidden/deleted
	// through the latest sequence.
	dmDirectoryMessage(t, l, f.second, 1, f.peers[1].identity, f.viewerIdentity, "", true, true)
	mustExec(t, l.cp, `INSERT INTO c_dm.settings(conversation_id,identity_id,version,pinned,muted,note,hidden_through,deleted_through)
 VALUES($1,$2,1,false,false,'',1,1)`, f.second, f.viewerIdentity)

	// Mirror the delete projection without the last-identity restriction: the
	// directory only observes the soft-deleted tombstone and its version bump.
	dmDirectoryDeleteIdentity(t, l, f.peers[1].identity)
	block, e := l.c.ChangeDMBlock(ctx, f.viewer.SessionToken, uuid.NewString(), DMBlockInput{
		IdentityID: f.viewerIdentity.String(), ConversationID: f.third, Blocked: true,
	})
	if e != nil {
		t.Fatal("account block fixture", e)
	}
	f.blockID = block.Value.(DMBlockResult).BlockID
	if f.blockID == "" {
		t.Fatal("block fixture returned no block id")
	}
	return f
}

// dmDirectorySides returns one identity below and one above the viewer so the
// directory exercises both endpoint branches deterministically.
func dmDirectorySides(viewer uuid.UUID) (uuid.UUID, uuid.UUID) {
	low := uuid.MustParse("00000000-0000-4000-8000-000000000000")
	high := viewer
	for i := len(high) - 1; i >= 0; i-- {
		if high[i] < 0xff {
			high[i]++
			break
		}
	}
	if bytes.Compare(low[:], viewer[:]) >= 0 || bytes.Compare(high[:], viewer[:]) <= 0 {
		panic("viewer identity is at an endpoint of the UUID space")
	}
	return low, high
}

// dmDirectorySeedIdentity inserts a counter-consistent identity; the lifecycle
// shape triggers require the cumulative counter and the identity row in one
// transaction.
func dmDirectorySeedIdentity(t *testing.T, l *lab, account, id uuid.UUID, nickname string, original bool) {
	t.Helper()
	ctx := context.Background()
	tx, e := l.cp.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `INSERT INTO public.identity_account_state(account_id,created_count,last_created_at)
	 VALUES($1,1,now()) ON CONFLICT(account_id) DO UPDATE
	 SET created_count=identity_account_state.created_count+1,last_created_at=EXCLUDED.last_created_at`, account); e != nil {
		t.Fatal(e)
	}
	if _, e = tx.Exec(ctx, `INSERT INTO public.community_identities(identity_id,account_id,nickname,avatar,is_original,created_at)
	 VALUES($1,$2,$3,'default-v1',$4,now())`, id, account, nickname, original); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
}

func dmDirectoryDeleteIdentity(t *testing.T, l *lab, id uuid.UUID) {
	t.Helper()
	mustExec(t, l.cp, `UPDATE public.community_identities SET nickname=NULL,avatar=NULL,last_renamed_at=NULL,deleted_at=now() WHERE identity_id=$1`, id)
}

func dmDirectoryConversation(t *testing.T, l *lab, id string, viewer, peer uuid.UUID, post string, sequence int64, lowSent, highSent bool) string {
	t.Helper()
	low, high := viewer, peer
	if bytes.Compare(peer[:], viewer[:]) < 0 {
		low, high = peer, viewer
	}
	mustExec(t, l.cp, `INSERT INTO c_dm.conversations(conversation_id,low_identity,high_identity,source_post,sequence,low_sent,high_sent)
 VALUES($1,$2,$3,$4,$5,$6,$7)`, id, low, high, post, sequence, lowSent, highSent)
	return id
}

func dmDirectoryMessage(t *testing.T, l *lab, conversation string, sequence int64, sender, recipient uuid.UUID, body string, acknowledged, read bool) {
	t.Helper()
	var value any
	if body != "" {
		value = body
	}
	mustExec(t, l.cp, `INSERT INTO c_dm.messages(message_id,conversation_id,sequence,sender,recipient,body,accepted_at,acknowledged,read)
 VALUES($1,$2,$3,$4,$5,$6,now(),$7,$8)`, uuid.New(), conversation, sequence, sender, recipient, value, acknowledged, read)
}

// dmDirectorySeedScale inserts n conversations with peers split evenly below
// and above the viewer and returns their conversation IDs in ascending order.
// All rows go in one transaction so the lifecycle shape triggers see consistent
// accounts.
func dmDirectorySeedScale(t *testing.T, l *lab, viewer uuid.UUID, post string, n int) []string {
	t.Helper()
	ctx := context.Background()
	ids := make([]uuid.UUID, n)
	accounts := make([]uuid.UUID, n)
	conversations := make([]uuid.UUID, n)
	low := make([]uuid.UUID, n)
	high := make([]uuid.UUID, n)
	for i := 0; i < n; i++ {
		if i%2 == 0 {
			ids[i] = uuid.MustParse(fmt.Sprintf("00000000-0000-4000-8000-%012x", i/2+1))
			if bytes.Compare(ids[i][:], viewer[:]) >= 0 {
				t.Fatalf("scale identity %s is not below the viewer %s", ids[i], viewer)
			}
			low[i], high[i] = ids[i], viewer
		} else {
			ids[i] = uuid.MustParse(fmt.Sprintf("ffffffff-ffff-4fff-bfff-%012x", i/2+1))
			if bytes.Compare(ids[i][:], viewer[:]) <= 0 {
				t.Fatalf("scale identity %s is not above the viewer %s", ids[i], viewer)
			}
			low[i], high[i] = viewer, ids[i]
		}
		accounts[i] = uuid.New()
		conversations[i] = uuid.MustParse(fmt.Sprintf("00000000-0000-4000-8000-%012d", 1000+i))
	}

	// Normal signups draw random platform numbers, so find a free block first.
	base := 900000
	for {
		var used int
		if e := l.cp.QueryRow(ctx, `SELECT count(*) FROM c_auth.accounts WHERE platform_number::int BETWEEN $1 AND $2`, base, base+n-1).Scan(&used); e != nil {
			t.Fatal(e)
		}
		if used == 0 {
			break
		}
		base += 1000
		if base+n-1 > 999999 {
			t.Fatal("no free platform-number block for the scale fixture")
		}
	}
	tx, e := l.cp.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `INSERT INTO c_auth.accounts(account_id,platform_number,username,password_hash,password_salt,password_params_version)
	 SELECT a,lpad((($2::int)-1+o)::text,6,'0'),'dm_scale_'||o,decode(repeat('11',32),'hex'),decode(repeat('22',16),'hex'),1
	 FROM unnest($1::uuid[]) WITH ORDINALITY AS x(a,o)`, accounts, base); e != nil {
		t.Fatal(e)
	}
	if _, e = tx.Exec(ctx, `INSERT INTO public.identity_account_state(account_id,created_count,last_created_at)
	 SELECT a,1,now() FROM unnest($1::uuid[]) AS a`, accounts); e != nil {
		t.Fatal(e)
	}
	if _, e = tx.Exec(ctx, `INSERT INTO public.community_identities(identity_id,account_id,nickname,avatar,is_original,created_at)
	 SELECT i,a,'目录规模'||o,'default-v1',true,now()
	 FROM unnest($1::uuid[],$2::uuid[]) WITH ORDINALITY AS x(i,a,o)`, ids, accounts); e != nil {
		t.Fatal(e)
	}
	if _, e = tx.Exec(ctx, `INSERT INTO c_dm.conversations(conversation_id,low_identity,high_identity,source_post,sequence,low_sent,high_sent)
	 SELECT c,l,h,$4,0,true,true FROM unnest($1::uuid[],$2::uuid[],$3::uuid[]) AS x(c,l,h)`, conversations, low, high, post); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	out := make([]string, n)
	for i := range conversations {
		out[i] = conversations[i].String()
	}
	return out
}

func dmDirectoryValue(t *testing.T, response PostResponse) DMChatDirectory {
	t.Helper()
	value, ok := response.Value.(DMChatDirectory)
	if !ok {
		t.Fatalf("unexpected directory value %T", response.Value)
	}
	return value
}

func dmDirectoryItems(directory DMChatDirectory) map[string]DMChatConversation {
	items := make(map[string]DMChatConversation, len(directory.Items))
	for _, item := range directory.Items {
		items[item.ID] = item
	}
	return items
}

func TestDMChatDirectoryPreparedEquivalencePostgres(t *testing.T) {
	f := newDMDirectoryFixture(t)
	ctx := context.Background()
	legacy := *f.l.c
	legacy.dmDirectoryReads = false
	for _, after := range []string{"", dmDirFirst} {
		want, e := legacy.ReadDMChatDirectory(ctx, f.viewer.SessionToken, f.viewerIdentity.String(), after)
		if e != nil {
			t.Fatal("legacy directory", e)
		}
		got, e := f.l.c.ReadDMChatDirectory(ctx, f.viewer.SessionToken, f.viewerIdentity.String(), after)
		if e != nil {
			t.Fatal("prepared directory", e)
		}
		old, e := json.Marshal(want.Value)
		if e != nil {
			t.Fatal(e)
		}
		fresh, e := json.Marshal(got.Value)
		if e != nil {
			t.Fatal(e)
		}
		if !bytes.Equal(old, fresh) {
			t.Fatalf("cursor %q changed directory projection\nlegacy: %s\nprepared: %s", after, old, fresh)
		}
	}
	items := dmDirectoryItems(dmDirectoryValue(t, mustDirectory(t, f.l.c, ctx, f.viewer.SessionToken, f.viewerIdentity, "")))
	if len(items) != 3 {
		t.Fatalf("first page has %d conversations", len(items))
	}
	first := items[dmDirFirst]
	if first.Unread != 2 || first.UnavailableUnread != 1 || !first.Pinned || !first.Muted || first.Note != "置顶备注" ||
		first.Version != 3 || first.SettingsVersion != 3 || !first.CanShare || first.Waiting {
		t.Fatalf("first conversation projection changed: %+v", first)
	}
	second := items[dmDirSecond]
	if !strings.HasPrefix(second.PeerName, "注销身份 · ") || second.PeerAvatar != "inactive-v1" || !second.Hidden ||
		!second.Deleted || second.DeletedThrough != 1 || second.Unread != 0 || second.UnavailableUnread != 0 ||
		second.CanSend || second.CanShare || second.SourceTitle != "" {
		t.Fatalf("deleted peer conversation projection changed: %+v", second)
	}
	third := items[dmDirThird]
	if third.OwnBlockID != f.blockID || third.CanSend || third.CanShare || third.SourceTitle != "目录来源" {
		t.Fatalf("blocked conversation projection changed: %+v", third)
	}
	page := dmDirectoryItems(dmDirectoryValue(t, mustDirectory(t, f.l.c, ctx, f.viewer.SessionToken, f.viewerIdentity, dmDirFirst)))
	if len(page) != 2 || page[dmDirFirst].ID != "" {
		t.Fatalf("cursor page changed: %+v", page)
	}
}

// TestDMChatDirectoryPreparedPageLimitPostgres exercises the 100-item page
// boundary and the conversation-ID cursor across several pages while both
// identity branches hold more than one page of rows, and checks that the legacy
// and prepared paths still agree at that size.
func TestDMChatDirectoryPreparedPageLimitPostgres(t *testing.T) {
	f := newDMDirectoryFixture(t)
	ctx := context.Background()
	scale := dmDirectorySeedScale(t, f.l, f.viewerIdentity, f.post, 400)
	want := append([]string{dmDirFirst, dmDirSecond, dmDirThird}, scale...)
	legacy := *f.l.c
	legacy.dmDirectoryReads = false

	after := ""
	for page := 0; page*100 <= len(want); page++ {
		size := len(want) - page*100
		if size > 100 {
			size = 100
		}
		got, e := f.l.c.ReadDMChatDirectory(ctx, f.viewer.SessionToken, f.viewerIdentity.String(), after)
		if e != nil {
			t.Fatal("prepared directory", e)
		}
		items := dmDirectoryValue(t, got).Items
		if len(items) != size {
			t.Fatalf("page %d has %d conversations, want %d (cursor %q)", page, len(items), size, after)
		}
		for i, item := range items {
			if id := want[page*100+i]; item.ID != id {
				t.Fatalf("page %d item %d is %s, want %s", page, i, item.ID, id)
			}
			if i > 0 && items[i-1].ID >= item.ID {
				t.Fatalf("page %d is not ordered by conversation id", page)
			}
		}
		old, e := legacy.ReadDMChatDirectory(ctx, f.viewer.SessionToken, f.viewerIdentity.String(), after)
		if e != nil {
			t.Fatal("legacy directory", e)
		}
		oldJSON, e := json.Marshal(old.Value)
		if e != nil {
			t.Fatal(e)
		}
		newJSON, e := json.Marshal(got.Value)
		if e != nil {
			t.Fatal(e)
		}
		if !bytes.Equal(oldJSON, newJSON) {
			t.Fatalf("page %d projection differs between legacy and prepared", page)
		}
		after = items[len(items)-1].ID
	}
	if items := dmDirectoryValue(t, mustDirectory(t, f.l.c, ctx, f.viewer.SessionToken, f.viewerIdentity, after)).Items; len(items) != 0 {
		t.Fatalf("directory continued past the last page with %d conversations", len(items))
	}
}

func mustDirectory(t *testing.T, c *Community, ctx context.Context, bearer [32]byte, identity uuid.UUID, after string) PostResponse {
	t.Helper()
	response, e := c.ReadDMChatDirectory(ctx, bearer, identity.String(), after)
	if e != nil {
		t.Fatal(e)
	}
	return response
}

func dmDirectoryPreparedPool(t *testing.T, l *lab, prefix string, change func(int)) {
	t.Helper()
	tracer := &preparedReadTracer{prefix: prefix, change: change}
	cfg := l.cp.Config()
	cfg.ConnConfig.Tracer = tracer
	pool, e := pgxpool.NewWithConfig(context.Background(), cfg)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(pool.Close)
	l.c.pool = pool
}

func TestDMChatDirectoryPreparedRebuildPostgres(t *testing.T) {
	t.Run("message", func(t *testing.T) {
		f := newDMDirectoryFixture(t)
		dmDirectoryPreparedPool(t, f.l, "WITH page AS (", func(n int) {
			if n != 1 {
				return
			}
			dmDirectoryMessage(t, f.l, f.first, 3, f.peers[0].identity, f.viewerIdentity, "", true, false)
			mustExec(t, f.l.cp, `UPDATE c_dm.conversations SET sequence=3 WHERE conversation_id=$1`, f.first)
		})
		first := dmDirectoryItems(dmDirectoryValue(t, mustDirectory(t, f.l.c, context.Background(), f.viewer.SessionToken, f.viewerIdentity, "")))[dmDirFirst]
		if first.Unread != 3 || first.UnavailableUnread != 2 {
			t.Fatalf("rebuilt unread counts did not include the committed message: %+v", first)
		}
	})
	t.Run("new_conversation", func(t *testing.T) {
		f := newDMDirectoryFixture(t)
		dmDirectoryPreparedPool(t, f.l, "WITH page AS (", func(n int) {
			if n != 1 {
				return
			}
			dmDirectoryConversation(t, f.l, dmDirInsert, f.viewerIdentity, f.peers[3].identity, f.post, 0, false, false)
		})
		directory := dmDirectoryValue(t, mustDirectory(t, f.l.c, context.Background(), f.viewer.SessionToken, f.viewerIdentity, ""))
		if len(directory.Items) != 4 || dmDirectoryItems(directory)[dmDirInsert].ID != dmDirInsert {
			t.Fatalf("new conversation inside the returned interval was not rebuilt: %+v", directory.Items)
		}
	})
	t.Run("short_page_new_conversation_after_last", func(t *testing.T) {
		// A short page is the client's final page. A conversation created after
		// preparation with an id larger than the last returned one still belongs
		// on that page, so the range check must not be capped at the last id.
		f := newDMDirectoryFixture(t)
		dmDirectoryPreparedPool(t, f.l, "WITH page AS (", func(n int) {
			if n != 1 {
				return
			}
			dmDirectoryConversation(t, f.l, dmDirLate, f.viewerIdentity, f.peers[3].identity, f.post, 0, false, false)
		})
		directory := dmDirectoryValue(t, mustDirectory(t, f.l.c, context.Background(), f.viewer.SessionToken, f.viewerIdentity, ""))
		if len(directory.Items) != 4 || dmDirectoryItems(directory)[dmDirLate].ID != dmDirLate {
			t.Fatalf("short page missed a conversation created after its last id: %+v", directory.Items)
		}
	})
	t.Run("identity_deleted", func(t *testing.T) {
		f := newDMDirectoryFixture(t)
		dmDirectoryPreparedPool(t, f.l, "WITH page AS (", func(n int) {
			if n == 1 {
				// The viewer owns source posts, so use the real delete path: it
				// performs the required same-transaction private post cleanup.
				identityChange(t, f.l.c, f.viewer.SessionToken, "DELETE", f.viewerIdentity, "")
			}
		})
		directory := dmDirectoryValue(t, mustDirectory(t, f.l.c, context.Background(), f.viewer.SessionToken, f.viewerIdentity, ""))
		if directory.Items == nil || len(directory.Items) != 0 {
			t.Fatalf("deleted viewer identity still received a page: %+v", directory.Items)
		}
	})
	t.Run("author_block", func(t *testing.T) {
		f := newDMDirectoryFixture(t)
		blockID := uuid.NewString()
		dmDirectoryPreparedPool(t, f.l, "WITH page AS (", func(n int) {
			if n != 1 {
				return
			}
			mustExec(t, f.l.cp, `INSERT INTO c_posts.blocked_authors(block_id,account_id,author_account_id,author_state,nickname,avatar,short_code,created_at)
 VALUES($1,$2,$3,'ACTIVE','目录作者','default-v1','ABCDEFGHIJKL',now())`, blockID, f.viewer.AccountID, f.peers[0].account.AccountID)
		})
		first := dmDirectoryItems(dmDirectoryValue(t, mustDirectory(t, f.l.c, context.Background(), f.viewer.SessionToken, f.viewerIdentity, "")))[dmDirFirst]
		if first.OwnBlockID != blockID || first.CanSend || first.CanShare {
			t.Fatalf("committed account block was not rebuilt: %+v", first)
		}
	})
	t.Run("legacy_dm_block", func(t *testing.T) {
		f := newDMDirectoryFixture(t)
		dmDirectoryPreparedPool(t, f.l, "WITH page AS (", func(n int) {
			if n != 1 {
				return
			}
			mustExec(t, f.l.cp, `INSERT INTO c_dm.blocks(block_id,owner_account,peer_account,identity_id,peer_id,conversation_id,peer_name,peer_avatar,created_at)
 VALUES($1,$2,$3,$4,$5,$6,'目录作者','default-v1',now())`,
				uuid.New(), f.viewer.AccountID, f.peers[0].account.AccountID, f.viewerIdentity, f.peers[0].identity, f.first)
		})
		first := dmDirectoryItems(dmDirectoryValue(t, mustDirectory(t, f.l.c, context.Background(), f.viewer.SessionToken, f.viewerIdentity, "")))[dmDirFirst]
		if first.CanSend || first.CanShare || first.OwnBlockID != "" {
			t.Fatalf("legacy DM block was not rebuilt: %+v", first)
		}
	})
	t.Run("revoked_session", func(t *testing.T) {
		f := newDMDirectoryFixture(t)
		dmDirectoryPreparedPool(t, f.l, "WITH page AS (", func(n int) {
			if n == 1 {
				if e := f.l.c.RevokeSession(context.Background(), f.viewer.RevokeSecret); e != nil {
					t.Fatal(e)
				}
			}
		})
		response, e := f.l.c.ReadDMChatDirectory(context.Background(), f.viewer.SessionToken, f.viewerIdentity.String(), "")
		if !errors.Is(e, ErrSessionInvalid) || response.Value != nil {
			t.Fatalf("revoked session released a prepared page: %v %#v", e, response.Value)
		}
	})
}

type dmDirectorySQLCounter struct {
	statements int
}

func (q *dmDirectorySQLCounter) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	if strings.HasPrefix(d.SQL, "WITH viewer AS (") || strings.HasPrefix(d.SQL, "WITH page AS (") ||
		strings.HasPrefix(d.SQL, "SELECT c.conversation_id::text") {
		q.statements++
	}
	return ctx
}

func (q *dmDirectorySQLCounter) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func TestDMChatDirectoryPreparedSQLCountPostgres(t *testing.T) {
	f := newDMDirectoryFixture(t)
	counter := &dmDirectorySQLCounter{}
	cfg := f.l.cp.Config()
	cfg.ConnConfig.Tracer = counter
	pool, e := pgxpool.NewWithConfig(context.Background(), cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	f.l.c.pool = pool
	readCtx, trace := readtiming.Start(context.Background())
	if _, e = f.l.c.ReadDMChatDirectory(readCtx, f.viewer.SessionToken, f.viewerIdentity.String(), ""); e != nil {
		t.Fatal(e)
	}
	if counter.statements != 5 {
		t.Fatalf("one directory page used %d statement executions, want 5", counter.statements)
	}
	stages := trace.Snapshot().Stages
	if stages["dm-directory.prepare_read"].Calls != 1 || stages["dm-directory.finalize_read"].Calls != 1 ||
		stages["dm-directory.prepared_read_rebuild"].Calls != 0 {
		t.Fatalf("unexpected prepared phases: %+v", stages)
	}
}

func TestDMChatDirectoryPreparedIndexesPostgres(t *testing.T) {
	l := newLab(t)
	ctx := context.Background()
	tx, e := l.cp.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `SET LOCAL enable_seqscan=off; SET LOCAL enable_bitmapscan=off`); e != nil {
		t.Fatal(e)
	}
	rows, e := tx.Query(ctx, "EXPLAIN "+dmDirectoryCandidatesSQL, uuid.New(), uuid.New(), nil)
	if e != nil {
		t.Fatal(e)
	}
	var plan strings.Builder
	for rows.Next() {
		var line string
		if e = rows.Scan(&line); e != nil {
			rows.Close()
			t.Fatal(e)
		}
		plan.WriteString(line)
		plan.WriteByte('\n')
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		t.Fatal(e)
	}
	text := plan.String()
	if !strings.Contains(text, "dm_conversations_low_cursor") || !strings.Contains(text, "dm_conversations_high_cursor") {
		t.Fatalf("directory branches did not use both cursor indexes:\n%s", text)
	}
}

// execOpsScript applies an operator SQL file one statement at a time on one
// connection, mirroring `psql -v ON_ERROR_STOP=1 -f`. Executing the whole file
// as one pgx batch hides a missing BEGIN because PostgreSQL wraps the batch in
// an implicit transaction that LOCK TABLE can use.
func execOpsScript(t *testing.T, pool *pgxpool.Pool, body string) {
	t.Helper()
	ctx := context.Background()
	conn, e := pool.Acquire(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer conn.Release()
	var code strings.Builder
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}
		code.WriteString(line)
		code.WriteByte('\n')
	}
	for _, statement := range strings.Split(code.String(), ";") {
		statement = strings.TrimSpace(statement)
		if statement == "" {
			continue
		}
		if _, e := conn.Exec(ctx, statement); e != nil {
			t.Fatalf("operator statement failed: %v\n%s", e, statement)
		}
	}
}

// TestDMDirectoryIndexRollbackPostgres verifies the operator rollback path.
// The goose Down stays forward-only, so abandoning the access path uses the
// checked-in ops scripts while the ledger remains at 0047.
func TestDMDirectoryIndexRollbackPostgres(t *testing.T) {
	f := newDMDirectoryFixture(t)
	ctx := context.Background()
	if _, e := f.l.c.ReadDMChatDirectory(ctx, f.viewer.SessionToken, f.viewerIdentity.String(), ""); e != nil {
		t.Fatal("prepared directory before rollback", e)
	}
	ops := map[string]string{
		"rollback": filepath.Join("..", "..", "ops", "0047_dm_directory_cursor_indexes_rollback.sql"),
		"reapply":  filepath.Join("..", "..", "ops", "0047_dm_directory_cursor_indexes_reapply.sql"),
	}
	script := func(name string) string {
		t.Helper()
		body, e := os.ReadFile(ops[name])
		if e != nil {
			t.Fatal(name, e)
		}
		return string(body)
	}
	indexes := func() int {
		t.Helper()
		return count(t, f.l.cp, `SELECT count(*) FROM pg_indexes WHERE schemaname='c_dm' AND indexname IN ('dm_conversations_low_cursor','dm_conversations_high_cursor')`)
	}
	if indexes() != 2 {
		t.Fatal("fixture did not install both cursor indexes")
	}
	execOpsScript(t, f.l.cp, script("rollback"))
	if indexes() != 0 {
		t.Fatal("rollback script left a cursor index behind")
	}
	if _, e := f.l.c.WithDirectMessages(ctx, DirectMessageConfig{PreparedDirectoryReads: true}); e == nil {
		t.Fatal("prepared directory started without its indexes")
	}
	legacy := *f.l.c
	legacy.dmDirectoryReads = false
	if _, e := legacy.ReadDMChatDirectory(ctx, f.viewer.SessionToken, f.viewerIdentity.String(), ""); e != nil {
		t.Fatal("legacy directory after rollback", e)
	}
	execOpsScript(t, f.l.cp, script("reapply"))
	if indexes() != 2 {
		t.Fatal("reapply script did not restore both cursor indexes")
	}
	c, e := f.l.c.WithDirectMessages(ctx, DirectMessageConfig{PreparedDirectoryReads: true})
	if e != nil {
		t.Fatal("prepared directory after reapply", e)
	}
	if _, e = c.ReadDMChatDirectory(ctx, f.viewer.SessionToken, f.viewerIdentity.String(), ""); e != nil {
		t.Fatal("prepared directory read after reapply", e)
	}
}
