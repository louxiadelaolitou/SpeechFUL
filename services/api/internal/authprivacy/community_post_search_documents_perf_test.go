package authprivacy

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/posts"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/readtiming"
)

type searchPerfResult struct {
	Case       string                `json:"case"`
	Phase      string                `json:"phase"`
	Round      int                   `json:"round"`
	OldP95MS   float64               `json:"oldP95Ms"`
	NewP95MS   float64               `json:"newP95Ms"`
	Ratio      float64               `json:"ratio"`
	Stages     map[string][2]float64 `json:"stageP95MsOldNew"`
	OldSamples []float64             `json:"oldSamplesMs"`
	NewSamples []float64             `json:"newSamplesMs"`
}

type searchPerfReport struct {
	DiagnosticOnly  bool                       `json:"diagnosticOnly"`
	Rows            int                        `json:"rows"`
	Rounds          int                        `json:"rounds"`
	Samples         int                        `json:"samplesPerModePerRound"`
	Warmups         int                        `json:"warmupsPerModePerRound"`
	Concurrency     int                        `json:"concurrency"`
	Postgres        string                     `json:"postgres"`
	BackfillMS      float64                    `json:"backfillMs"`
	MaxBatchMS      float64                    `json:"maxBackfillBatchMs"`
	IndexBytes      int64                      `json:"indexBytes"`
	ProjectionBytes int64                      `json:"projectionBytes"`
	WriteP95MS      [2]float64                 `json:"writeStatementP95MsOldNew"`
	Results         []searchPerfResult         `json:"results"`
	Plans           map[string]json.RawMessage `json:"plans"`
	Completed       bool                       `json:"completed"`
	Accepted        bool                       `json:"accepted"`
}

// Captures the actual production candidate statement, including all injected
// visibility filters, so EXPLAIN is never a hand-simplified substitute.
type searchPerfPlanTrace struct {
	sql  string
	args []any
}

func (tr *searchPerfPlanTrace) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	if strings.Contains(d.SQL, "WITH matched AS (") {
		tr.sql = d.SQL
		tr.args = append([]any(nil), d.Args...)
	}
	return ctx
}
func (*searchPerfPlanTrace) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func searchPerfP95(values []float64) float64 {
	x := append([]float64(nil), values...)
	sort.Float64s(x)
	return x[int(math.Ceil(float64(len(x))*.95))-1]
}

// Opt-in expensive product acceptance. No DSN is accepted here: newGateControl
// requires the existing explicitly owned disposable C/V authlab databases.
// Constants below are the frozen REQ-005 workload, not tunable success knobs.
func TestFeedPostSearchDocumentsPerformancePostgres(t *testing.T) {
	if os.Getenv("POST_SEARCH_PERF") != "1" {
		t.Skip("opt-in 100k search performance acceptance")
	}
	control := newGateControl(t)
	l, ctx := control.lab, context.Background()
	enablePostLab(t, l)
	owner := sessionTestAccount(t, l, "search_perf_owner")
	reader := sessionTestAccount(t, l, "search_perf_reader")
	identity := identityChange(t, l.c, owner.SessionToken, "CREATE", uuid.Nil, "性能作者").IdentityID
	intent := postTestIntent(t, l, identity)
	intent.Title = "性能基线"
	intent.Body = "学校校园生活"
	postTestCommand(t, l, owner.SessionToken, postTestKey(t), intent)
	postTestPublish(t, l)
	channels := feedTestChannels(t, l)
	report := searchPerfReport{Rows: 100000, Rounds: 3, Samples: 20, Warmups: 3, Concurrency: 1, Plans: make(map[string]json.RawMessage)}
	if os.Getenv("POST_SEARCH_PERF_QUICK") == "1" {
		report.DiagnosticOnly = true
		report.Rounds = 1
		report.Samples = 3
		report.Warmups = 1
	}
	defer func() {
		report.Accepted = report.Completed && !report.DiagnosticOnly && !t.Failed()
		if path := os.Getenv("POST_SEARCH_PERF_OUTPUT"); path != "" {
			data, err := json.MarshalIndent(report, "", "  ")
			if err != nil {
				t.Error(err)
				return
			}
			if err = os.WriteFile(path, data, 0600); err != nil {
				t.Error(err)
			}
		}
	}()
	if err := l.cp.QueryRow(ctx, `SELECT version()`).Scan(&report.Postgres); err != nil {
		t.Fatal(err)
	}
	tx, err := l.cp.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	// Model pre-existing history. Only the new projection hook is temporarily
	// disabled in this private fixture, then restored before it commits.
	for _, sql := range []string{`UPDATE c_posts.post_search_document_state SET ready=false`, `ALTER TABLE c_posts.posts DISABLE TRIGGER post_search_document_post`} {
		if _, err = tx.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = tx.Exec(ctx, `CREATE TEMP TABLE search_perf_seed ON COMMIT DROP AS
 SELECT n,md5('req005.post.'||n)::uuid AS post_id,md5('req005.task.'||n)::uuid AS task_id,
 ($1::uuid[])[1+n%cardinality($1::uuid[])] AS channel_id,$2::timestamptz-n*interval '1 millisecond' AS at,
 CASE WHEN n%997=0 THEN '稀有词' ELSE '校园生活'||(n%97) END AS title,
 repeat('学校校园生活学习交流，固定种子里的普通讨论。',8)||md5(n::text)||
 CASE WHEN n%701=0 THEN ' needle' ELSE '' END AS body,
 ARRAY['校园',CASE WHEN n%571=0 THEN 'Lab研究' ELSE '日常' END]::text[] AS tags
 FROM generate_series(1,99999) n`, channels, control.clock.now()); err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO c_posts.posts(post_id,owner_account_id,channel_id) SELECT post_id,$1,channel_id FROM search_perf_seed`, []any{owner.AccountID}},
		{`INSERT INTO c_posts.post_identity_bindings(account_id,post_id,identity_id,bound_at) SELECT $1,post_id,$2,at FROM search_perf_seed`, []any{owner.AccountID, identity}},
		{`INSERT INTO c_posts.publication_tasks(task_id,post_id,owner_account_id,identity_id,channel_id,latest_attempt_version) SELECT task_id,post_id,$1,$2,channel_id,1 FROM search_perf_seed`, []any{owner.AccountID, identity}},
		{`INSERT INTO c_posts.publication_attempts(task_id,version,accepted_stop_generation,state,accepted_at,acceptance_ordinal,terminal_at) SELECT task_id,1,0,'PUBLISHED',at,nextval('c_posts.acceptance_ordinal_seq'),at FROM search_perf_seed`, nil},
		{`INSERT INTO c_posts.attempt_contents(task_id,version,title,body,request_digest,tags) SELECT task_id,1,title,body,decode(repeat('11',32),'hex'),tags FROM search_perf_seed`, nil},
		{`UPDATE c_posts.posts p SET visibility='PUBLISHED',published_attempt_version=1,published_at=s.at,publication_ordinal=nextval('c_posts.publication_ordinal_seq') FROM search_perf_seed s WHERE s.post_id=p.post_id`, nil},
		{`SET CONSTRAINTS ALL IMMEDIATE`, nil},
		{`ALTER TABLE c_posts.posts ENABLE TRIGGER post_search_document_post`, nil},
	} {
		if _, err = tx.Exec(ctx, step.sql, step.args...); err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	t.Log("fixed 100,000-post history committed with source constraints checked")
	if count(t, l.cp, `SELECT count(*) FROM c_posts.posts WHERE visibility='PUBLISHED'`) != report.Rows {
		t.Fatal("wrong performance population")
	}
	started := time.Now()
	for {
		var n int
		at := time.Now()
		if err = l.cp.QueryRow(ctx, `SELECT c_posts.backfill_post_search_documents(1000)`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n < 0 {
			t.Fatal("unexpected concurrent source writer in frozen workload")
		}
		report.MaxBatchMS = math.Max(report.MaxBatchMS, float64(time.Since(at))/1e6)
		if n == 0 {
			break
		}
	}
	report.BackfillMS = float64(time.Since(started)) / 1e6
	t.Logf("backfill completed %.0fms, maximum 1000-row batch %.0fms", report.BackfillMS, report.MaxBatchMS)
	mustExec(t, l.cp, `SELECT c_posts.validate_post_search_documents()`)
	if err = l.c.checkPostSearchDocumentSchema(ctx); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"c_posts.posts", "c_posts.publication_tasks", "c_posts.attempt_contents", "c_posts.post_search_documents", "c_posts.blocked_authors", "c_posts.hidden_posts"} {
		mustExec(t, l.cp, `ANALYZE `+rel)
	}
	if err = l.cp.QueryRow(ctx, `SELECT pg_relation_size('c_posts.post_search_documents_trgm_idx'),pg_total_relation_size('c_posts.post_search_documents')`).Scan(&report.IndexBytes, &report.ProjectionBytes); err != nil {
		t.Fatal(err)
	}
	trace := &searchPerfPlanTrace{}
	cfg := l.cp.Config()
	cfg.ConnConfig.Tracer = trace
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	legacy, optimized := *l.c, *l.c
	legacy.pool, optimized.pool = pool, pool
	service := *l.c.posts
	service.searchDocuments = true
	optimized.posts = &service
	for _, scenario := range []struct {
		name, keyword string
		selective     bool
	}{
		{"selective_chinese", "稀有词", true}, {"selective_ascii", "needle", true}, {"selective_tag_mixed", "Lab研究", true},
		{"short_1", "学", false}, {"short_2", "校园", false}, {"high_hit", "校园生活", false},
	} {
		for _, phase := range []string{"first", "continuation"} {
			q := posts.SearchQuery{Keyword: scenario.keyword, Sort: posts.SearchRelevance, Limit: 20}
			if phase == "continuation" {
				response, err := legacy.SearchPosts(ctx, reader.SessionToken, q)
				if err != nil {
					t.Fatal(err)
				}
				cursor := response.Value.(posts.FeedPage).NextCursor
				if cursor == nil {
					t.Fatal("missing continuation fixture")
				}
				q.Cursor = *cursor
			}
			for round := 0; round < report.Rounds; round++ {
				var samples [2][]float64
				var observations [2][]readtiming.Observation
				var results [2]posts.FeedPage
				for sample := -report.Warmups; sample < report.Samples; sample++ {
					for order := 0; order < 2; order++ {
						mode := (order + sample + report.Warmups + round) % 2
						c := &legacy
						if mode == 1 {
							c = &optimized
						}
						request, observation := readtiming.Start(ctx)
						at := time.Now()
						response, err := c.SearchPosts(request, reader.SessionToken, q)
						elapsed := float64(time.Since(at)) / 1e6
						if err != nil {
							t.Fatalf("%s/%s round%d mode%d: %v", scenario.name, phase, round, mode, err)
						}
						results[mode] = response.Value.(posts.FeedPage)
						if sample >= 0 {
							samples[mode] = append(samples[mode], elapsed)
							observations[mode] = append(observations[mode], observation.Snapshot())
						}
						key := fmt.Sprintf("%s/%s/%d", scenario.name, phase, mode)
						if sample == -report.Warmups && round == 0 {
							if trace.sql == "" {
								t.Fatal("production SQL not captured")
							}
							var plan []byte
							if err = pool.QueryRow(ctx, `EXPLAIN (ANALYZE,BUFFERS,FORMAT JSON) `+trace.sql, trace.args...).Scan(&plan); err != nil {
								t.Fatal(err)
							}
							report.Plans[key] = json.RawMessage(plan)
						}
					}
					if !reflect.DeepEqual(results[0].Items, results[1].Items) || (results[0].NextCursor == nil) != (results[1].NextCursor == nil) {
						t.Fatal("performance workload changed search results")
					}
				}
				entry := searchPerfResult{Case: scenario.name, Phase: phase, Round: round + 1, OldSamples: samples[0], NewSamples: samples[1], Stages: make(map[string][2]float64)}
				entry.OldP95MS, entry.NewP95MS = searchPerfP95(samples[0]), searchPerfP95(samples[1])
				entry.Ratio = entry.NewP95MS / entry.OldP95MS
				names := make(map[string]bool)
				for mode := 0; mode < 2; mode++ {
					for name := range observations[mode][0].Stages {
						names[name] = true
					}
				}
				for name := range names {
					var values [2][]float64
					for mode := 0; mode < 2; mode++ {
						for _, observation := range observations[mode] {
							if !observation.ClockValid || observation.Truncated {
								t.Fatal("invalid timing observation")
							}
							values[mode] = append(values[mode], float64(observation.Stages[name].Nanoseconds)/1e6)
						}
					}
					entry.Stages[name] = [2]float64{searchPerfP95(values[0]), searchPerfP95(values[1])}
				}
				report.Results = append(report.Results, entry)
				t.Logf("%s/%s round%d old=%.2fms new=%.2fms ratio=%.3f", scenario.name, phase, round+1, entry.OldP95MS, entry.NewP95MS, entry.Ratio)
				threshold := 1.10
				if scenario.selective {
					threshold = .50
				}
				if !report.DiagnosticOnly && entry.Ratio > threshold {
					t.Errorf("hard performance gate: %s/%s round%d ratio %.3f > %.2f", scenario.name, phase, round+1, entry.Ratio, threshold)
				}
			}
		}
	}
	// Isolate additional statement cost; both modes retain every other source
	// hook, and the complete transaction (including trigger DDL) is rolled back.
	for mode := 0; mode < 2; mode++ {
		var elapsed []float64
		for sample := 0; sample < 20; sample++ {
			tx, err := l.cp.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if mode == 0 {
				if _, err = tx.Exec(ctx, `ALTER TABLE c_posts.posts DISABLE TRIGGER post_search_document_post`); err != nil {
					t.Fatal(err)
				}
			}
			at := time.Now()
			_, err = tx.Exec(ctx, `UPDATE c_posts.posts SET edit_revision=gen_random_uuid(),edited_at=now(),edited_title='成本测量',edited_body=repeat('写入成本普通正文',24)||$1 WHERE post_id=md5('req005.post.997')::uuid`, fmt.Sprint(sample))
			elapsed = append(elapsed, float64(time.Since(at))/1e6)
			if err != nil {
				t.Fatal(err)
			}
			if err = tx.Rollback(ctx); err != nil {
				t.Fatal(err)
			}
		}
		report.WriteP95MS[mode] = searchPerfP95(elapsed)
	}
	report.Completed = true
}
