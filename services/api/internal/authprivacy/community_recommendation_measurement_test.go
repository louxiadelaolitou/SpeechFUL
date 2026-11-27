package authprivacy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
)

type recommendationSQLCount struct {
	Calls, Statements, RowsAffected int
	Categories                      map[string]int
}
type recommendationSQLTrace struct {
	mu     sync.Mutex
	active bool
	count  recommendationSQLCount
}

func (t *recommendationSQLTrace) begin() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.active = true
	t.count = recommendationSQLCount{Categories: map[string]int{}}
}
func (t *recommendationSQLTrace) finish() recommendationSQLCount {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.active = false
	return t.count
}
func (t *recommendationSQLTrace) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.active {
		return ctx
	}
	sql := strings.TrimSpace(d.SQL)
	n := strings.Count(strings.TrimRight(sql, ";"), ";") + 1
	category := "other"
	switch {
	case strings.Contains(sql, "/* hnuhole:post_projection */") || strings.Contains(sql, "/* list:projection */"):
		category = "A1_content_projection"
	case strings.Contains(sql, "WITH keys AS MATERIALIZED"):
		category = "candidate_keys"
	case strings.Contains(sql, "comment_keys AS MATERIALIZED"):
		category = "candidate_facts"
	case strings.Contains(sql, "c_recommendation"):
		category = "recommendation_state"
	case strings.Contains(sql, "c_auth.authorization_gate"):
		category = "gate"
	case strings.Contains(sql, "c_auth"):
		category = "account_session"
	case strings.Contains(sql, "c_posts.feed_preferences") || strings.Contains(sql, "public.channels"):
		category = "scope"
	case strings.Contains(sql, "unnest($1::uuid[])"):
		category = "test_projection_port"
	case strings.Contains(sql, "c_posts.protocol_keys"):
		category = "post_protocol"
	case strings.HasPrefix(strings.ToLower(sql), "begin") || strings.HasPrefix(strings.ToLower(sql), "commit") || strings.HasPrefix(strings.ToLower(sql), "rollback") || strings.HasPrefix(sql, "SET LOCAL"):
		category = "transaction"
	}
	t.count.Calls++
	t.count.Statements += n
	t.count.Categories[category] += n
	return ctx
}
func (t *recommendationSQLTrace) TraceQueryEnd(_ context.Context, _ *pgx.Conn, d pgx.TraceQueryEndData) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.active {
		t.count.RowsAffected += int(d.CommandTag.RowsAffected())
	}
}

func TestRecommendationSQLCountPlansAndPrivateFingerprintsPostgres(t *testing.T) {
	p := newRecommendationPG(t, 12)
	ctx := context.Background()
	a := sessionTestAccount(t, p.l, "reco_measure")
	counts := map[string]recommendationSQLCount{}
	p.trace.begin()
	request := p.request(t, a)
	counts["bootstrap_internal_seam"] = p.trace.finish()
	p.trace.begin()
	page := p.start(t, a, request)
	counts["start"] = p.trace.finish()
	p.trace.begin()
	p.start(t, a, request)
	counts["same_intent_retry"] = p.trace.finish()
	p.trace.begin()
	p.read(t, a, page.RoundID, 10)
	counts["next_page"] = p.trace.finish()
	p.trace.begin()
	if _, e := p.service.FeedbackRecommendation(ctx, a.SessionToken, "", page.RoundID, 0, []int{0, 1}); e != nil {
		t.Fatal(e)
	}
	counts["feedback"] = p.trace.finish()
	if counts["start"].Categories["candidate_keys"] != 1 || counts["start"].Categories["candidate_facts"] != 1 || counts["same_intent_retry"].Categories["candidate_keys"] != 0 {
		t.Fatal("candidate or retry count boundary")
	}
	for _, c := range counts {
		if c.Statements == 0 || c.Calls == 0 {
			t.Fatal("empty actual SQL trace")
		}
	}
	scopeResponse, e := p.service.RecommendationContext(ctx, a.SessionToken, "")
	if e != nil {
		t.Fatal(e)
	}
	scope := scopeResponse.Value.(RecommendationScope)
	plans := map[string]json.RawMessage{}
	for name, q := range map[string]string{"base": recommendationBaseSQL, "facts": recommendationFactsSQL} {
		var raw []byte
		var e error
		if name == "base" {
			e = p.runtime.QueryRow(ctx, `EXPLAIN (ANALYZE,BUFFERS,FORMAT JSON) `+q, scope.At, scope.Channels, string(scope.Binding.Account)).Scan(&raw)
		} else {
			e = p.runtime.QueryRow(ctx, `EXPLAIN (ANALYZE,BUFFERS,FORMAT JSON) `+q, scope.At).Scan(&raw)
		}
		if e != nil {
			t.Fatal(e)
		}
		plans[name] = raw
	}
	var dataFingerprint string
	if e = p.l.cp.QueryRow(ctx, `SELECT md5(COALESCE(string_agg(row_to_json(h)::text,E'\n' ORDER BY account_id,post_id),'')) FROM c_recommendation.history h`).Scan(&dataFingerprint); e != nil {
		t.Fatal(e)
	}
	var actualRole string
	var roleGrantFingerprint string
	if e = p.runtime.QueryRow(ctx, `SELECT md5(COALESCE(string_agg(table_name||':'||column_name||':'||privilege_type,E'\n' ORDER BY table_name,column_name,privilege_type),'')) FROM information_schema.column_privileges WHERE table_schema='c_recommendation' AND grantee=current_user`).Scan(&roleGrantFingerprint); e != nil {
		t.Fatal("actual private grants fingerprint")
	}
	var super, createdb, createrole, temp, createSchema bool
	if e = p.runtime.QueryRow(ctx, `SELECT current_user,rolsuper,rolcreatedb,rolcreaterole,has_database_privilege(current_user,current_database(),'TEMP'),has_schema_privilege(current_user,'c_recommendation','CREATE') FROM pg_roles WHERE rolname=current_user`).Scan(&actualRole, &super, &createdb, &createrole, &temp, &createSchema); e != nil || super || createdb || createrole || temp || createSchema {
		t.Fatal("actual runtime authority", e)
	}
	out := os.Getenv("HNUB3B_EVIDENCE_DIR")
	if out == "" {
		t.Fatal("owned evidence destination missing")
	}
	evidence := map[string]any{"scope": "real low-concurrency Community methods; test projection port; not HTTP/production runtime", "sql": counts, "plans": plans, "historyDataFingerprint": dataFingerprint, "role": actualRole, "superuser": super, "createdb": createdb, "createrole": createrole, "temp": temp, "schemaCreate": createSchema, "capacityProfile": "SYNTHETIC_NOT_A_MEASUREMENT", "serverInternalSQL": "driver count excludes nested trigger/function statements; full accounting shape and work charges are separately specified in migration/report", "rowMutationBudget": "insert/delete=1, update=2; cascade and ledger overhead covered by synthetic work/growth profile, not a measured latency or cleanup SLA"}
	evidence["privateColumnGrantMD5"] = roleGrantFingerprint
	evidence["configSnapshot"] = map[string]any{"selector": p.config.Selector, "policy": p.config.Policy, "capacity": p.config.Capacity, "enabled": true, "admissionEnabled": true, "feedbackEnabled": true, "likeTimeEnabled": true, "syntheticLab": true, "newRound": "crypto UUID; fixed seed 20261025", "cleanupBatch": "16..128 changed data rows including cascades and account revision; maxRows objects; <=2*maxRows data mutations; cooperative 2s ceiling, inherited Gate flock limits remain", "writeDebt": "saturating successful-mutation telemetry; not a hard quota or physical proof"}
	var actualControl string
	if e := p.l.cp.QueryRow(ctx, `SELECT row_to_json(c)::text FROM c_recommendation.control c`).Scan(&actualControl); e != nil {
		t.Fatal(e)
	}
	var controlSnapshot map[string]any
	if e := json.Unmarshal([]byte(actualControl), &controlSnapshot); e != nil {
		t.Fatal(e)
	}
	evidence["actualControl"] = controlSnapshot
	b, e := json.MarshalIndent(evidence, "", "  ")
	if e != nil || os.WriteFile(filepath.Join(out, "B-B3b-sql-plans.json"), append(b, '\n'), 0600) != nil {
		t.Fatal("write numeric-only evidence")
	}
	cfg := p.l.cp.Config().ConnConfig
	command := exec.Command("pg_dump", "--schema-only", "--no-owner", "--no-privileges", "--schema=c_recommendation", "-h", cfg.Host, "-p", strconv.Itoa(int(cfg.Port)), "-U", cfg.User, "-d", cfg.Database)
	command.Env = append(os.Environ(), "PGPASSWORD="+cfg.Password)
	dump, e := command.Output()
	if e != nil {
		t.Fatal("owned schema fingerprint export")
	}
	if e = os.WriteFile(filepath.Join(out, "B-B3b-schema.sql"), dump, 0600); e != nil {
		t.Fatal(e)
	}
	sum := sha256.Sum256(dump)
	t.Log("own private schema SHA256", hex.EncodeToString(sum[:]))
	for name, c := range counts {
		t.Log("SQL", name, "client statements", c.Statements, "calls", c.Calls)
	}
}
