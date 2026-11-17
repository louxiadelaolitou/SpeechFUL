package authprivacyruntime

import (
	"context"
	"encoding/json"
	"os"
	"strings"
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

func TestPasswordLoginMigrationDoesNotRequirePreparedReadsPostgres(t *testing.T) {
	owner, runtime := moderationStartupLabThrough(t, 39)
	ctx := context.Background()
	var oldSchema bool
	if err := owner.QueryRow(ctx, `SELECT to_regclass('c_comments.read_scope_versions') IS NULL
 AND to_regclass('c_posts.read_feed_scope_versions') IS NULL
 AND (SELECT max(version_id) FROM public.goose_db_version WHERE is_applied)=39`).Scan(&oldSchema); err != nil || !oldSchema {
		t.Fatal("fixture did not install the actual pre-optimization schema", err)
	}
	if err := CheckDatabase(ctx, runtime, authprivacyhttp.CommunityService); err == nil {
		t.Fatal("missing mandatory password-login schema accepted")
	}
	// The mandatory authentication migration is independent of optional 0040+
	// prepared reads. Apply only 0051 to the frozen C39 fixture, with an honest
	// ledger entry, and keep all prepared-read tables absent.
	migration, err := os.ReadFile("../../migrations/0051_password_login_limits.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = owner.Exec(ctx, strings.SplitN(string(migration), "-- +goose Down", 2)[0]); err != nil {
		t.Fatal(err)
	}
	if _, err = owner.Exec(ctx, `INSERT INTO public.goose_db_version VALUES(51,true);
GRANT SELECT,INSERT,UPDATE ON c_auth.password_login_limits TO hnuhole_c_runtime`); err != nil {
		t.Fatal(err)
	}
	if err = CheckDatabase(ctx, runtime, authprivacyhttp.CommunityService); err != nil {
		t.Fatal("password-login schema incorrectly required optional prepared reads", err)
	}
	if err = owner.QueryRow(ctx, `SELECT to_regclass('c_comments.read_scope_versions') IS NULL
 AND to_regclass('c_posts.read_feed_scope_versions') IS NULL`).Scan(&oldSchema); err != nil || !oldSchema {
		t.Fatal("authentication migration installed optional prepared reads", err)
	}
}
