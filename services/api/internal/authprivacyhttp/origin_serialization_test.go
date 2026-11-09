package authprivacyhttp

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBrowserDefaultHTTPSOriginLoadsAdminWithoutTrustingOtherAuthorities(t *testing.T) {
	pki := newHTTPTestPKI(t)
	for _, configured := range []string{"https://speechful.example:443", "https://[::1]:443", "https://speechful.example:8443"} {
		b, err := newBoundary(CommunityService, PeerIdentity{Environment: "lab", Service: VerifierService, Roots: pki.roots}, []string{configured}, httpTestLimits())
		if err != nil {
			t.Fatal(err)
		}
		for _, origin := range []string{configured, "https://speechful.example", "https://[::1]", "https://speechful.example:8443", "http://speechful.example", "https://speechful.example.evil", "https://untrusted.invalid", "null"} {
			want := origin == configured || configured == "https://speechful.example:443" && origin == "https://speechful.example" || configured == "https://[::1]:443" && origin == "https://[::1]"
			called := false
			handler := b.wrap(false, func(w http.ResponseWriter, r *http.Request) { called = true; w.WriteHeader(200) })
			r := httptest.NewRequest("GET", "https://speechful.example/admin/app.mjs", nil)
			r.Header.Set("Origin", origin)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if called != want || want && (w.Code != 200 || w.Header().Get("Access-Control-Allow-Origin") != origin) || !want && w.Code != 403 {
				t.Fatalf("configured=%q origin=%q status=%d handler=%v", configured, origin, w.Code, called)
			}
		}
	}
}
