package authprivacy

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/posts"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/ratelimit"
	"strings"
	"testing"
	"time"
)

type policyRejectProcessor struct{}

func (policyRejectProcessor) PreparePassword(context.Context, string, string) (PasswordMaterial, error) {
	return PasswordMaterial{}, ErrPasswordPolicy
}

func TestResetPolicyRejectionFencesLateCommitPostgres(t *testing.T) {
	l := newLab(t)
	ctx := context.Background()
	_, code := recoveryTestAccount(t, l, "policy_receipt_owner")
	intent, err := l.c.CreateRecoveryCodeResetIntent(ctx, code)
	if err != nil {
		t.Fatal(err)
	}
	request := recoveryTestRequest(t, intent)
	if err = l.c.CommitPasswordReset(ctx, request, policyRejectProcessor{}); !errors.Is(err, ErrPasswordPolicy) {
		t.Fatal(err)
	}
	state, err := l.c.GetPasswordResetResult(ctx, request.IdempotencyKey, request.IntentID)
	if err != nil || state != "NOT_COMMITTED" {
		t.Fatalf("refusal state=%s err=%v", state, err)
	}
	worker := &fixedRecoveryProcessor{}
	if err = l.c.CommitPasswordReset(ctx, request, worker); !errors.Is(err, ErrResetIntentInvalid) || worker.calls != 0 {
		t.Fatal("late key reopened rejection", err, worker.calls)
	}
	request.IdempotencyKey[0] ^= 1
	if err = l.c.CommitPasswordReset(ctx, request, worker); err != nil {
		t.Fatal("corrected operation cannot reuse active proof", err)
	}
	state, err = l.c.GetPasswordResetResult(ctx, request.IdempotencyKey, request.IntentID)
	if err != nil || state != "COMMITTED" {
		t.Fatal(state, err)
	}
}

func TestSharedRateAdmissionSQLRecoveryBudgetPostgres(t *testing.T) {
	l := newPostLab(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	address := sharedRedisAddress(t)
	engine, cfg, store := sharedEngine(t, l, address, 5, func(c *ratelimit.Config) { c.DeadlineMilliseconds = 100 })
	ns, err := store.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := l.cp.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	var generation int64
	if err = tx.QueryRow(ctx, `SELECT generation FROM c_rate_limit.namespace WHERE singleton_id=1 FOR SHARE`).Scan(&generation); err != nil {
		t.Fatal(err)
	}
	operator := redis.NewClient(&redis.Options{Addr: address, MaxRetries: -1})
	defer operator.Close()
	if err = operator.Del(ctx, "hnuhole:"+cfg.Namespace+":namespace").Err(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, e := engine.Admit(ctx, strings.Repeat("a", 64), []ratelimit.Charge{{Action: "post.create", Cost: 1}}, ratelimit.Proof{})
		done <- e
	}()
	select {
	case e := <-done:
		if !errors.Is(e, ratelimit.ErrLost) {
			t.Fatal(e)
		}
	case <-time.After(350 * time.Millisecond):
		t.Fatal("Recover escaped total admission budget")
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	unchanged, err := store.Load(ctx)
	if err != nil || unchanged.Generation != ns.Generation || unchanged.State != "READY" {
		t.Fatal("cancelled SQL recovery partially committed", unchanged, err)
	}
	// The missing namespace still fails closed and a later attempt can recover.
	if _, err = engine.Admit(ctx, strings.Repeat("a", 64), []ratelimit.Charge{{Action: "post.create", Cost: 1}}, ratelimit.Proof{}); !errors.Is(err, ratelimit.ErrLost) {
		t.Fatal(err)
	}
	next, err := store.Load(ctx)
	if err != nil || next.State != "RECOVERING" || next.Generation != ns.Generation+1 {
		t.Fatal(next, err)
	}
}

func TestFeedSearchContentEditInvalidatesContinuationPostgres(t *testing.T) {
	l := newPostLab(t)
	ctx := context.Background()
	owner := sessionTestAccount(t, l, "search_edit_owner")
	identity := identityChange(t, l.c, owner.SessionToken, "CREATE", uuid.Nil, "搜索编辑").IdentityID
	ids := []string{}
	for _, title := range []string{"needle", "other needle", "body match"} {
		intent := postTestIntent(t, l, identity)
		intent.Title = title
		intent.Body = "needle"
		accepted := postTestCommand(t, l, owner.SessionToken, postTestKey(t), intent)
		postTestPublish(t, l)
		ids = append(ids, accepted.PostID)
	}
	query := posts.SearchQuery{Keyword: "needle", Limit: 1}
	response, err := l.c.SearchPosts(ctx, owner.SessionToken, query)
	if err != nil {
		t.Fatal(err)
	}
	first := response.Value.(posts.FeedPage)
	if first.NextCursor == nil || first.Items[0].PostID != ids[0] {
		t.Fatal(first)
	}
	read, err := l.c.ReadPostEdit(ctx, owner.SessionToken, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	snapshot := read.Value.(posts.EditSnapshot)
	saved, err := l.c.SavePostEdit(ctx, owner.SessionToken, ids[0], postTestKey(t), posts.EditInput{Revision: snapshot.Revision, Title: "changed", Body: "changed"})
	if err != nil || saved.Value.(posts.EditResult).State != "SAVED" {
		t.Fatal(saved, err)
	}
	query.Cursor = *first.NextCursor
	if _, err = l.c.SearchPosts(ctx, owner.SessionToken, query); !errors.Is(err, posts.ErrCursorInvalid) {
		t.Fatal("edited rank accepted an old cursor", err)
	}
	query.Cursor = ""
	query.PostID = ids[0]
	response, err = l.c.SearchPosts(ctx, owner.SessionToken, query)
	if err != nil || len(response.Value.(posts.FeedPage).Items) != 0 {
		t.Fatal("edited membership not removed", response, err)
	}
	query.PostID = ids[1]
	response, err = l.c.SearchPosts(ctx, owner.SessionToken, query)
	if err != nil || len(response.Value.(posts.FeedPage).Items) != 1 {
		t.Fatal("matching membership lost", response, err)
	}
}
