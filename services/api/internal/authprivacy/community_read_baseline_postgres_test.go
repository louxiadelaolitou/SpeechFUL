package authprivacy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	commentmodel "github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/comments"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/postmedia"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/posts"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/readtiming"
)

type baselineAcquireKey struct{}
type baselineOperationKey struct{}
type baselinePoolTracer struct{ reportError func(string, string, uint32) }

func (baselinePoolTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	return ctx
}
func (tracer baselinePoolTracer) TraceQueryEnd(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryEndData) {
	if data.Err == nil {
		return
	}
	code, class := "client", "other"
	var serverError interface{ SQLState() string }
	if errors.As(data.Err, &serverError) {
		code = serverError.SQLState()
	} else {
		code = "client:" + fmt.Sprintf("%T", data.Err)
	}
	switch code {
	case "40P01":
		class = "deadlock"
	case "40001":
		class = "serialization"
	case "55P03":
		class = "lock_timeout"
	case "57014":
		class = "query_cancelled"
		var server *pgconn.PgError
		if errors.As(data.Err, &server) {
			switch server.Message {
			case "canceling statement due to user request":
				class = "user_cancel"
			case "canceling statement due to statement timeout":
				class = "statement_timeout"
			case "canceling statement due to conflict with recovery":
				class = "recovery_conflict"
			}
		}
		code += "/" + class
	case "25P02":
		class = "failed_transaction"
	case "08003", "08006", "57P01":
		class = "connection"
	}
	readtiming.Span(ctx, "baseline_sql_error_"+class)()
	if tracer.reportError != nil {
		operation, _ := ctx.Value(baselineOperationKey{}).(string)
		tracer.reportError(code, operation, conn.PgConn().PID())
	}
}
func (baselinePoolTracer) TraceAcquireStart(ctx context.Context, _ *pgxpool.Pool, _ pgxpool.TraceAcquireStartData) context.Context {
	return context.WithValue(ctx, baselineAcquireKey{}, readtiming.Span(ctx, "pool_wait"))
}
func (baselinePoolTracer) TraceAcquireEnd(ctx context.Context, _ *pgxpool.Pool, _ pgxpool.TraceAcquireEndData) {
	if finish, ok := ctx.Value(baselineAcquireKey{}).(func()); ok {
		finish()
	}
}

type readBaselineSample struct {
	Kind          string                 `json:"kind"`
	Index         int                    `json:"index"`
	ElapsedNS     int64                  `json:"elapsedNS"`
	WallStartNS   int64                  `json:"wallStartNS"`
	WallEndNS     int64                  `json:"wallEndNS"`
	WallElapsedNS int64                  `json:"wallElapsedNS"`
	RawElapsedNS  int64                  `json:"rawElapsedNS"`
	WallGoDriftNS int64                  `json:"wallGoDriftNS"`
	RawGoDriftNS  int64                  `json:"rawGoDriftNS"`
	ClockStable   bool                   `json:"clockStable"`
	Error         string                 `json:"error,omitempty"`
	ErrorCode     string                 `json:"errorCode,omitempty"`
	ErrorStatus   int                    `json:"errorStatus,omitempty"`
	Observation   readtiming.Observation `json:"observation"`
}

func baselineInt(t *testing.T, key string, fallback, lower, upper int) int {
	t.Helper()
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < lower || n > upper {
		t.Fatalf("%s must be between %d and %d", key, lower, upper)
	}
	return n
}

// Synthetic rows only prepare the owned fixture. All measured operations call
// production Community methods, Gate checks, PostgreSQL and durable commits.
// This is a service-method benchmark, not HTTP/device/production acceptance.
func TestReadCriticalSectionBaselinePostgres(t *testing.T) {
	if os.Getenv("REQ001_BASELINE") != "1" {
		t.Skip("opt-in mixed-load measurement")
	}
	if os.Getenv("AUTHLAB_C_DSN") == "" || os.Getenv("AUTHLAB_V_DSN") == "" || os.Getenv("POST_MEDIA_PYTHON") == "" || os.Getenv("POST_MEDIA_FIXTURES") == "" {
		t.Fatal("owned C/V SQL and real image validator fixtures are required")
	}
	output := os.Getenv("REQ001_BASELINE_OUTPUT")
	if !filepath.IsAbs(output) || !strings.HasSuffix(output, ".json") {
		t.Fatal("REQ001_BASELINE_OUTPUT must be an absolute JSON artifact path outside source")
	}
	root, err := filepath.Abs("../../../..")
	if err != nil || output == root || strings.HasPrefix(output, root+string(os.PathSeparator)) {
		t.Fatal("baseline artifacts must stay outside source")
	}
	comments := baselineInt(t, "REQ001_BASELINE_COMMENTS", 1000, 50, 10000)
	likes := baselineInt(t, "REQ001_BASELINE_LIKES", min(comments, 1000), 0, comments)
	samples := baselineInt(t, "REQ001_BASELINE_SAMPLES", 80, 8, 1000)
	warmup := baselineInt(t, "REQ001_BASELINE_WARMUP", 10, 1, 100)
	fresh := os.Getenv("REQ001_BASELINE_FRESH") == "1"
	commentLike := os.Getenv("REQ001_BASELINE_COMMENT_LIKE") == "1"
	mode := commentmodel.SortMode(os.Getenv("REQ001_BASELINE_SORT"))
	if !mode.Valid() {
		t.Fatal("invalid comment sort mode")
	}
	if fresh && samples+warmup > 120 {
		t.Fatal("fresh nonce measurement must stay within existing account snapshot quota")
	}
	prepared := os.Getenv("REQ001_BASELINE_PREPARED") == "1"
	l := newLab(t)
	postConfig := PostConfig{Environment: "text-post-lab"}
	for _, key := range []*[32]byte{&postConfig.KeyKey, &postConfig.FingerprintKey, &postConfig.CursorKey} {
		value, keyErr := random32()
		if keyErr != nil {
			t.Fatal(keyErr)
		}
		*key = value
	}
	preparedFeedReads, err := setBaselinePreparedRead(&postConfig, "PreparedFeedReads", prepared)
	if err != nil {
		t.Fatal(err)
	}
	preparedMediaReads, err := setBaselinePreparedRead(&postConfig, "PreparedMediaReads", prepared)
	if err != nil {
		t.Fatal(err)
	}
	l.c, err = l.c.WithPosts(context.Background(), postConfig)
	if err != nil {
		t.Fatal(err)
	}
	commentConfig := CommentConfig{}
	commentConfig.KeyKey, err = random32()
	if err != nil {
		t.Fatal(err)
	}
	commentConfig.FingerprintKey, err = random32()
	if err != nil {
		t.Fatal(err)
	}
	preparedReads, err := setBaselinePreparedRead(&commentConfig, "PreparedReads", prepared)
	if err != nil {
		t.Fatal(err)
	}
	l.c, err = l.c.WithComments(context.Background(), commentConfig)
	if err != nil {
		t.Fatal(err)
	}
	enableImageLab(t, l)
	ctx := context.Background()
	owner := sessionTestAccount(t, l, "baseline_owner")
	identity := identityChange(t, l.c, owner.SessionToken, "CREATE", uuid.Nil, "基线作者").IdentityID
	selection, image := imageFixture(t, "png")
	intent := postTestIntent(t, l, identity)
	intent.Images = []postmedia.Selection{selection}
	accepted := postTestCommand(t, l, owner.SessionToken, postTestKey(t), intent)
	grant := imageGrant(t, l, owner.SessionToken, accepted, selection.MediaID)
	imageUpload(t, l, accepted, selection, grant, image)
	postTestPublish(t, l)
	for n := 1; n < 20; n++ {
		postTestCommand(t, l, owner.SessionToken, postTestKey(t), postTestIntent(t, l, identity))
	}
	postTestPublish(t, l)
	readers := [4][32]byte{}
	for n := range readers {
		readers[n] = sessionTestAccount(t, l, fmt.Sprintf("baseline_reader_%d", n)).SessionToken
	}
	// Establish the comment protocol key commitment through an authorized empty
	// read before inserting synthetic historical receipts into the owned fixture.
	if _, err = l.c.ReadComments(ctx, owner.SessionToken, CommentQuery{ID: accepted.PostID}); err != nil {
		t.Fatal(err)
	}
	seedReadBaselineComments(t, l, owner.AccountID, identity, accepted.PostID, comments, likes)
	_, snapshotConfig := snapshotSQLStore(t, l)
	if os.Getenv("REQ001_BASELINE_BACKEND") == "postgres" {
		snapshotConfig.CreateBackend = "postgres"
		l.c.comments.snapshotConfig = &snapshotConfig
	}
	manifest, err := l.c.ReadPostImages(ctx, readers[2], accepted.PostID, "")
	if err != nil {
		t.Fatal(err)
	}
	content := manifest.Value.(postmedia.Group).Images[0].ContentVersionID
	var likedComment string
	if err = l.cp.QueryRow(ctx, `SELECT comment_id::text FROM c_comments.tasks WHERE post_id=$1 ORDER BY comment_id LIMIT 1`, accepted.PostID).Scan(&likedComment); err != nil {
		t.Fatal(err)
	}
	poolConfig := l.cp.Config()
	poolConfig.MaxConns, poolConfig.MinConns = 12, 0
	poolConfig.ConnConfig.Tracer = baselinePoolTracer{reportError: func(code, operation string, pid uint32) {
		t.Logf("baseline SQL error state=%s operation=%s backend_pid=%d (SQL and parameters omitted)", code, operation, pid)
	}}
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	clone := *l.c
	clone.pool = pool
	l.c = &clone
	l.gate.pool = pool
	settingsTx, err := begin(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	var deadlockTimeout, lockTimeout, statementTimeout string
	err = settingsTx.QueryRow(ctx, `SELECT current_setting('deadlock_timeout'),current_setting('lock_timeout'),current_setting('statement_timeout')`).Scan(&deadlockTimeout, &lockTimeout, &statementTimeout)
	rollbackErr := settingsTx.Rollback(ctx)
	if err != nil || rollbackErr != nil {
		t.Fatal("cannot inspect fixture transaction timeouts", err, rollbackErr)
	}
	transactionSettings := map[string]string{"deadlock_timeout": deadlockTimeout, "lock_timeout": lockTimeout, "statement_timeout": statementTimeout}
	t.Logf("baseline transaction settings=%s", mustBaselineJSON(transactionSettings))
	operations := []struct {
		kind string
		run  func(context.Context, int) error
	}{
		{"comments", func(ctx context.Context, _ int) error {
			_, err := l.c.ReadComments(ctx, readers[0], CommentQuery{ID: accepted.PostID, Limit: 50})
			return err
		}},
		{"feed", func(ctx context.Context, _ int) error {
			_, err := l.c.ReadPosts(ctx, readers[1], PostQuery{Kind: PostFeed, Limit: 20})
			return err
		}},
		{"image", func(ctx context.Context, _ int) error {
			_, err := l.c.ReadPostImages(ctx, readers[2], accepted.PostID, content)
			return err
		}},
		{"like", func(ctx context.Context, n int) error {
			_, err := l.c.SetPostInteraction(ctx, readers[3], accepted.PostID, posts.PostLike, n%2 == 0)
			return err
		}},
	}
	likeTarget := "post-like-same-post"
	if commentLike {
		likeTarget = "comment-like-same-post"
		keys := make([]string, warmup+samples)
		for n := range keys {
			keys[n] = postTestKey(t)
		}
		operations[3].run = func(ctx context.Context, n int) error {
			liked := n%2 == 0
			_, err := l.c.ExecuteCommentCommand(ctx, readers[3], keys[n], commentmodel.Intent{Operation: commentmodel.SetLike, CommentID: likedComment, Liked: &liked})
			return err
		}
	}
	// Reuse a committed nonce so this test measures reads rather than growing
	// per-account live reservations into the quota ceiling during sampling.
	nonce := postTestKey(t)
	nonces := make([]string, warmup+samples)
	for n := range nonces {
		nonces[n] = nonce
		if fresh {
			nonces[n] = postTestKey(t)
		}
	}
	operations[0].run = func(ctx context.Context, n int) error {
		_, err := l.c.ReadComments(ctx, readers[0], CommentQuery{ID: accepted.PostID, Limit: 50, Sort: mode, SnapshotRequest: nonces[n]})
		return err
	}
	var mu sync.Mutex
	measurements := make([]readBaselineSample, 0, samples*len(operations))
	warmupFailures := make([]readBaselineSample, 0)
	_, cohortRawTrace := readtiming.Start(ctx)
	cohortClockStart := time.Now()
	for wave := -warmup; wave < samples; wave++ {
		start := make(chan struct{})
		var wg sync.WaitGroup
		// Fixed rotating dispatch order prevents one path always being first.
		for n := range operations {
			operation := operations[(n+wave+warmup)%len(operations)]
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				requestCtx, trace := readtiming.Start(ctx)
				requestCtx = context.WithValue(requestCtx, baselineOperationKey{}, operation.kind)
				started := time.Now()
				err := operation.run(requestCtx, wave+warmup)
				elapsed := time.Since(started).Nanoseconds()
				wallEnd := time.Now().UnixNano()
				observation := trace.Snapshot()
				wallElapsed := wallEnd - started.UnixNano()
				sample := readBaselineSample{Kind: operation.kind, Index: wave, ElapsedNS: elapsed, Observation: observation,
					WallStartNS: started.UnixNano(), WallEndNS: wallEnd, WallElapsedNS: wallElapsed, RawElapsedNS: observation.ElapsedNanoseconds,
					WallGoDriftNS: wallElapsed - elapsed, RawGoDriftNS: observation.ElapsedNanoseconds - elapsed,
					ClockStable: baselineClockStable(wallElapsed, elapsed, observation.ElapsedNanoseconds)}
				if err != nil {
					sample.Error = err.Error()
					var business *PostError
					if errors.As(err, &business) {
						sample.ErrorCode, sample.ErrorStatus = business.Code, business.Status
					} else if errors.Is(err, ErrAuthorizationUnavailable) {
						sample.ErrorCode, sample.ErrorStatus = "AUTHORIZATION_UNAVAILABLE", 503
					}
				}
				if wave >= 0 {
					mu.Lock()
					measurements = append(measurements, sample)
					mu.Unlock()
				} else if sample.Error != "" || !sample.ClockStable {
					mu.Lock()
					warmupFailures = append(warmupFailures, sample)
					mu.Unlock()
				}
			}()
		}
		close(start)
		wg.Wait()
	}
	cohortClockEnd := time.Now()
	cohortGoElapsed := cohortClockEnd.Sub(cohortClockStart).Nanoseconds()
	cohortWallElapsed := cohortClockEnd.UnixNano() - cohortClockStart.UnixNano()
	cohortRawElapsed := cohortRawTrace.Snapshot().ElapsedNanoseconds
	environmentValid := baselineClockStable(cohortWallElapsed, cohortGoElapsed, cohortRawElapsed)
	sort.Slice(measurements, func(i, j int) bool {
		if measurements[i].Index != measurements[j].Index {
			return measurements[i].Index < measurements[j].Index
		}
		return measurements[i].Kind < measurements[j].Kind
	})
	result := map[string]any{
		"specVersion": 1, "scope": "service-method-real-postgresql", "syntheticSeed": true, "productionDeployment": false,
		"sourceCommit": os.Getenv("REQ001_BASELINE_SOURCE"), "label": os.Getenv("REQ001_BASELINE_LABEL"),
		"comments": comments, "commentLikeRows": likes, "posts": 20, "pageSize": 50, "feedPageSize": 20, "poolMaxConns": 12,
		"warmupWaves": warmup, "sampleWaves": samples, "concurrency": 4, "mix": "comments:feed:image:like=1:1:1:1",
		"dispatch": "wave barrier; rotating launch order; next wave waits for all four completions",
		"nonce":    "Redis fixed nonce reuses committed reservation; PostgreSQL initial reads remain within existing quota", "snapshotBackend": snapshotConfig.CreateBackend,
		"freshNonce": fresh, "commentSort": mode, "likeTarget": likeTarget, "warmupFailures": warmupFailures,
		"preparedReads": preparedReads, "preparedFeedReads": preparedFeedReads, "preparedMediaReads": preparedMediaReads,
		"clock": "CLOCK_MONOTONIC_RAW stages; Go monotonic client elapsed", "samples": measurements,
		"goVersion": runtime.Version(), "goMaxProcs": runtime.GOMAXPROCS(0),
		"transactionSettings": transactionSettings,
		"cohortClock":         map[string]any{"wallStartNS": cohortClockStart.UnixNano(), "wallEndNS": cohortClockEnd.UnixNano(), "wallElapsedNS": cohortWallElapsed, "goElapsedNS": cohortGoElapsed, "rawElapsedNS": cohortRawElapsed, "wallGoDriftNS": cohortWallElapsed - cohortGoElapsed, "maximumDriftNS": int64(250 * time.Millisecond)},
		"holdBoundary":        "successful locking SELECT response to transaction completion response; failure boundary separately flagged; sums include traced precheck/final transactions; independent Abort freeze persistence is covered by gate_abort, not original gate_hold",
	}
	if fresh {
		result["nonce"] = "distinct initial snapshot nonce per wave; bounded within existing account quota"
	}
	stats := map[string]any{}
	failures := len(warmupFailures)
	errorCounts := map[string]int{}
	for _, operation := range operations {
		var elapsed, hold, waits, anchor, acquire []int64
		var holdCalls int64
		var conflicts, rebuilds int64
		requestFailures := 0
		for _, sample := range measurements {
			if sample.Kind != operation.kind {
				continue
			}
			if sample.Error != "" || !sample.Observation.ClockValid || sample.Observation.Truncated || !sample.ClockStable {
				failures++
			}
			if !sample.ClockStable {
				environmentValid = false
			}
			if sample.Error != "" {
				requestFailures++
				errorCounts[fmt.Sprintf("%d:%s", sample.ErrorStatus, sample.ErrorCode)]++
			}
			elapsed = append(elapsed, sample.ElapsedNS)
			holdNS, waitNS, anchorNS, acquireNS := int64(0), int64(0), int64(0), int64(0)
			requestHoldCalls, requestAnchorCalls := int64(0), int64(0)
			for name, stage := range sample.Observation.Stages {
				switch {
				case strings.HasSuffix(name, "gate_hold"):
					holdNS += stage.Nanoseconds
					holdCalls += stage.Calls
					requestHoldCalls += stage.Calls
				case strings.HasSuffix(name, "gate_row_lock"):
					waitNS += stage.Nanoseconds
				case strings.HasSuffix(name, "anchor_write"):
					anchorNS += stage.Nanoseconds
					requestAnchorCalls += stage.Calls
				case strings.HasSuffix(name, "pool_wait"):
					acquireNS += stage.Nanoseconds
				case strings.HasSuffix(name, "gate_hold_boundary_uncertain"):
					failures++
				case strings.HasSuffix(name, "prepared_read_conflict"):
					conflicts += stage.Calls
				case strings.HasSuffix(name, "prepared_read_rebuild"):
					rebuilds += stage.Calls
				}
			}
			if requestHoldCalls == 0 || requestAnchorCalls != 1 {
				failures++
			}
			hold, waits, anchor, acquire = append(hold, holdNS), append(waits, waitNS), append(anchor, anchorNS), append(acquire, acquireNS)
		}
		stats[operation.kind] = map[string]any{"elapsed": baselineQuantiles(elapsed), "gateHoldPerRequest": baselineQuantiles(hold), "gateHoldCalls": holdCalls,
			"gateLockQueryIncludingWait": baselineQuantiles(waits), "anchorWrite": baselineQuantiles(anchor), "poolAcquire": baselineQuantiles(acquire), "requestFailures": requestFailures,
			"readConflicts": conflicts, "readRebuilds": rebuilds}
	}
	result["stats"], result["failures"], result["errorCounts"] = stats, failures, errorCounts
	for _, sample := range warmupFailures {
		if !sample.ClockStable {
			environmentValid = false
		}
	}
	result["environmentValid"] = environmentValid
	result["environmentResult"] = "PASS"
	if !environmentValid {
		result["environmentResult"] = "INVALID"
		failures++
		result["failures"] = failures
	}
	if failures != 0 {
		var state, reason string
		if err := l.cp.QueryRow(ctx, `SELECT gate_state,COALESCE(freeze_reason,'') FROM c_auth.authorization_gate WHERE singleton_id=1`).Scan(&state, &reason); err == nil {
			t.Logf("failed measurement Gate state=%s reason=%s", state, reason)
		}
	}
	result["result"] = "PASS"
	if failures != 0 {
		result["result"] = "FAIL"
	}
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(output, append(encoded, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("REQ001 baseline comments=%d samples=%d failures=%d artifact=%s stats=%s", comments, samples, failures, output, mustBaselineJSON(stats))
	if failures != 0 {
		t.Fatal("mixed load contained failures or invalid measurements")
	}
}

// Reflect only exported bools so the same six-file overlay compiles against
// the fixed old product. A disabled absent flag is the old path; an enabled
// absent flag must fail before measurements can imply coverage.
func setBaselinePreparedRead(config any, name string, enabled bool) (bool, error) {
	value := reflect.ValueOf(config)
	if value.Kind() != reflect.Pointer || value.IsNil() || value.Elem().Kind() != reflect.Struct {
		return false, errors.New("invalid prepared-read configuration")
	}
	field := value.Elem().FieldByName(name)
	if !field.IsValid() {
		if !enabled {
			return false, nil
		}
		return false, fmt.Errorf("requested prepared-read feature %s is unavailable", name)
	}
	if field.Kind() != reflect.Bool || !field.CanSet() {
		return false, fmt.Errorf("prepared-read feature %s must be an exported bool", name)
	}
	field.SetBool(enabled)
	return field.Bool(), nil
}

func baselineClockStable(wall, monotonic, raw int64) bool {
	if wall < 0 || monotonic < 0 || raw < 0 {
		return false
	}
	for _, drift := range []int64{wall - monotonic, raw - monotonic, wall - raw} {
		if drift > int64(250*time.Millisecond) || drift < -int64(250*time.Millisecond) {
			return false
		}
	}
	return true
}

func TestBaselineClockGuardRejectsWallJumpDespiteMatchingMonotonicClocks(t *testing.T) {
	if baselineClockStable(int64(11300*time.Millisecond), int64(83*time.Millisecond), int64(83*time.Millisecond)) {
		t.Fatal("matching Go and RAW clocks hid a wall-clock discontinuity")
	}
	if !baselineClockStable(int64(100*time.Millisecond), int64(100*time.Millisecond), int64(100*time.Millisecond+time.Microsecond)) {
		t.Fatal("stable observation was rejected")
	}
	if baselineClockStable(-1, 0, 0) || baselineClockStable(int64(time.Second), int64(time.Second), int64(1500*time.Millisecond)) {
		t.Fatal("negative wall time or RAW drift was accepted")
	}
}

func TestBaselineSQLCancellationUsesOnlyWhitelistedReasons(t *testing.T) {
	for _, test := range []struct{ message, class string }{
		{"canceling statement due to user request", "user_cancel"},
		{"canceling statement due to statement timeout", "statement_timeout"},
		{"canceling statement due to conflict with recovery", "recovery_conflict"},
		{"unrecognized server text", "query_cancelled"},
	} {
		ctx, trace := readtiming.Start(context.Background())
		baselinePoolTracer{}.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{Err: &pgconn.PgError{Code: "57014", Message: test.message}})
		if trace.Snapshot().Stages["baseline_sql_error_"+test.class].Calls != 1 {
			t.Fatal("unexpected safe cancellation classification", test.class)
		}
	}
}

func TestBaselinePreparedSwitchAcceptsFunctionFieldsAndRejectsUnsupportedSource(t *testing.T) {
	config := struct {
		ObserveRead        func(CommentReadTiming)
		PreparedReads      bool
		PreparedFeedReads  bool
		PreparedMediaReads bool
	}{}
	for _, name := range []string{"PreparedReads", "PreparedFeedReads", "PreparedMediaReads"} {
		for _, enabled := range []bool{true, false} {
			actual, err := setBaselinePreparedRead(&config, name, enabled)
			if err != nil || actual != enabled || reflect.ValueOf(config).FieldByName(name).Bool() != enabled {
				t.Fatal("function-valued configuration prevented the exported feature switch", name, err)
			}
		}
		unsupported := struct{ ObserveRead func(CommentReadTiming) }{}
		if actual, err := setBaselinePreparedRead(&unsupported, name, false); err != nil || actual {
			t.Fatal("old disabled source was rejected", name, err)
		}
		if _, err := setBaselinePreparedRead(&unsupported, name, true); err == nil {
			t.Fatal("source without the requested feature was silently accepted", name)
		}
	}
	for _, invalid := range []any{nil, config, &struct{ PreparedReads string }{}, &struct{ preparedReads bool }{}} {
		name := "PreparedReads"
		if _, ok := invalid.(*struct{ preparedReads bool }); ok {
			name = "preparedReads"
		}
		if _, err := setBaselinePreparedRead(invalid, name, true); err == nil {
			t.Fatal("invalid or non-exported switch was accepted")
		}
	}
}

func seedReadBaselineComments(t *testing.T, l *lab, owner, identity uuid.UUID, post string, size, likes int) {
	t.Helper()
	ctx := context.Background()
	tx, err := l.cp.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	statements := []struct {
		sql  string
		args []any
	}{
		{`CREATE TEMP TABLE req001_comment_seed ON COMMIT DROP AS SELECT md5('req001-task-'||i)::uuid AS task,md5('req001-comment-'||i)::uuid AS comment,clock_timestamp()-interval '1 day'+i*interval '1 millisecond' AS at,i FROM generate_series(1,$1::int) i`, []any{size}},
		{`CREATE INDEX req001_receipt_lookup ON c_comments.receipts(task_id,version)`, nil},
		{`INSERT INTO c_comments.tasks(task_id,comment_id,owner_account_id,post_id,identity_id) SELECT task,comment,$1,$2,$3 FROM req001_comment_seed`, []any{owner, post, identity}},
		{`INSERT INTO c_comments.attempts(task_id,version,state,accepted_at,accepted_stop_generation,body) SELECT task,1,'ACCEPTED',at,0,'合成评论 '||i||' '||repeat(md5(i::text),8) FROM req001_comment_seed`, nil},
		{`INSERT INTO c_comments.receipts(key_digest,owner_account_id,intent_fingerprint,operation,outcome,task_id,version,committed_at) SELECT decode(md5(task::text)||md5(task::text),'hex'),$1,decode(repeat('00',32),'hex'),'CREATE','ACCEPTED',task,1,at FROM req001_comment_seed`, []any{owner}},
		{`UPDATE c_comments.attempts a SET state='PUBLISHED',terminal_at=s.at FROM req001_comment_seed s WHERE a.task_id=s.task`, nil},
		{`INSERT INTO c_comments.likes(account_id,comment_id) SELECT $1,comment FROM req001_comment_seed WHERE i<=$2`, []any{owner, likes}},
		{`SET CONSTRAINTS ALL IMMEDIATE`, nil},
		{`DROP INDEX c_comments.req001_receipt_lookup`, nil},
	}
	for n, statement := range statements {
		if _, err = tx.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("synthetic baseline seed statement %d: %v", n, err)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = l.cp.Exec(ctx, `ANALYZE c_comments.tasks; ANALYZE c_comments.attempts; ANALYZE c_comments.likes; ANALYZE c_posts.posts`); err != nil {
		t.Fatal(err)
	}
}

func baselineQuantiles(values []int64) map[string]int64 {
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	result := map[string]int64{}
	for name, percentile := range map[string]float64{"p50NS": .5, "p95NS": .95, "p99NS": .99} {
		if len(values) != 0 {
			result[name] = values[int(math.Ceil(percentile*float64(len(values))))-1]
		}
	}
	return result
}

func mustBaselineJSON(value any) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
