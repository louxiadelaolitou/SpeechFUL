package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCaptchaFixtureRejectsUnmarkedOrNonVerifierDatabases(t *testing.T) {
	for _, mode := range []string{"isolation", "marker", "role", "database", "remote"} {
		t.Run(mode, func(t *testing.T) {
			directory := t.TempDir()
			tag := "hnuhole_runtime_captcha_fixture"
			dsn := "postgres://hnuhole_v_runtime@127.0.0.1:65534/hnuhole_v?sslmode=disable&application_name=" + tag
			t.Setenv("AUTHRUNTIME_ISOLATED", "1")
			t.Setenv("AUTHRUNTIME_TAG", tag)
			marker := tag
			want := "refusing unmarked verifier captcha fixture"
			switch mode {
			case "isolation":
				t.Setenv("AUTHRUNTIME_ISOLATED", "0")
			case "marker":
				marker = "another-owned-run"
			case "role":
				dsn = strings.Replace(dsn, "hnuhole_v_runtime@", "hnuhole_c_runtime@", 1)
			case "database":
				dsn = strings.Replace(dsn, "/hnuhole_v?", "/hnuhole_c?", 1)
			case "remote":
				dsn = strings.Replace(dsn, "127.0.0.1", "192.0.2.1", 1)
				want = "captcha fixture requires literal loopback"
			}
			t.Setenv("AUTHRUNTIME_V_DSN", dsn)
			if err := os.WriteFile(filepath.Join(directory, ".runtime-tag"), []byte(marker), 0600); err != nil {
				t.Fatal(err)
			}
			p := probe{statePath: filepath.Join(directory, "state.json")}
			pool, err := p.captchaFixturePool(context.Background())
			if pool != nil || err == nil || err.Error() != want {
				t.Fatalf("fixture guard failed before any database connection: %v", err)
			}
		})
	}
}
