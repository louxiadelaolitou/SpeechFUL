package authprivacyhttp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"reflect"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/posts"
)

// Opt-in real TLS + PostgreSQL comparison. Never accepts a database or token
// from the caller: e2eNewServices owns and resets only the isolated authlab DB.
func TestRealHTTPSFeedBatchPerformancePostgres(t *testing.T) {
	if os.Getenv("REQ002_PERFORMANCE") != "1" {
		t.Skip("explicit frozen performance run required")
	}
	// This exact measurement file also compiles in the frozen pre-REQ002
	// checkout. Never compare both modes against an already optimized Gate.
	mode := os.Getenv("REQ002_PERFORMANCE_MODE")
	if mode != "legacy" && mode != "batch" {
		t.Fatal("explicit legacy/batch mode and separate frozen checkout required")
	}
	round, e := strconv.Atoi(os.Getenv("REQ002_PERFORMANCE_ROUND"))
	if e != nil || round < 1 || round > 3 {
		t.Fatal("explicit round 1..3 required")
	}
	output := os.Getenv("REQ002_PERFORMANCE_OUTPUT")
	if output == "" {
		t.Fatal("artifact required")
	}
	if _, e := os.Stat(output); !os.IsNotExist(e) {
		t.Fatal("refusing existing artifact")
	}
	type cohort struct {
		Round      int `json:"round"`
		Kind, Mode string
		Samples    []float64
		Clocks     []struct {
			RawMS, GoLowerMS, GoUpperMS, UncertaintyMS float64
			Valid                                      bool
		}
		P95    float64
		Errors []string
	}
	report := struct {
		Source                  string
		Clock, ClockGuard       string
		Warmup, Samples, Rounds int
		Cohorts                 []cohort
		Complete                bool
		Threshold               float64
	}{Source: os.Getenv("REQ002_SOURCE"), Clock: "CLOCK_MONOTONIC_RAW", ClockGuard: "Every warmup and sample: RAW lies within Go bracket plus max(50us,1% RAW); bracket uncertainty <= max(50us,1% RAW); no exclusions", Warmup: 10, Samples: 80, Rounds: 1, Threshold: .35}
	defer func() {
		data, e := json.MarshalIndent(report, "", "  ")
		if e != nil {
			t.Error(e)
			return
		}
		if e = os.WriteFile(output, data, 0600); e != nil {
			t.Error(e)
		}
	}()
	s := e2eNewServices(t)
	owner, _ := e2ePrivacySignup(t, s, "batch_perf_owner")
	reader, _ := e2ePrivacySignup(t, s, "batch_perf_reader")
	channel := postHTTPSChannel(t, s, owner)
	identity := postHTTPSIdentity(t, s, owner, "性能作者", 151)
	cfg := s.postConfig
	cfg.PreparedFeedReads = true
	legacyFlag := reflect.ValueOf(&cfg).Elem().FieldByName("LegacyListReads")
	if legacyFlag.IsValid() {
		legacyFlag.SetBool(mode == "legacy")
	} else if mode != "legacy" {
		t.Fatal("batch mode unavailable in frozen baseline")
	}
	selected, e := s.c.WithPosts(context.Background(), cfg)
	if e != nil {
		t.Fatal(e)
	}
	*s.c = *selected
	publish := func(start, count int) {
		for n := start; n < start+count; n++ {
			postHTTPSJSON(t, s, "PUT", fmt.Sprintf("/api/v1/post-commands/%s", httpTestEncoding(16, byte(n+10))), owner, "", posts.CreatePostInput{ChannelID: channel, IdentityID: identity, Title: fmt.Sprintf("Frozen %02d", n), Body: "Frozen chronological feed performance body."}, 202)
		}
		if _, e := s.c.PublishAcceptedPosts(context.Background(), 100); e != nil {
			t.Fatal(e)
		}
	}
	// Both checkouts publish the same 49+12 text posts. The old checkout has
	// no incremental protocol: its equivalent latest-five and after-ten pages
	// provide the same data boundaries for the added real-protocol cohorts.
	publish(0, 49)
	firstPath := "/api/v1/feed?limit=10"
	if mode == "batch" {
		firstPath += "&paging=incremental-v1"
	}
	before := postHTTPSJSON(t, s, "GET", firstPath, reader, "", nil, 200)
	publish(49, 12)
	first20 := postHTTPSJSON(t, s, "GET", "/api/v1/feed?limit=20", reader, "", nil, 200)
	first5 := postHTTPSJSON(t, s, "GET", "/api/v1/feed?limit=5", reader, "", nil, 200)
	first10 := postHTTPSJSON(t, s, "GET", firstPath, reader, "", nil, 200)
	nextPath := "/api/v1/feed?limit=5&cursor=" + first10["nextCursor"].(string)
	refreshPath := "/api/v1/feed?limit=5"
	if mode == "batch" {
		nextPath += "&paging=incremental-v1"
		refreshPath += "&paging=incremental-v1&refreshCursor=" + before["refreshCursor"].(string)
	} else {
		// Legacy cursors bind their original page size. Walk two five-item
		// pages to reach after-ten without relaxing the production binding.
		second5 := postHTTPSJSON(t, s, "GET", "/api/v1/feed?limit=5&cursor="+first5["nextCursor"].(string), reader, "", nil, 200)
		nextPath = "/api/v1/feed?limit=5&cursor=" + second5["nextCursor"].(string)
	}
	routes := []struct {
		kind, path string
		size       int
		firstTitle int
	}{
		{"home20", "/api/v1/feed?limit=20", 20, 60}, {"next20", "/api/v1/feed?limit=20&cursor=" + first20["nextCursor"].(string), 20, 40},
		{"home10", "/api/v1/feed?limit=10", 10, 60}, {"next5", "/api/v1/feed?limit=5&cursor=" + first5["nextCursor"].(string), 5, 55},
		{"incrementalHome10", firstPath, 10, 60}, {"incrementalNext5", nextPath, 5, 50}, {"incrementalRefresh5", refreshPath, 5, 60},
	}
	for _, route := range routes {
		report.Cohorts = append(report.Cohorts, cohort{Round: round, Kind: route.kind, Mode: mode, Samples: []float64{}})
		c := &report.Cohorts[len(report.Cohorts)-1]
		for n := 0; n < report.Warmup+report.Samples; n++ {
			req, e := http.NewRequest(http.MethodGet, s.cPublic.URL+route.path, nil)
			if e != nil {
				t.Fatal(e)
			}
			req.Header.Set("Authorization", "Bearer "+reader)
			req.Header.Set("X-Post-Preview", "thumbnail-v1")
			outerStart := time.Now()
			rawStart, startOK := feedPerformanceRawClock()
			started := time.Now()
			resp, e := s.client.Do(req)
			if e != nil {
				c.Errors = append(c.Errors, "transport")
				t.Fatal("measurement transport failed")
			}
			data, e := io.ReadAll(io.LimitReader(resp.Body, 262144))
			resp.Body.Close()
			innerEnd := time.Now()
			rawEnd, endOK := feedPerformanceRawClock()
			outerEnd := time.Now()
			elapsed := float64(rawEnd-rawStart) / float64(time.Millisecond)
			lower := float64(innerEnd.Sub(started)) / float64(time.Millisecond)
			upper := float64(outerEnd.Sub(outerStart)) / float64(time.Millisecond)
			tolerance := math.Max(.05, .01*elapsed)
			valid := startOK && endOK && elapsed > 0 && lower > 0 && upper >= lower && upper-lower <= tolerance && elapsed >= lower-tolerance && elapsed <= upper+tolerance
			c.Clocks = append(c.Clocks, struct {
				RawMS, GoLowerMS, GoUpperMS, UncertaintyMS float64
				Valid                                      bool
			}{elapsed, lower, upper, upper - lower, valid})
			if !valid {
				c.Errors = append(c.Errors, fmt.Sprintf("clock_interval_invalid_%d", n))
			}
			if e != nil || resp.StatusCode != 200 {
				c.Errors = append(c.Errors, fmt.Sprintf("HTTP_%d", resp.StatusCode))
				t.Fatalf("measurement %s %s status %d", route.kind, mode, resp.StatusCode)
			}
			var page struct {
				posts.CrossChannelFeedPage
				RefreshCursor *string `json:"refreshCursor"`
			}
			if json.Unmarshal(data, &page) != nil || len(page.Items) != route.size {
				c.Errors = append(c.Errors, "incomplete_page")
				t.Fatal("incomplete measured page")
			}
			for i, item := range page.Items {
				if item.Title != fmt.Sprintf("Frozen %02d", route.firstTitle-i) || item.Body != "Frozen chronological feed performance body." {
					c.Errors = append(c.Errors, "wrong_page_boundary_or_content")
					t.Fatalf("wrong %s page boundary or content at %d", route.kind, i)
				}
			}
			if mode == "batch" {
				switch route.kind {
				case "incrementalHome10":
					if page.RefreshCursor == nil || page.NextCursor == nil {
						t.Fatal("missing incremental initial cursors")
					}
				case "incrementalNext5":
					if page.RefreshCursor != nil || page.NextCursor == nil {
						t.Fatal("invalid incremental older cursors")
					}
				case "incrementalRefresh5":
					if page.RefreshCursor == nil || page.NextCursor != nil {
						t.Fatal("invalid incremental refresh cursors")
					}
				}
			}
			if n >= report.Warmup {
				c.Samples = append(c.Samples, elapsed)
			}
		}
		sorted := append([]float64{}, c.Samples...)
		sort.Float64s(sorted)
		c.P95 = sorted[int(math.Ceil(.95*float64(len(sorted))))-1]
	}
	report.Complete = true
	for _, c := range report.Cohorts {
		t.Logf("round%d %s %s p95=%.3fms (cross-checkout hard gate runs in compare-feed-performance.py)", c.Round, c.Kind, c.Mode, c.P95)
	}
}
