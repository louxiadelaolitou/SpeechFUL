package authprivacy

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"reflect"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/recommendation"
	rm "github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/recommendation/roundmodel"
)

type recommendationPGRepository struct {
	tx      pgx.Tx
	scope   RecommendationScope
	history rm.History
}

var _ rm.CandidateRepository = recommendationPGRepository{}

func (r recommendationPGRepository) ReadCandidates(ctx context.Context, q rm.CandidateQuery) (rm.CandidateSnapshot, error) {
	if q.Binding != r.scope.Binding || !q.At.Equal(r.scope.At) || !reflect.DeepEqual(q.Channels, r.scope.Channels) {
		return rm.CandidateSnapshot{}, rm.ErrScope
	}
	s, _, e := readRecommendationCandidates(ctx, r.tx, r.scope, q.Seed, r.history)
	return s, e
}

// Metadata and private current facts only. Windows are limited BEFORE joins;
// this is a finite sampled discussion/interest source, not a global hot chart.
const recommendationBaseSQL = `WITH keys AS MATERIALIZED (
 SELECT post_id,channel_id,owner_account_id,published_at,publication_ordinal,published_attempt_version
 FROM c_posts.posts WHERE visibility='PUBLISHED' AND NOT moderation_hidden AND published_at<=$1 AND published_at>$1-interval '30 days'
 ORDER BY published_at DESC,publication_ordinal DESC LIMIT 2000)
 SELECT k.post_id::text,k.channel_id::text,t.identity_id::text,k.owner_account_id::text,k.published_at,k.publication_ordinal
 FROM keys k JOIN c_posts.publication_tasks t USING(post_id)
 JOIN c_posts.attempt_contents x ON x.task_id=t.task_id AND x.version=k.published_attempt_version
 WHERE k.channel_id=ANY($2::uuid[]) AND x.erased_at IS NULL
 AND NOT c_posts.accounts_blocked($3::uuid,k.owner_account_id)
 AND NOT EXISTS(SELECT 1 FROM c_posts.hidden_posts hp WHERE hp.account_id=$3::uuid AND hp.post_id=k.post_id AND hp.hidden)
 ORDER BY k.published_at DESC,k.publication_ordinal DESC`

const recommendationFactsSQL = `WITH
 likes AS MATERIALIZED (SELECT post_id,account_id,liked_at at FROM c_posts.post_likes WHERE liked_at<=$1 AND liked_at>$1-interval '30 days' ORDER BY liked_at DESC,post_id,account_id LIMIT 1024),
 favorites AS MATERIALIZED (SELECT post_id,account_id,saved_at at FROM c_posts.post_favorites WHERE saved_at<=$1 AND saved_at>$1-interval '30 days' ORDER BY saved_at DESC,post_id,account_id LIMIT 1024),
 comment_keys AS MATERIALIZED (SELECT task_id,terminal_at at FROM c_comments.attempts WHERE state='PUBLISHED' AND terminal_at<=$1 AND terminal_at>$1-interval '30 days' ORDER BY terminal_at DESC,task_id LIMIT 1024),
 facts AS (
 SELECT post_id,account_id,at,0 kind FROM likes UNION ALL SELECT post_id,account_id,at,1 FROM favorites
 UNION ALL SELECT t.post_id,t.owner_account_id,k.at,2 FROM comment_keys k JOIN c_comments.tasks t USING(task_id)
 LEFT JOIN c_comments.tasks root ON root.comment_id=t.root_comment_id
 WHERE t.deleted_at IS NULL AND NOT t.moderation_hidden AND (t.root_comment_id IS NULL OR (root.deleted_at IS NULL AND NOT root.moderation_hidden)))
 SELECT f.post_id::text,f.account_id::text,f.at,f.kind FROM facts f JOIN c_auth.accounts a ON a.account_id=f.account_id
 WHERE a.state<>'CLOSED' ORDER BY f.at DESC,f.post_id,f.account_id,f.kind`

func readRecommendationCandidates(ctx context.Context, tx pgx.Tx, scope RecommendationScope, seed uint64, h rm.History) (rm.CandidateSnapshot, rm.CandidateQuery, error) {
	q := rm.CandidateQuery{At: scope.At, Seed: seed, Binding: scope.Binding, Channels: append([]string{}, scope.Channels...)}
	s := rm.CandidateSnapshot{Input: recommendation.Input{Now: scope.At, Seed: seed, Channels: append([]string{}, scope.Channels...), Candidates: []recommendation.Candidate{}, Signals: []recommendation.InterestSignal{}}}
	if len(scope.Channels) == 0 {
		return s, q, nil
	}
	rows, err := tx.Query(ctx, recommendationBaseSQL, scope.At, scope.Channels, string(scope.Binding.Account))
	if err != nil {
		return s, q, err
	}
	base := []recommendation.Candidate{}
	positions := map[string]int{}
	excluded, delivered := map[string]bool{}, map[string]bool{}
	for _, e := range h.Exposures {
		if e.Until.After(scope.At) {
			excluded[string(e.ID)] = true
		}
	}
	for _, d := range h.Deliveries {
		if d.Until.After(scope.At) {
			delivered[string(d.ID)] = true
		}
	}
	for rows.Next() {
		var p recommendation.Candidate
		if err = rows.Scan(&p.ID, &p.Channel, &p.AuthorGroup, &p.OwnerActorKey, &p.PublishedAt, &p.Ordinal); err != nil {
			break
		}
		p.PublishedAt = p.PublishedAt.UTC()
		p.Eligible = !excluded[p.ID]
		positions[p.ID] = len(base)
		base = append(base, p)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return s, q, err
	}
	// Charge the full bounded key window; returned filtered rows are not a scan
	// measurement. EXPLAIN is recorded separately by the isolated SQL fixture.
	s.Work.BaseKeys = 2000
	facts, err := tx.Query(ctx, recommendationFactsSQL, scope.At)
	if err != nil {
		return s, q, err
	}
	heat := map[string]int{}
	interest := map[string]float64{}
	counts := map[string][3]int{}
	own := [3]int{}
	for facts.Next() {
		var post, actor string
		var at time.Time
		var kind int
		if err = facts.Scan(&post, &actor, &at, &kind); err != nil {
			break
		}
		s.Work.DiscussionFacts++
		index, ok := positions[post]
		if !ok {
			continue
		}
		p := &base[index]
		at = at.UTC()
		if at.Before(p.PublishedAt) {
			continue
		}
		if actor == string(scope.Binding.Account) && own[kind] < 256 {
			weight := 1.0
			if kind == 1 {
				weight = 2
			}
			s.Input.Signals = append(s.Input.Signals, recommendation.InterestSignal{Channel: p.Channel, At: at, Weight: weight})
			interest[p.Channel] += weight
			own[kind]++
		}
		if scope.At.Sub(at) <= 7*24*time.Hour {
			heat[post]++
			limits := [3]int{3, 3, 4}
			used := counts[post]
			if used[kind] < limits[kind] {
				p.Interactions = append(p.Interactions, recommendation.Interaction{ActorKey: actor, At: at, Kind: recommendation.InteractionKind(kind), Eligible: true})
				used[kind]++
				counts[post] = used
			}
		}
	}
	if err == nil {
		err = facts.Err()
	}
	facts.Close()
	if err != nil {
		return s, q, err
	}
	s.Work.InterestFacts = len(s.Input.Signals)
	byRecent := append([]recommendation.Candidate{}, base...)
	byInterest := append([]recommendation.Candidate{}, base...)
	byHeat := append([]recommendation.Candidate{}, base...)
	byExplore := append([]recommendation.Candidate{}, base...)
	sort.SliceStable(byInterest, func(i, j int) bool { return interest[byInterest[i].Channel] > interest[byInterest[j].Channel] })
	sort.SliceStable(byHeat, func(i, j int) bool { return heat[byHeat[i].ID] > heat[byHeat[j].ID] })
	digest := func(id string) [32]byte {
		b := make([]byte, 8, 8+len(id))
		binary.BigEndian.PutUint64(b, seed)
		return sha256.Sum256(append(b, []byte(id)...))
	}
	sort.SliceStable(byExplore, func(i, j int) bool {
		a, b := digest(byExplore[i].ID), digest(byExplore[j].ID)
		for k := range a {
			if a[k] != b[k] {
				return a[k] < b[k]
			}
		}
		return byExplore[i].ID < byExplore[j].ID
	})
	chosen := map[string]bool{}
	for source, list := range [][]recommendation.Candidate{byRecent, byInterest, byHeat, byExplore} {
		for _, p := range list {
			if !p.Eligible {
				continue
			}
			if len(s.Input.Pools[source]) >= 100 {
				break
			}
			s.Input.Pools[source] = append(s.Input.Pools[source], p.ID)
			chosen[p.ID] = true
		}
	}
	for _, p := range base {
		if !chosen[p.ID] {
			continue
		}
		s.Input.Candidates = append(s.Input.Candidates, p)
		s.Work.HeatFacts += len(p.Interactions)
		// The B1 cooldown gate consumes a decision-time membership marker. It is
		// NEVER persisted as a new delivery/event timestamp. Absolute stored Until
		// selected membership above, so a new config cannot shorten old windows.
		if delivered[p.ID] {
			s.Input.State.Delivered = append(s.Input.State.Delivered, recommendation.TimedID{ID: p.ID, At: scope.At})
		}
	}
	return s, q, rm.ValidateCandidates(q, s)
}
