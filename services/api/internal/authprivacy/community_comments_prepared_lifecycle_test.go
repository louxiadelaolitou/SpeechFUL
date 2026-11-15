package authprivacy

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/comments"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/commentsnapshot"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/readtiming"
)

type preparedReadDurableState struct {
	gate                        string
	generation, version         int64
	highwater, activity, expiry time.Time
}

func preparedReadState(t *testing.T, l *lab, token [32]byte) preparedReadDurableState {
	t.Helper()
	var s preparedReadDurableState
	e := l.cp.QueryRow(context.Background(), `SELECT g.gate_state,g.authorization_generation,g.evidence_version,g.trusted_high_watermark,s.last_activity_at,s.expires_at
 FROM c_auth.authorization_gate g CROSS JOIN c_auth.sessions s WHERE g.singleton_id=1 AND s.token_digest=$1`, token[:]).Scan(&s.gate, &s.generation, &s.version, &s.highwater, &s.activity, &s.expiry)
	if e != nil {
		t.Fatal(e)
	}
	return s
}

func assertPreparedReadAnchorMatches(t *testing.T, l *lab, s preparedReadDurableState) {
	t.Helper()
	if e := l.gate.anchor.withLocked(func(a *authorizationAnchor, exists bool) error {
		if !exists || a.Frozen || s.gate != "OPEN" || a.Domain != l.gate.domain || a.Generation != uint64(s.generation) || a.Version != uint64(s.version) || !a.Highwater.Equal(s.highwater) {
			t.Fatal("SQL and independent anchor do not agree on an open durable state")
		}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
}

func preparedLifecycleScope(l *lab, account uuid.UUID, post string, cfg commentsnapshot.Config) commentsnapshot.Scope {
	return commentsnapshot.Scope{Owner: commentsnapshot.OwnerToken(l.c.comments.codec.IntentFingerprint, account, cfg.Redis.Namespace), Post: uuid.MustParse(post), Kind: comments.MainComments, Sort: comments.Relevant}
}

func preparedLifecycleLookup(t *testing.T, l *lab, store *commentsnapshot.Store, scope commentsnapshot.Scope, nonce string, at time.Time) (commentsnapshot.Snapshot, bool) {
	t.Helper()
	raw, e := snapshotNonce(nonce)
	if e != nil {
		t.Fatal(e)
	}
	s, found, e := store.Lookup(context.Background(), scope, commentsnapshot.Request(l.c.comments.codec.IntentFingerprint, scope.Owner, scope, raw), at)
	if e != nil {
		t.Fatal(e)
	}
	return s, found
}

func TestCommentPreparedCancellationBeforeCheckpointPostgres(t *testing.T) {
	l := commentTextLab(t)
	l.c.comments.preparedReads = true
	reader := sessionTestAccount(t, l, "prep_cancel_reader")
	identity := identityChange(t, l.c, reader.SessionToken, "CREATE", uuid.Nil, "取消作者").IdentityID
	post := commentTextPost(t, l, reader.SessionToken, identity)
	preparedComment(t, l, reader.SessionToken, post, identity, "")
	store, cfg := snapshotSQLStore(t, l)
	activeNonce := postTestKey(t)
	active := snapshotRead(t, l, reader.SessionToken, CommentQuery{ID: post, SnapshotRequest: activeNonce})
	token := sha256.Sum256(reader.SessionToken[:])
	mustExec(t, l.cp, `UPDATE c_auth.sessions SET expires_at=last_activity_at+interval '1 hour' WHERE token_digest=$1`, token[:])
	before := preparedReadState(t, l, token)
	ctx, trace := readtiming.Start(context.Background())
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	barrier := false
	preparedReadPool(t, l, func(n int) {
		if n == 1 {
			barrier = true
			cancel()
		}
	})
	r, e := l.c.ReadComments(ctx, reader.SessionToken, CommentQuery{ID: post, SnapshotRequest: postTestKey(t)})
	if e == nil || r.Value != nil || !r.ExpiresAt.IsZero() || !barrier {
		t.Fatal("pre-checkpoint cancellation released a prepared result", e)
	}
	stages := trace.Snapshot().Stages
	if stages["comments.prepare_read"].Calls != 1 || stages["comments.anchor_write"].Calls != 0 || stages["comments.sql_commit"].Calls != 0 || stages["comments.redis_publish"].Calls != 0 {
		t.Fatal("cancellation did not occur between preparation and checkpoint", stages)
	}
	after := preparedReadState(t, l, token)
	if !after.highwater.Equal(before.highwater) || !after.activity.Equal(before.activity) || !after.expiry.Equal(before.expiry) {
		t.Fatal("cancelled preparation advanced checkpoint or session")
	}
	assertPreparedReadAnchorMatches(t, l, after)
	scope := preparedLifecycleScope(l, reader.AccountID, post, cfg)
	old, found := preparedLifecycleLookup(t, l, store, scope, activeNonce, after.highwater)
	if !found || old.Ref.ID.String() != mustSnapshotUUID(t, active.SnapshotID).String() {
		t.Fatal("cancellation removed an existing active nonce")
	}
	usage, e := store.Usage(context.Background(), scope.Owner)
	if e != nil || usage.AccountCount != 1 {
		t.Fatal("cancelled preparation changed Redis quota", e, usage)
	}
}

func mustSnapshotUUID(t *testing.T, encoded string) uuid.UUID {
	t.Helper()
	data, e := base64.RawURLEncoding.DecodeString(encoded)
	if e != nil {
		t.Fatal(e)
	}
	id, e := uuid.FromBytes(data)
	if e != nil {
		t.Fatal(e)
	}
	return id
}

// commitACKProxy is confined to the private Unix socket of connectLab's
// already verified disposable C database. It relays the startup/authentication
// stream without changing it and drops exactly one durable COMMIT response.
type commitACKProxy struct {
	listener      net.Listener
	upstream      string
	armed, closed atomic.Bool
	dropped       chan struct{}
	mu            sync.Mutex
	connections   map[net.Conn]bool
	wg            sync.WaitGroup
}

func newCommitACKProxyPool(t *testing.T, source *pgxpool.Pool) (*pgxpool.Pool, *commitACKProxy) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("native PGwire fault fixture requires a Linux Unix socket")
	}
	cfg := source.Config()
	tag := os.Getenv("AUTHLAB_RUNTIME_TAG")
	if os.Getenv("AUTHLAB_ALLOW_SCHEMA_RESET") != "1" || !strings.HasPrefix(tag, "hnuhole_authlab_") || cfg.ConnConfig.RuntimeParams["application_name"] != tag || !filepath.IsAbs(cfg.ConnConfig.Host) || cfg.ConnConfig.TLSConfig != nil || len(cfg.ConnConfig.Fallbacks) != 0 {
		t.Fatal("proxy requires the verified private-socket authlab configuration")
	}
	dir := t.TempDir()
	socket := filepath.Join(dir, fmt.Sprintf(".s.PGSQL.%d", cfg.ConnConfig.Port))
	listener, e := net.Listen("unix", socket)
	if e != nil {
		t.Fatal(e)
	}
	p := &commitACKProxy{listener: listener, upstream: filepath.Join(cfg.ConnConfig.Host, fmt.Sprintf(".s.PGSQL.%d", cfg.ConnConfig.Port)), dropped: make(chan struct{}), connections: map[net.Conn]bool{}}
	p.wg.Add(1)
	go p.accept()
	t.Cleanup(func() {
		p.closed.Store(true)
		_ = p.listener.Close()
		p.mu.Lock()
		for c := range p.connections {
			_ = c.Close()
		}
		p.mu.Unlock()
		p.wg.Wait()
	})
	cfg.ConnConfig.Host = dir
	pool, e := pgxpool.NewWithConfig(context.Background(), cfg)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(pool.Close)
	// Retain connectLab's marker and recheck the endpoint reached through the
	// proxy. No caller-supplied DSN or external database is accepted here.
	var db, user string
	var super bool
	var address *string
	e = pool.QueryRow(context.Background(), `SELECT current_database(),current_user,(SELECT rolsuper FROM pg_roles WHERE rolname=current_user),inet_server_addr()::text`).Scan(&db, &user, &super, &address)
	if e != nil || db != "hnuhole_c" || user != "hnuhole_c" || super || address != nil {
		t.Fatal("proxy did not reach the owned disposable private C database", e)
	}
	return pool, p
}

func (p *commitACKProxy) accept() {
	defer p.wg.Done()
	for {
		front, e := p.listener.Accept()
		if e != nil {
			return
		}
		back, e := net.DialTimeout("unix", p.upstream, 3*time.Second)
		if e != nil {
			_ = front.Close()
			continue
		}
		p.mu.Lock()
		if p.closed.Load() {
			p.mu.Unlock()
			_ = front.Close()
			_ = back.Close()
			return
		}
		p.connections[front] = true
		p.connections[back] = true
		p.wg.Add(1)
		p.mu.Unlock()
		go p.relay(front, back)
	}
}

func (p *commitACKProxy) relay(front, back net.Conn) {
	defer p.wg.Done()
	frontendDone := make(chan struct{})
	go func() { _, _ = io.Copy(back, front); _ = back.Close(); close(frontendDone) }()
	defer func() {
		_ = front.Close()
		_ = back.Close()
		<-frontendDone
		p.mu.Lock()
		delete(p.connections, front)
		delete(p.connections, back)
		p.mu.Unlock()
	}()
	for {
		var header [5]byte
		if _, e := io.ReadFull(back, header[:]); e != nil {
			return
		}
		n := binary.BigEndian.Uint32(header[1:])
		if n < 4 || n > 16<<20 {
			return
		}
		payload := make([]byte, int(n)-4)
		if _, e := io.ReadFull(back, payload); e != nil {
			return
		}
		// PostgreSQL sends CommandComplete(COMMIT) only after finishing the
		// transaction. Drop this ACK and close the client before ReadyForQuery.
		if header[0] == 'C' && string(payload) == "COMMIT\x00" && p.armed.CompareAndSwap(true, false) {
			close(p.dropped)
			return
		}
		if _, e := front.Write(header[:]); e != nil {
			return
		}
		if _, e := front.Write(payload); e != nil {
			return
		}
	}
}

func TestCommentPreparedCommitACKLossPostgres(t *testing.T) {
	l := commentTextLab(t)
	l.c.comments.preparedReads = true
	reader := sessionTestAccount(t, l, "prep_commit_reader")
	identity := identityChange(t, l.c, reader.SessionToken, "CREATE", uuid.Nil, "提交作者").IdentityID
	post := commentTextPost(t, l, reader.SessionToken, identity)
	preparedComment(t, l, reader.SessionToken, post, identity, "")
	preparedComment(t, l, reader.SessionToken, post, identity, "")
	store, cfg := snapshotSQLStore(t, l)
	activeNonce := postTestKey(t)
	active := snapshotRead(t, l, reader.SessionToken, CommentQuery{ID: post, Limit: 1, SnapshotRequest: activeNonce})
	if active.NextCursor == nil {
		t.Fatal("fixture did not create protected content and a continuation")
	}
	token := sha256.Sum256(reader.SessionToken[:])
	mustExec(t, l.cp, `UPDATE c_auth.sessions SET expires_at=last_activity_at+interval '1 hour' WHERE token_digest=$1`, token[:])
	before := preparedReadState(t, l, token)
	pool, proxy := newCommitACKProxyPool(t, l.cp)
	l.c.pool = pool
	if !l.c.nativePreparedReads() {
		t.Fatal("unknown-commit test accidentally selected custom-Gate fallback")
	}
	newNonce := postTestKey(t)
	ctx, trace := readtiming.Start(context.Background())
	proxy.armed.Store(true)
	r, e := l.c.ReadComments(ctx, reader.SessionToken, CommentQuery{ID: post, Limit: 1, SnapshotRequest: newNonce})
	if !errors.Is(e, ErrAuthorizationUnavailable) || r.Value != nil || !r.ExpiresAt.IsZero() {
		t.Fatal("unknown COMMIT response released a body or cursor", e)
	}
	select {
	case <-proxy.dropped:
	default:
		t.Fatal("proxy did not drop a real COMMIT CommandComplete")
	}
	stages := trace.Snapshot().Stages
	if stages["comments.prepare_read"].Calls != 1 || stages["comments.anchor_write"].Calls != 1 || stages["comments.sql_commit"].Calls != 1 || stages["comments.redis_publish"].Calls != 0 {
		t.Fatal("unknown commit did not exercise the native prepared finalization", stages)
	}
	after := preparedReadState(t, l, token)
	// This is an unknown client result, not a rolled-back transaction. The
	// persisted checkpoint and legitimate session renewal must remain visible.
	if !after.highwater.After(before.highwater) || !after.activity.Equal(after.highwater) || !after.expiry.After(before.expiry) {
		t.Fatal("COMMIT was not durably applied before its ACK was lost")
	}
	assertPreparedReadAnchorMatches(t, l, after)
	scope := preparedLifecycleScope(l, reader.AccountID, post, cfg)
	if _, found := preparedLifecycleLookup(t, l, store, scope, newNonce, after.highwater); found {
		t.Fatal("unconfirmed request published a new Redis nonce")
	}
	old, found := preparedLifecycleLookup(t, l, store, scope, activeNonce, after.highwater)
	if !found || old.Ref.ID != mustSnapshotUUID(t, active.SnapshotID) {
		t.Fatal("unknown commit compensation deleted an existing ACTIVE snapshot")
	}
	usage, e := store.Usage(context.Background(), scope.Owner)
	if e != nil || usage.AccountCount != 1 {
		t.Fatal("unknown commit leaked a new Redis reservation", e, usage)
	}
	t.Log("durable SQL checkpoint and session renewal matched the open anchor; body/cursor withheld; only the new Redis reservation was aborted")
}
