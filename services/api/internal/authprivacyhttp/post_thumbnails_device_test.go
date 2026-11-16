package authprivacyhttp

import (
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/postmedia"
)

// Opt-in bridge for an isolated Android rendering measurement. Only this test's
// private loopback fixture can change its derivative switch; no product route.
func serveThumbnailDeviceFixture(t *testing.T, s *e2eServices, v *postmedia.ProcessValidator, root, token string) {
	t.Helper()
	if !filepath.IsAbs(root) {
		t.Fatal("absolute owned device root required")
	}
	if e := os.MkdirAll(root, 0700); e != nil {
		t.Fatal(e)
	}
	var mu sync.RWMutex
	original := s.cPublic.Config.Handler
	// Dart's TLS stack requires the same ECDSA fixture used by other mobile
	// integration tests; the regular Go-only fixture uses Ed25519.
	config, e := PublicTLSConfig(mobileLabCertificate(t))
	if e != nil {
		t.Fatal(e)
	}
	s.cPublic = e2eServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.RLock()
		defer mu.RUnlock()
		original.ServeHTTP(w, r)
	}), config)
	s.vPublic = e2eServer(t, s.vPublic.Config.Handler, config.Clone())
	control := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Method != "POST" {
			w.WriteHeader(405)
			return
		}
		if r.URL.Path == "/stop" {
			os.WriteFile(filepath.Join(root, "stop"), []byte("stop"), 0600)
			w.WriteHeader(204)
			return
		}
		next, e := s.c.WithPostImages(r.Context(), v)
		if e == nil && r.URL.Path == "/reuse" {
			next, e = next.WithPostThumbnailReuse(r.Context())
		}
		if e != nil || (r.URL.Path != "/reuse" && r.URL.Path != "/baseline") {
			w.WriteHeader(503)
			return
		}
		*s.c = *next
		w.WriteHeader(204)
	}))
	defer control.Close()
	chain := s.cPublic.TLS.Certificates[0].Certificate
	ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: chain[len(chain)-1]})
	raw, e := json.Marshal(map[string]any{"community": s.cPublic.URL, "verifier": s.vPublic.URL, "control": control.URL, "certificatePem": string(ca), "token": token})
	if e != nil || os.WriteFile(filepath.Join(root, "fixture.private.json"), raw, 0600) != nil {
		t.Fatal("fixture export failed")
	}
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	renew := time.NewTicker(time.Minute)
	defer renew.Stop()
	timer := time.NewTimer(30 * time.Minute)
	defer timer.Stop()
	for {
		select {
		case <-timer.C:
			t.Fatal("device fixture timed out")
		case <-renew.C:
			// Publish freshly signed evidence at real time, as in the existing
			// mobile fixtures. No clock offset or Gate result is overridden.
			s.advanceC(0)
			s.advanceV(0)
		case <-ticker.C:
			if _, e = os.Stat(filepath.Join(root, "stop")); e == nil {
				return
			}
		}
	}
}
