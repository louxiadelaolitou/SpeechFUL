package authprivacy

// This bridge is compiled by go test only. Production has neither a key/profile
// loader nor an exported activated constructor. All SQL belongs to the existing
// nonce/owner-verified disposable-PG fixture.
import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/posts"
	rm "github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/recommendation/roundmodel"
	"testing"
	"time"

	rw "github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/recommendation/transport"
)

type RecommendationHTTPLab struct {
	p             *recommendationPG
	Reader, Other SignupResult
	key           rw.Key
	ChannelID     string
}

func NewRecommendationHTTPLab(t *testing.T, posts int) *RecommendationHTTPLab {
	t.Helper()
	p := newRecommendationPG(t, posts)
	f := &RecommendationHTTPLab{p: p, Reader: sessionTestAccount(t, p.l, "reco_http_reader"), Other: sessionTestAccount(t, p.l, "reco_http_other")}
	now := p.control.clock.now().UTC().Truncate(time.Microsecond)
	f.key = rw.Key{ID: 1, IssueFrom: now.Add(-time.Hour), IssueUntil: now.Add(23 * time.Hour), VerifyUntil: now.Add(23*time.Hour + rw.MaxLifetime)}
	if _, err := rand.Read(f.key.Secret[:]); err != nil {
		t.Fatal(err)
	}
	for _, other := range [][32]byte{p.l.c.requestKey, p.postConfig.KeyKey, p.postConfig.FingerprintKey, p.postConfig.CursorKey} {
		if other == f.key.Secret {
			t.Fatal("recommendation key reused another domain key")
		}
	}
	if err := p.l.cp.QueryRow(context.Background(), `SELECT id::text FROM public.channels ORDER BY id LIMIT 1`).Scan(&f.ChannelID); err != nil {
		t.Fatal(err)
	}
	f.Rebuild(t)
	return f
}
func (f *RecommendationHTTPLab) Community() *Community { return f.p.service }
func (f *RecommendationHTTPLab) UsePreparedFeed()      { f.p.service.posts.preparedFeedReads = true }

// PublishAllChannels creates one published post in every catalog channel so
// the public recommendation path can prove the mainline ten-channel scope.
func (f *RecommendationHTTPLab) PublishAllChannels(t *testing.T) []string {
	t.Helper()
	ctx := context.Background()
	identity := identityChange(t, f.p.l.c, f.p.author.SessionToken, "CREATE", uuid.Nil, "跨频道作者").IdentityID
	rows, err := f.p.l.cp.Query(ctx, `SELECT id::text FROM public.channels ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	channels := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		channels = append(channels, id)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		t.Fatal(err)
	}
	rows.Close()
	for _, channel := range channels {
		intent := postTestIntent(t, f.p.l, identity)
		intent.ChannelID = channel
		postTestCommand(t, f.p.l, f.p.author.SessionToken, postTestKey(t), intent)
	}
	postTestPublish(t, f.p.l)
	return channels
}
func (f *RecommendationHTTPLab) Rebuild(t *testing.T) {
	t.Helper()
	codec, err := rw.NewCodec("b4b-owned-functional", 1, []rw.Key{f.key})
	if err != nil {
		t.Fatal(err)
	}
	cfg := f.p.config
	cfg.transport = codec
	f.p.service, err = f.p.service.WithRecommendation(context.Background(), cfg, recommendationFeedProjection{})
	if err != nil {
		t.Fatal(err)
	}
}
func (f *RecommendationHTTPLab) Off(t *testing.T, m0 bool) {
	t.Helper()
	copy := *f.p.service
	copy.recommendation = nil
	if m0 {
		mustExec(t, f.p.l.cp, `DROP SCHEMA c_recommendation CASCADE; ALTER TABLE c_posts.post_likes DROP COLUMN liked_at; DROP INDEX c_posts.recommendation_favorite_time,c_posts.recommendation_favorite_post_time,c_comments.recommendation_comment_time`)
	}
	c, err := copy.WithRecommendation(context.Background(), RecommendationConfig{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	f.p.service = c
}
func (f *RecommendationHTTPLab) Advance(t *testing.T, d time.Duration) {
	t.Helper()
	advanceClosureClock(t, f.p.control, f.p.control.clock.now().Add(d))
}
func (f *RecommendationHTTPLab) Hide(t *testing.T, ids []string, hidden bool) {
	t.Helper()
	mustExec(t, f.p.l.cp, `UPDATE c_posts.posts SET moderation_hidden=$2 WHERE post_id=ANY($1::uuid[])`, ids, hidden)
}

type recommendationBadProducer struct{}

func (recommendationBadProducer) ProjectRecommendation(ctx context.Context, tx pgx.Tx, s RecommendationScope, ids []rm.ID) (RecommendationProjection, error) {
	if len(ids) > 0 {
		ids = append(append([]rm.ID{}, ids...), ids[0])
	}
	return (recommendationFeedProjection{}).ProjectRecommendation(ctx, tx, s, ids)
}
func (f *RecommendationHTTPLab) ProducerFault() {
	f.p.service.recommendation.projection = recommendationBadProducer{}
}
func (f *RecommendationHTTPLab) DeletePost(t *testing.T, id string) {
	t.Helper()
	if _, err := f.p.service.ExecutePostCommand(context.Background(), f.p.author.SessionToken, postTestKey(t), posts.Intent{Operation: posts.DeletePost, PostID: id}); err != nil {
		t.Fatal(err)
	}
}
func (f *RecommendationHTTPLab) Exposures(t *testing.T, id string) int {
	t.Helper()
	return count(t, f.p.l.cp, `SELECT count(*) FROM c_recommendation.history WHERE post_id=$1 AND exposure_until IS NOT NULL`, id)
}
func (f *RecommendationHTTPLab) FeedbackFault(t *testing.T, on bool) {
	t.Helper()
	if on {
		mustExec(t, f.p.l.cp, `CREATE FUNCTION c_recommendation.b4b_fault() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic history write failure'; END $$; CREATE TRIGGER b4b_fault BEFORE INSERT OR UPDATE ON c_recommendation.history FOR EACH ROW EXECUTE FUNCTION c_recommendation.b4b_fault()`)
	} else {
		mustExec(t, f.p.l.cp, `DROP TRIGGER b4b_fault ON c_recommendation.history; DROP FUNCTION c_recommendation.b4b_fault()`)
	}
}
func (f *RecommendationHTTPLab) Snapshot(t *testing.T) [32]byte {
	t.Helper()
	h := sha256.New()
	for _, table := range []string{"accounts", "intents", "rounds", "pages", "history", "control"} {
		var raw []byte
		if err := f.p.l.cp.QueryRow(context.Background(), `SELECT COALESCE(jsonb_agg(j ORDER BY j::text),'[]'::jsonb)::text FROM (SELECT to_jsonb(x) j FROM c_recommendation.`+table+` x) s`).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		h.Write(raw)
	}
	var result [32]byte
	copy(result[:], h.Sum(nil))
	return result
}
func (f *RecommendationHTTPLab) ExposureUntil(t *testing.T) time.Time {
	t.Helper()
	var at time.Time
	if err := f.p.l.cp.QueryRow(context.Background(), `SELECT max(exposure_until) FROM c_recommendation.history WHERE account_id=$1`, f.Reader.AccountID).Scan(&at); err != nil {
		t.Fatal(err)
	}
	return at
}
func (f *RecommendationHTTPLab) Rounds(t *testing.T) int {
	t.Helper()
	return count(t, f.p.l.cp, `SELECT count(*) FROM c_recommendation.rounds`)
}
func (f *RecommendationHTTPLab) BeginTrace() { f.p.trace.begin() }
func (f *RecommendationHTTPLab) FinishTrace() []byte {
	b, _ := json.Marshal(f.p.trace.finish())
	return b
}
func (f *RecommendationHTTPLab) Epoch(t *testing.T, account bool) {
	t.Helper()
	if account {
		mustExec(t, f.p.l.cp, `UPDATE c_recommendation.accounts SET epoch=epoch+1,revision=revision+1 WHERE account_id=$1`, f.Reader.AccountID)
	} else {
		mustExec(t, f.p.l.cp, `UPDATE c_recommendation.control SET security_epoch=security_epoch+1`)
	}
}
func (f *RecommendationHTTPLab) Disable(t *testing.T) {
	t.Helper()
	mustExec(t, f.p.l.cp, `UPDATE c_recommendation.accounts SET epoch=epoch+1,revision=revision+1,disabled=true WHERE account_id=$1`, f.Reader.AccountID)
}
func (f *RecommendationHTTPLab) Freeze(t *testing.T) {
	t.Helper()
	if err := f.p.l.gate.Freeze(context.Background(), "B4b explicit test freeze"); err != nil {
		t.Fatal(err)
	}
}
func (f *RecommendationHTTPLab) Revoke(t *testing.T) {
	t.Helper()
	if err := f.p.service.RevokeSession(context.Background(), f.Reader.RevokeSecret); err != nil {
		t.Fatal(err)
	}
}
func (f *RecommendationHTTPLab) ReplaceSession(t *testing.T) [32]byte {
	t.Helper()
	r, err := f.p.service.CreateSession(context.Background(), sessionTestRequest(t, "reco_http_reader"), sessionTestPasswordWorker(t, 1))
	if err != nil {
		t.Fatal(err)
	}
	return r.SessionToken
}
func (f *RecommendationHTTPLab) CloseAccount(t *testing.T) {
	t.Helper()
	request, _ := closureTestRequest(t, f.Reader.SessionToken)
	if _, err := f.p.service.RequestAccountClosure(context.Background(), request, sessionTestPasswordWorker(t, 1)); err != nil {
		t.Fatal(err)
	}
}
