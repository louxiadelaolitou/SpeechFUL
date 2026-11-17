package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/authprivacy/protocol"
)

func (p *probe) captchaFixturePool(ctx context.Context) (*pgxpool.Pool, error) {
	tag := os.Getenv("AUTHRUNTIME_TAG")
	config, err := pgxpool.ParseConfig(os.Getenv("AUTHRUNTIME_V_DSN"))
	marker, markerErr := os.ReadFile(filepath.Join(filepath.Dir(p.statePath), ".runtime-tag"))
	if os.Getenv("AUTHRUNTIME_ISOLATED") != "1" || err != nil || markerErr != nil ||
		!strings.HasPrefix(tag, "hnuhole_runtime_") || len(tag) < 24 || strings.TrimSpace(string(marker)) != tag ||
		config.ConnConfig.RuntimeParams["application_name"] != tag || config.ConnConfig.Database != "hnuhole_v" || config.ConnConfig.User != "hnuhole_v_runtime" {
		return nil, errors.New("refusing unmarked verifier captcha fixture")
	}
	hosts := []string{config.ConnConfig.Host}
	for _, fallback := range config.ConnConfig.Fallbacks {
		hosts = append(hosts, fallback.Host)
	}
	for _, host := range hosts {
		if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
			return nil, errors.New("captcha fixture requires literal loopback")
		}
	}
	config.ConnConfig.Password = os.Getenv("AUTHRUNTIME_V_PASSWORD")
	config.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, errors.New("cannot open verifier captcha fixture")
	}
	var restricted bool
	err = pool.QueryRow(ctx, `SELECT current_database()='hnuhole_v' AND current_user='hnuhole_v_runtime'
 AND NOT (rolsuper OR rolcreatedb OR rolcreaterole OR rolreplication OR rolbypassrls)
 AND to_regnamespace('c_auth') IS NULL
 AND NOT EXISTS(SELECT 1 FROM pg_class WHERE relnamespace='v_auth'::regnamespace AND relowner=(SELECT oid FROM pg_roles WHERE rolname=current_user))
 FROM pg_roles WHERE rolname=current_user`).Scan(&restricted)
	if err != nil || !restricted {
		pool.Close()
		return nil, errors.New("captcha fixture is not a restricted verifier runtime")
	}
	return pool, nil
}

// The isolated operator reads only its own just-issued puzzle answer through
// the restricted V role. All challenge, solution and mail traffic still uses
// the real public HTTPS APIs. No product answer endpoint or Gate bypass exists.
func (p *probe) captcha(ctx context.Context, action, email string, headers map[string]string) (string, error) {
	pool, err := p.captchaFixturePool(ctx)
	if err != nil {
		return "", err
	}
	defer pool.Close()
	challenge, _, err := p.request(ctx, "POST", p.v, "/api/v1/captcha/challenges", map[string]string{
		"action": action, "installationId": headers["V-Installation-ID"], "requestKey": headers["Idempotency-Key"], "subject": email,
	}, nil, 200)
	if err != nil {
		return "", err
	}
	id, err := field(challenge, "id")
	if err != nil {
		return "", err
	}
	raw, err := protocol.DecodeCanonicalBase64url(id, 32)
	if err != nil {
		return "", errors.New("invalid probe challenge identifier")
	}
	digest := sha256.Sum256(raw)
	var answer int
	if err = pool.QueryRow(ctx, `SELECT answer_x FROM v_auth.captcha_challenges WHERE id=$1 AND expires_at>clock_timestamp()`, digest[:]).Scan(&answer); err != nil {
		return "", errors.New("own probe challenge answer unavailable")
	}
	proof, _, err := p.request(ctx, "POST", p.v, "/api/v1/captcha/solutions", map[string]any{"id": id, "x": answer}, nil, 200)
	if err != nil {
		return "", err
	}
	return field(proof, "token")
}
