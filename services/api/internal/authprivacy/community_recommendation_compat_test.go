package authprivacy

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/authprivacy/protocol"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/posts"
)

func TestRecommendationRawM0M1UpgradeAndClosedCompatibilityPostgres(t *testing.T) {
	p := newRecommendationPG(t, 1)
	ctx := context.Background()
	a := sessionTestAccount(t, p.l, "reco_upgrade")
	var id string
	if e := p.l.cp.QueryRow(ctx, `SELECT post_id::text FROM c_posts.posts WHERE visibility='PUBLISHED'`).Scan(&id); e != nil {
		t.Fatal(e)
	}
	if _, e := p.l.cp.Exec(ctx, `DROP SCHEMA c_recommendation CASCADE; ALTER TABLE c_posts.post_likes DROP COLUMN liked_at; DROP INDEX c_posts.recommendation_favorite_time,c_posts.recommendation_favorite_post_time,c_comments.recommendation_comment_time`); e != nil {
		t.Fatal(e)
	}
	legacy := *p.service
	legacy.recommendation = nil
	if _, e := legacy.WithRecommendation(ctx, RecommendationConfig{}, nil); e != nil {
		t.Fatal("M0 closed compatibility", e)
	}
	if _, e := legacy.WithRecommendation(ctx, p.config, p.port); e == nil {
		t.Fatal("M0 activation accepted")
	}
	if _, e := legacy.SetPostInteraction(ctx, a.SessionToken, id, posts.PostLike, true); e != nil {
		t.Fatal("old writer on M0", e)
	}
	data, e := os.ReadFile("../../migrations/0056_recommendation_state.sql")
	if e != nil {
		t.Fatal(e)
	}
	tx, e := p.l.cp.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = tx.Exec(ctx, strings.SplitN(string(data), "-- +goose Down", 2)[0]); e != nil {
		tx.Rollback(ctx)
		t.Fatal("M0 to M1 raw Up", e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	if e = checkRecommendationSchema(ctx, p.l.cp, true); e == nil {
		t.Fatal("old physical-byte accounting accepted")
	}
	data, e = os.ReadFile("../../migrations/0057_recommendation_logical_bytes.sql")
	if e != nil {
		t.Fatal(e)
	}
	tx, e = p.l.cp.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = tx.Exec(ctx, strings.SplitN(string(data), "-- +goose Down", 2)[0]); e != nil {
		_ = tx.Rollback(ctx)
		t.Fatal("logical-byte forward Up", e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	if e = checkRecommendationSchema(ctx, p.l.cp, true); e != nil {
		t.Fatal("upgraded structural check", e)
	}
	if count(t, p.l.cp, `SELECT count(*) FROM c_posts.post_likes WHERE account_id=$1 AND liked_at IS NULL`, a.AccountID) != 1 {
		t.Fatal("upgrade backfilled unknown like")
	}
	if count(t, p.l.cp, `SELECT count(*) FROM c_recommendation.control WHERE enabled OR feedback_enabled OR like_time_enabled`) != 0 {
		t.Fatal("migration enabled collection")
	}
	if _, e = legacy.WithRecommendation(ctx, RecommendationConfig{}, nil); e == nil {
		t.Fatal("installed M1 without lifecycle grants accepted")
	}
	recommendationGrant(t, p.l)
	closed, e := legacy.WithRecommendation(ctx, RecommendationConfig{}, nil)
	if e != nil {
		t.Fatal("M1 closed runtime compatibility", e)
	}
	legacy = *closed
	worker, e := NewCommunityWorker(closed, DefaultWorkerConfig(), func(context.Context, uint32, []byte) ([]byte, error) { return nil, nil }, func(context.Context, string, protocol.Purpose) error { return nil }, nil)
	if e != nil {
		t.Fatal(e)
	}
	attached := false
	for _, task := range worker.tasks {
		if task.name == "recommendation_cleanup" {
			attached = true
			if _, e = task.run(ctx, 16); e != nil {
				t.Fatal("closed cleanup task", e)
			}
		}
	}
	if !attached {
		t.Fatal("feature-off M1 lost cleanup worker")
	}
	if _, e = legacy.SetPostInteraction(ctx, a.SessionToken, id, posts.PostLike, false); e != nil {
		t.Fatal("old writer unlike after M1", e)
	}
	if _, e = legacy.SetPostInteraction(ctx, a.SessionToken, id, posts.PostLike, true); e != nil {
		t.Fatal("old writer insert after M1", e)
	}
	if count(t, p.l.cp, `SELECT count(*) FROM c_posts.post_likes WHERE account_id=$1 AND liked_at IS NULL`, a.AccountID) != 1 {
		t.Fatal("old binary timestamp fabricated")
	}
	var tracker bool
	if e = p.l.cp.QueryRow(ctx, `SELECT to_regclass('public.goose_db_version') IS NOT NULL`).Scan(&tracker); e != nil || tracker {
		t.Fatal("fixture forged migration tracker", e)
	}
}
