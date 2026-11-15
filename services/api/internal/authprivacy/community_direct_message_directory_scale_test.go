package authprivacy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

type dmScaleViewer struct {
	name     string
	account  SignupResult
	identity uuid.UUID
	peers    []uuid.UUID
}

type dmScaleExplainSpec struct {
	sql  string
	args []any
}

type dmScaleMeasurement struct {
	Viewer                          string           `json:"viewer"`
	Conversations                   int              `json:"conversations"`
	PageItems                       int              `json:"pageItems"`
	LegacyNoIndexNS                 map[string]int64 `json:"legacyNoIndexNs"`
	LegacyNS                        map[string]int64 `json:"legacyNs"`
	PreparedNS                      map[string]int64 `json:"preparedNs"`
	P95ChangePercent                float64          `json:"p95ChangePercentVsPreIndexLegacy"`
	P95ChangeVsIndexedLegacyPercent float64          `json:"p95ChangePercentVsIndexedLegacy"`
}

// TestDMDirectoryScalePostgres measures the production directory methods on a
// synthetic million-conversation dataset. It is opt-in because seeding and the
// sequential measurements take minutes and must not run in ordinary CI.
//
// The performance baseline is the production state before 0043: the fallback
// query without the cursor indexes. Measuring the fallback with the new
// indexes installed only compares two paths that both benefit from the index,
// which hides the migration's actual effect.
func TestDMDirectoryScalePostgres(t *testing.T) {
	if os.Getenv("DM_DIRECTORY_SCALE") != "1" {
		t.Skip("opt-in million-conversation directory measurement")
	}
	if os.Getenv("AUTHLAB_C_DSN") == "" || os.Getenv("AUTHLAB_V_DSN") == "" {
		t.Fatal("owned C/V lab DSNs required")
	}
	output := os.Getenv("DM_DIRECTORY_SCALE_OUTPUT")
	if output != "" {
		if !filepath.IsAbs(output) || !strings.HasSuffix(output, ".json") {
			t.Fatal("DM_DIRECTORY_SCALE_OUTPUT must be an absolute JSON artifact path")
		}
		root, e := filepath.Abs("../../../..")
		if e != nil || output == root || strings.HasPrefix(output, root+string(os.PathSeparator)) {
			t.Fatal("scale artifacts must stay outside source")
		}
	}
	conversations := baselineInt(t, "DM_DIRECTORY_SCALE_CONVERSATIONS", 1_000_000, 10_000, 2_000_000)
	samples := baselineInt(t, "DM_DIRECTORY_SCALE_SAMPLES", 30, 5, 200)
	warmup := baselineInt(t, "DM_DIRECTORY_SCALE_WARMUP", 5, 1, 50)
	explainRequested := os.Getenv("DM_DIRECTORY_SCALE_EXPLAIN") == "1"
	l, viewers, peerCount := newDMDirectoryScaleFixture(t, conversations)
	ctx := context.Background()
	legacy := *l.c
	legacy.dmDirectoryReads = false

	pageOf := func(v dmScaleViewer) int {
		if len(v.peers) > dmDirectoryPageLimit {
			return dmDirectoryPageLimit
		}
		return len(v.peers)
	}
	read := func(v dmScaleViewer, c *Community) int64 {
		t.Helper()
		start := time.Now()
		response, e := c.ReadDMChatDirectory(ctx, v.account.SessionToken, v.identity.String(), "")
		if e != nil {
			t.Fatalf("%s directory: %v", v.name, e)
		}
		items := dmDirectoryValue(t, response).Items
		if len(items) != pageOf(v) {
			t.Fatalf("%s directory returned %d items, want %d", v.name, len(items), pageOf(v))
		}
		return time.Since(start).Nanoseconds()
	}
	quantiles := func(ns []int64) map[string]int64 {
		t.Helper()
		return baselineQuantiles(ns)
	}

	// Pre-change baseline: 0043 is not deployed in production yet.
	if e := dmDirectoryScaleOps(t, l, "0043_dm_directory_cursor_indexes_rollback.sql"); e != nil {
		t.Fatal("drop cursor indexes for pre-index baseline:", e)
	}
	if got := dmDirectoryScaleIndexes(t, l); got != 0 {
		t.Fatalf("pre-index baseline still has %d cursor indexes", got)
	}
	explain := map[string]string{}
	if explainRequested {
		for name, plan := range dmDirectoryScaleExplain(t, l, viewers[2], false) {
			explain["preIndex_"+name] = plan
		}
	}
	preIndexNS := map[string][]int64{}
	for _, v := range viewers {
		for i := 0; i < warmup; i++ {
			read(v, &legacy)
		}
		ns := make([]int64, 0, samples)
		for i := 0; i < samples; i++ {
			ns = append(ns, read(v, &legacy))
		}
		preIndexNS[v.name] = ns
		q := quantiles(ns)
		t.Logf("%s pre-index legacy: p50/p95/p99 %.2f/%.2f/%.2f ms", v.name,
			float64(q["p50NS"])/1e6, float64(q["p95NS"])/1e6, float64(q["p99NS"])/1e6)
	}

	// Delivered state: both cursor indexes, then compare indexed fallback with
	// the prepared two-phase path.
	if e := dmDirectoryScaleOps(t, l, "0043_dm_directory_cursor_indexes_reapply.sql"); e != nil {
		t.Fatal("restore cursor indexes:", e)
	}
	if got := dmDirectoryScaleIndexes(t, l); got != 2 {
		t.Fatalf("reapply left %d cursor indexes", got)
	}
	if _, e := l.cp.Exec(ctx, `ANALYZE c_dm.conversations`); e != nil {
		t.Fatal("analyze conversations after index reapply:", e)
	}
	if explainRequested {
		for name, plan := range dmDirectoryScaleExplain(t, l, viewers[2], true) {
			explain[name] = plan
		}
		for name, plan := range explain {
			t.Logf("%s plan:\n%s", name, plan)
		}
	}

	measurements := make([]dmScaleMeasurement, 0, len(viewers))
	failures := make([]string, 0)
	for _, v := range viewers {
		for i := 0; i < warmup; i++ {
			read(v, &legacy)
			read(v, l.c)
		}
		legacyNS := make([]int64, 0, samples)
		preparedNS := make([]int64, 0, samples)
		for i := 0; i < samples; i++ {
			if i%2 == 0 {
				legacyNS = append(legacyNS, read(v, &legacy))
				preparedNS = append(preparedNS, read(v, l.c))
			} else {
				preparedNS = append(preparedNS, read(v, l.c))
				legacyNS = append(legacyNS, read(v, &legacy))
			}
		}
		preIndexQuantiles := quantiles(preIndexNS[v.name])
		legacyQuantiles, preparedQuantiles := quantiles(legacyNS), quantiles(preparedNS)
		change := 100 * (1 - float64(preparedQuantiles["p95NS"])/float64(preIndexQuantiles["p95NS"]))
		indexedChange := 100 * (1 - float64(preparedQuantiles["p95NS"])/float64(legacyQuantiles["p95NS"]))
		measurements = append(measurements, dmScaleMeasurement{
			Viewer: v.name, Conversations: len(v.peers), PageItems: pageOf(v),
			LegacyNoIndexNS: preIndexQuantiles, LegacyNS: legacyQuantiles, PreparedNS: preparedQuantiles,
			P95ChangePercent: change, P95ChangeVsIndexedLegacyPercent: indexedChange,
		})
		t.Logf("%s (%d conversations): pre-index legacy p50/p95/p99 %.2f/%.2f/%.2f ms, indexed legacy %.2f/%.2f/%.2f ms, prepared %.2f/%.2f/%.2f ms, p95 change %+.1f%% vs pre-index (%.1f%% vs indexed)",
			v.name, len(v.peers),
			float64(preIndexQuantiles["p50NS"])/1e6, float64(preIndexQuantiles["p95NS"])/1e6, float64(preIndexQuantiles["p99NS"])/1e6,
			float64(legacyQuantiles["p50NS"])/1e6, float64(legacyQuantiles["p95NS"])/1e6, float64(legacyQuantiles["p99NS"])/1e6,
			float64(preparedQuantiles["p50NS"])/1e6, float64(preparedQuantiles["p95NS"])/1e6, float64(preparedQuantiles["p99NS"])/1e6,
			change, indexedChange)
		if conversations >= 1_000_000 && v.name == "dm_scale_1000" && change < 40 {
			failures = append(failures, fmt.Sprintf("prepared directory p95 improved only %.1f%% over the pre-index fallback at 1000 conversations", change))
		}
	}

	var postgres string
	if e := l.cp.QueryRow(ctx, `SELECT current_setting('server_version')`).Scan(&postgres); e != nil {
		t.Fatal(e)
	}
	result := map[string]any{
		"specVersion": 1, "scope": "dm-directory-service-method-real-postgresql",
		"syntheticSeed": true, "productionDeployment": false,
		"dataset": map[string]any{"conversations": conversations, "peerIdentities": peerCount, "viewers": len(viewers)},
		"warmup":  warmup, "samples": samples, "pageSize": dmDirectoryPageLimit,
		"baseline":     "fallback query with 0043 cursor indexes dropped (production state before this change)",
		"measurements": measurements, "explain": explain,
		"postgres": postgres, "goVersion": runtime.Version(), "goMaxProcs": runtime.GOMAXPROCS(0),
		"note": "synthetic 1M-row lab; both paths call the production Community method and C Safety Gate; sequential single-client service-method measurement, not production acceptance",
	}
	encoded, e := json.MarshalIndent(result, "", "  ")
	if e != nil {
		t.Fatal(e)
	}
	t.Logf("DM directory scale result:\n%s", encoded)
	if output != "" {
		if e = os.WriteFile(output, encoded, 0o600); e != nil {
			t.Fatal(e)
		}
	}
	for _, failure := range failures {
		t.Error(failure)
	}
}

func dmDirectoryScaleOps(t *testing.T, l *lab, name string) error {
	t.Helper()
	body, e := os.ReadFile(filepath.Join("..", "..", "ops", name))
	if e != nil {
		return e
	}
	_, e = l.cp.Exec(context.Background(), string(body))
	return e
}

func dmDirectoryScaleIndexes(t *testing.T, l *lab) int {
	t.Helper()
	return count(t, l.cp, `SELECT count(*) FROM pg_indexes WHERE schemaname='c_dm' AND indexname IN ('dm_conversations_low_cursor','dm_conversations_high_cursor')`)
}

// dmDirectoryScaleExplain captures the production plans for the fallback and,
// when withIndexes is true, the three prepared statements. It is diagnostic
// evidence, not a gate.
func dmDirectoryScaleExplain(t *testing.T, l *lab, v dmScaleViewer, withIndexes bool) map[string]string {
	t.Helper()
	ctx := context.Background()
	var account uuid.UUID
	if e := l.cp.QueryRow(ctx, `SELECT account_id FROM public.community_identities WHERE identity_id=$1`, v.identity).Scan(&account); e != nil {
		t.Fatal("explain viewer account:", e)
	}
	var ids []string
	rows, e := l.cp.Query(ctx, dmDirectoryCandidatesSQL, v.identity.String(), account, nil)
	if e != nil {
		t.Fatal("explain candidates:", e)
	}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			t.Fatal("explain candidates scan:", e)
		}
		ids = append(ids, id)
	}
	rows.Close()
	if e = rows.Err(); e != nil {
		t.Fatal("explain candidates rows:", e)
	}
	var last any
	if len(ids) > 0 {
		last = ids[len(ids)-1]
	}
	specs := map[string]dmScaleExplainSpec{
		"legacy": {dmDirectoryDirectSQL, []any{account, v.identity.String(), ""}},
	}
	if withIndexes {
		specs["candidates"] = dmScaleExplainSpec{dmDirectoryCandidatesSQL, []any{v.identity.String(), account, nil}}
		specs["versions"] = dmScaleExplainSpec{dmDirectoryVersionsSQL, []any{account, v.identity.String(), ids}}
		specs["range"] = dmScaleExplainSpec{dmDirectoryRangeSQL, []any{account, v.identity.String(), nil, last, ids}}
	}
	plans := map[string]string{}
	for name, spec := range specs {
		explainRows, e := l.cp.Query(ctx, "EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT) "+spec.sql, spec.args...)
		if e != nil {
			t.Fatal("explain "+name+":", e)
		}
		var plan strings.Builder
		for explainRows.Next() {
			var line string
			if e = explainRows.Scan(&line); e != nil {
				break
			}
			plan.WriteString(line)
			plan.WriteByte('\n')
		}
		explainRows.Close()
		plans[name] = plan.String()
	}
	return plans
}

// newDMDirectoryScaleFixture builds a viewer with 10/100/1000 conversations
// (each split evenly across both identity endpoints) and fills the rest of the
// dataset with peer-to-peer conversations. Every peer has its own account so
// the real identity and account projections are measured.
func newDMDirectoryScaleFixture(t *testing.T, conversations int) (*lab, []dmScaleViewer, int) {
	t.Helper()
	l := newPostLab(t)
	ctx := context.Background()
	var e error
	l.c, e = l.c.WithDirectMessages(ctx, DirectMessageConfig{PreparedDirectoryReads: true})
	if e != nil {
		t.Fatal("prepared directory rejected the complete schema", e)
	}
	if !l.c.preparedDMDirectoryReads() {
		t.Fatal("scale fixture did not select the prepared directory")
	}
	specs := []struct {
		name      string
		low, high int
	}{
		{"dm_scale_10", 5, 5},
		{"dm_scale_100", 50, 50},
		{"dm_scale_1000", 500, 500},
	}
	viewerPairs := 0
	for _, spec := range specs {
		viewerPairs += spec.low + spec.high
	}
	if conversations < viewerPairs {
		t.Fatalf("scale fixture needs at least %d conversations", viewerPairs)
	}
	filler := conversations - viewerPairs
	peerCount := 1000
	for peerCount*(peerCount-1)/2 < filler {
		peerCount++
	}
	peers := dmScaleSeedPeers(t, l, peerCount)
	viewers := make([]dmScaleViewer, 0, len(specs))
	for _, spec := range specs {
		account := sessionTestAccount(t, l, spec.name)
		created, e := l.c.ChangeOwnIdentity(ctx, IdentityChangeRequest{
			Bearer: account.SessionToken, ChangeID: identityRequestKey(t), Operation: "CREATE",
			Nickname: "目录" + strings.TrimPrefix(spec.name, "dm_scale_"),
		})
		if e != nil || created.State != "COMMITTED" || created.IdentityID == uuid.Nil || created.ExpiresAt.IsZero() {
			t.Fatalf("%s identity: state=%q code=%q id=%s expires=%v err=%v", spec.name, created.State, created.ErrorCode, created.IdentityID, created.ExpiresAt, e)
		}
		identity := created.IdentityID
		v := dmScaleViewer{name: spec.name, account: account, identity: identity}
		for i := 0; i < spec.low; i++ {
			id := peers[2*i]
			if bytes.Compare(id[:], identity[:]) >= 0 {
				t.Fatalf("low peer %s is not below viewer %s", id, identity)
			}
			v.peers = append(v.peers, id)
		}
		for i := 0; i < spec.high; i++ {
			id := peers[2*i+1]
			if bytes.Compare(id[:], identity[:]) <= 0 {
				t.Fatalf("high peer %s is not above viewer %s", id, identity)
			}
			v.peers = append(v.peers, id)
		}
		viewers = append(viewers, v)
	}
	intent := postTestIntent(t, l, viewers[0].identity)
	intent.Title, intent.Body = "规模来源", "规模正文"
	post := postTestCommand(t, l, viewers[0].account.SessionToken, postTestKey(t), intent).PostID
	postTestPublish(t, l)
	for _, v := range viewers {
		if _, e = l.cp.Exec(ctx, `INSERT INTO c_dm.conversations(conversation_id,low_identity,high_identity,source_post,sequence,low_sent,high_sent)
 SELECT gen_random_uuid(),least($1,p),greatest($1,p),$2,0,false,false FROM unnest($3::uuid[]) AS p`, v.identity, post, v.peers); e != nil {
			t.Fatalf("seed conversations for %s: %v", v.name, e)
		}
	}
	if filler > 0 {
		if _, e = l.cp.Exec(ctx, `INSERT INTO c_dm.conversations(conversation_id,low_identity,high_identity,source_post,sequence,low_sent,high_sent)
 SELECT gen_random_uuid(),least(a.id,b.id),greatest(a.id,b.id),$1,0,false,false
 FROM unnest($2::uuid[]) WITH ORDINALITY AS a(id,ra)
 JOIN unnest($2::uuid[]) WITH ORDINALITY AS b(id,rb) ON a.ra<rb
 LIMIT $3`, post, peers, filler); e != nil {
			t.Fatal("seed filler conversations", e)
		}
	}
	if got := count(t, l.cp, `SELECT count(*) FROM c_dm.conversations`); got != conversations {
		t.Fatalf("seeded %d conversations, want %d", got, conversations)
	}
	if _, e = l.cp.Exec(ctx, `ANALYZE c_dm.conversations; ANALYZE public.community_identities; ANALYZE c_auth.accounts`); e != nil {
		t.Fatal("analyze scale fixture", e)
	}
	return l, viewers, peerCount
}

// dmScaleSeedPeers inserts accounts and identities directly. Low peers use the
// bottom UUID range and high peers the top range so every viewer has a known
// mix of both directory branches.
func dmScaleSeedPeers(t *testing.T, l *lab, n int) []uuid.UUID {
	t.Helper()
	ctx := context.Background()
	ids := make([]uuid.UUID, n)
	accounts := make([]uuid.UUID, n)
	for i := 0; i < n; i++ {
		if i%2 == 0 {
			ids[i] = uuid.MustParse(fmt.Sprintf("00000000-0000-4000-8000-%012x", i/2+1))
		} else {
			ids[i] = uuid.MustParse(fmt.Sprintf("ffffffff-ffff-4fff-bfff-%012x", i/2+1))
		}
		accounts[i] = uuid.New()
	}
	base := 700000
	for {
		var used int
		if e := l.cp.QueryRow(ctx, `SELECT count(*) FROM c_auth.accounts WHERE platform_number::int BETWEEN $1 AND $2`, base, base+n-1).Scan(&used); e != nil {
			t.Fatal(e)
		}
		if used == 0 {
			break
		}
		base += 10000
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
 SELECT a,lpad((($2::int)-1+o)::text,6,'0'),'dm_scale_peer_'||o,decode(repeat('11',32),'hex'),decode(repeat('22',16),'hex'),1
 FROM unnest($1::uuid[]) WITH ORDINALITY AS x(a,o)`, accounts, base); e != nil {
		t.Fatal(e)
	}
	if _, e = tx.Exec(ctx, `INSERT INTO public.identity_account_state(account_id,created_count,last_created_at)
 SELECT a,1,now() FROM unnest($1::uuid[]) AS a`, accounts); e != nil {
		t.Fatal(e)
	}
	if _, e = tx.Exec(ctx, `INSERT INTO public.community_identities(identity_id,account_id,nickname,avatar,is_original,created_at)
 SELECT i,a,'规模身份'||o,'default-v1',true,now()
 FROM unnest($1::uuid[],$2::uuid[]) WITH ORDINALITY AS x(i,a,o)`, ids, accounts); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	return ids
}
