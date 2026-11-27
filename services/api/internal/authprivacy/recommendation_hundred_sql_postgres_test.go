package authprivacy_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
	"time"

	rw "github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/recommendation/transport"
)

// Uses the existing owned-PG fixture, HTTPS boundary, real A1 projection and
// driver tracer. It is a 100-text-post functional/count regression, not the
// retained mixed J environment, production capacity, nested SQL or P95 proof.
func TestRecommendationHTTPSHundredSQLPostgres(t *testing.T) {
	s := newRecoHTTPS(t, 100)
	type sample struct {
		DriverSQL json.RawMessage
		WallNS    int64
	}
	counts := map[string]sample{}
	measure := func(name string, call func()) {
		s.lab.BeginTrace()
		started := time.Now()
		call()
		elapsed := time.Since(started)
		counts[name] = sample{s.lab.FinishTrace(), elapsed.Nanoseconds()}
	}
	var input rw.Start
	measure("feed_context", func() { input = s.bootstrap() })
	var first, repeated rw.Page
	measure("start", func() { first = s.start(jsonBytes(input)) })
	measure("same_intent", func() { repeated = s.start(jsonBytes(input)) })
	if !reflect.DeepEqual(pageIDs(first), pageIDs(repeated)) || s.lab.Rounds(t) != 1 {
		t.Fatal("same intent created a new round or reordered its first page")
	}
	seen := map[string]bool{}
	page := first
	pageCount := 0
	for {
		pageCount++
		for _, id := range pageIDs(page) {
			if seen[id] {
				t.Fatal("duplicate across manifest pages")
			}
			seen[id] = true
		}
		if page.NextCursor == nil {
			break
		}
		if pageCount >= 10 {
			t.Fatal("manifest exceeded ten pages")
		}
		cursor := page.NextCursor
		measure("page_"+strconv.Itoa(pageCount+1), func() {
			page = s.page(s.ref("page", cursor, s.lab.Reader.SessionToken, 200))
		})
	}
	if len(seen) != 100 || pageCount != 10 {
		t.Fatalf("coverage: %d IDs, %d pages", len(seen), pageCount)
	}
	measure("feedback", func() {
		s.check(s.request("POST", "/api/v1/feed/recommendation/feedback", s.lab.Reader.SessionToken,
			jsonBytes(rw.Feedback{Receipt: *first.FeedbackReceipt, PostIDs: pageIDs(first)[:1]})), 200)
	})
	for name, value := range counts {
		var c struct {
			Statements int
			Categories map[string]int
		}
		if json.Unmarshal(value.DriverSQL, &c) != nil || c.Statements == 0 || c.Categories["test_projection_port"] != 0 {
			t.Fatal("invalid real-projection driver count", name)
		}
		if name == "start" {
			if c.Categories["candidate_keys"] != 1 || c.Categories["candidate_facts"] != 1 {
				t.Fatal("cold candidate count")
			}
		} else if name != "feed_context" && (c.Categories["candidate_keys"] != 0 || c.Categories["candidate_facts"] != 0) {
			t.Fatal("retry/page/feedback recalled candidates", name)
		}
	}
	if dir := os.Getenv("HNUB3B_EVIDENCE_DIR"); dir != "" {
		raw, err := json.MarshalIndent(map[string]any{"scope": "owned 100-text-post HTTPS; actual A1 projection; driver counts exclude nested SQL; not retained J/P95", "uniqueIDs": len(seen), "pages": pageCount, "samples": counts}, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(dir, "B-hundred-http-sql.json"), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
