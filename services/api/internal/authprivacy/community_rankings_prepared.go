package authprivacy

import (
	"context"
	"errors"
	"math/big"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/posts"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/rankings"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/readtiming"
)

// The SQL mirror is loaded in one RR snapshot, including shard links and every
// block searched by the existing exact fallback algorithm. Workers may replace
// or physically evict that immutable mirror after preparation; neither action
// revokes a loaded list that still passes its logical time and resource proof.
func (c *Community) readPreparedRankings(ctx context.Context, bearer [32]byte) (PostResponse, error) {
	ctx = readtiming.Phase(ctx, "rankings")
	return c.withPreparedReadPlan(ctx, bearer, rankingsUnavailable(), func(prepareCtx context.Context, tx pgx.Tx, s readSubject) (*preparedReadPlan, error) {
		proof := &feedReadProof{}
		page := posts.PostRanking{Items: []posts.RankedPost{}}
		var snapshot rankings.Snapshot
		var expires time.Time
		pending := func() error {
			at := s.decision.TrustedAt
			preferences, _, e := readFeedPreferences(prepareCtx, tx, s.account)
			if e != nil {
				return e
			}
			// Preserve the original cache/SQL availability combinations and
			// frozen version. All source projection and mirror fallback reads
			// now share this transaction's consistent resource snapshot.
			cacheCtx, cancel := context.WithTimeout(prepareCtx, 40*time.Millisecond)
			cached, cacheError := c.rankings.Snapshot(cacheCtx)
			var cachedIDs []string
			useCache := cacheError == nil && cached.ValidAt(s.decision.Generation, at)
			if useCache {
				var count int64
				for _, n := range cached.Counts {
					count += min(n, rankings.ReadLimit)
				}
				cachedIDs, cacheError = c.rankings.Candidates(cacheCtx, cached, "all", count)
				useCache = cacheError == nil
			}
			cancel()
			if useCache {
				snapshot = cached
				proof.ids = cachedIDs[:min(len(cachedIDs), rankings.ReadLimit)]
				page, e = c.preparedRankingProjection(prepareCtx, tx, s.account, snapshot, proof.ids, preferences.BlockedChannelIDs, at, true)
				if errors.Is(e, errRankingNeedsMore) && len(cachedIDs) > rankings.ReadLimit {
					proof.ids = cachedIDs
					page, e = c.preparedRankingProjection(prepareCtx, tx, s.account, snapshot, cachedIDs, preferences.BlockedChannelIDs, at, false)
				}
				if e == nil {
					return nil
				}
				if !errors.Is(e, errRankingNeedsMore) {
					return e
				}
			} else {
				if e = tx.QueryRow(prepareCtx, readRankSnapshotSQL, s.decision.Generation, &at, at.Add(-rankings.MaxAge)).Scan(&snapshot.Version, &snapshot.Generation, &snapshot.Bucket, &snapshot.Channels, &snapshot.Counts); e != nil {
					return rankingsUnavailable()
				}
				if !snapshot.ValidAt(s.decision.Generation, at) {
					return rankingsUnavailable()
				}
			}
			snapshot.Bucket = snapshot.Bucket.UTC()
			page = posts.PostRanking{AsOf: snapshot.Bucket, Items: []posts.RankedPost{}}
			ids, e := c.preparedRankingCandidates(prepareCtx, tx, snapshot, preferences.BlockedChannelIDs, s.account, at)
			proof.ids = ids
			if e != nil {
				return e
			}
			if len(ids) == 0 {
				return nil
			}
			projection := projectRankedPostsSQL
			args := []any{ids, preferences.BlockedChannelIDs, at, s.account}
			if c.posts.hiddenPostsEnabled {
				projection = strings.Replace(projection, rankedPostVisibleSQL, rankedPostVisibleSQL+` AND NOT EXISTS(SELECT 1 FROM c_posts.hidden_posts hp WHERE hp.account_id=$5 AND hp.post_id=p.post_id AND hp.hidden)`, 1)
				args = append(args, s.account)
			}
			rows, e := tx.Query(prepareCtx, projection, args...)
			if e != nil {
				return e
			}
			page.Items, e = pgx.CollectRows(rows, func(row pgx.CollectableRow) (item posts.RankedPost, e error) {
				var channels []string
				var counts []int64
				e = row.Scan(&item.PostID, &item.ChannelID, &item.Title, &item.ViewCount, &channels, &counts)
				return
			})
			if e != nil {
				return e
			}
			if len(page.Items) != len(ids) {
				return rankingsUnavailable()
			}
			return nil
		}()
		if pending == nil {
			proof.ids = make([]string, len(page.Items))
			for i, item := range page.Items {
				proof.ids[i] = item.PostID
			}
			if len(proof.ids) > 0 {
				pending = tx.QueryRow(prepareCtx, `SELECT min(published_at)+interval '168 hours' FROM c_posts.posts WHERE post_id=ANY($1::uuid[])`, proof.ids).Scan(&expires)
			}
		}
		var e error
		proof.versions, e = proof.read(prepareCtx, tx, s.account)
		if e != nil {
			pending = e
		}
		return &preparedReadPlan{
			Revalidate: func(tx pgx.Tx, a uuid.UUID, d AuthorizationDecision, _ restriction) error {
				if e := proof.revalidate(ctx, tx, a); e != nil {
					return e
				}
				if snapshot.Version != "" && !snapshot.ValidAt(d.Generation, d.TrustedAt) {
					return errReadConflict
				}
				if !expires.IsZero() && !d.TrustedAt.Before(expires) {
					return errReadConflict
				}
				return nil
			},
			Complete: func(tx pgx.Tx, _ uuid.UUID, _ AuthorizationDecision, _ restriction) (any, error) {
				views, e := readPreparedViewCounts(ctx, tx, proof.ids)
				if e != nil {
					return nil, e
				}
				for i := range page.Items {
					page.Items[i].ViewCount = views[page.Items[i].PostID]
				}
				return page, nil
			},
		}, pending
	})
}

func (c *Community) preparedRankingProjection(ctx context.Context, tx pgx.Tx, account uuid.UUID, snapshot rankings.Snapshot, ids, blocked []string, at time.Time, completePrefix bool) (posts.PostRanking, error) {
	page := posts.PostRanking{AsOf: snapshot.Bucket, Items: []posts.RankedPost{}}
	var count int64
	for i, ch := range snapshot.Channels {
		if !slices.Contains(blocked, ch) {
			count += snapshot.Counts[i]
		}
	}
	expected := int(min(count, posts.PostRankingLimit))
	if len(ids) == 0 {
		if expected > 0 {
			return page, errRankingNeedsMore
		}
		return page, nil
	}
	projection := projectRankedPostsSQL
	args := []any{ids, blocked, at, account}
	if c.posts.hiddenPostsEnabled {
		projection = strings.Replace(projection, rankedPostVisibleSQL, rankedPostVisibleSQL+` AND NOT EXISTS(SELECT 1 FROM c_posts.hidden_posts hp WHERE hp.account_id=$5 AND hp.post_id=p.post_id AND hp.hidden)`, 1)
		args = append(args, account)
	}
	rows, e := tx.Query(ctx, projection, args...)
	if e != nil {
		return page, e
	}
	var coverageChannels []string
	var coverageCounts []int64
	page.Items, e = pgx.CollectRows(rows, func(row pgx.CollectableRow) (item posts.RankedPost, e error) {
		e = row.Scan(&item.PostID, &item.ChannelID, &item.Title, &item.ViewCount, &coverageChannels, &coverageCounts)
		return
	})
	if e != nil {
		return page, e
	}
	if page.Items == nil {
		page.Items = []posts.RankedPost{}
	}
	if !completePrefix {
		for i, ch := range snapshot.Channels {
			if slices.Contains(blocked, ch) || snapshot.Counts[i] <= rankings.ReadLimit {
				continue
			}
			j := slices.Index(coverageChannels, ch)
			if j < 0 || coverageCounts[j] < posts.PostRankingLimit {
				return page, errRankingNeedsMore
			}
		}
	}
	if len(page.Items) < expected {
		return page, errRankingNeedsMore
	}
	return page, nil
}

func readPreparedRankingBlock(ctx context.Context, tx pgx.Tx, snapshot rankings.Snapshot, channels []string, block int64) ([]rankingCandidate, error) {
	rows, e := tx.Query(ctx, rankBlockSQL, snapshot.Version, channels, block)
	if e != nil {
		return nil, e
	}
	items, e := pgx.CollectRows(rows, func(row pgx.CollectableRow) (item rankingCandidate, e error) {
		var score string
		e = row.Scan(&item.channel, &item.id, &item.positive, &score, &item.published, &item.ordinal)
		if e == nil {
			var ok bool
			item.score, ok = new(big.Rat).SetString(score)
			if !ok {
				e = rankingsUnavailable()
			}
		}
		return
	})
	if e != nil {
		return nil, e
	}
	counts := map[string]int64{}
	for _, item := range items {
		counts[item.channel]++
	}
	for _, channel := range channels {
		i := slices.Index(snapshot.Channels, channel)
		if i < 0 || counts[channel] != min(max(snapshot.Counts[i]-block*rankings.ReadLimit, 0), rankings.ReadLimit) {
			return nil, rankingsUnavailable()
		}
	}
	return items, nil
}

// Keep the original 64-entry blocks, first 20 live entries per channel, and
// exact rational score comparison. Only the transaction owning the reads changes.
func (c *Community) preparedRankingCandidates(ctx context.Context, tx pgx.Tx, snapshot rankings.Snapshot, blocked []string, account uuid.UUID, at time.Time) ([]string, error) {
	counts := map[string]int{}
	var candidates []rankingCandidate
	for block := int64(0); ; block++ {
		var pending []string
		for i, channel := range snapshot.Channels {
			if !slices.Contains(blocked, channel) && counts[channel] < posts.PostRankingLimit && block*rankings.ReadLimit < snapshot.Counts[i] {
				pending = append(pending, channel)
			}
		}
		if len(pending) == 0 {
			break
		}
		batch, e := readPreparedRankingBlock(ctx, tx, snapshot, pending, block)
		if e != nil {
			return nil, e
		}
		ids := make([]string, len(batch))
		for i, item := range batch {
			ids[i] = item.id
		}
		if len(ids) == 0 {
			return nil, rankingsUnavailable()
		}
		query := liveRankCandidatesSQL
		args := []any{ids, blocked, at, account}
		if c.posts.hiddenPostsEnabled {
			query = strings.Replace(query, rankedPostVisibleSQL, rankedPostVisibleSQL+` AND NOT EXISTS(SELECT 1 FROM c_posts.hidden_posts hp WHERE hp.account_id=$5 AND hp.post_id=p.post_id AND hp.hidden)`, 1)
			args = append(args, account)
		}
		rows, e := tx.Query(ctx, query, args...)
		if e != nil {
			return nil, e
		}
		live, e := pgx.CollectRows(rows, pgx.RowTo[string])
		if e != nil {
			return nil, e
		}
		visible := map[string]bool{}
		for _, id := range live {
			visible[id] = true
		}
		for _, item := range batch {
			if visible[item.id] && counts[item.channel] < posts.PostRankingLimit {
				candidates = append(candidates, item)
				counts[item.channel]++
			}
		}
	}
	sortRankingCandidates(candidates)
	ids := make([]string, 0, min(len(candidates), posts.PostRankingLimit))
	for _, item := range candidates[:min(len(candidates), posts.PostRankingLimit)] {
		ids = append(ids, item.id)
	}
	return ids, nil
}
