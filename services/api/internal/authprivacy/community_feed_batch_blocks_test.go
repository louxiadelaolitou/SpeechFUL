package authprivacy

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/posts"
)

// Compare the bulk predicate with the shared SQL function using real block
// rows, including both directions, self/unrelated accounts and NULL operands.
func TestFeedBatchBlockPredicatePostgres(t *testing.T) {
	l := preparedFeedLab(t)
	ctx := context.Background()
	a := sessionTestAccount(t, l, "predicate_a")
	b := sessionTestAccount(t, l, "predicate_b")
	c := sessionTestAccount(t, l, "predicate_c")
	// A policy change must revisit the inlined list predicate even if an old
	// truth-table fixture happens not to exercise the new condition.
	var body string
	if e := l.cp.QueryRow(ctx, `SELECT prosrc FROM pg_proc WHERE oid='c_posts.accounts_blocked(uuid,uuid)'::regprocedure`).Scan(&body); e != nil {
		t.Fatal(e)
	}
	expected := `SELECT EXISTS (SELECT 1 FROM c_posts.blocked_authors b WHERE b.account_id = viewer AND b.author_account_id = author) OR EXISTS (SELECT 1 FROM c_posts.blocked_authors b WHERE b.account_id = author AND b.author_account_id = viewer)`
	if strings.Join(strings.Fields(body), " ") != expected {
		t.Fatal("author block policy changed; review the bulk list predicate")
	}
	mustExec(t, l.cp, `CREATE TABLE IF NOT EXISTS public.goose_db_version(version_id bigint,is_applied boolean)`)
	grant, e := os.ReadFile("../../../../infra/postgres/grant-community.sql")
	if e != nil {
		t.Fatal(e)
	}
	mustExec(t, l.cp, string(grant))
	dsn := os.Getenv("AUTHLAB_POSTS_RUNTIME_DSN")
	if dsn == "" {
		t.Fatal("owned least-privilege runtime fixture required")
	}
	runtime, e := pgxpool.New(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(runtime.Close)
	var actor string
	var allowed, excess bool
	if e = runtime.QueryRow(ctx, `SELECT current_user,has_table_privilege(current_user,'c_posts.blocked_authors','SELECT'),has_table_privilege(current_user,'c_posts.blocked_authors','UPDATE,TRUNCATE')`).Scan(&actor, &allowed, &excess); e != nil || actor != "hnuhole_c_runtime" || !allowed || excess {
		t.Fatal("unexpected runtime authority", actor, allowed, excess, e)
	}
	values := []any{a.AccountID, b.AccountID, c.AccountID, uuid.New(), nil}
	for _, mode := range []string{"none", "forward", "both", "reverse", "removed"} {
		switch mode {
		case "forward", "both":
			x, y := a.AccountID, b.AccountID
			if mode == "both" {
				x, y = y, x
			}
			mustExec(t, l.cp, `INSERT INTO c_posts.blocked_authors(block_id,account_id,author_account_id,author_state,nickname,avatar,short_code,created_at) VALUES($1,$2,$3,'ACTIVE','test','default-v1','T',now())`, uuid.New(), x, y)
		case "reverse":
			mustExec(t, l.cp, `DELETE FROM c_posts.blocked_authors WHERE account_id=$1`, a.AccountID)
		case "removed":
			mustExec(t, l.cp, `DELETE FROM c_posts.blocked_authors`)
		}
		for _, viewer := range values {
			for _, author := range values {
				var shared, bulk bool
				if e := runtime.QueryRow(ctx, `SELECT NOT c_posts.accounts_blocked($1::uuid,p.owner_account_id),`+publicListVisibleAuthorSQL("$1")+` FROM (VALUES($2::uuid)) p(owner_account_id)`, viewer, author).Scan(&shared, &bulk); e != nil {
					t.Fatal(e)
				}
				if shared != bulk {
					t.Fatalf("%s block predicate differs for %v/%v", mode, viewer, author)
				}
			}
		}
	}
}

// The projection must retain its own block check even when the caller uses
// READ COMMITTED and a block commits after candidate selection.
func TestFeedBatchProjectionFreshBlocksPostgres(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		t.Run(map[bool]string{false: "forward", true: "reverse"}[reverse], func(t *testing.T) {
			l := preparedFeedLab(t)
			ctx := context.Background()
			owner := sessionTestAccount(t, l, "projection_owner")
			reader := sessionTestAccount(t, l, "projection_reader")
			identity := identityChange(t, l.c, owner.SessionToken, "CREATE", uuid.Nil, "投影作者").IdentityID
			id := preparedFeedPublish(t, l, owner.SessionToken, identity, "visible")
			tx, e := l.cp.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
			if e != nil {
				t.Fatal(e)
			}
			defer tx.Rollback(ctx)
			q := PostQuery{Kind: PostFeed, Limit: 5}
			p, blocked, e := l.c.publicListPosition(ctx, tx, reader.AccountID, q, time.Now().UTC())
			if e != nil {
				t.Fatal(e)
			}
			ids, e := l.c.publicListCandidates(ctx, tx, reader.AccountID, q, p, blocked, nil)
			if e != nil || len(ids) != 1 || ids[0] != id {
				t.Fatal("candidate", ids, e)
			}
			a, b := reader.AccountID, owner.AccountID
			if reverse {
				a, b = b, a
			}
			mustExec(t, l.cp, `INSERT INTO c_posts.blocked_authors(block_id,account_id,author_account_id,author_state,nickname,avatar,short_code,created_at) VALUES($1,$2,$3,'ACTIVE','test','default-v1','T',now())`, uuid.New(), a, b)
			rows, e := l.c.publicListProjection(ctx, tx, reader.AccountID, ids, q, nil)
			if !errors.Is(e, pgx.ErrNoRows) || len(rows) != 0 {
				t.Fatal("projection reused stale eligibility", rows, e)
			}
		})
	}
}

// Mutations here commit between candidate selection and DTO projection in
// the same RR preparation. Final proof checking must rebuild before delivery.
func TestFeedBatchProjectionInterleavingPostgres(t *testing.T) {
	for _, mode := range []string{"block", "reverse_block", "hide", "delete", "profile"} {
		t.Run(mode, func(t *testing.T) {
			l := preparedFeedLab(t)
			ctx := context.Background()
			owner := sessionTestAccount(t, l, "projection_race_owner")
			reader := sessionTestAccount(t, l, "projection_race_reader")
			identity := identityChange(t, l.c, owner.SessionToken, "CREATE", uuid.Nil, "交错作者").IdentityID
			id := preparedFeedPublish(t, l, owner.SessionToken, identity, "selected")
			original := *l.c
			tracer := &preparedReadTracer{prefix: "/* list:projection */", change: func(n int) {
				if n != 1 {
					return
				}
				switch mode {
				case "block":
					if _, e := original.BlockPostAuthor(ctx, reader.SessionToken, id); e != nil {
						t.Fatal(e)
					}
				case "reverse_block":
					mustExec(t, l.cp, `INSERT INTO c_posts.blocked_authors(block_id,account_id,author_account_id,author_state,nickname,avatar,short_code,created_at) VALUES($1,$2,$3,'ACTIVE','test','default-v1','T',now())`, uuid.New(), owner.AccountID, reader.AccountID)
				case "hide":
					state, e := original.ReadHiddenPost(ctx, reader.SessionToken, id)
					if e != nil {
						t.Fatal(e)
					}
					if _, e = original.SetHiddenPost(ctx, reader.SessionToken, id, true, state.InteractionVersion); e != nil {
						t.Fatal(e)
					}
				case "delete":
					postTestCommand(t, &lab{c: &original, cp: l.cp}, owner.SessionToken, postTestKey(t), posts.Intent{Operation: posts.DeletePost, PostID: id})
				case "profile":
					identityChange(t, &original, owner.SessionToken, "RENAME", identity, "更新作者")
				}
			}}
			cfg := l.cp.Config()
			cfg.ConnConfig.Tracer = tracer
			pool, e := pgxpool.NewWithConfig(ctx, cfg)
			if e != nil {
				t.Fatal(e)
			}
			t.Cleanup(pool.Close)
			l.c.pool = pool
			r, e := l.c.ReadPosts(ctx, reader.SessionToken, PostQuery{Kind: PostFeed, Limit: 5})
			if e != nil {
				t.Fatal(e)
			}
			page := r.Value.(posts.CrossChannelFeedPage)
			if mode == "profile" {
				if tracer.calls != 2 || len(page.Items) != 1 || page.Items[0].Author.Nickname != "更新作者" {
					t.Fatal("stale profile", page, tracer.calls)
				}
			} else if tracer.calls != 1 || len(page.Items) != 0 || page.NextCursor != nil {
				t.Fatal("stale candidate escaped", mode, page, tracer.calls)
			}
		})
	}
}
