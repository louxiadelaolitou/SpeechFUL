package authprivacyhttp

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/authcaptcha"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/authprivacy/protocol"
)

func TestCaptchaHTTPSPostgres(t *testing.T) {
	for _, party := range []string{"v"} {
		t.Run(party, func(t *testing.T) {
			pool := e2ePool(t, party)
			schema := party + "_auth"
			service := CommunityService
			actions := []string{"/api/v1/auth/sessions"}
			if party == "v" {
				service = VerifierService
				actions = []string{"/api/v1/eligibility/otp-requests", "/api/v1/email-login/otp-requests", "/api/v1/email-reset/otp-requests"}
			}
			pki := newHTTPTestPKI(t)
			peer := VerifierService
			if party == "v" {
				peer = CommunityService
			}
			b, err := newBoundary(service, PeerIdentity{Environment: "lab", Service: peer, Roots: pki.roots}, []string{"https://app.example"}, httpTestLimits())
			if err != nil {
				t.Fatal(err)
			}
			b.captcha = authcaptcha.NewSQL(pool, schema)
			var calls int
			server, client := httpTestPublicServer(t, b.wrap(false, func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(204) }), pki, service)
			installation, key := httpTestEncoding(16, 31), httpTestEncoding(32, 32)
			for _, action := range actions {
				subject := "private_user"
				business := `{"username":"private_user","password":"IndependentPassword123","installationId":"` + installation + `"}`
				headers := map[string][]string{"Content-Type": {"application/json"}, "Idempotency-Key": {key}}
				if party == "v" {
					subject = "test@hainanu.edu.cn"
					business = `{"email":"test@hainanu.edu.cn"}`
					headers["V-Installation-ID"] = []string{installation}
				}
				response, _, _ := doHTTPTest(t, client, "POST", server.URL+action, business, headers)
				if response.StatusCode != 428 {
					t.Fatal("missing proof admitted")
				}
				raw, _ := json.Marshal(map[string]string{"action": action, "installationId": installation, "requestKey": key, "subject": subject})
				response, m, _ := doHTTPTest(t, client, "POST", server.URL+"/api/v1/captcha/challenges", string(raw), map[string][]string{"Content-Type": {"application/json"}})
				if response.StatusCode != 200 {
					t.Fatalf("challenge: %d", response.StatusCode)
				}
				if _, ok := m["x"]; ok {
					t.Fatal("answer disclosed")
				}
				id := m["id"].(string)
				decoded, _ := protocol.DecodeCanonicalBase64url(id, 32)
				hash := sha256.Sum256(decoded)
				// Test-only SQL oracle. No answer/oracle route exists in the product.
				var answer int
				if err := pool.QueryRow(context.Background(), "SELECT answer_x FROM "+schema+".captcha_challenges WHERE id=$1", hash[:]).Scan(&answer); err != nil {
					t.Fatal(err)
				}
				raw, _ = json.Marshal(map[string]any{"id": id, "x": answer})
				response, m, _ = doHTTPTest(t, client, "POST", server.URL+"/api/v1/captcha/solutions", string(raw), map[string][]string{"Content-Type": {"application/json"}})
				if response.StatusCode != 200 {
					t.Fatalf("solution: %d", response.StatusCode)
				}
				token := m["token"].(string)
				headers["Captcha-Token"] = []string{token}
				before := calls
				response, _, _ = doHTTPTest(t, client, "POST", server.URL+action, business, headers)
				if response.StatusCode != 204 || calls != before+1 {
					t.Fatal("valid proof failed to reach business handler")
				}
				response, _, _ = doHTTPTest(t, client, "POST", server.URL+action, business, headers)
				if response.StatusCode != 422 || calls != before+1 {
					t.Fatal("replayed proof admitted")
				}
				// Invalid purpose cannot request a challenge on the other privacy boundary.
				badAction := "/api/v1/email-login/otp-requests"
				if party == "v" {
					badAction = "/api/v1/auth/sessions"
				}
				raw, _ = json.Marshal(map[string]string{"action": badAction, "installationId": installation, "requestKey": key, "subject": subject})
				response, _, _ = doHTTPTest(t, client, "POST", server.URL+"/api/v1/captcha/challenges", string(raw), map[string][]string{"Content-Type": {"application/json"}})
				if response.StatusCode != 400 {
					t.Fatal("cross-boundary challenge issued")
				}
			}
			// PostgreSQL DELETE RETURNING gives exactly one winner across service instances.
			binding := authcaptcha.Binding(actions[0], installation, key, "concurrent")
			proof := httpTestEncoding(32, 81)
			decoded, _ := protocol.DecodeCanonicalBase64url(proof, 32)
			hash := sha256.Sum256(decoded)
			store := authcaptcha.NewSQL(pool, schema)
			if _, err := pool.Exec(context.Background(), "INSERT INTO "+schema+".captcha_proofs VALUES($1,$2,clock_timestamp()+interval '1 minute')", hash[:], binding[:]); err != nil {
				t.Fatal(err)
			}
			var successes atomic.Int32
			var wg sync.WaitGroup
			for i := 0; i < 8; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					if err := store.Verify(context.Background(), proof, binding); err == nil {
						successes.Add(1)
					} else if !errors.Is(err, authcaptcha.ErrInvalid) {
						t.Error(err)
					}
				}()
			}
			wg.Wait()
			if successes.Load() != 1 {
				t.Fatal("proof had multiple consumers")
			}
			// Database time is authoritative even if an answering process clock drifts.
			for _, kind := range []string{"challenge", "proof"} {
				expired := httpTestEncoding(32, 82)
				raw, _ := protocol.DecodeCanonicalBase64url(expired, 32)
				digest := sha256.Sum256(raw)
				query := "INSERT INTO " + schema + ".captcha_proofs VALUES($1,$2,clock_timestamp()-interval '1 second')"
				if kind == "challenge" {
					query = "INSERT INTO " + schema + ".captcha_challenges VALUES($1,$2,100,clock_timestamp()-interval '1 second')"
				}
				if _, err := pool.Exec(context.Background(), query, digest[:], binding[:]); err != nil {
					t.Fatal(err)
				}
				if kind == "challenge" {
					if _, err := store.Solve(context.Background(), expired, 100); !errors.Is(err, authcaptcha.ErrInvalid) {
						t.Fatal("expired challenge accepted", err)
					}
				} else if err := store.Verify(context.Background(), expired, binding); !errors.Is(err, authcaptcha.ErrInvalid) {
					t.Fatal("expired proof accepted", err)
				}
			}
			// A token header is allowed only for documented protected POST preflights.
			r := httptest.NewRequest("OPTIONS", actions[0], strings.NewReader(""))
			r.Header.Set("Origin", "https://app.example")
			r.Header.Set("Access-Control-Request-Method", "POST")
			r.Header.Set("Access-Control-Request-Headers", "captcha-token")
			w := httptest.NewRecorder()
			b.preflight(w, r)
			if w.Code != 204 {
				t.Fatal("captcha CORS blocked")
			}
		})
	}
}
