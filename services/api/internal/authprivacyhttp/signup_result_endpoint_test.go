package authprivacyhttp

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
)

type httpTestSignupResults struct {
	*httpTestCommunity
	state   string
	key, id [32]byte
	calls   int
}

func (f *httpTestSignupResults) GetSignupResult(_ context.Context, key, id [32]byte) (string, error) {
	f.key, f.id = key, id
	f.calls++
	return f.state, nil
}

func TestSignupResultEndpoint(t *testing.T) {
	for _, state := range []string{"COMMITTED", "NOT_COMMITTED", "PENDING"} {
		t.Run(state, func(t *testing.T) {
			base, _, pki := newHTTPTestCommunity(t)
			backend := &httpTestSignupResults{httpTestCommunity: base, state: state}
			endpoints, err := NewCommunityEndpoints(CommunityOptions{Backend: backend, Passwords: httpTestPasswords{backend: base}, InternalPeer: PeerIdentity{Environment: "lab", Service: VerifierService, Roots: pki.roots}, Limits: httpTestLimits()})
			if err != nil {
				t.Fatal(err)
			}
			for _, scheme := range []string{"Bearer", "SignupResult"} {
				r := httptest.NewRequest("GET", "https://community.lab/api/v1/auth/registration-result", nil)
				r.Header.Set("Authorization", scheme+" "+httpTestEncoding(32, 7))
				r.Header.Set("Signup-Intent-ID", httpTestEncoding(32, 8))
				w := httptest.NewRecorder()
				endpoints.Public.ServeHTTP(w, r)
				if scheme == "Bearer" {
					if w.Code != 401 || backend.calls != 0 {
						t.Fatal("session capability admitted as signup result", w.Code)
					}
					continue
				}
				want := 200
				if state == "PENDING" {
					want = 202
				}
				if w.Code != want || strings.TrimSpace(w.Body.String()) != `{"state":"`+state+`"}` || w.Header().Get("Cache-Control") != "no-store" {
					t.Fatal("unexpected result envelope", w.Code, w.Body.String())
				}
				for i := 0; i < 32; i++ {
					if backend.key[i] != 7 || backend.id[i] != 8 {
						t.Fatal("original capability changed")
					}
				}
			}
		})
	}
}

func TestSignupResultRejectsMixedCapabilitiesAndBody(t *testing.T) {
	base, _, pki := newHTTPTestCommunity(t)
	backend := &httpTestSignupResults{httpTestCommunity: base, state: "COMMITTED"}
	endpoints, err := NewCommunityEndpoints(CommunityOptions{Backend: backend, Passwords: httpTestPasswords{backend: base}, InternalPeer: PeerIdentity{Environment: "lab", Service: VerifierService, Roots: pki.roots}, Limits: httpTestLimits()})
	if err != nil {
		t.Fatal(err)
	}
	for _, extra := range []string{"Reset-Intent-ID", "Credential-Change-ID", "Idempotency-Key", "V-Installation-ID", "body"} {
		t.Run(extra, func(t *testing.T) {
			body := ""
			if extra == "body" {
				body = "{}"
			}
			r := httptest.NewRequest("GET", "https://community.lab/api/v1/auth/registration-result", strings.NewReader(body))
			r.Header.Set("Authorization", "SignupResult "+httpTestEncoding(32, 7))
			r.Header.Set("Signup-Intent-ID", httpTestEncoding(32, 8))
			if extra != "body" {
				r.Header.Set(extra, httpTestEncoding(32, 9))
			}
			w := httptest.NewRecorder()
			endpoints.Public.ServeHTTP(w, r)
			if w.Code != 400 || backend.calls != 0 {
				t.Fatal("mixed result capability reached backend", w.Code)
			}
		})
	}
}

func TestSignupResultPreflightKeepsHeaderScopedToOriginalResult(t *testing.T) {
	base, _, pki := newHTTPTestCommunity(t)
	backend := &httpTestSignupResults{httpTestCommunity: base, state: "COMMITTED"}
	endpoints, err := NewCommunityEndpoints(CommunityOptions{Backend: backend, Passwords: httpTestPasswords{backend: base}, InternalPeer: PeerIdentity{Environment: "lab", Service: VerifierService, Roots: pki.roots}, AllowedOrigins: []string{"https://client.lab"}, Limits: httpTestLimits()})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path, method string
		status       int
	}{
		{"/api/v1/auth/registration-result", "GET", 204},
		{"/api/v1/auth/registration-result", "POST", 400},
		{"/api/v1/auth/session", "GET", 400},
	} {
		r := httptest.NewRequest("OPTIONS", "https://community.lab"+tc.path, nil)
		r.Header.Set("Origin", "https://client.lab")
		r.Header.Set("Access-Control-Request-Method", tc.method)
		r.Header.Set("Access-Control-Request-Headers", "authorization, signup-intent-id")
		w := httptest.NewRecorder()
		endpoints.Public.ServeHTTP(w, r)
		if w.Code != tc.status || backend.calls != 0 {
			t.Fatalf("unexpected preflight for %s %s: %d", tc.method, tc.path, w.Code)
		}
		if tc.status == 204 && w.Header().Get("Access-Control-Allow-Headers") != "authorization, signup-intent-id" {
			t.Fatal("missing scoped result header")
		}
	}
}
