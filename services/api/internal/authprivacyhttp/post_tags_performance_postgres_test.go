package authprivacyhttp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/posts"
)

// TestPostTagsScalePerformancePostgres uses only the guarded disposable HTTPS
// lab. It creates complete publication graphs and checks their tag projection.
// Deliberately opt-in: it creates a million tag relationships and is not a CI microtest.
func TestPostTagsScalePerformancePostgres(t *testing.T) {
	if os.Getenv("POST_TAG_PERF") != "1" {
		t.Skip("set POST_TAG_PERF=1 in a disposable HTTPS lab")
	}
	ctx := context.Background()
	s := e2eNewServices(t)
	owner, _ := e2ePrivacySignup(t, s, "tags_perf_owner")
	channel := postHTTPSChannel(t, s, owner)
	identity := postHTTPSIdentity(t, s, owner, "性能作者", 171)
	initial := posts.CreatePostInput{ChannelID: channel, IdentityID: identity, Title: "性能帖子", Body: "正文", Tags: []string{"NBA"}}
	created := postHTTPSJSON(t, s, "PUT", "/api/v1/post-commands/"+httpTestEncoding(16, 172), owner, "", initial, 202)
	post := created["postId"].(string)
	if _, err := s.c.PublishAcceptedPosts(ctx, 100); err != nil {
		t.Fatal(err)
	}
	var dbPost, dbOwner, dbChannel, dbIdentity string
	if err := s.cp.QueryRow(ctx, `SELECT p.post_id::text,p.owner_account_id::text,p.channel_id::text,t.identity_id::text FROM c_posts.posts p JOIN c_posts.publication_tasks t USING(post_id) LIMIT 1`).Scan(&dbPost, &dbOwner, &dbChannel, &dbIdentity); err != nil {
		t.Fatal(err)
	}
	rollback, err := os.ReadFile("../../authlab/rollback-post-tag-sync.sql")
	if err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../../migrations/0046_post_tag_sync_skip.sql")
	if err != nil {
		t.Fatal(err)
	}
	enable := strings.SplitN(string(migration), "-- +goose Down", 2)[0]
	run := func(q string, args ...any) {
		t.Helper()
		if _, err := s.cp.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	seed := func(first, last int) {
		t.Helper()
		for a := first; a <= last; a += 5000 {
			b := a + 4999
			if b > last {
				b = last
			}
			tx, e := s.cp.Begin(ctx)
			if e != nil {
				t.Fatal(e)
			}
			stmts := []string{
				`INSERT INTO c_posts.posts(post_id,owner_account_id,channel_id,visibility,published_attempt_version,published_at,publication_ordinal,tags) SELECT md5('perf-post-'||g)::uuid,$3::uuid,$4::uuid,'PUBLISHED',1,now(),nextval('c_posts.publication_ordinal_seq'),ARRAY['TagA','TagB','TagC'] FROM generate_series($1::int,$2::int) g`,
				`INSERT INTO c_posts.post_identity_bindings(account_id,post_id,identity_id,bound_at) SELECT $3::uuid,md5('perf-post-'||g)::uuid,$5::uuid,now() FROM generate_series($1::int,$2::int) g CROSS JOIN (SELECT $4::uuid) ignored`,
				`INSERT INTO c_posts.publication_tasks(task_id,post_id,owner_account_id,identity_id,channel_id,latest_attempt_version) SELECT md5('perf-task-'||g)::uuid,md5('perf-post-'||g)::uuid,$3::uuid,$5::uuid,$4::uuid,1 FROM generate_series($1::int,$2::int) g`,
				`INSERT INTO c_posts.publication_attempts(task_id,version,accepted_stop_generation,state,accepted_at,acceptance_ordinal,terminal_at) SELECT md5('perf-task-'||g)::uuid,1,0,'PUBLISHED',now(),nextval('c_posts.acceptance_ordinal_seq'),now() FROM generate_series($1::int,$2::int) g`,
				`INSERT INTO c_posts.attempt_contents(task_id,version,title,body,request_digest,tags) SELECT md5('perf-task-'||g)::uuid,1,'title','body',decode(repeat('11',32),'hex'),ARRAY['TagA','TagB','TagC'] FROM generate_series($1::int,$2::int) g`,
				`INSERT INTO c_posts.post_tag_terms(post_id,tag_key,label) SELECT md5('perf-post-'||g)::uuid,lower(tag COLLATE "C"),tag FROM generate_series($1::int,$2::int) g CROSS JOIN unnest(ARRAY['TagA','TagB','TagC']) tag`,
			}
			for _, q := range stmts {
				args := []any{a, b}
				if strings.Contains(q, "$3") {
					args = append(args, dbOwner)
				}
				if strings.Contains(q, "$4") {
					args = append(args, dbChannel)
				}
				if strings.Contains(q, "$5") {
					args = append(args, dbIdentity)
				}
				if _, e = tx.Exec(ctx, q, args...); e != nil {
					_ = tx.Rollback(ctx)
					t.Fatalf("seed %d-%d: %v", a, b, e)
				}
			}
			if e = tx.Commit(ctx); e != nil {
				t.Fatalf("seed commit %d-%d: %v", a, b, e)
			}
			if b%25000 == 0 || b == last {
				t.Logf("seeded %d posts", b)
			}
		}
	}
	p95 := func(a []float64) float64 { sort.Float64s(a); return a[(95*len(a)+99)/100-1] }
	plan := func(statement string) []string {
		t.Helper()
		tx, e := s.cp.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		defer tx.Rollback(ctx)
		rows, e := tx.Query(ctx, "EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT) "+statement, dbPost)
		if e != nil {
			t.Fatal(e)
		}
		var out []string
		for rows.Next() {
			var line string
			if e = rows.Scan(&line); e != nil {
				t.Fatal(e)
			}
			out = append(out, line)
		}
		if e = rows.Err(); e != nil {
			t.Fatal(e)
		}
		rows.Close()
		return out
	}
	measure := func(scale int, stage string) {
		t.Helper()
		run(`ANALYZE c_posts.posts`)
		run(`ANALYZE c_posts.post_tag_terms`)
		var count int
		if e := s.cp.QueryRow(ctx, `SELECT count(*) FROM c_posts.post_tag_terms`).Scan(&count); e != nil || count != scale {
			t.Fatalf("bad relation count: %d %v", count, e)
		}
		var changed, same, api []float64
		for i := 0; i < 50; i++ {
			tag := "NBA"
			if i%2 == 0 {
				tag = "CBA"
			}
			start := time.Now()
			run(`UPDATE c_posts.posts SET tags=ARRAY[$2::text] WHERE post_id=$1::uuid`, dbPost, tag)
			ms := float64(time.Since(start).Microseconds()) / 1000
			if i >= 10 {
				changed = append(changed, ms)
			}
		}
		for i := 0; i < 50; i++ {
			start := time.Now()
			run(`UPDATE c_posts.posts SET tags=tags WHERE post_id=$1::uuid`, dbPost)
			ms := float64(time.Since(start).Microseconds()) / 1000
			if i >= 10 {
				same = append(same, ms)
			}
		}
		editor := "/api/v1/me/posts/" + post
		snap := postHTTPSJSON(t, s, "GET", editor+"/edit", owner, "", nil, 200)
		rev := snap["revision"].(string)
		for i := 0; i < 40; i++ {
			id := uuid.New()
			key := base64.RawURLEncoding.EncodeToString(id[:])
			tag := "NBA"
			if i%2 == 0 {
				tag = "CBA"
			}
			body := map[string]any{"revision": rev, "title": "性能帖子", "body": "正文", "tags": []string{tag}}
			start := time.Now()
			result := postHTTPSJSON(t, s, "PUT", editor+"/edit-commands/"+key, owner, "", body, 200)
			ms := float64(time.Since(start).Microseconds()) / 1000
			if result["state"] != "SAVED" {
				t.Fatalf("edit did not save: %v", result)
			}
			rev = result["revision"].(string)
			if i >= 10 {
				api = append(api, ms)
			}
		}
		triggerSample := func(statement string) float64 {
			lines := plan(statement)
			for _, line := range lines {
				const prefix = "Trigger post_tag_terms_sync: time="
				if strings.HasPrefix(strings.TrimSpace(line), prefix) {
					value := strings.Fields(strings.TrimPrefix(strings.TrimSpace(line), prefix))[0]
					ms, err := strconv.ParseFloat(value, 64)
					if err != nil {
						t.Fatal(err)
					}
					return ms
				}
			}
			t.Fatal("missing tag trigger timing")
			return 0
		}
		var triggerChanged, triggerSame []float64
		for i := 0; i < 35; i++ {
			v := triggerSample(`UPDATE c_posts.posts SET tags=ARRAY['PERF'] WHERE post_id=$1::uuid`)
			sameValue := triggerSample(`UPDATE c_posts.posts SET tags=tags WHERE post_id=$1::uuid`)
			if i >= 5 {
				triggerChanged = append(triggerChanged, v)
				triggerSame = append(triggerSame, sameValue)
			}
		}
		payload := map[string]any{"scale": scale, "stage": stage, "samples_sql": len(changed), "samples_api": len(api), "samples_trigger": len(triggerChanged), "trigger_changed_p95_ms": p95(triggerChanged), "trigger_same_p95_ms": p95(triggerSame), "update_changed_p95_ms": p95(changed), "update_same_p95_ms": p95(same), "api_edit_p95_ms": p95(api), "delete_plan": plan(`DELETE FROM c_posts.post_tag_terms WHERE post_id=$1::uuid`), "update_plan": plan(`UPDATE c_posts.posts SET tags=ARRAY['PERF'] WHERE post_id=$1::uuid`)}
		data, _ := json.Marshal(payload)
		t.Logf("PERF_RESULT %s", data)
	}
	scales := []struct{ scale, first, last int }{{100000, 1, 33333}, {1000000, 33334, 333333}}
	if os.Getenv("POST_TAG_PERF_QUICK") == "1" {
		scales = []struct{ scale, first, last int }{{301, 1, 100}, {1000, 101, 333}}
	}
	for _, item := range scales {
		seed(item.first, item.last)
		run(string(rollback))
		run(`DROP INDEX CONCURRENTLY c_posts.post_tag_terms_post_idx`)
		measure(item.scale, "baseline")
		run(`CREATE INDEX CONCURRENTLY post_tag_terms_post_idx ON c_posts.post_tag_terms(post_id)`)
		measure(item.scale, "index")
		tx, e := s.cp.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		start := time.Now()
		if _, e = tx.Exec(ctx, enable); e != nil {
			_ = tx.Rollback(ctx)
			t.Fatal(e)
		}
		if e = tx.Commit(ctx); e != nil {
			t.Fatal(e)
		}
		t.Logf("ENABLE_CHECK scale=%d ms=%.3f", item.scale, float64(time.Since(start).Microseconds())/1000)
		measure(item.scale, "optimized")

		// Verify lifecycle behavior at both scales after the timed samples.
		expectLabels := func(want string) {
			t.Helper()
			var got string
			if err := s.cp.QueryRow(ctx, `SELECT COALESCE(string_agg(label,',' ORDER BY label COLLATE "C"),'') FROM c_posts.post_tag_terms WHERE post_id=$1::uuid`, dbPost).Scan(&got); err != nil || got != want {
				t.Fatalf("scale %d labels=%q want=%q err=%v", item.scale, got, want, err)
			}
		}
		run(`UPDATE c_posts.posts SET tags=ARRAY['NBA','CBA'] WHERE post_id=$1::uuid`, dbPost)
		expectLabels("CBA,NBA")
		run(`UPDATE c_posts.posts SET tags=ARRAY['CASE'] WHERE post_id=$1::uuid`, dbPost)
		expectLabels("CASE")
		tx, e = s.cp.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = tx.Exec(ctx, `UPDATE c_posts.posts SET tags=ARRAY['ROLLBACK'] WHERE post_id=$1::uuid`, dbPost); e != nil {
			t.Fatal(e)
		}
		if e = tx.Rollback(ctx); e != nil {
			t.Fatal(e)
		}
		expectLabels("CASE")
		run(`UPDATE c_posts.posts SET tags='{}' WHERE post_id=$1::uuid`, dbPost)
		expectLabels("")
		run(`UPDATE c_posts.posts SET tags=ARRAY['NBA'] WHERE post_id=$1::uuid`, dbPost)
		run(`UPDATE c_posts.posts SET moderation_hidden=true WHERE post_id=$1::uuid`, dbPost)
		expectLabels("")
		run(`UPDATE c_posts.posts SET moderation_hidden=false WHERE post_id=$1::uuid`, dbPost)
		expectLabels("NBA")

		deleteKey := uuid.New()
		createdForDelete := postHTTPSJSON(t, s, "PUT", "/api/v1/post-commands/"+base64.RawURLEncoding.EncodeToString(deleteKey[:]), owner, "", posts.CreatePostInput{ChannelID: channel, IdentityID: identity, Title: "删除验证", Body: "正文", Tags: []string{"DelScale"}}, 202)
		if _, e = s.c.PublishAcceptedPosts(ctx, 100); e != nil {
			t.Fatal(e)
		}
		var deleteCount int
		if e = s.cp.QueryRow(ctx, `SELECT count(*) FROM c_posts.post_tag_terms WHERE tag_key='delscale'`).Scan(&deleteCount); e != nil || deleteCount != 1 {
			t.Fatalf("delete setup: %d %v", deleteCount, e)
		}
		deleteKey = uuid.New()
		postHTTPSJSON(t, s, "POST", "/api/v1/posts/"+createdForDelete["postId"].(string)+"/delete", owner, base64.RawURLEncoding.EncodeToString(deleteKey[:]), nil, 200)
		if e = s.cp.QueryRow(ctx, `SELECT count(*) FROM c_posts.post_tag_terms WHERE tag_key='delscale'`).Scan(&deleteCount); e != nil || deleteCount != 0 {
			t.Fatalf("deleted projection: %d %v", deleteCount, e)
		}
		t.Logf("SCALE_LIFECYCLE_PASS scale=%d", item.scale)
	}
}
