package authprivacyruntime

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/authprivacyhttp"
)

func TestPreparedReadConfigurationDefaultsAndIsolation(t *testing.T) {
	for _, test := range []struct {
		raw                   string
		feed, media, comments bool
	}{
		{`{"posts":{},"comments":{}}`, false, false, false},
		{`{"posts":{"preparedFeedReads":true},"comments":{}}`, true, false, false},
		{`{"posts":{"preparedMediaReads":true},"comments":{}}`, false, true, false},
		{`{"posts":{},"comments":{"preparedReads":true}}`, false, false, true},
	} {
		var c Config
		if err := json.Unmarshal([]byte(test.raw), &c); err != nil {
			t.Fatal(err)
		}
		if c.Posts.PreparedFeedReads != test.feed || c.Posts.PreparedMediaReads != test.media || c.Comments.PreparedReads != test.comments {
			t.Fatal("independent read switches changed defaults or shared state")
		}
	}
}

func TestPostSearchDocumentConfigurationIsIndependent(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		raw := `{"posts":{},"comments":{}}`
		if enabled {
			raw = `{"posts":{"searchDocuments":true},"comments":{}}`
		}
		var config Config
		if err := json.Unmarshal([]byte(raw), &config); err != nil {
			t.Fatal(err)
		}
		if config.Posts.SearchDocuments != enabled || config.Posts.PreparedFeedReads || config.Posts.PreparedMediaReads || config.Comments.PreparedReads {
			t.Fatal("search switch changed another read mode")
		}
	}
}

func TestPreparedReadsDisabledRetainsC39StartupPostgres(t *testing.T) {
	owner, runtime := moderationStartupLabThrough(t, 39)
	ctx := context.Background()
	var oldSchema bool
	if err := owner.QueryRow(ctx, `SELECT to_regclass('c_comments.read_scope_versions') IS NULL
 AND to_regclass('c_posts.read_feed_scope_versions') IS NULL
 AND (SELECT max(version_id) FROM public.goose_db_version WHERE is_applied)=39`).Scan(&oldSchema); err != nil || !oldSchema {
		t.Fatal("fixture did not install the actual pre-optimization schema", err)
	}
	if err := CheckDatabase(ctx, runtime, authprivacyhttp.CommunityService); err != nil {
		t.Fatal("disabled prepared reads raised common C's minimum schema", err)
	}
}
