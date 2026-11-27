package authprivacy

import (
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/posts"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/recommendation"
	rm "github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/recommendation/roundmodel"
)

// Explicit test port only: checks current IDs/scope with one SELECT. It does
// not implement A's content/author/image integrity projection or any HTTP DTO.
type recommendationTestPort struct {
	mu      sync.Mutex
	failure error
}

func (p *recommendationTestPort) ProjectRecommendation(ctx context.Context, tx pgx.Tx, s RecommendationScope, ids []rm.ID) (RecommendationProjection, error) {
	p.mu.Lock()
	failure := p.failure
	p.mu.Unlock()
	if failure != nil {
		return RecommendationProjection{}, failure
	}
	rows, e := tx.Query(ctx, `SELECT p.post_id::text FROM unnest($1::uuid[]) ids(id) JOIN c_posts.posts p ON p.post_id=ids.id JOIN c_posts.publication_tasks t ON t.post_id=p.post_id JOIN c_posts.attempt_contents x ON x.task_id=t.task_id AND x.version=p.published_attempt_version WHERE p.visibility='PUBLISHED' AND NOT p.moderation_hidden AND x.erased_at IS NULL AND p.channel_id=ANY($2::uuid[])`, recommendationIDs(ids), s.Channels)
	if e != nil {
		return RecommendationProjection{}, e
	}
	visible := map[string]bool{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			break
		}
		visible[id] = true
	}
	if e == nil {
		e = rows.Err()
	}
	rows.Close()
	if e != nil {
		return RecommendationProjection{}, e
	}
	result := RecommendationProjection{Unavailable: []int{}}
	for i, id := range ids {
		if !visible[string(id)] {
			result.Unavailable = append(result.Unavailable, i)
		}
	}
	return result, nil
}

type recommendationPG struct {
	l          *lab
	runtime    *pgxpool.Pool
	service    *Community
	port       *recommendationTestPort
	postConfig PostConfig
	config     RecommendationConfig
	control    *gateControl
	trace      *recommendationSQLTrace
	author     SignupResult
}

func newRecommendationPG(t *testing.T, postCount int) *recommendationPG {
	t.Helper()
	if os.Getenv("FEED_RECOMMENDATION_OWNED") != "1" {
		t.Skip("owned feed recommendation fixture absent")
	}
	control := newGateControl(t)
	l := control.lab
	postConfig := enablePostLab(t, l)
	owner := sessionTestAccount(t, l, "reco_author")
	identity := identityChange(t, l.c, owner.SessionToken, "CREATE", uuid.Nil, "合成作者")
	for i := 0; i < postCount; i++ {
		postTestCommand(t, l, owner.SessionToken, postTestKey(t), postTestIntent(t, l, identity.IdentityID))
	}
	if postCount > 0 {
		postTestPublish(t, l)
	}
	recommendationGrant(t, l)
	config, e := pgxpool.ParseConfig(os.Getenv("AUTHLAB_POSTS_RUNTIME_DSN"))
	if e != nil {
		t.Fatal(e)
	}
	config.MaxConns = 4
	trace := &recommendationSQLTrace{}
	config.ConnConfig.Tracer = trace
	runtime, e := pgxpool.NewWithConfig(context.Background(), config)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(runtime.Close)
	if e = checkRecommendationSchema(context.Background(), runtime, true); e != nil {
		t.Fatal("actual restricted runtime schema", e)
	}
	copy := *l.c
	copy.pool = runtime
	gate := *l.gate
	gate.pool = runtime
	copy.gate = &gate
	port := &recommendationTestPort{}
	now := control.clock.now().UTC().Truncate(time.Microsecond)
	profile := rm.Digest(sha256.Sum256([]byte("B3B/SYNTHETIC/FUNCTIONAL/NOT-A-MEASUREMENT")))
	cfg := RecommendationConfig{Enabled: true, FeedbackEnabled: true, LikeTimeEnabled: true, Selector: recommendation.ExperimentalConfig(), Policy: rm.ExperimentalPolicy(), Capacity: rm.ConservedCapacity{Profile: profile, MeasuredAt: now.Add(-time.Minute), ValidUntil: now.Add(30 * time.Minute), MinUnitsPerSecond: 100000, MaxPause: time.Second, WorstGrowthBytesPerMutation: 65536}, NewRound: func() (uuid.UUID, uint64, error) { id, e := uuid.NewRandom(); return id, 20261025, e }, syntheticLab: true}
	if _, e = l.cp.Exec(context.Background(), `UPDATE c_recommendation.control SET enabled=true,admission_enabled=true,feedback_enabled=true,like_time_enabled=true,profile_hash=$1,baseline_file_bytes=(SELECT sum(pg_total_relation_size(c.oid)) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='c_recommendation' AND c.relkind='r')`, profile[:]); e != nil {
		t.Fatal(e)
	}
	service, e := copy.WithRecommendation(context.Background(), cfg, port)
	if e != nil {
		t.Fatal(e)
	}
	return &recommendationPG{l, runtime, service, port, postConfig, cfg, control, trace, owner}
}

func (p *recommendationPG) request(t *testing.T, a SignupResult) RecommendationStart {
	t.Helper()
	r, e := p.service.RecommendationContext(context.Background(), a.SessionToken, "")
	if e != nil {
		t.Fatal(e)
	}
	s := r.Value.(RecommendationScope)
	return RecommendationStart{Tuple: rm.Tuple{Version: rm.Version, Nonce: rm.ID(uuid.NewString()), CreatedAt: r.ServerTime.UTC(), Session: s.Binding.Session, AccountEpoch: s.Binding.AccountEpoch, SecurityEpoch: s.Binding.SecurityEpoch, FilterRevision: s.Binding.FilterRevision, Scope: s.Binding.Scope, Policy: rm.ExperimentalIntentPolicy().Digest()}}
}
func recommendationPage(t *testing.T, response PostResponse, e error) RecommendationPage {
	t.Helper()
	if e != nil {
		t.Fatal(e)
	}
	return response.Value.(RecommendationPage)
}
func (p *recommendationPG) start(t *testing.T, a SignupResult, r RecommendationStart) RecommendationPage {
	t.Helper()
	response, e := p.service.StartRecommendation(context.Background(), a.SessionToken, r)
	return recommendationPage(t, response, e)
}
func (p *recommendationPG) read(t *testing.T, a SignupResult, id rm.ID, offset int) RecommendationPage {
	t.Helper()
	r, e := p.service.ReadRecommendation(context.Background(), a.SessionToken, "", id, offset)
	return recommendationPage(t, r, e)
}

func TestRecommendationFeedbackUsesFinalLockedControlPostgres(t *testing.T) {
	p := newRecommendationPG(t, 12)
	ctx := context.Background()
	a := sessionTestAccount(t, p.l, "feedback_control")
	page := p.start(t, a, p.request(t, a))
	state := func() string {
		var value string
		if err := p.l.cp.QueryRow(ctx, `SELECT md5(string_agg(x::text,E'\n' ORDER BY x::text)) FROM (
 SELECT to_jsonb(r) x FROM c_recommendation.rounds r
 UNION ALL SELECT to_jsonb(p) FROM c_recommendation.pages p
 UNION ALL SELECT to_jsonb(h) FROM c_recommendation.history h
 UNION ALL SELECT to_jsonb(a) FROM c_recommendation.accounts a) q`).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	before := state()
	if _, err := p.l.cp.Exec(ctx, `UPDATE c_recommendation.control SET feedback_enabled=false`); err != nil {
		t.Fatal(err)
	}
	if _, err := p.service.FeedbackRecommendation(ctx, a.SessionToken, "", page.RoundID, 0, []int{0}); !errors.Is(err, ErrAuthorizationUnavailable) {
		t.Fatal("database feedback switch was not honored", err)
	}
	if state() != before {
		t.Fatal("disabled feedback changed promised state")
	}
	if _, err := p.l.cp.Exec(ctx, `UPDATE c_recommendation.control SET feedback_enabled=true`); err != nil {
		t.Fatal(err)
	}
	if _, err := p.service.FeedbackRecommendation(ctx, a.SessionToken, "", page.RoundID, 0, []int{0}); err != nil {
		t.Fatal(err)
	}
	if count(t, p.l.cp, `SELECT count(*) FROM c_recommendation.history WHERE account_id=$1 AND exposure_until IS NOT NULL`, a.AccountID) != 1 {
		t.Fatal("reenabling feedback lost the valid delivered receipt")
	}
}

func TestRecommendationPositiveTwoAccountsTwoRoundsPostgres(t *testing.T) {
	p := newRecommendationPG(t, 100)
	ctx := context.Background()
	for _, name := range []string{"reco_first", "reco_second"} {
		a := sessionTestAccount(t, p.l, name)
		for round := 0; round < 2; round++ {
			request := p.request(t, a)
			page := p.start(t, a, request)
			if len(page.View.IDs) != 10 || page.View.Next == nil || *page.View.Next != 10 {
				t.Fatal("positive first page")
			}
			retry := p.start(t, a, request)
			if page.RoundID != retry.RoundID || !reflect.DeepEqual(page.View, retry.View) || page.Revisions != retry.Revisions {
				t.Fatal("lost response replay changed committed page")
			}
			next := p.read(t, a, page.RoundID, 10)
			if len(next.View.IDs) != 10 {
				t.Fatal("positive next page")
			}
			for _, id := range next.View.IDs {
				for _, first := range page.View.IDs {
					if id == first {
						t.Fatal("same-round duplicate")
					}
				}
			}
			if _, e := p.service.FeedbackRecommendation(ctx, a.SessionToken, "", page.RoundID, 0, []int{0, 1}); e != nil {
				t.Fatal(e)
			}
			before := count(t, p.l.cp, `SELECT count(*) FROM c_recommendation.history WHERE account_id=$1 AND exposure_until IS NOT NULL`, a.AccountID)
			if _, e := p.service.FeedbackRecommendation(ctx, a.SessionToken, "", page.RoundID, 0, []int{0, 1}); e != nil {
				t.Fatal(e)
			}
			if count(t, p.l.cp, `SELECT count(*) FROM c_recommendation.history WHERE account_id=$1 AND exposure_until IS NOT NULL`, a.AccountID) != before {
				t.Fatal("duplicate feedback")
			}
		}
	}
	if count(t, p.l.cp, `SELECT count(*) FROM c_recommendation.rounds`) != 4 {
		t.Fatal("did not exercise four successful rounds")
	}
	if count(t, p.l.cp, `SELECT live_rows FROM c_recommendation.control`) <= 0 || count(t, p.l.cp, `SELECT reserved_rows FROM c_recommendation.control`) <= 0 {
		t.Fatal("missing live or promised state")
	}
	if count(t, p.l.cp, `SELECT count(*) FROM c_posts.post_daily_views`) != 0 || count(t, p.l.cp, `SELECT count(*) FROM c_posts.post_view_totals`) != 0 {
		t.Fatal("recommendation feedback changed body view semantics")
	}
}

func TestRecommendationEligiblePoolsRefillPastExposedPrefixPostgres(t *testing.T) {
	p := newRecommendationPG(t, 100)
	ctx := context.Background()
	writer := sessionTestAccount(t, p.l, "reco_pool_writer")
	identity := identityChange(t, p.l.c, writer.SessionToken, "CREATE", uuid.Nil, "候选池作者").IdentityID
	for i := 0; i < 50; i++ {
		postTestCommand(t, p.l, writer.SessionToken, postTestKey(t), postTestIntent(t, p.l, identity))
	}
	postTestPublish(t, p.l)
	a := sessionTestAccount(t, p.l, "reco_pool_reader")
	now := p.control.clock.now()
	mustExec(t, p.l.cp, `INSERT INTO c_recommendation.accounts(account_id,last_at) VALUES($1,$2)`, a.AccountID, now)
	// Valid retained exposure from earlier rounds; no new auth or quota bypass
	// occurs in the measured read/start path. Only this nonce fixture is seeded.
	mustExec(t, p.l.cp, `INSERT INTO c_recommendation.history(account_id,post_id,exposure_until)
 SELECT $1,post_id,$2 FROM c_posts.posts WHERE visibility='PUBLISHED'
 ORDER BY published_at DESC,publication_ordinal DESC LIMIT 100`, a.AccountID, now.Add(72*time.Hour))
	response, err := p.service.RecommendationContext(ctx, a.SessionToken, "")
	if err != nil {
		t.Fatal(err)
	}
	scope := response.Value.(RecommendationScope)
	tx, err := p.runtime.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	history, err := loadRecommendationHistory(ctx, tx, a.AccountID)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _, err := readRecommendationCandidates(ctx, tx, scope, 20261025, history)
	if err != nil {
		t.Fatal(err)
	}
	for source, ids := range snapshot.Input.Pools {
		if len(ids) != 50 {
			t.Fatalf("source %d kept exposed rows ahead of 50 eligible posts: %d", source, len(ids))
		}
	}
	if len(snapshot.Input.Candidates) != 50 {
		t.Fatal("eligible union did not contain the complete bounded remainder")
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	page := p.start(t, a, p.request(t, a))
	if count(t, p.l.cp, `SELECT cardinality(ids) FROM c_recommendation.rounds WHERE round_id=$1`, string(page.RoundID)) != 50 {
		t.Fatal("valid candidates were lost before the frozen manifest")
	}
}

func TestRecommendationExpandedExposurePreservesWindowAndCapPostgres(t *testing.T) {
	for _, stored := range []int{1200, rm.MaxExposureHistory} {
		t.Run(fmtExposureCount(stored), func(t *testing.T) {
			p := newRecommendationPG(t, 20)
			ctx := context.Background()
			a := sessionTestAccount(t, p.l, "reco_exposure_cap")
			now := p.control.clock.now()
			ids := make([]string, stored)
			for i := range ids {
				ids[i] = uuid.NewString()
			}
			mustExec(t, p.l.cp, `INSERT INTO c_recommendation.accounts(account_id,last_at) VALUES($1,$2)`, a.AccountID, now)
			mustExec(t, p.l.cp, `INSERT INTO c_recommendation.history(account_id,post_id,exposure_until)
 SELECT $1,id,$3 FROM unnest($2::uuid[]) id`, a.AccountID, ids, now.Add(72*time.Hour))
			request := p.request(t, a)
			response, err := p.service.StartRecommendation(ctx, a.SessionToken, request)
			if stored == rm.MaxExposureHistory {
				if !errors.Is(err, rm.ErrCapacity) || count(t, p.l.cp, `SELECT count(*) FROM c_recommendation.rounds`) != 0 {
					t.Fatal("full personal exposure quota admitted a new promise", err)
				}
			} else {
				if err != nil {
					t.Fatal("confirmed hour workload hit the old 1024 limit", err)
				}
				page := response.Value.(RecommendationPage)
				if _, err = p.service.FeedbackRecommendation(ctx, a.SessionToken, "", page.RoundID, 0, []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}); err != nil {
					t.Fatal(err)
				}
				if count(t, p.l.cp, `SELECT count(*) FROM c_recommendation.history WHERE exposure_until IS NOT NULL`) != stored+10 {
					t.Fatal("new feedback evicted retained exposures")
				}
			}
			if count(t, p.l.cp, `SELECT count(*) FROM c_recommendation.history WHERE account_id=$1 AND post_id=ANY($2::uuid[]) AND exposure_until=$3`, a.AccountID, ids, now.Add(72*time.Hour)) != stored {
				t.Fatal("existing 72-hour exposure promises changed")
			}
		})
	}
}

func fmtExposureCount(count int) string {
	if count == rm.MaxExposureHistory {
		return "full_2048_rejected"
	}
	return "hour_1200_preserved"
}

func TestRecommendationAtomicFailureFixedMissesAndCASPostgres(t *testing.T) {
	p := newRecommendationPG(t, 24)
	a := sessionTestAccount(t, p.l, "reco_atomic")
	ctx := context.Background()
	request := p.request(t, a)
	p.port.failure = errors.New("synthetic projection failure")
	if _, e := p.service.StartRecommendation(ctx, a.SessionToken, request); e == nil {
		t.Fatal("projection failure acknowledged")
	}
	if count(t, p.l.cp, `SELECT count(*) FROM c_recommendation.intents`) != 0 || count(t, p.l.cp, `SELECT live_rows FROM c_recommendation.control`) != 0 {
		t.Fatal("partial initial transaction")
	}
	p.port.failure = nil
	page := p.start(t, a, request)
	if _, e := p.l.cp.Exec(ctx, `UPDATE c_posts.posts SET moderation_hidden=true WHERE post_id=$1`, string(page.View.IDs[1])); e != nil {
		t.Fatal(e)
	}
	retry := p.read(t, a, page.RoundID, 0)
	if len(retry.View.IDs) != 9 || retry.View.End != 10 || *retry.View.Next != 10 {
		t.Fatal("refilled unavailable slot")
	}
	if _, e := p.l.cp.Exec(ctx, `UPDATE c_posts.posts SET moderation_hidden=false WHERE post_id=$1`, string(page.View.IDs[1])); e != nil {
		t.Fatal(e)
	}
	if len(p.read(t, a, page.RoundID, 0).View.IDs) != 9 {
		t.Fatal("slot resurrected")
	}
	if _, e := p.service.FeedbackRecommendation(ctx, a.SessionToken, "", page.RoundID, 10, []int{0}); e == nil {
		t.Fatal("undelivered batch accepted")
	}
	// Reusing a stale account history pair with the actual SQL save is rejected.
	tx, e := p.runtime.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	r, e := loadRecommendation(ctx, tx, a.AccountID, page.RoundID)
	if e != nil {
		t.Fatal(e)
	}
	if e = tx.Rollback(ctx); e != nil {
		t.Fatal(e)
	}
	if _, e = p.service.FeedbackRecommendation(ctx, a.SessionToken, "", page.RoundID, 0, []int{0}); e != nil {
		t.Fatal(e)
	}
	tx, e = p.runtime.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	stale := r
	stale.State.Revision++
	stale.State.LastAt = time.Now().UTC().Truncate(time.Microsecond)
	if e = p.service.saveRecommendation(ctx, tx, r, stale); !errors.Is(e, rm.ErrConflict) {
		t.Fatal("stale SQL pair accepted", e)
	}
}

func TestRecommendationLikeTimeRevokeAndFormalClosurePostgres(t *testing.T) {
	p := newRecommendationPG(t, 1)
	ctx := context.Background()
	a := sessionTestAccount(t, p.l, "reco_like_reader")
	var post string
	if e := p.l.cp.QueryRow(ctx, `SELECT post_id::text FROM c_posts.posts WHERE visibility='PUBLISHED'`).Scan(&post); e != nil {
		t.Fatal(e)
	}
	// The old two-column writer remains valid and time-unknown after M1.
	if _, e := p.runtime.Exec(ctx, `INSERT INTO c_posts.post_likes(account_id,post_id) VALUES($1,$2)`, a.AccountID, post); e != nil {
		t.Fatal(e)
	}
	if _, e := p.service.SetPostInteraction(ctx, a.SessionToken, post, posts.PostLike, true); e != nil {
		t.Fatal(e)
	}
	if count(t, p.l.cp, `SELECT count(*) FROM c_posts.post_likes WHERE account_id=$1 AND liked_at IS NULL`, a.AccountID) != 1 {
		t.Fatal("historical/no-op like time invented")
	}
	if _, e := p.service.SetPostInteraction(ctx, a.SessionToken, post, posts.PostLike, false); e != nil {
		t.Fatal(e)
	}
	if _, e := p.service.SetPostInteraction(ctx, a.SessionToken, post, posts.PostLike, true); e != nil {
		t.Fatal(e)
	}
	var first time.Time
	if e := p.l.cp.QueryRow(ctx, `SELECT liked_at FROM c_posts.post_likes WHERE account_id=$1`, a.AccountID).Scan(&first); e != nil {
		t.Fatal(e)
	}
	advanceClosureClock(t, p.control, p.control.clock.now().Add(time.Second))
	if _, e := p.service.SetPostInteraction(ctx, a.SessionToken, post, posts.PostLike, true); e != nil {
		t.Fatal(e)
	}
	var same time.Time
	if e := p.l.cp.QueryRow(ctx, `SELECT liked_at FROM c_posts.post_likes WHERE account_id=$1`, a.AccountID).Scan(&same); e != nil || !same.Equal(first) {
		t.Fatal("no-op refreshed time", e)
	}
	page := p.start(t, a, p.request(t, a))
	if _, e := p.service.FeedbackRecommendation(ctx, a.SessionToken, "", page.RoundID, 0, []int{0}); e != nil {
		t.Fatal(e)
	}
	if _, e := p.service.RevokeRecommendation(ctx, a.SessionToken); e != nil {
		t.Fatal(e)
	}
	if count(t, p.l.cp, `SELECT count(*) FROM c_recommendation.rounds WHERE account_id=$1`, a.AccountID) != 0 || count(t, p.l.cp, `SELECT count(*) FROM c_recommendation.history WHERE account_id=$1`, a.AccountID) != 0 {
		t.Fatal("revoke retained detail")
	}
	if _, e := p.service.ReadRecommendation(ctx, a.SessionToken, "", page.RoundID, 0); e == nil {
		t.Fatal("revoked round revived")
	}
	// Actual closure path, with recommendation and post HTTP capabilities off.
	request, _ := closureTestRequest(t, a.SessionToken)
	closure, e := p.service.RequestAccountClosure(ctx, request, sessionTestPasswordWorker(t, 1))
	if e != nil {
		t.Fatal(e)
	}
	advanceClosureClock(t, p.control, closure.DueAt)
	off := *p.service
	off.recommendation = nil
	off.posts = nil
	if n, e := off.FinalizeDueClosures(ctx, 10); e != nil || n != 1 {
		t.Fatal("feature-off formal closure", n, e)
	}
	if count(t, p.l.cp, `SELECT count(*) FROM c_recommendation.accounts WHERE account_id=$1`, a.AccountID) != 0 || count(t, p.l.cp, `SELECT count(*) FROM c_posts.post_likes WHERE account_id=$1`, a.AccountID) != 0 {
		t.Fatal("closure retained private state")
	}
}

func TestRecommendationLastCapacityConcurrentAndRetryPostgres(t *testing.T) {
	p := newRecommendationPG(t, 0)
	ctx := context.Background()
	a := sessionTestAccount(t, p.l, "reco_capacity_a")
	b := sessionTestAccount(t, p.l, "reco_capacity_b")
	requests := []RecommendationStart{p.request(t, a), p.request(t, b)}
	if _, e := p.l.cp.Exec(ctx, `UPDATE c_recommendation.control SET max_rows=4`); e != nil {
		t.Fatal(e)
	}
	accounts := []SignupResult{a, b}
	results := make([]PostResponse, 2)
	errs := make([]error, 2)
	var group sync.WaitGroup
	start := make(chan struct{})
	for i := range accounts {
		group.Add(1)
		go func(i int) {
			defer group.Done()
			<-start
			results[i], errs[i] = p.service.StartRecommendation(ctx, accounts[i].SessionToken, requests[i])
		}(i)
	}
	close(start)
	group.Wait()
	winner := -1
	success := 0
	for i, e := range errs {
		if e == nil {
			winner = i
			success++
		}
	}
	if success != 1 || count(t, p.l.cp, `SELECT count(*) FROM c_recommendation.rounds`) != 1 || count(t, p.l.cp, `SELECT live_rows FROM c_recommendation.control`) != 4 {
		t.Fatal("last slot was oversold or all refused", success, errs)
	}
	before := results[winner].Value.(RecommendationPage)
	retry := p.start(t, accounts[winner], requests[winner])
	if before.RoundID != retry.RoundID || !reflect.DeepEqual(before.View, retry.View) {
		t.Fatal("capacity stole committed replay")
	}
}

func TestRecommendationSchemaPermissionsAndFiniteCleanupPostgres(t *testing.T) {
	p := newRecommendationPG(t, 12)
	ctx := context.Background()
	a := sessionTestAccount(t, p.l, "reco_cleanup")
	request := p.request(t, a)
	page := p.start(t, a, request)
	prefix := "hnuhole"
	for _, role := range []string{prefix + "_c_recovery", prefix + "_business", prefix + "_v"} {
		var access bool
		if e := p.l.cp.QueryRow(ctx, `SELECT has_table_privilege($1,'c_recommendation.history','SELECT')`, role).Scan(&access); e != nil || access {
			t.Fatal("private state role leakage", e)
		}
	}
	for _, pair := range [][2]string{
		{`REVOKE DELETE ON c_recommendation.history FROM ` + prefix + `_c_runtime`, `GRANT DELETE ON c_recommendation.history TO ` + prefix + `_c_runtime`},
		{`GRANT UPDATE(enabled) ON c_recommendation.control TO ` + prefix + `_c_runtime`, `REVOKE UPDATE(enabled) ON c_recommendation.control FROM ` + prefix + `_c_runtime`},
		{`ALTER TABLE c_auth.accounts DISABLE TRIGGER recommendation_account_closed`, `ALTER TABLE c_auth.accounts ENABLE TRIGGER recommendation_account_closed`},
		{`ALTER TABLE c_recommendation.pages DISABLE TRIGGER recommendation_guard`, `ALTER TABLE c_recommendation.pages ENABLE TRIGGER recommendation_guard`},
	} {
		if _, e := p.l.cp.Exec(ctx, pair[0]); e != nil {
			t.Fatal(e)
		}
		if checkRecommendationSchema(ctx, p.runtime, false) == nil {
			t.Fatal("bad installed M1 accepted")
		}
		if _, e := p.l.cp.Exec(ctx, pair[1]); e != nil {
			t.Fatal(e)
		}
	}
	if e := checkRecommendationSchema(ctx, p.runtime, true); e != nil {
		t.Fatal(e)
	}
	// Expiry is driven by the trusted Gate clock, not SQL wall time or a forged
	// client timestamp. No service timing or cleanup SLA is inferred here.
	advanceClosureClock(t, p.control, p.control.clock.now().Add(6*time.Minute))
	before := count(t, p.l.cp, `SELECT count(*) FROM c_recommendation.history`)
	if n, e := p.service.CleanRecommendation(ctx, 16); e != nil || n != min(before, 15) {
		t.Fatal("bounded oldest cleanup", n, e)
	}
	if count(t, p.l.cp, `SELECT count(*) FROM c_recommendation.history`) != before-min(before, 15) {
		t.Fatal("cleanup removed wrong amount")
	}
	// Old physical rows still count. Replays can remain empty after omissions,
	// and no worker extends the original round's stored boundary/TTL.
	var expiry time.Time
	if e := p.l.cp.QueryRow(ctx, `SELECT expires_at FROM c_recommendation.rounds WHERE round_id=$1`, string(page.RoundID)).Scan(&expiry); e != nil {
		t.Fatal(e)
	}
	advanceClosureClock(t, p.control, expiry.Add(11*time.Minute))
	for i := 0; i < 30; i++ {
		n, e := p.service.CleanRecommendation(ctx, 16)
		if e != nil {
			t.Fatal(e)
		}
		if n == 0 {
			break
		}
	}
	if count(t, p.l.cp, `SELECT count(*) FROM c_recommendation.rounds`) != 0 || count(t, p.l.cp, `SELECT count(*) FROM c_recommendation.intents`) != 0 {
		t.Fatal("round/receipt expiry backlog")
	}
	if _, e := p.service.StartRecommendation(ctx, a.SessionToken, request); !errors.Is(e, rm.ErrExpired) {
		t.Fatal("cleaned old intent recreated", e)
	}
}

func TestRecommendationExhaustedByteBudgetCanCleanAndRevokePostgres(t *testing.T) {
	p := newRecommendationPG(t, 2)
	ctx := context.Background()
	a := sessionTestAccount(t, p.l, "reco_full_bytes")
	page := p.start(t, a, p.request(t, a))
	if _, e := p.service.FeedbackRecommendation(ctx, a.SessionToken, "", page.RoundID, 0, []int{0}); e != nil {
		t.Fatal(e)
	}
	// Fill logical quotas and close fresh physical admission independently. File
	// watermarks and cumulative telemetry must not gate promised consumption.
	if _, e := p.l.cp.Exec(ctx, `UPDATE c_recommendation.control SET max_rows=live_rows+reserved_rows,max_bytes=GREATEST(4096,live_bytes+reserved_bytes),max_units=live_units+reserved_units,max_file_bytes=1048576`); e != nil {
		t.Fatal(e)
	}
	advanceClosureClock(t, p.control, p.control.clock.now().Add(6*time.Minute))
	if n, e := p.service.CleanRecommendation(ctx, 16); e != nil || n != 2 {
		t.Fatal("full budget blocked promised cleanup batch", n, e)
	}
	if count(t, p.l.cp, `SELECT count(*) FROM c_recommendation.history WHERE delivery_until IS NOT NULL`) != 0 || count(t, p.l.cp, `SELECT count(*) FROM c_recommendation.history WHERE exposure_until IS NOT NULL`) != 1 {
		t.Fatal("expiry changed hard exposure")
	}
	if _, e := p.service.FeedbackRecommendation(ctx, a.SessionToken, "", page.RoundID, 0, []int{1}); e != nil {
		t.Fatal("watermark blocked accepted feedback", e)
	}
	if _, e := p.service.StartRecommendation(ctx, a.SessionToken, p.request(t, a)); !errors.Is(e, rm.ErrCapacity) {
		t.Fatal("physical watermark admitted fresh work", e)
	}
	if _, e := p.service.RevokeRecommendation(ctx, a.SessionToken); e != nil {
		t.Fatal("full budget blocked revoke", e)
	}
	if _, e := p.service.RevokeRecommendation(ctx, a.SessionToken); e != nil {
		t.Fatal("duplicate revoke should be no-op", e)
	}
	if count(t, p.l.cp, `SELECT reserved_rows FROM c_recommendation.control`) != 0 || count(t, p.l.cp, `SELECT count(*) FROM c_recommendation.history`) != 0 {
		t.Fatal("budget promises leaked")
	}
}

// Logical-byte accounting regression. Uses the existing disposable owned-PG fixture;
// NEVER run this fixture against retained J (the fixture resets schemas).
// Each shape rolls back DDL and data after exercising the forward repair.
func TestRecommendationLogicalBytesConservationPostgres(t *testing.T) {
	p := newRecommendationPG(t, 0)
	ctx := context.Background()
	old, err := os.ReadFile("../../migrations/0056_recommendation_state.sql")
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(old), "CREATE FUNCTION c_recommendation.account_change()")
	if start < 0 {
		t.Fatal("old accounting function missing")
	}
	end := strings.Index(string(old)[start:], "-- +goose StatementEnd")
	if end < 0 {
		t.Fatal("old accounting function boundary missing")
	}
	oldFunction := strings.Replace(string(old)[start:start+end], "CREATE FUNCTION", "CREATE OR REPLACE FUNCTION", 1)
	forward, err := os.ReadFile("../../migrations/0057_recommendation_logical_bytes.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := strings.Split(string(forward), "-- +goose Down")[0]
	// The fixture is enabled: forward migration must fail before changing state.
	rejected, err := p.l.cp.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rejected.Exec(ctx, up)
	_ = rejected.Rollback(ctx)
	if err == nil {
		t.Fatal("migration accepted enabled control")
	}
	const actual = `SELECT COALESCE(sum(n),0) FROM (
 SELECT 32+octet_length(pg_catalog.record_send(r)) n FROM c_recommendation.accounts r
 UNION ALL SELECT 32+octet_length(pg_catalog.record_send(r)) FROM c_recommendation.rounds r
 UNION ALL SELECT 32+octet_length(pg_catalog.record_send(r)) FROM c_recommendation.pages r
 UNION ALL SELECT 32+octet_length(pg_catalog.record_send(r)) FROM c_recommendation.intents r
 UNION ALL SELECT 32+octet_length(pg_catalog.record_send(r)) FROM c_recommendation.history r) q`
	for _, shape := range []string{"max_compressible", "max_incompressible"} {
		t.Run(shape, func(t *testing.T) {
			tx, e := p.l.cp.Begin(ctx)
			if e != nil {
				t.Fatal(e)
			}
			defer tx.Rollback(ctx)
			exec := func(sql string, args ...any) {
				t.Helper()
				if _, e := tx.Exec(ctx, sql, args...); e != nil {
					t.Fatal(e)
				}
			}
			check := func(stage string) {
				t.Helper()
				var stored, expected int64
				if e := tx.QueryRow(ctx, "SELECT live_bytes FROM c_recommendation.control").Scan(&stored); e != nil {
					t.Fatal(e)
				}
				if e := tx.QueryRow(ctx, actual).Scan(&expected); e != nil {
					t.Fatal(e)
				}
				if stored != expected {
					t.Fatalf("%s logical bytes %d != actual rows %d", stage, stored, expected)
				}
			}
			// Restore only the original function in this rollback-only test tx,
			// allowing the same test to run when the fixture already installs logical-byte accounting.
			// Respect the existing stop-admissions-before-profile-change guard.
			exec(`UPDATE c_recommendation.control SET enabled=false,admission_enabled=false,feedback_enabled=false,like_time_enabled=false`)
			exec(`UPDATE c_recommendation.control SET profile_hash=NULL`)
			exec(oldFunction)
			exec(`DROP FUNCTION IF EXISTS c_recommendation.logical_row_bytes(bytea)`)
			binding, policy := make([]byte, 1024), make([]byte, 512)
			for i := range binding {
				binding[i] = 1
			}
			for i := range policy {
				policy[i] = 2
			}
			if shape == "max_incompressible" {
				for i := 0; i < len(binding)/32; i++ {
					h := sha256.Sum256([]byte{byte(i), 17, 39})
					copy(binding[i*32:], h[:])
				}
				for i := 0; i < len(policy)/32; i++ {
					h := sha256.Sum256([]byte{byte(i), 91, 73})
					copy(policy[i*32:], h[:])
				}
			}
			ids := make([]string, 100)
			for i := range ids {
				ids[i] = uuid.NewString()
			}
			id, account := uuid.NewString(), p.author.AccountID
			seed := func() {
				exec(`INSERT INTO c_recommendation.accounts(account_id,last_at) VALUES($1,clock_timestamp())`, account)
				exec(`INSERT INTO c_recommendation.rounds(round_id,account_id,binding,policy,config_hash,seed,ids,future_rows,future_exposures,future_writes,revision,created_at,last_at,expires_at,feedback_until)
 SELECT $1,$2,$3,$4,decode(repeat('03',32),'hex'),decode(repeat('04',8),'hex'),$5::uuid[],$5::uuid[],$5::uuid[],1024,1,stamp,stamp,stamp+interval '30 minutes',stamp+interval '40 minutes' FROM (SELECT clock_timestamp() stamp) q`, id, account, binding, policy, ids)
				exec(`INSERT INTO c_recommendation.pages(round_id,page_index) SELECT $1,n FROM generate_series(0,9)n`, id)
				exec(`INSERT INTO c_recommendation.intents(account_id,key_digest,round_id,accept_until,retain_until) SELECT $1,decode(repeat('05',32),'hex'),$2,stamp,stamp+interval '1 minute' FROM (SELECT clock_timestamp() stamp) q`, account, id)
				exec(`INSERT INTO c_recommendation.history(account_id,post_id,delivery_at,delivery_until,exposure_until) SELECT $1,id,stamp,stamp+interval '1 minute',stamp+interval '1 hour' FROM unnest($2::uuid[])id CROSS JOIN (SELECT clock_timestamp() stamp)q`, account, ids[:16])
			}
			seed()
			var compression *string
			var storedBinding, logicalBinding int
			if e := tx.QueryRow(ctx, `SELECT pg_column_compression(binding),pg_column_size(binding),octet_length(binding) FROM c_recommendation.rounds WHERE round_id=$1`, id).Scan(&compression, &storedBinding, &logicalBinding); e != nil {
				t.Fatal(e)
			}
			if logicalBinding != 1024 || (shape == "max_compressible" && (compression == nil || storedBinding >= logicalBinding)) || (shape == "max_incompressible" && compression != nil) {
				t.Fatal("fixture did not exercise intended storage representation")
			}
			t.Logf("%s binding logical=%d stored=%d compressed=%t", shape, logicalBinding, storedBinding, compression != nil)
			// Reconcile from NONEMPTY real tables, never an injected control charge.
			exec(up)
			check("forward migration of retained rows")
			var beforeDebt, restoredDebt int64
			if e := tx.QueryRow(ctx, `SELECT write_debt_bytes FROM c_recommendation.control`).Scan(&beforeDebt); e != nil {
				t.Fatal(e)
			}
			exec(`SAVEPOINT mutation_roundtrip`)
			exec(`UPDATE c_recommendation.rounds SET future_rows=ARRAY[]::uuid[],future_exposures=ARRAY[]::uuid[],future_writes=0,revision=revision+1 WHERE round_id=$1`, id)
			exec(`UPDATE c_recommendation.accounts SET revision=revision+1 WHERE account_id=$1`, account)
			exec(`UPDATE c_recommendation.history SET delivery_at=NULL,delivery_until=NULL WHERE account_id=$1`, account)
			check("updates and NULL transitions")
			exec(`DELETE FROM c_recommendation.accounts WHERE account_id=$1`, account)
			check("cascade delete")
			var remaining int64
			if e := tx.QueryRow(ctx, `SELECT live_rows+live_bytes+live_units+reserved_rows+reserved_bytes+reserved_units+reserved_writes FROM c_recommendation.control`).Scan(&remaining); e != nil || remaining != 0 {
				t.Fatal("not drained", remaining, e)
			}
			exec(`ROLLBACK TO SAVEPOINT mutation_roundtrip`)
			check("rollback restored full rows")
			if e := tx.QueryRow(ctx, `SELECT write_debt_bytes FROM c_recommendation.control`).Scan(&restoredDebt); e != nil || beforeDebt != restoredDebt {
				t.Fatal("rollback changed debt", e)
			}
			// A post-migration insert must also use the new representation.
			exec(`INSERT INTO c_recommendation.history(account_id,post_id,exposure_until) VALUES($1,$2,clock_timestamp()+interval '1 hour')`, account, ids[16])
			check("new insert")
			exec(`DELETE FROM c_recommendation.accounts WHERE account_id=$1`, account)
			check("final cascade")
			seed()
			check("new maximum-shape insert")
			exec(`DELETE FROM c_recommendation.accounts WHERE account_id=$1`, account)
			check("new maximum-shape cascade")
		})
	}
}

func TestRecommendationLogicalBytesGooseUpgradePostgres(t *testing.T) {
	if os.Getenv("FEED_RECOMMENDATION_OWNED") != "1" {
		t.Skip("owned disposable recommendation fixture absent")
	}
	if os.Getenv("HNULIMIT_GOOSE") == "" {
		t.Fatal("existing pinned Goose path required")
	}
	t.Setenv("HNULIMIT_FORMAL_GOOSE", "1")
	l := newLabThrough(t, 56)
	ctx := context.Background()
	a := sessionTestAccount(t, l, "logical_upgrade")
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, e := l.cp.Exec(ctx, sql, args...); e != nil {
			t.Fatal(e)
		}
	}
	run := func() error {
		return sharedGooseCommand(t, "../../migrations", l.cp.Config().ConnString(), "up-to", "57")
	}
	assertVersion := func(want int) {
		t.Helper()
		if got := count(t, l.cp, `SELECT max(version_id) FROM public.goose_db_version WHERE is_applied`); got != want {
			t.Fatalf("Goose version %d != %d", got, want)
		}
	}
	assertVersion(56)
	exec(`INSERT INTO c_recommendation.accounts(account_id,last_at) VALUES($1,clock_timestamp())`, a.AccountID)
	exec(`INSERT INTO c_recommendation.rounds(round_id,account_id,binding,policy,config_hash,seed,ids,future_rows,future_exposures,future_writes,revision,created_at,last_at,expires_at,feedback_until)
 SELECT $1,$2,decode(repeat('01',1024),'hex'),decode(repeat('02',512),'hex'),decode(repeat('03',32),'hex'),decode(repeat('04',8),'hex'),ids,ids,ids,1024,1,stamp,stamp,stamp+interval '30 minutes',stamp+interval '40 minutes'
 FROM (SELECT ARRAY(SELECT md5(n::text)::uuid FROM generate_series(1,100)n) ids,clock_timestamp() stamp)q`, uuid.NewString(), a.AccountID)
	var originalFunction string
	if e := l.cp.QueryRow(ctx, `SELECT prosrc FROM pg_proc WHERE oid='c_recommendation.account_change()'::regprocedure`).Scan(&originalFunction); e != nil {
		t.Fatal(e)
	}
	originalBytes := count(t, l.cp, `SELECT live_bytes FROM c_recommendation.control`)
	assertUnchanged := func() {
		t.Helper()
		assertVersion(56)
		var unchanged bool
		if e := l.cp.QueryRow(ctx, `SELECT to_regprocedure('c_recommendation.logical_row_bytes(bytea)') IS NULL AND (SELECT prosrc FROM pg_proc WHERE oid='c_recommendation.account_change()'::regprocedure)=$1 AND (SELECT live_bytes FROM c_recommendation.control)=$2 AND (SELECT count(*) FROM c_recommendation.rounds)=1`, originalFunction, originalBytes).Scan(&unchanged); e != nil || !unchanged {
			t.Fatal("failed migration changed data/function/counter", e)
		}
	}
	exec(`UPDATE c_recommendation.control SET enabled=true`)
	if run() == nil {
		t.Fatal("Goose accepted enabled control")
	}
	assertUnchanged()
	exec(`UPDATE c_recommendation.control SET enabled=false,max_bytes=GREATEST(4096,live_bytes+reserved_bytes)`)
	// Binary field headers and the explicit allowance exceed the old tuple
	// tariff for this shape. Fail AFTER function replacement, at reconciliation.
	if run() == nil {
		t.Fatal("Goose reconciliation exceeded quota without rollback")
	}
	assertUnchanged()
	exec(`UPDATE c_recommendation.control SET max_bytes=16777216`)
	if e := run(); e != nil {
		t.Fatal("formal Goose 56->57 failed")
	}
	assertVersion(57)
	var balanceOK bool
	if e := l.cp.QueryRow(ctx, `SELECT live_bytes=(SELECT sum(n) FROM (
 SELECT 32+octet_length(record_send(r)) n FROM c_recommendation.accounts r
 UNION ALL SELECT 32+octet_length(record_send(r)) FROM c_recommendation.rounds r)q)
 AND NOT enabled AND NOT admission_enabled AND profile_hash IS NULL FROM c_recommendation.control`).Scan(&balanceOK); e != nil || !balanceOK {
		t.Fatal("nonempty upgrade reconciliation", e)
	}
	recommendationGrant(t, l)
	runtime, e := pgxpool.New(ctx, os.Getenv("AUTHLAB_POSTS_RUNTIME_DSN"))
	if e != nil {
		t.Fatal(e)
	}
	defer runtime.Close()
	role := "hnuhole_c_runtime"
	var restricted bool
	if e = runtime.QueryRow(ctx, `SELECT current_user=$1 AND NOT (SELECT rolsuper OR rolcreatedb OR rolcreaterole OR rolbypassrls FROM pg_roles WHERE rolname=current_user)`, role).Scan(&restricted); e != nil || !restricted {
		t.Fatal("wrong runtime role", e)
	}
	if e = checkRecommendationSchema(ctx, runtime, true); e != nil {
		t.Fatal("0057 runtime contract", e)
	}
	exec(`REVOKE EXECUTE ON FUNCTION c_recommendation.logical_row_bytes(bytea) FROM ` + pgx.Identifier{role}.Sanitize())
	if e = checkRecommendationSchema(ctx, runtime, true); e == nil {
		t.Fatal("missing helper permission accepted")
	}
	if _, e = runtime.Exec(ctx, `DELETE FROM c_recommendation.accounts WHERE account_id=$1`, a.AccountID); e == nil {
		t.Fatal("unprivileged trigger helper executed")
	}
	if count(t, l.cp, `SELECT count(*) FROM c_recommendation.rounds`) != 1 {
		t.Fatal("failed runtime delete not atomic")
	}
	recommendationGrant(t, l)
	if _, e = runtime.Exec(ctx, `DELETE FROM c_recommendation.accounts WHERE account_id=$1`, a.AccountID); e != nil {
		t.Fatal("restricted runtime cascade", e)
	}
	if count(t, l.cp, `SELECT live_rows+live_bytes+live_units+reserved_rows+reserved_bytes+reserved_units+reserved_writes FROM c_recommendation.control`) != 0 {
		t.Fatal("restricted cascade did not drain")
	}
	exec(`CREATE OR REPLACE FUNCTION c_recommendation.logical_row_bytes(encoded bytea) RETURNS bigint LANGUAGE sql IMMUTABLE STRICT SET search_path=pg_catalog AS 'SELECT 1::bigint'`)
	if e = checkRecommendationSchema(ctx, runtime, true); e == nil {
		t.Fatal("altered accounting function accepted")
	}
	// This disposable fixture is removed by the existing runner, not upgraded J.
}

func TestRecommendationFormalGooseEmptyInstallAndMainlineUpgradePostgres(t *testing.T) {
	if os.Getenv("FEED_RECOMMENDATION_OWNED") != "1" {
		t.Skip("owned disposable recommendation fixture absent")
	}
	if os.Getenv("HNULIMIT_GOOSE") == "" {
		t.Fatal("existing pinned Goose path required")
	}
	t.Setenv("HNULIMIT_FORMAL_GOOSE", "1")
	assertCeilings := func(t *testing.T, l *lab) {
		t.Helper()
		ctx := context.Background()
		var clean bool
		if e := l.cp.QueryRow(ctx, `SELECT NOT(enabled OR admission_enabled OR feedback_enabled OR like_time_enabled) AND profile_hash IS NULL FROM c_recommendation.control`).Scan(&clean); e != nil || !clean {
			t.Fatal("default-off recommendation control", e)
		}
		if _, e := l.cp.Exec(ctx, `UPDATE c_recommendation.control SET max_rows=1048576,max_bytes=1073741824,max_units=4194304,max_file_bytes=8589934592`); e != nil {
			t.Fatal("0058 ceilings not installed", e)
		}
	}
	t.Run("empty_install", func(t *testing.T) {
		l := newLabThrough(t, 0)
		ctx := context.Background()
		if got := count(t, l.cp, `SELECT max(version_id) FROM public.goose_db_version WHERE is_applied`); got != 58 {
			t.Fatalf("empty install Goose version %d != 58", got)
		}
		if e := checkRecommendationSchema(ctx, l.cp, true); e != nil {
			t.Fatal("empty install schema", e)
		}
		assertCeilings(t, l)
		recommendationGrant(t, l)
		runtime, e := pgxpool.New(ctx, os.Getenv("AUTHLAB_POSTS_RUNTIME_DSN"))
		if e != nil {
			t.Fatal(e)
		}
		defer runtime.Close()
		if e = checkRecommendationSchema(ctx, runtime, true); e != nil {
			t.Fatal("empty install restricted runtime contract", e)
		}
	})
	t.Run("mainline_55_to_58", func(t *testing.T) {
		l := newLabThrough(t, 55)
		ctx := context.Background()
		a := sessionTestAccount(t, l, "goose_upgrade_reader")
		if got := count(t, l.cp, `SELECT max(version_id) FROM public.goose_db_version WHERE is_applied`); got != 55 {
			t.Fatalf("mainline baseline Goose version %d != 55", got)
		}
		if e := sharedGooseCommand(t, "../../migrations", l.cp.Config().ConnString(), "up-to", "58"); e != nil {
			t.Fatal("formal Goose 55->58 failed", e)
		}
		if got := count(t, l.cp, `SELECT max(version_id) FROM public.goose_db_version WHERE is_applied`); got != 58 {
			t.Fatalf("mainline upgrade Goose version %d != 58", got)
		}
		if count(t, l.cp, `SELECT count(*) FROM c_auth.accounts WHERE account_id=$1`, a.AccountID) != 1 {
			t.Fatal("upgrade discarded existing auth data")
		}
		if e := checkRecommendationSchema(ctx, l.cp, true); e != nil {
			t.Fatal("upgraded schema", e)
		}
		assertCeilings(t, l)
		recommendationGrant(t, l)
		runtime, e := pgxpool.New(ctx, os.Getenv("AUTHLAB_POSTS_RUNTIME_DSN"))
		if e != nil {
			t.Fatal(e)
		}
		defer runtime.Close()
		if e = checkRecommendationSchema(ctx, runtime, true); e != nil {
			t.Fatal("upgraded restricted runtime contract", e)
		}
	})
}

func recommendationGrant(t *testing.T, l *lab) {
	t.Helper()
	prefix := "hnuhole"
	grant, e := os.ReadFile(filepath.Join("..", "..", "..", "..", "infra", "postgres", "grant-community.sql"))
	if e != nil {
		t.Fatal(e)
	}
	rendered := strings.NewReplacer("hnuhole_c_runtime", prefix+"_c_runtime", "hnuhole_c_recovery", prefix+"_c_recovery", "hnuhole_business", prefix+"_business", "hnuhole_c", prefix+"_c").Replace(string(grant))
	// This legacy raw-Up fixture deliberately has no Goose tracker. Do not
	// fabricate one merely to pass the formal deployment startup check.
	if count(t, l.cp, `SELECT count(*) FROM pg_class WHERE oid=to_regclass('public.goose_db_version')`) == 0 {
		rendered = strings.ReplaceAll(rendered, "GRANT SELECT ON public.goose_db_version TO "+prefix+"_c_runtime;", "")
	}
	if _, e = l.cp.Exec(context.Background(), rendered); e != nil {
		t.Fatal(e)
	}
}
