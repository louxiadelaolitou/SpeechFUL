package authprivacy

import (
	"context"
	"crypto/sha256"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/posts"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/rankings"
)

type preparedRankingFixture struct {
	control       *gateControl
	l             *lab
	owner, reader SignupResult
	identity      uuid.UUID
	redis         *redis.Client
	prefix        string
}

func preparedRankingLab(t *testing.T) *preparedRankingFixture {
	t.Helper()
	socket := os.Getenv("RANKING_REDIS_SOCKET")
	if socket == "" {
		t.Skip("owned rankings Redis fixture is required")
	}
	control := newGateControl(t)
	l := control.lab
	cfg := enablePostLab(t, l)
	cfg.PreparedFeedReads = true
	var e error
	l.c, e = l.c.WithPosts(context.Background(), cfg)
	if e != nil {
		t.Fatal(e)
	}
	l.c = withRankingTestComments(t, l.c)
	env := uuid.NewString()
	options := &redis.Options{Network: "unix", Addr: socket}
	cache := rankings.NewCache(options, env)
	t.Cleanup(func() { _ = cache.Close() })
	l.c, e = l.c.WithRankings(context.Background(), cache)
	if e != nil {
		t.Fatal(e)
	}
	f := &preparedRankingFixture{control: control, l: l, prefix: "hnuhole:channel-rankings:v2:" + env + ":", redis: redis.NewClient(options)}
	t.Cleanup(func() { _ = f.redis.Close() })
	f.owner = sessionTestAccount(t, l, "pr_owner")
	f.reader = sessionTestAccount(t, l, "pr_reader")
	f.identity = identityChange(t, l.c, f.owner.SessionToken, "CREATE", uuid.Nil, "热榜作者").IdentityID
	return f
}

func (f *preparedRankingFixture) build(t *testing.T) {
	t.Helper()
	advanceClosureClock(t, f.control, f.control.clock.now().Truncate(rankings.Interval).Add(rankings.Interval))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, e := f.l.c.RefreshRankings(ctx, f.l.cp, DefaultRankingWeights()); e != nil {
		t.Fatal(e)
	}
}

func rankingIDs(page posts.PostRanking) []string {
	ids := make([]string, len(page.Items))
	for i, item := range page.Items {
		ids[i] = item.PostID
	}
	return ids
}

func TestFeedPreparedRankingStorageCombinationsPostgres(t *testing.T) {
	for _, mode := range []string{"both", "cache_missing", "mirror_missing", "mirror_partial", "both_missing", "cache_missing_mirror_partial", "wrong_generation", "future_cache", "expired_cache", "cache_partial", "invalid_cache"} {
		t.Run(mode, func(t *testing.T) {
			f := preparedRankingLab(t)
			l := f.l
			preparedFeedPublish(t, l, f.owner.SessionToken, f.identity, "first")
			preparedFeedPublish(t, l, f.owner.SessionToken, f.identity, "second")
			f.build(t)
			if mode == "wrong_generation" || mode == "future_cache" || mode == "expired_cache" || mode == "cache_partial" {
				snapshot, e := l.c.rankings.Snapshot(context.Background())
				if e != nil {
					t.Fatal(e)
				}
				if mode == "cache_partial" {
					if e = f.redis.Del(context.Background(), f.prefix+snapshot.Version+":all").Err(); e != nil {
						t.Fatal(e)
					}
				} else {
					var count int64
					for _, n := range snapshot.Counts {
						count += min(n, rankings.ReadLimit)
					}
					ids, e := l.c.rankings.Candidates(context.Background(), snapshot, "all", count)
					if e != nil {
						t.Fatal(e)
					}
					switch mode {
					case "wrong_generation":
						snapshot.Generation++
					case "future_cache":
						snapshot.Bucket = snapshot.Bucket.Add(rankings.Interval)
					case "expired_cache":
						snapshot.Bucket = snapshot.Bucket.Add(-rankings.MaxAge)
					}
					snapshot.Version = rankings.Version(snapshot.Generation, snapshot.Bucket)
					entries := make([]rankings.Entry, len(ids))
					for i, id := range ids {
						entries[i] = rankings.Entry{Rank: int64(i + 1), PostID: id}
					}
					if e = l.c.rankings.Stage(context.Background(), snapshot, "all", entries); e != nil {
						t.Fatal(e)
					}
					if e = f.redis.Del(context.Background(), f.prefix+"active").Err(); e != nil {
						t.Fatal(e)
					}
					if e = l.c.rankings.Publish(context.Background(), snapshot); e != nil {
						t.Fatal(e)
					}
				}
			}
			if mode == "invalid_cache" {
				if e := f.redis.Set(context.Background(), f.prefix+"active", "{", time.Minute).Err(); e != nil {
					t.Fatal(e)
				}
			}
			if mode == "cache_missing" || mode == "both_missing" || mode == "cache_missing_mirror_partial" {
				if e := f.redis.Del(context.Background(), f.prefix+"active").Err(); e != nil {
					t.Fatal(e)
				}
			}
			if mode == "mirror_missing" || mode == "both_missing" {
				mustExec(t, l.cp, `DELETE FROM c_posts.hot_snapshots`)
			}
			if mode == "mirror_partial" || mode == "cache_missing_mirror_partial" {
				mustExec(t, l.cp, `DELETE FROM c_posts.hot_blocks`)
			}
			l.c.posts.preparedFeedReads = false
			old, oldError := l.c.ReadRankings(context.Background(), f.reader.SessionToken)
			l.c.posts.preparedFeedReads = true
			fresh, newError := l.c.ReadRankings(context.Background(), f.reader.SessionToken)
			if mode == "both_missing" || mode == "cache_missing_mirror_partial" {
				postTestError(t, oldError, "RANKINGS_UNAVAILABLE", 503)
				postTestError(t, newError, "RANKINGS_UNAVAILABLE", 503)
				if fresh.Value != nil {
					t.Fatal("unavailable mirror released a board")
				}
				return
			}
			if oldError != nil || newError != nil {
				t.Fatal("changed storage fallback behavior", oldError, newError)
			}
			a, b := old.Value.(posts.PostRanking), fresh.Value.(posts.PostRanking)
			if !a.AsOf.Equal(b.AsOf) || !slices.Equal(rankingIDs(a), rankingIDs(b)) {
				t.Fatal("prepared board changed frozen order", a, b)
			}
		})
	}
}

func TestFeedPreparedRankingInterleavingPostgres(t *testing.T) {
	for _, mode := range []string{"mirror_gc", "cache_eviction", "new_version", "hide", "restore_candidate", "block", "snapshot_age", "window_168h", "twice"} {
		t.Run(mode, func(t *testing.T) {
			f := preparedRankingLab(t)
			l := f.l
			first := preparedFeedPublish(t, l, f.owner.SessionToken, f.identity, "first")
			latest := preparedFeedPublish(t, l, f.owner.SessionToken, f.identity, "latest")
			var published time.Time
			if e := l.cp.QueryRow(context.Background(), `SELECT published_at FROM c_posts.posts WHERE post_id=$1`, latest).Scan(&published); e != nil {
				t.Fatal(e)
			}
			if mode == "window_168h" {
				advanceClosureClock(t, f.control, published.Add(168*time.Hour-time.Second))
			}
			if mode == "window_168h" {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				_, e := l.c.RefreshRankings(ctx, l.cp, DefaultRankingWeights())
				cancel()
				if e != nil {
					t.Fatal(e)
				}
			} else {
				f.build(t)
			}
			if mode == "restore_candidate" {
				mustExec(t, l.cp, `UPDATE c_posts.posts SET moderation_hidden=true WHERE post_id=$1`, latest)
			}
			before, e := l.c.ReadRankings(context.Background(), f.reader.SessionToken)
			if e != nil {
				t.Fatal(e)
			}
			asOf := before.Value.(posts.PostRanking).AsOf
			original := *l.c
			token := sha256.Sum256(f.reader.SessionToken[:])
			mustExec(t, l.cp, `UPDATE c_auth.sessions SET expires_at=last_activity_at+interval '1 hour' WHERE token_digest=$1`, token[:])
			durableBefore := preparedReadState(t, l, token)
			calls := 0
			preparedReadPool(t, l, func(n int) {
				calls = n
				if n > 1 && mode != "twice" {
					return
				}
				switch mode {
				case "mirror_gc":
					mustExec(t, l.cp, `DELETE FROM c_posts.hot_snapshots; DELETE FROM c_posts.hot_shard_data`)
				case "cache_eviction":
					if e := f.redis.Del(context.Background(), f.prefix+"active").Err(); e != nil {
						t.Fatal(e)
					}
				case "new_version":
					advanceClosureClock(t, f.control, f.control.clock.now().Add(rankings.Interval))
					if _, e := original.RefreshRankings(context.Background(), l.cp, DefaultRankingWeights()); e != nil {
						t.Fatal(e)
					}
				case "hide":
					state, e := original.ReadHiddenPost(context.Background(), f.reader.SessionToken, latest)
					if e != nil {
						t.Fatal(e)
					}
					if _, e = original.SetHiddenPost(context.Background(), f.reader.SessionToken, latest, true, state.InteractionVersion); e != nil {
						t.Fatal(e)
					}
				case "restore_candidate":
					mustExec(t, l.cp, `UPDATE c_posts.posts SET moderation_hidden=false WHERE post_id=$1`, latest)
				case "block":
					if _, e := original.BlockPostAuthor(context.Background(), f.reader.SessionToken, latest); e != nil {
						t.Fatal(e)
					}
				case "snapshot_age":
					advanceClosureClock(t, f.control, asOf.Add(rankings.MaxAge))
				case "window_168h":
					advanceClosureClock(t, f.control, published.Add(168*time.Hour))
				case "twice":
					mustExec(t, l.cp, `UPDATE c_posts.posts SET moderation_hidden=NOT moderation_hidden WHERE post_id=$1`, latest)
				}
			})
			r, e := l.c.ReadRankings(context.Background(), f.reader.SessionToken)
			if mode == "snapshot_age" || mode == "twice" {
				postTestError(t, e, "RANKINGS_UNAVAILABLE", 503)
				if calls != 2 {
					t.Fatal("ranking conflict was not bounded", calls)
				}
				after := preparedReadState(t, l, token)
				if !after.activity.Equal(durableBefore.activity) || !after.expiry.Equal(durableBefore.expiry) {
					t.Fatal("ranking conflict renewed the session")
				}
				assertPreparedReadAnchorMatches(t, l, after)
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			page := r.Value.(posts.PostRanking)
			switch mode {
			case "mirror_gc", "cache_eviction", "new_version":
				if calls != 1 || !page.AsOf.Equal(asOf) || !slices.Equal(rankingIDs(page), rankingIDs(before.Value.(posts.PostRanking))) {
					t.Fatal("immutable loaded board was revoked or replaced", page, calls)
				}
			case "hide":
				if calls != 2 || !slices.Equal(rankingIDs(page), []string{first}) {
					t.Fatal("hidden hot candidate escaped", page, calls)
				}
			case "restore_candidate":
				if calls != 2 || !slices.Equal(rankingIDs(page), []string{latest, first}) {
					t.Fatal("unselected candidate changed without rebuilding complete hot range", page, calls)
				}
			case "block", "window_168h":
				if calls != 2 || len(page.Items) != 0 {
					t.Fatal("filtered/aged candidate escaped", page, calls)
				}
			}
		})
	}
}

func TestFeedPreparedRankingLifecyclePostgres(t *testing.T) {
	for _, mode := range []string{"cancel", "generation", "commit_ack_loss"} {
		t.Run(mode, func(t *testing.T) {
			f := preparedRankingLab(t)
			l := f.l
			preparedFeedPublish(t, l, f.owner.SessionToken, f.identity, "private ranking")
			f.build(t)
			token := sha256.Sum256(f.reader.SessionToken[:])
			mustExec(t, l.cp, `UPDATE c_auth.sessions SET expires_at=last_activity_at+interval '1 hour' WHERE token_digest=$1`, token[:])
			before := preparedReadState(t, l, token)
			advanceClosureClock(t, f.control, f.control.clock.now().Add(time.Minute))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "commit_ack_loss" {
				pool, proxy := newCommitACKProxyPool(t, l.cp)
				l.c.pool = pool
				proxy.armed.Store(true)
			} else {
				preparedReadPool(t, l, func(n int) {
					if n == 1 {
						if mode == "cancel" {
							cancel()
						} else {
							mustExec(t, l.cp, `UPDATE c_auth.accounts SET session_generation=session_generation+1 WHERE account_id=$1`, f.reader.AccountID)
						}
					}
				})
			}
			r, e := l.c.ReadRankings(ctx, f.reader.SessionToken)
			if e == nil || r.Value != nil || !r.ExpiresAt.IsZero() {
				t.Fatal("native ranking denial/unknown commit released title", e)
			}
			after := preparedReadState(t, l, token)
			assertPreparedReadAnchorMatches(t, l, after)
			if mode == "commit_ack_loss" {
				if !after.highwater.After(before.highwater) || !after.expiry.After(before.expiry) {
					t.Fatal("ranking COMMIT ACK fixture was not durable")
				}
			} else if !after.activity.Equal(before.activity) || !after.expiry.Equal(before.expiry) {
				t.Fatal("denied ranking renewed session")
			}
		})
	}
}
