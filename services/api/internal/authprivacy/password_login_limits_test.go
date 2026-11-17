package authprivacy

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type rejectedLoginVerifier struct{}

func (rejectedLoginVerifier) VerifyPassword(context.Context, string, PasswordMaterial) (bool, error) {
	return false, nil
}

func TestPasswordLoginFreezePostgres(t *testing.T) {
	for _, known := range []bool{true, false} {
		t.Run(map[bool]string{true: "existing", false: "unknown"}[known], func(t *testing.T) {
			l := newLab(t)
			ctx := context.Background()
			username := "password_freeze_user"
			worker := sessionTestPasswordWorker(t, 2)
			subject := l.c.passwordLoginSubject(username)
			if known {
				sessionTestAccount(t, l, username)
				for i := 0; i < 2; i++ {
					if _, err := l.c.CreateSession(ctx, sessionTestRequest(t, username), rejectedLoginVerifier{}); !errors.Is(err, ErrAuthenticationFailed) {
						t.Fatal(err)
					}
				}
				if _, err := l.c.CreateSession(ctx, sessionTestRequest(t, username), worker); err != nil {
					t.Fatal("valid password did not clear failures", err)
				}
				if count(t, l.cp, `SELECT failures FROM c_auth.password_login_limits WHERE subject_digest=$1`, subject[:]) != 0 {
					t.Fatal("success retained failure count")
				}
			}
			for i := 1; i <= 5; i++ {
				request := sessionTestRequest(t, username)
				request.Password = "a deliberately incorrect password"
				_, err := l.c.CreateSession(ctx, request, worker)
				var locked *PasswordLoginLocked
				if i < 5 && !errors.Is(err, ErrAuthenticationFailed) || i == 5 && (!errors.As(err, &locked) || locked.RetryAfterSeconds != 300) {
					t.Fatalf("attempt %d returned %v", i, err)
				}
				if i == 1 {
					for retry := 0; retry < 3; retry++ {
						if _, err := l.c.CreateSession(ctx, request, worker); !errors.Is(err, ErrAuthenticationFailed) {
							t.Fatal("same-key failure changed", err)
						}
					}
				}
				if count(t, l.cp, `SELECT failures FROM c_auth.password_login_limits WHERE subject_digest=$1`, subject[:]) != i {
					t.Fatal("wrong failure count", i)
				}
			}
			var before, after time.Time
			if err := l.cp.QueryRow(ctx, `SELECT locked_until FROM c_auth.password_login_limits WHERE subject_digest=$1`, subject[:]).Scan(&before); err != nil {
				t.Fatal(err)
			}
			// Different installations and a newly constructed service share SQL state.
			otherInstance := *l.c
			for i := 0; i < 3; i++ {
				var locked *PasswordLoginLocked
				if _, err := otherInstance.CreateSession(ctx, sessionTestRequest(t, username), worker); !errors.As(err, &locked) || locked.RetryAfterSeconds < 295 || locked.RetryAfterSeconds > 300 {
					t.Fatal("freeze bypassed or duration changed", err)
				}
			}
			if err := l.cp.QueryRow(ctx, `SELECT locked_until FROM c_auth.password_login_limits WHERE subject_digest=$1`, subject[:]).Scan(&after); err != nil || !before.Equal(after) {
				t.Fatal("retry extended freeze", err)
			}
			mustExec(t, l.cp, `UPDATE c_auth.password_login_limits SET locked_until=clock_timestamp()-interval '1 second' WHERE subject_digest=$1`, subject[:])
			_, err := l.c.CreateSession(ctx, sessionTestRequest(t, username), worker)
			if known && err != nil || !known && !errors.Is(err, ErrAuthenticationFailed) {
				t.Fatal("expired freeze did not permit a fresh attempt", err)
			}
			want := 1
			if known {
				want = 0
			}
			if count(t, l.cp, `SELECT failures FROM c_auth.password_login_limits WHERE subject_digest=$1`, subject[:]) != want {
				t.Fatal("expiry did not reset count")
			}
		})
	}
}

func TestPasswordLoginConcurrentFreezePostgres(t *testing.T) {
	l := newLab(t)
	ctx := context.Background()
	username := "parallel_freeze_user"
	subject := l.c.passwordLoginSubject(username)
	requests := make([]SessionCreateRequest, 8)
	for i := range requests {
		requests[i] = sessionTestRequest(t, username)
	}
	var ordinary, locked atomic.Int32
	var wg sync.WaitGroup
	for _, request := range requests {
		wg.Add(1)
		go func(request SessionCreateRequest) {
			defer wg.Done()
			_, err := l.c.CreateSession(ctx, request, rejectedLoginVerifier{})
			var freeze *PasswordLoginLocked
			if errors.As(err, &freeze) {
				locked.Add(1)
			} else if errors.Is(err, ErrAuthenticationFailed) {
				ordinary.Add(1)
			} else {
				t.Error(err)
			}
		}(request)
	}
	wg.Wait()
	if ordinary.Load() != 4 || locked.Load() != 4 || count(t, l.cp, `SELECT failures FROM c_auth.password_login_limits WHERE subject_digest=$1`, subject[:]) != 5 {
		t.Fatal("concurrent failures bypassed fifth-attempt freeze", ordinary.Load(), locked.Load())
	}
}

func TestPasswordLoginInFlightSuccessCannotCrossFreezePostgres(t *testing.T) {
	l := newLab(t)
	ctx := context.Background()
	username := "inflight_password_freeze"
	sessionTestAccount(t, l, username)
	worker := &pausingPasswordVerifier{inner: sessionTestPasswordWorker(t, 1), ready: make(chan struct{}), resume: make(chan struct{})}
	request := sessionTestRequest(t, username)
	done := make(chan error, 1)
	go func() { _, err := l.c.CreateSession(ctx, request, worker); done <- err }()
	<-worker.ready
	for i := 0; i < 5; i++ {
		_, _ = l.c.CreateSession(ctx, sessionTestRequest(t, username), rejectedLoginVerifier{})
	}
	close(worker.resume)
	var locked *PasswordLoginLocked
	if err := <-done; !errors.As(err, &locked) {
		t.Fatal("paused password proof crossed freeze", err)
	}
}
