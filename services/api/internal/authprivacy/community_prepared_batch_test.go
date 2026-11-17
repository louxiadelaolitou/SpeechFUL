package authprivacy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/posts"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/readtiming"
)

// Retain the pre-change query as an independent semantic oracle, not a second
// production implementation. No persisted protocol uses either representation.
const legacyFeedProofSQL = `WITH channels AS (
 SELECT id FROM public.channels WHERE $3::uuid IS NULL OR id=$3
), sources AS (
 SELECT p.post_id,p.owner_account_id,t.identity_id FROM c_posts.posts p
 JOIN c_posts.publication_tasks t ON t.post_id=p.post_id WHERE p.post_id=ANY($2::uuid[])
), identities AS (SELECT DISTINCT identity_id FROM sources),
 accounts AS (SELECT owner_account_id AS account_id FROM sources UNION SELECT $1::uuid)
 SELECT jsonb_build_object(
 'channels',(SELECT coalesce(jsonb_agg(jsonb_build_array(c.id,v.membership_revision,CASE WHEN $4 THEN v.publication_revision ELSE 0 END) ORDER BY c.id),'[]'::jsonb) FROM channels c LEFT JOIN c_posts.read_feed_scope_versions v ON v.channel_id=c.id),
 'posts',(SELECT coalesce(jsonb_agg(jsonb_build_array(s.post_id,v.projection_revision) ORDER BY s.post_id),'[]'::jsonb) FROM sources s LEFT JOIN c_posts.read_feed_post_versions v USING(post_id)),
 'identities',(SELECT coalesce(jsonb_agg(jsonb_build_array(i.identity_id,v.projection_revision) ORDER BY i.identity_id),'[]'::jsonb) FROM identities i LEFT JOIN c_posts.read_identity_versions v USING(identity_id)),
 'accounts',(SELECT coalesce(jsonb_agg(jsonb_build_array(a.account_id,v.projection_revision) ORDER BY a.account_id),'[]'::jsonb) FROM accounts a LEFT JOIN c_posts.read_account_versions v USING(account_id)),
 'viewer',(SELECT visibility_revision FROM c_posts.read_account_versions WHERE account_id=$1),
 'filters',coalesce((SELECT revision FROM c_posts.feed_preferences WHERE account_id=$1),0)),
 NOT EXISTS(SELECT 1 FROM channels c LEFT JOIN c_posts.read_feed_scope_versions v ON v.channel_id=c.id WHERE v.channel_id IS NULL)
 AND NOT EXISTS(SELECT 1 FROM sources s LEFT JOIN c_posts.read_feed_post_versions v USING(post_id) WHERE v.post_id IS NULL)
 AND NOT EXISTS(SELECT 1 FROM identities i LEFT JOIN c_posts.read_identity_versions v USING(identity_id) WHERE v.identity_id IS NULL)
 AND NOT EXISTS(SELECT 1 FROM accounts a LEFT JOIN c_posts.read_account_versions v USING(account_id) WHERE v.account_id IS NULL)`

func legacyFeedProofBytes(t *testing.T, raw []byte, viewer uuid.UUID) []byte {
	t.Helper()
	var proof struct {
		Channels, Posts, Identities, Accounts [][]json.RawMessage
		Viewer, Filters                       int64
	}
	if e := json.Unmarshal(raw, &proof); e != nil {
		t.Fatal(e)
	}
	type record struct {
		kind                uint32
		id                  uuid.UUID
		revision, secondary int64
	}
	records := []record{}
	for kind, group := range [][][]json.RawMessage{proof.Channels, proof.Posts, proof.Identities, proof.Accounts} {
		for _, entry := range group {
			r := record{kind: uint32(kind)}
			if e := json.Unmarshal(entry[0], &r.id); e != nil {
				t.Fatal(e)
			}
			if e := json.Unmarshal(entry[1], &r.revision); e != nil {
				t.Fatal(e)
			}
			if kind == 0 {
				if e := json.Unmarshal(entry[2], &r.secondary); e != nil {
					t.Fatal(e)
				}
			}
			if kind == 3 && r.id == viewer {
				r.secondary = proof.Viewer
			}
			records = append(records, r)
		}
	}
	records = append(records, record{kind: 4, id: viewer, revision: proof.Filters})
	sort.Slice(records, func(i, j int) bool {
		if records[i].kind != records[j].kind {
			return records[i].kind < records[j].kind
		}
		return bytes.Compare(records[i].id[:], records[j].id[:]) < 0
	})
	var out bytes.Buffer
	for _, r := range records {
		for _, part := range []any{r.kind, r.id, r.revision, r.secondary} {
			if e := binary.Write(&out, binary.BigEndian, part); e != nil {
				t.Fatal(e)
			}
		}
	}
	return out.Bytes()
}

func TestFeedBinaryProofEquivalencePostgres(t *testing.T) {
	l := preparedFeedLab(t)
	ctx := context.Background()
	owner := sessionTestAccount(t, l, "binary_owner")
	reader := sessionTestAccount(t, l, "binary_reader")
	identity := identityChange(t, l.c, owner.SessionToken, "CREATE", uuid.Nil, "proof").IdentityID
	ids := []string{}
	for n := 0; n < 12; n++ {
		ids = append(ids, postTestCommand(t, l, owner.SessionToken, postTestKey(t), postTestIntent(t, l, identity)).PostID)
	}
	postTestPublish(t, l)
	var channel string
	if e := l.cp.QueryRow(ctx, `SELECT channel_id::text FROM c_posts.posts WHERE post_id=$1`, ids[0]).Scan(&channel); e != nil {
		t.Fatal(e)
	}
	check := func(tx pgx.Tx, p feedReadProof) {
		t.Helper()
		var original []byte
		var intact bool
		if e := tx.QueryRow(ctx, legacyFeedProofSQL, reader.AccountID, p.ids, nullablePostID(p.channel), p.initial).Scan(&original, &intact); e != nil {
			t.Fatal(e)
		}
		got, e := p.read(ctx, tx, reader.AccountID)
		if !intact {
			if !errors.Is(e, ErrAuthorizationUnavailable) {
				t.Fatal("missing version accepted", e)
			}
			return
		}
		if e != nil || !bytes.Equal(got, legacyFeedProofBytes(t, original, reader.AccountID)) {
			t.Fatal("binary proof lost a dependency", e)
		}
	}
	tx, e := l.cp.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	// Values above 2^53 must stay exact, with no floating-point JSON coercion.
	if _, e = tx.Exec(ctx, `UPDATE c_posts.read_feed_post_versions SET projection_revision=9007199254740993 WHERE post_id=$1`, ids[0]); e != nil {
		t.Fatal(e)
	}
	for _, scope := range []string{"", channel, uuid.NewString()} {
		for _, initial := range []bool{false, true} {
			for _, size := range []int{0, 1, 5, 12} {
				check(tx, feedReadProof{ids: ids[:size], channel: scope, initial: initial})
			}
		}
	}
	before, e := (&feedReadProof{ids: ids, initial: true}).read(ctx, tx, reader.AccountID)
	if e != nil {
		t.Fatal(e)
	}
	reversed := append([]string{}, ids...)
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}
	after, e := (&feedReadProof{ids: reversed, initial: true}).read(ctx, tx, reader.AccountID)
	if e != nil || !bytes.Equal(before, after) {
		t.Fatal("noncanonical proof order", e)
	}
	if e = tx.Rollback(ctx); e != nil {
		t.Fatal(e)
	}
	for _, missing := range []struct {
		sql string
		id  any
	}{
		{`DELETE FROM c_posts.read_feed_scope_versions WHERE channel_id=$1`, channel},
		{`DELETE FROM c_posts.read_feed_post_versions WHERE post_id=$1`, ids[0]},
		{`DELETE FROM c_posts.read_identity_versions WHERE identity_id=$1`, identity},
		{`DELETE FROM c_posts.read_account_versions WHERE account_id=$1`, owner.AccountID},
		{`DELETE FROM c_posts.read_account_versions WHERE account_id=$1`, reader.AccountID},
	} {
		tx, e := l.cp.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = tx.Exec(ctx, missing.sql, missing.id); e != nil {
			t.Fatal(e)
		}
		check(tx, feedReadProof{ids: ids, initial: true})
		if e = tx.Rollback(ctx); e != nil {
			t.Fatal(e)
		}
	}
}

func TestPreparedLockBatchOrderCancellationAndReusePostgres(t *testing.T) {
	l := preparedFeedLab(t)
	ctx := context.Background()
	reader := sessionTestAccount(t, l, "ordered_batch")
	token := sha256.Sum256(reader.SessionToken[:])
	cfg := l.cp.Config()
	cfg.MaxConns = 1
	cfg.MinConns = 0
	pool, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	locks := []struct {
		name, sql string
		args      []any
	}{
		{"account", `SELECT 1 FROM c_auth.accounts WHERE account_id=$1 FOR UPDATE`, []any{reader.AccountID}},
		{"restriction", `SELECT 1 FROM c_auth.account_restrictions WHERE account_id=$1 FOR UPDATE`, []any{reader.AccountID}},
		{"session", `SELECT 1 FROM c_auth.sessions WHERE token_digest=$1 FOR UPDATE`, []any{token[:]}},
	}
	for target, lock := range locks {
		for _, cancelled := range []bool{false, true} {
			t.Run(lock.name+map[bool]string{false: "/timeout", true: "/cancel"}[cancelled], func(t *testing.T) {
				blocker, e := l.cp.Begin(ctx)
				if e != nil {
					t.Fatal(e)
				}
				defer blocker.Rollback(ctx)
				var one int
				if e = blocker.QueryRow(ctx, lock.sql, lock.args...).Scan(&one); e != nil {
					t.Fatal(e)
				}
				tx, e := begin(ctx, pool)
				if e != nil {
					t.Fatal(e)
				}
				defer tx.Rollback(ctx)
				if _, e = tx.Exec(ctx, `SET LOCAL lock_timeout='300ms'`); e != nil {
					t.Fatal(e)
				}
				var pid uint32
				if e = tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); e != nil {
					t.Fatal(e)
				}
				request, cancel := context.WithCancel(ctx)
				defer cancel()
				done := make(chan error, 1)
				go func() { _, _, _, e := lockPreparedPostSession(request, tx, reader.AccountID, token); done <- e }()
				waiting := false
				deadline := time.Now().Add(2 * time.Second)
				for time.Now().Before(deadline) {
					if e = l.cp.QueryRow(ctx, `SELECT coalesce(wait_event_type='Lock',false) FROM pg_stat_activity WHERE pid=$1`, pid).Scan(&waiting); e != nil {
						t.Fatal(e)
					}
					if waiting {
						break
					}
					time.Sleep(time.Millisecond)
				}
				if !waiting {
					t.Fatal("batch never reached the blocked command")
				}
				for n, probe := range locks {
					if n == target {
						continue
					}
					probeTx, e := l.cp.Begin(ctx)
					if e != nil {
						t.Fatal(e)
					}
					e = probeTx.QueryRow(ctx, probe.sql+` NOWAIT`, probe.args...).Scan(&one)
					var pgerr *pgconn.PgError
					blocked := errors.As(e, &pgerr) && pgerr.Code == "55P03"
					if n < target && !blocked || n > target && e != nil {
						t.Fatal("lock order changed", target, n, e)
					}
					_ = probeTx.Rollback(ctx)
				}
				if cancelled {
					cancel()
				}
				select {
				case e = <-done:
					if e == nil {
						t.Fatal("blocked/cancelled batch succeeded")
					}
				case <-time.After(3 * time.Second):
					t.Fatal("batch did not drain")
				}
				_ = tx.Rollback(ctx)
				_ = blocker.Rollback(ctx)
				// With one pool permit, actual bounded reuse proves there is no retained
				// batch/transaction lease; a destroyed connection may be replaced by pgx.
				reuse, cancelReuse := context.WithTimeout(ctx, 2*time.Second)
				defer cancelReuse()
				conn, e := pool.Acquire(reuse)
				if e != nil {
					t.Fatal("batch retained pool lease", e)
				}
				defer conn.Release()
				if conn.Conn().PgConn().TxStatus() != 'I' {
					t.Fatal("transaction leaked into pool")
				}
				var isolation, access, timeout string
				if e = conn.QueryRow(reuse, `SELECT current_setting('transaction_isolation'),current_setting('transaction_read_only'),current_setting('lock_timeout')`).Scan(&isolation, &access, &timeout); e != nil || isolation != "read committed" || access != "off" || timeout != "0" {
					t.Fatal("batch leaked local transaction state", e, isolation, access, timeout)
				}
				for _, probe := range locks {
					if e = conn.QueryRow(reuse, probe.sql+` NOWAIT`, probe.args...).Scan(&one); e != nil {
						t.Fatal("row lock leaked", e)
					}
				}
			})
		}
	}
}

type preparedBatchCloseFailure struct{ pgx.Tx }
type preparedBatchCloseResult struct{ pgx.BatchResults }

func (tx preparedBatchCloseFailure) SendBatch(ctx context.Context, batch *pgx.Batch) pgx.BatchResults {
	return preparedBatchCloseResult{tx.Tx.SendBatch(ctx, batch)}
}
func (r preparedBatchCloseResult) Close() error {
	if e := r.BatchResults.Close(); e != nil {
		return e
	}
	return errors.New("injected batch close failure")
}

func TestPreparedBatchCountsAndCloseFailurePostgres(t *testing.T) {
	l := preparedFeedLab(t)
	ctx := context.Background()
	reader := sessionTestAccount(t, l, "batch_close")
	identity := identityChange(t, l.c, reader.SessionToken, "CREATE", uuid.Nil, "counts").IdentityID
	id := preparedFeedPublish(t, l, reader.SessionToken, identity, "counter")
	token := sha256.Sum256(reader.SessionToken[:])
	p := feedReadProof{ids: []string{id}, initial: true}
	tx, e := l.cp.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	p.versions, e = p.read(ctx, tx, reader.AccountID)
	if e != nil {
		t.Fatal(e)
	}
	counter := &listCountingTx{Tx: tx}
	if _, _, _, e = lockPreparedPostSession(ctx, counter, reader.AccountID, token); e != nil || counter.queries != 3 {
		t.Fatal("lock batch did not count three SQL statements", e, counter.queries)
	}
	counter.queries = 0
	counts, e := p.revalidateWithViewCounts(ctx, counter, reader.AccountID, []string{id})
	if e != nil || len(counts) != 1 || counter.queries != 2 {
		t.Fatal("proof/counts did not count two SQL statements", e, counter.queries)
	}
	if _, _, _, e = lockPreparedPostSession(ctx, preparedBatchCloseFailure{tx}, reader.AccountID, token); !errors.Is(e, ErrAuthorizationUnavailable) {
		t.Fatal("close failure accepted locked authority", e)
	}
	counts, e = p.revalidateWithViewCounts(ctx, preparedBatchCloseFailure{tx}, reader.AccountID, []string{id})
	if e == nil || counts != nil {
		t.Fatal("close failure released counts", e, counts)
	}
	p.versions = append(p.versions, 1)
	counts, e = p.revalidateWithViewCounts(ctx, tx, reader.AccountID, []string{id})
	if !errors.Is(e, errReadConflict) || counts != nil {
		t.Fatal("conflicting proof released queued counts", e, counts)
	}
	var one int
	if e = tx.QueryRow(ctx, `SELECT 1`).Scan(&one); e != nil {
		t.Fatal("conflict left batch undrained", e)
	}
}

type preparedCountTracer struct {
	preparedReadTracer
	countQueries int
}

func (q *preparedCountTracer) TraceBatchQuery(_ context.Context, _ *pgx.Conn, data pgx.TraceBatchQueryData) {
	if data.SQL == preparedViewCountsSQL {
		q.countQueries++
	}
}

func TestFeedPreparedBatchFreshCountsPostgres(t *testing.T) {
	for _, mode := range []string{"fresh", "conflict", "expired"} {
		t.Run(mode, func(t *testing.T) {
			control := newGateControl(t)
			l := control.lab
			ctx := context.Background()
			cfg := enablePostLab(t, l)
			cfg.PreparedFeedReads = true
			var e error
			l.c, e = l.c.WithPosts(ctx, cfg)
			if e != nil {
				t.Fatal(e)
			}
			reader := sessionTestAccount(t, l, "fresh_counts")
			identity := identityChange(t, l.c, reader.SessionToken, "CREATE", uuid.Nil, "counter").IdentityID
			old := preparedFeedPublish(t, l, reader.SessionToken, identity, "older")
			latest := preparedFeedPublish(t, l, reader.SessionToken, identity, "newer")
			mustExec(t, l.cp, `INSERT INTO c_posts.post_view_totals(post_id,view_count) VALUES($1,3),($2,4) ON CONFLICT(post_id) DO UPDATE SET view_count=excluded.view_count`, old, latest)
			query := PostQuery{Kind: PostFeed, Limit: 1}
			if mode == "expired" {
				r, e := l.c.ReadPosts(ctx, reader.SessionToken, query)
				if e != nil {
					t.Fatal(e)
				}
				query.Cursor = *r.Value.(posts.CrossChannelFeedPage).NextCursor
			}
			token := sha256.Sum256(reader.SessionToken[:])
			before := preparedReadState(t, l, token)
			original := *l.c
			tracer := &preparedCountTracer{}
			tracer.change = func(n int) {
				if n == 1 {
					mustExec(t, l.cp, `UPDATE c_posts.post_view_totals SET view_count=9`)
				} else {
					mustExec(t, l.cp, `UPDATE c_posts.post_view_totals SET view_count=17`)
				}
				if mode == "conflict" && n == 1 {
					identityChange(t, &original, reader.SessionToken, "RENAME", identity, "changed")
				}
				if mode == "expired" && n == 1 {
					advanceClosureClock(t, control, control.clock.now().Add(posts.CursorTTL+time.Second))
				}
			}
			poolCfg := l.cp.Config()
			poolCfg.ConnConfig.Tracer = tracer
			pool, e := pgxpool.NewWithConfig(ctx, poolCfg)
			if e != nil {
				t.Fatal(e)
			}
			defer pool.Close()
			l.c.pool = pool
			result, e := l.c.ReadPosts(ctx, reader.SessionToken, query)
			if mode == "expired" {
				after := preparedReadState(t, l, token)
				if e == nil || result.Value != nil || !result.ExpiresAt.IsZero() || tracer.calls != 1 || tracer.countQueries != 1 || !after.activity.Equal(before.activity) || !after.expiry.Equal(before.expiry) {
					t.Fatal("expired final cursor released counts or renewed", e, tracer.calls, tracer.countQueries)
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			page := result.Value.(posts.CrossChannelFeedPage)
			expected, calls := int64(9), 1
			if mode == "conflict" {
				expected, calls = 17, 2
			}
			if tracer.calls != calls || tracer.countQueries != calls || len(page.Items) != 1 || page.Items[0].ViewCount != expected {
				t.Fatal("counts were stale, skipped or read twice in an attempt", page, tracer.calls, tracer.countQueries)
			}
		})
	}
}

func TestFeedPreparedBatchRestrictedRuntimePostgres(t *testing.T) {
	l := preparedFeedLab(t)
	ctx := context.Background()
	reader := sessionTestAccount(t, l, "batch_runtime")
	identity := identityChange(t, l.c, reader.SessionToken, "CREATE", uuid.Nil, "runtime").IdentityID
	id := preparedFeedPublish(t, l, reader.SessionToken, identity, "protected")
	mustExec(t, l.cp, `INSERT INTO c_posts.post_view_totals(post_id,view_count) VALUES($1,11) ON CONFLICT(post_id) DO UPDATE SET view_count=excluded.view_count`, id)
	mustExec(t, l.cp, `CREATE TABLE IF NOT EXISTS public.goose_db_version(version_id bigint,is_applied boolean)`)
	grant, e := os.ReadFile("../../../../infra/postgres/grant-community.sql")
	if e != nil {
		t.Fatal(e)
	}
	mustExec(t, l.cp, string(grant))
	dsn := os.Getenv("AUTHLAB_POSTS_RUNTIME_DSN")
	if dsn == "" {
		t.Fatal("owned least-privilege fixture required")
	}
	pool, e := pgxpool.New(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	var actor string
	if e = pool.QueryRow(ctx, `SELECT current_user`).Scan(&actor); e != nil || actor != "hnuhole_c_runtime" {
		t.Fatal("not the restricted runtime", e, actor)
	}
	clone := *l.c
	clone.pool = pool
	if e = clone.checkPreparedFeedReadSchema(ctx); e != nil {
		t.Fatal(e)
	}
	traced, trace := readtiming.Start(ctx)
	r, e := clone.ReadPosts(traced, reader.SessionToken, PostQuery{Kind: PostFeed, Limit: 5})
	if e != nil {
		t.Fatal(e)
	}
	page := r.Value.(posts.CrossChannelFeedPage)
	if len(page.Items) != 1 || page.Items[0].PostID != id || page.Items[0].ViewCount != 11 || trace.Snapshot().Stages["feed.anchor_write"].Calls != 1 || trace.Snapshot().Stages["feed.list_proof"].Calls != 2 {
		t.Fatal("restricted complete prepared path did not finish", page)
	}
}

func TestPreparedBatchErrorObservationPostgres(t *testing.T) {
	l := newLab(t)
	ctx := context.Background()
	var errorsSeen []string
	cfg := l.cp.Config()
	cfg.ConnConfig.Tracer = baselinePoolTracer{reportError: func(code, _ string, _ uint32) { errorsSeen = append(errorsSeen, code) }}
	pool, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	for _, failure := range []struct{ sql, code string }{{"SELECT 1/0", "22012"}, {"SELECT FROM", "42601"}} {
		errorsSeen = nil
		tx, e := pool.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		batch := &pgx.Batch{}
		batch.Queue(failure.sql)
		batch.Queue("SELECT 1")
		result := tx.SendBatch(ctx, batch)
		var one int
		scanErr := result.QueryRow().Scan(&one)
		closeErr := result.Close()
		_ = tx.Rollback(ctx)
		if scanErr == nil || closeErr == nil || len(errorsSeen) != 1 || errorsSeen[0] != failure.code {
			t.Fatal("batch SQL failure missing or counted twice", failure.code, errorsSeen, scanErr, closeErr)
		}
	}
}

// Count statements from both protocols. Geometry must begin after the entire
// projection/proof batch closes; the returned proof remains fresh on both sides.
type preparedProjectionTracer struct {
	open, body, proof, counts int
	geometryDuringBatch       bool
}

func (q *preparedProjectionTracer) record(sql string) {
	if strings.HasPrefix(sql, "/* list:") {
		q.body++
	}
	if sql == feedReadVersionsSQL {
		q.proof++
	}
	if sql == preparedViewCountsSQL {
		q.counts++
	}
	if strings.HasPrefix(sql, "/* list:images */") && q.open != 0 {
		q.geometryDuringBatch = true
	}
}
func (q *preparedProjectionTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	q.record(d.SQL)
	return ctx
}
func (*preparedProjectionTracer) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}
func (q *preparedProjectionTracer) TraceBatchStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceBatchStartData) context.Context {
	q.open++
	return ctx
}
func (q *preparedProjectionTracer) TraceBatchQuery(_ context.Context, _ *pgx.Conn, d pgx.TraceBatchQueryData) {
	q.record(d.SQL)
}
func (q *preparedProjectionTracer) TraceBatchEnd(context.Context, *pgx.Conn, pgx.TraceBatchEndData) {
	q.open--
}

func checkNativePreparedProjection(t *testing.T, l *lab, bearer [32]byte, limit, body int) any {
	t.Helper()
	ctx := context.Background()
	tracer := &preparedProjectionTracer{}
	cfg := l.cp.Config()
	cfg.ConnConfig.Tracer = tracer
	pool, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	clone := *l.c
	clone.pool = pool
	clone.posts.preparedFeedReads = true
	traced, spans := readtiming.Start(ctx)
	result, e := clone.ReadPosts(traced, bearer, PostQuery{Kind: PostFeed, Limit: limit, ImageSizes: true})
	if e != nil {
		t.Fatal(e)
	}
	if tracer.body != body || tracer.proof != 2 || tracer.counts != 1 || tracer.open != 0 || tracer.geometryDuringBatch || spans.Snapshot().Stages["feed.list_proof"].Calls != 2 || spans.Snapshot().Stages["feed.anchor_write"].Calls != 1 {
		t.Fatalf("native prepared SQL / batch boundary: %+v", tracer)
	}
	t.Logf("native prepared limit%d: %d body + 2 fresh proof + 1 final counts; one checkpoint", limit, tracer.body)
	return result.Value
}

func TestFeedPreparedProjectionBatchFailurePostgres(t *testing.T) {
	l := preparedFeedLab(t)
	ctx := context.Background()
	owner := sessionTestAccount(t, l, "projection_failure")
	identity := identityChange(t, l.c, owner.SessionToken, "CREATE", uuid.Nil, "projection").IdentityID
	id := preparedFeedPublish(t, l, owner.SessionToken, identity, "body")
	cfg := l.cp.Config()
	cfg.MaxConns = 1
	cfg.MinConns = 0
	pool, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	for _, fault := range []string{"close", "cancel-proof"} {
		t.Run(fault, func(t *testing.T) {
			tx, e := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
			if e != nil {
				t.Fatal(e)
			}
			defer tx.Rollback(ctx)
			proof := &feedReadProof{ids: []string{id}, initial: true}
			q := PostQuery{Kind: PostFeed, Limit: 5, deferViewCounts: true}
			if fault == "close" {
				_, e = l.c.publicListProjection(ctx, preparedBatchCloseFailure{tx}, owner.AccountID, []string{id}, q, proof)
			} else {
				blocker, be := l.cp.Begin(ctx)
				if be != nil {
					t.Fatal(be)
				}
				defer blocker.Rollback(ctx)
				if _, be = blocker.Exec(ctx, `LOCK TABLE c_posts.read_feed_post_versions IN ACCESS EXCLUSIVE MODE`); be != nil {
					t.Fatal(be)
				}
				pid := tx.Conn().PgConn().PID()
				request, cancel := context.WithCancel(ctx)
				defer cancel()
				done := make(chan error, 1)
				go func() {
					_, failure := l.c.publicListProjection(request, tx, owner.AccountID, []string{id}, q, proof)
					done <- failure
				}()
				waiting := false
				deadline := time.Now().Add(2 * time.Second)
				for time.Now().Before(deadline) {
					if be = l.cp.QueryRow(ctx, `SELECT coalesce(wait_event_type='Lock',false) FROM pg_stat_activity WHERE pid=$1`, pid).Scan(&waiting); be != nil {
						t.Fatal(be)
					}
					if waiting {
						break
					}
					time.Sleep(time.Millisecond)
				}
				if !waiting {
					t.Fatal("proof never blocked")
				}
				cancel()
				select {
				case e = <-done:
				case <-time.After(3 * time.Second):
					t.Fatal("projection/proof batch did not drain")
				}
				_ = blocker.Rollback(ctx)
			}
			if e == nil || proof.prepared {
				t.Fatal("failed batch published a prepared proof", e)
			}
			_ = tx.Rollback(ctx)
			reuse, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			conn, e := pool.Acquire(reuse)
			if e != nil {
				t.Fatal("projection batch retained lease", e)
			}
			defer conn.Release()
			var one int
			if conn.Conn().PgConn().TxStatus() != 'I' {
				t.Fatal("leaked transaction")
			}
			if e = conn.QueryRow(reuse, `SELECT 1`).Scan(&one); e != nil || one != 1 {
				t.Fatal("pool not reusable", e)
			}
		})
	}
}
