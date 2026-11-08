package authprivacyhttp

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/authprivacy"
)

type resetHTTPBackend struct {
	*httpTestCommunity
	*httpTestRecoveryClosures
	last authprivacy.EmailResetIntentRequest
}

func (f *resetHTTPBackend) LegacyRecoveryPaused() bool { return true }
func (f *resetHTTPBackend) CreateEmailResetIntent(_ context.Context, r authprivacy.EmailResetIntentRequest) (authprivacy.PasswordResetIntent, error) {
	f.last = r
	return authprivacy.PasswordResetIntent{ID: [32]byte{6}, ExpiresAt: time.Now().Add(time.Minute), Username: "private_user"}, nil
}
func TestEmailResetHTTPStrictIntentAndNoSession(t *testing.T) {
	community, _, pki := newHTTPTestCommunity(t)
	backend := &resetHTTPBackend{httpTestCommunity: community, httpTestRecoveryClosures: &httpTestRecoveryClosures{}}
	endpoints, err := NewCommunityEndpoints(CommunityOptions{Backend: backend, Recovery: backend, Passwords: httpTestPasswords{backend: community}, InternalPeer: PeerIdentity{Environment: "lab", Service: VerifierService, Roots: pki.roots}, Limits: httpTestLimits()})
	if err != nil {
		t.Fatal(err)
	}
	server, client := httpTestPublicServer(t, endpoints.Public, pki, CommunityService)
	path := "/api/v1/auth/email-reset-intents"
	headers := map[string][]string{"Content-Type": {"application/json"}, "Idempotency-Key": {httpTestEncoding(32, 7)}}
	body := `{"proof":"` + httpTestEncoding(227, 4) + `","installationId":"` + httpTestEncoding(16, 5) + `"}`
	response, result, _ := doHTTPTest(t, client, "POST", server.URL+path, body, headers)
	if response.StatusCode != 201 || len(result) != 3 || result["username"] != "private_user" || result["resetIntentId"] == nil || result["expiresAt"] == nil || response.Header.Get("Session-Expires-At") != "" {
		t.Fatal("unexpected reset authority", response.StatusCode, result)
	}
	if backend.last.InstallationID == ([16]byte{}) || backend.last.IdempotencyKey == ([32]byte{}) {
		t.Fatal("lost reset binding")
	}
	for _, invalid := range []string{`{"proof":null,"installationId":"` + httpTestEncoding(16, 5) + `"}`, body[:len(body)-1] + `,"email":"private@hainanu.edu.cn"}`} {
		response, _, _ = doHTTPTest(t, client, "POST", server.URL+path, invalid, headers)
		if response.StatusCode != 400 {
			t.Fatal("accepted private or invalid field", response.StatusCode)
		}
	}
	headers["Authorization"] = []string{"Bearer " + httpTestEncoding(32, 8)}
	response, _, _ = doHTTPTest(t, client, "POST", server.URL+path, body, headers)
	if response.StatusCode != 400 {
		t.Fatal("accepted bearer", response.StatusCode)
	}
	delete(headers, "Authorization")
	response, _, _ = doHTTPTest(t, client, "POST", server.URL+"/api/v1/auth/recovery-code-reset-intents", `{"recoveryCode":"`+httpTestCode()+`"}`, headers)
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatal("legacy recovery open", response.StatusCode)
	}
}
