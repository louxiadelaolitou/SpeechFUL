package authprivacy

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	rm "github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/recommendation/roundmodel"
)

type recommendationControl struct {
	epoch, maxUnits, growth, maxFile int64
	profile                          []byte
	enabled, feedback, admission     bool
}

func (c *Community) recommendationControl(ctx context.Context, tx pgx.Tx, now time.Time, newOperation bool) (recommendationControl, error) {
	var ctl recommendationControl
	err := tx.QueryRow(ctx, `SELECT security_epoch,max_units,growth_bytes,max_file_bytes,profile_hash,enabled,feedback_enabled,admission_enabled FROM c_recommendation.control WHERE singleton_id=1 FOR UPDATE`).Scan(&ctl.epoch, &ctl.maxUnits, &ctl.growth, &ctl.maxFile, &ctl.profile, &ctl.enabled, &ctl.feedback, &ctl.admission)
	if err != nil {
		return ctl, err
	}
	if !ctl.enabled {
		return ctl, ErrAuthorizationUnavailable
	}
	if !bytes.Equal(ctl.profile, c.recommendation.config.Capacity.Profile[:]) {
		return ctl, ErrAuthorizationUnavailable
	}
	if newOperation {
		if !ctl.admission {
			return ctl, rm.ErrCapacity
		}
		if e := c.recommendation.config.Capacity.Validate(now.UTC(), c.recommendation.config.Capacity.Profile, ctl.maxUnits, ctl.growth); e != nil {
			return ctl, e
		}
		var oldest *time.Time
		if err = tx.QueryRow(ctx, `SELECT min(due) FROM (SELECT min(due_at) due FROM c_recommendation.history UNION ALL SELECT min(feedback_until) FROM c_recommendation.rounds UNION ALL SELECT min(retain_until) FROM c_recommendation.intents) q`).Scan(&oldest); err != nil {
			return ctl, err
		}
		if oldest != nil && oldest.Before(now) {
			rate := c.recommendation.config.Capacity.MinUnitsPerSecond / 2
			if now.Sub(*oldest)+c.recommendation.config.Capacity.MaxPause+time.Duration(ctl.maxUnits)*time.Second/time.Duration(rate) >= 10*time.Minute {
				return ctl, rm.ErrCapacity
			}
		}
		var physical int64
		if err = tx.QueryRow(ctx, `SELECT COALESCE(sum(pg_total_relation_size(c.oid)),0) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='c_recommendation' AND c.relkind='r'`).Scan(&physical); err != nil {
			return ctl, err
		}
		// Operational headroom only. This cannot account transactionally for
		// aborted MVCC versions or WAL; it is not a physical-storage proof.
		if physical+(64+16*rm.MaxIDs)*ctl.growth > ctl.maxFile {
			return ctl, rm.ErrCapacity
		}
	}
	return ctl, nil
}

func strictRecommendationJSON(data []byte, target any) error {
	if len(data) > 16384 {
		return rm.ErrCodec
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if d.Decode(target) != nil || d.Decode(new(any)) != io.EOF {
		return rm.ErrCodec
	}
	b, e := json.Marshal(target)
	if e != nil || !bytes.Equal(b, data) {
		return rm.ErrCodec
	}
	return nil
}

func recommendationIDs(ids []rm.ID) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = string(id)
	}
	return out
}

func loadRecommendation(ctx context.Context, tx pgx.Tx, a uuid.UUID, id rm.ID) (rm.Record, error) {
	r := rm.Record{}
	r.State.Version = rm.Version
	r.State.Manifest.RoundID = id
	var binding, policy, hash, seed []byte
	var ids []string
	var revision int64
	err := tx.QueryRow(ctx, `SELECT binding,policy,config_hash,seed,ids,revision,created_at,last_at FROM c_recommendation.rounds WHERE account_id=$1 AND round_id=$2`, a, string(id)).Scan(&binding, &policy, &hash, &seed, &ids, &revision, &r.State.Manifest.CreatedAt, &r.State.LastAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, rm.ErrExpired
	}
	if err != nil {
		return r, err
	}
	if strictRecommendationJSON(binding, &r.State.Manifest.Binding) != nil || strictRecommendationJSON(policy, &r.State.Manifest.Policy) != nil || len(hash) != 32 || len(seed) != 8 {
		return r, rm.ErrCodec
	}
	copy(r.State.Manifest.ConfigHash[:], hash)
	r.State.Manifest.Seed = binary.BigEndian.Uint64(seed)
	r.State.Revision = uint64(revision)
	r.State.Manifest.CreatedAt = r.State.Manifest.CreatedAt.UTC()
	r.State.LastAt = r.State.LastAt.UTC()
	r.State.Manifest.IDs = []rm.ID{}
	for _, id := range ids {
		r.State.Manifest.IDs = append(r.State.Manifest.IDs, rm.ID(id))
	}
	rows, err := tx.Query(ctx, `SELECT page_index,first_at,delivered,omitted,consumed FROM c_recommendation.pages WHERE round_id=$1 ORDER BY page_index`, string(id))
	if err != nil {
		return r, err
	}
	r.State.Pages = []rm.Page{}
	for rows.Next() {
		var index, d, o, k int
		var first *time.Time
		if err = rows.Scan(&index, &first, &d, &o, &k); err != nil {
			break
		}
		if index != len(r.State.Pages) {
			err = rm.ErrCodec
			break
		}
		p := rm.Page{Delivered: uint16(d), Omitted: uint16(o), Consumed: uint16(k)}
		if first != nil {
			p.FirstAt = first.UTC()
		}
		r.State.Pages = append(r.State.Pages, p)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return r, err
	}
	h, err := loadRecommendationHistory(ctx, tx, a)
	if err != nil {
		return r, err
	}
	r.History = h
	if r.Validate() != nil {
		return rm.Record{}, rm.ErrCodec
	}
	return r, nil
}

func loadRecommendationHistory(ctx context.Context, tx pgx.Tx, a uuid.UUID) (rm.History, error) {
	h := rm.History{Account: rm.ID(a.String()), Deliveries: []rm.Delivery{}, Exposures: []rm.Exposure{}}
	var revision int64
	if err := tx.QueryRow(ctx, `SELECT revision,last_at FROM c_recommendation.accounts WHERE account_id=$1`, a).Scan(&revision, &h.LastAt); err != nil {
		return h, err
	}
	h.Revision = uint64(revision)
	h.LastAt = h.LastAt.UTC()
	rows, err := tx.Query(ctx, `SELECT post_id::text,delivery_at,delivery_until,exposure_until FROM c_recommendation.history WHERE account_id=$1 ORDER BY post_id LIMIT $2`, a, rm.MaxDeliveryHistory+rm.MaxExposureHistory+1)
	if err != nil {
		return h, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var at, du, ex *time.Time
		if err = rows.Scan(&id, &at, &du, &ex); err != nil {
			return h, err
		}
		if at != nil && du != nil {
			h.Deliveries = append(h.Deliveries, rm.Delivery{ID: rm.ID(id), At: at.UTC(), Until: du.UTC()})
		}
		if ex != nil {
			h.Exposures = append(h.Exposures, rm.Exposure{ID: rm.ID(id), Until: ex.UTC()})
		}
	}
	if err = rows.Err(); err != nil {
		return h, err
	}
	if h.Validate() != nil {
		return rm.History{}, rm.ErrCodec
	}
	return h, nil
}

func (c *Community) startRecommendation(ctx context.Context, tx pgx.Tx, scope RecommendationScope, identity rm.IntentIdentity) (RecommendationPage, error) {
	a := uuid.MustParse(string(scope.Binding.Account))
	var oldID string
	err := tx.QueryRow(ctx, `SELECT round_id::text FROM c_recommendation.intents WHERE account_id=$1 AND key_digest=$2`, a, identity.Key[:]).Scan(&oldID)
	if err == nil {
		record, e := loadRecommendation(ctx, tx, a, rm.ID(oldID))
		if e != nil {
			return RecommendationPage{}, e
		}
		return c.projectRecommendation(ctx, tx, scope, record, 0)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return RecommendationPage{}, err
	}
	if _, e := c.recommendationControl(ctx, tx, scope.At, true); e != nil {
		return RecommendationPage{}, e
	}
	if _, err = tx.Exec(ctx, `INSERT INTO c_recommendation.accounts(account_id,last_at) VALUES($1,$2) ON CONFLICT DO NOTHING`, a, scope.At); err != nil {
		return RecommendationPage{}, err
	}
	if err = recommendationAccountCapacity(ctx, tx, a, 0); err != nil {
		return RecommendationPage{}, err
	}
	h, err := loadRecommendationHistory(ctx, tx, a)
	if err != nil {
		return RecommendationPage{}, err
	}
	id, seed, err := c.recommendation.config.NewRound()
	if err != nil {
		return RecommendationPage{}, err
	}
	config := c.recommendation.config.Selector
	hash, err := rm.SelectorConfigDigest(config)
	if err != nil {
		return RecommendationPage{}, err
	}
	m := rm.Manifest{RoundID: rm.ID(id.String()), Binding: scope.Binding, ConfigHash: hash, Seed: seed, CreatedAt: scope.At, Policy: c.recommendation.config.Policy, IDs: []rm.ID{}}
	query := rm.CandidateQuery{At: scope.At, Seed: seed, Binding: scope.Binding, Channels: append([]string{}, scope.Channels...)}
	snapshot, err := (recommendationPGRepository{tx: tx, scope: scope, history: h}).ReadCandidates(ctx, query)
	if err != nil {
		return RecommendationPage{}, err
	}
	var state rm.State
	if len(scope.Channels) == 0 {
		state, err = rm.New(m)
	} else {
		state, err = rm.BuildManifest(query, snapshot, config, m)
	}
	if err != nil {
		return RecommendationPage{}, err
	}
	if err = recommendationAccountCapacity(ctx, tx, a, len(state.Manifest.IDs)); err != nil {
		return RecommendationPage{}, err
	}
	binding, _ := json.Marshal(m.Binding)
	policy, _ := json.Marshal(m.Policy)
	seedBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(seedBytes, seed)
	ids := recommendationIDs(state.Manifest.IDs)
	if _, err = tx.Exec(ctx, `INSERT INTO c_recommendation.rounds(round_id,account_id,binding,policy,config_hash,seed,ids,future_rows,future_exposures,future_writes,revision,created_at,last_at,expires_at,feedback_until) VALUES($1,$2,$3,$4,$5,$6,$7,$7,$7,$8,1,$9,$9,$10,$11)`, id, a, binding, policy, hash[:], seedBytes, ids, 64+16*len(ids), scope.At, scope.At.Add(m.Policy.RoundTTL), scope.At.Add(m.Policy.RoundTTL+m.Policy.FeedbackGrace)); err != nil {
		return RecommendationPage{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO c_recommendation.pages(round_id,page_index) SELECT $1,generate_series(0,$2::int-1)`, id, len(state.Pages)); err != nil {
		return RecommendationPage{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO c_recommendation.intents(account_id,key_digest,round_id,accept_until,retain_until) VALUES($1,$2,$3,$4,$5)`, a, identity.Key[:], id, identity.AcceptUntil, identity.RetainUntil); err != nil {
		return RecommendationPage{}, err
	}
	return c.projectRecommendation(ctx, tx, scope, rm.Record{State: state, History: h}, 0)
}

func recommendationAccountCapacity(ctx context.Context, tx pgx.Tx, a uuid.UUID, additional int) error {
	var rounds, intents, deliveries, exposures, reservedRows, reservedExposure int
	err := tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM c_recommendation.rounds WHERE account_id=$1),(SELECT count(*) FROM c_recommendation.intents WHERE account_id=$1),
 (SELECT count(*) FILTER(WHERE delivery_until IS NOT NULL) FROM c_recommendation.history WHERE account_id=$1),
 (SELECT count(*) FILTER(WHERE exposure_until IS NOT NULL) FROM c_recommendation.history WHERE account_id=$1),
 COALESCE((SELECT sum(cardinality(future_rows)) FROM c_recommendation.rounds WHERE account_id=$1),0),
 COALESCE((SELECT sum(cardinality(future_exposures)) FROM c_recommendation.rounds WHERE account_id=$1),0)`, a).Scan(&rounds, &intents, &deliveries, &exposures, &reservedRows, &reservedExposure)
	if err != nil {
		return err
	}
	if rounds >= 16 || intents >= 64 || deliveries+reservedRows+additional > rm.MaxDeliveryHistory || exposures+reservedExposure+additional > rm.MaxExposureHistory {
		return rm.ErrCapacity
	}
	return nil
}

func (c *Community) projectRecommendation(ctx context.Context, tx pgx.Tx, scope RecommendationScope, record rm.Record, offset int) (RecommendationPage, error) {
	v, err := rm.PageInput(record, rm.Context{Now: scope.At, Binding: scope.Binding}, recommendationRevisions(record), offset)
	if err != nil {
		return RecommendationPage{}, err
	}
	p, err := c.recommendation.projection.ProjectRecommendation(ctx, tx, scope, v.IDs)
	if err != nil {
		return RecommendationPage{}, err
	}
	if err = checkRecommendationUnavailable(p.Unavailable, len(v.IDs)); err != nil {
		return RecommendationPage{}, err
	}
	next, v, err := rm.Project(record, rm.Context{Now: scope.At, Binding: scope.Binding}, recommendationRevisions(record), offset, p.Unavailable)
	if err != nil {
		return RecommendationPage{}, err
	}
	if err = c.saveRecommendation(ctx, tx, record, next); err != nil {
		return RecommendationPage{}, err
	}
	expires := record.State.Manifest.CreatedAt.Add(record.State.Manifest.Policy.RoundTTL)
	feedbackUntil := minTime(v.FirstAt.Add(record.State.Manifest.Policy.FeedbackGrace), expires.Add(record.State.Manifest.Policy.FeedbackGrace))
	return RecommendationPage{View: v, RoundID: record.State.Manifest.RoundID, Items: p.Items, Revisions: recommendationRevisions(next), ExpiresAt: expires, FeedbackUntil: feedbackUntil}, nil
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func (c *Community) saveRecommendation(ctx context.Context, tx pgx.Tx, old, next rm.Record) error {
	if reflect.DeepEqual(old, next) {
		return nil
	}
	if err := next.Validate(); err != nil {
		return err
	}
	a := uuid.MustParse(string(old.History.Account))
	var current int64
	if err := tx.QueryRow(ctx, `SELECT revision FROM c_recommendation.accounts WHERE account_id=$1 FOR UPDATE`, a).Scan(&current); err != nil {
		return err
	}
	if uint64(current) != old.History.Revision {
		return rm.ErrConflict
	}
	futureRows, futureExposure := []string{}, []string{}
	for i, id := range next.State.Manifest.IDs {
		p := next.State.Pages[i/10]
		bit := uint16(1 << (i % 10))
		if p.Omitted&bit != 0 {
			continue
		}
		if p.FirstAt.IsZero() {
			futureRows = append(futureRows, string(id))
		}
		if p.Consumed&bit == 0 {
			futureExposure = append(futureExposure, string(id))
		}
	}
	type entry struct{ at, du, ex *time.Time }
	before, after := map[rm.ID]entry{}, map[rm.ID]entry{}
	for _, d := range old.History.Deliveries {
		e := before[d.ID]
		at, du := d.At, d.Until
		e.at = &at
		e.du = &du
		before[d.ID] = e
	}
	for _, e := range old.History.Exposures {
		x := before[e.ID]
		until := e.Until
		x.ex = &until
		before[e.ID] = x
	}
	for _, d := range next.History.Deliveries {
		e := after[d.ID]
		at, du := d.At, d.Until
		e.at = &at
		e.du = &du
		after[d.ID] = e
	}
	for _, e := range next.History.Exposures {
		x := after[e.ID]
		until := e.Until
		x.ex = &until
		after[e.ID] = x
	}
	changed := []rm.ID{}
	for id, e := range after {
		if !reflect.DeepEqual(e, before[id]) {
			changed = append(changed, id)
		}
	}
	sort.Slice(changed, func(i, j int) bool { return changed[i] < changed[j] })
	pageChanges := 0
	for i, p := range next.State.Pages {
		if p != old.State.Pages[i] {
			pageChanges++
		}
	}
	cost := 2 + 2*len(changed) + 2*pageChanges
	if next.History.Revision != old.History.Revision {
		cost += 2
	}
	tag, err := tx.Exec(ctx, `UPDATE c_recommendation.rounds SET revision=$1,last_at=$2,future_rows=$3,future_exposures=$4,future_writes=future_writes-$5 WHERE round_id=$6 AND account_id=$7 AND revision=$8 AND future_writes>=$5`, int64(next.State.Revision), next.State.LastAt, futureRows, futureExposure, cost, string(old.State.Manifest.RoundID), a, int64(old.State.Revision))
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return rm.ErrConflict
	}
	for i, p := range next.State.Pages {
		if p == old.State.Pages[i] {
			continue
		}
		var first *time.Time
		if !p.FirstAt.IsZero() {
			value := p.FirstAt
			first = &value
		}
		if _, err = tx.Exec(ctx, `UPDATE c_recommendation.pages SET first_at=$1,delivered=$2,omitted=$3,consumed=$4 WHERE round_id=$5 AND page_index=$6`, first, int(p.Delivered), int(p.Omitted), int(p.Consumed), string(old.State.Manifest.RoundID), i); err != nil {
			return err
		}
	}
	if len(changed) > 0 {
		ids := []string{}
		ats, dus, exs := []*time.Time{}, []*time.Time{}, []*time.Time{}
		for _, id := range changed {
			e := after[id]
			ids = append(ids, string(id))
			ats = append(ats, e.at)
			dus = append(dus, e.du)
			exs = append(exs, e.ex)
		}
		if _, err = tx.Exec(ctx, `INSERT INTO c_recommendation.history(account_id,post_id,delivery_at,delivery_until,exposure_until) SELECT $1,id,at,du,ex FROM unnest($2::uuid[],$3::timestamptz[],$4::timestamptz[],$5::timestamptz[]) AS x(id,at,du,ex) ON CONFLICT(account_id,post_id) DO UPDATE SET delivery_at=EXCLUDED.delivery_at,delivery_until=EXCLUDED.delivery_until,exposure_until=EXCLUDED.exposure_until`, a, ids, ats, dus, exs); err != nil {
			return err
		}
	}
	if next.History.Revision != old.History.Revision {
		tag, err = tx.Exec(ctx, `UPDATE c_recommendation.accounts SET revision=$1,last_at=$2 WHERE account_id=$3 AND revision=$4`, int64(next.History.Revision), next.History.LastAt, a, int64(old.History.Revision))
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return rm.ErrConflict
		}
	}
	return nil
}
