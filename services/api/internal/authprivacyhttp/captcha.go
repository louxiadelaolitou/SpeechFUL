package authprivacyhttp

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/authcaptcha"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/authprivacy/protocol"
)

func captchaAction(service Service, path string) bool {
	if service == CommunityService {
		return false
	}
	return path == "/api/v1/eligibility/otp-requests" || path == "/api/v1/email-login/otp-requests" || path == "/api/v1/email-reset/otp-requests"
}
func captchaPath(path string) bool {
	return path == "/api/v1/captcha/challenges" || path == "/api/v1/captcha/solutions"
}

func (b *boundary) captchaFailure(w http.ResponseWriter, err error) {
	if errors.Is(err, authcaptcha.ErrInvalid) {
		b.fail(w, 422, "CAPTCHA_INVALID", 0)
	} else {
		b.fail(w, 503, "CAPTCHA_UNAVAILABLE", 0)
	}
}

func (b *boundary) captchaPublic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		b.fail(w, 405, "REQUEST_INVALID", 0)
		return
	}
	if b.captcha == nil {
		b.fail(w, 503, "CAPTCHA_UNAVAILABLE", 0)
		return
	}
	if hasHeader(r.Header, "Authorization") || hasHeader(r.Header, "Captcha-Token") {
		b.bad(w, errMalformed)
		return
	}
	if r.URL.Path == "/api/v1/captcha/solutions" {
		m, err := readRequestObject(r, 1024, []string{"id", "x"}, nil)
		if err != nil {
			b.bad(w, err)
			return
		}
		id, e := stringField(m, "id")
		x, ok := m["x"].(json.Number)
		var value float64
		if ok {
			value, err = x.Float64()
		}
		if e != nil || !ok || err != nil || value < 0 || value > 320 {
			b.bad(w, errMalformed)
			return
		}
		token, err := b.captcha.Solve(r.Context(), id, value)
		if err != nil {
			b.captchaFailure(w, err)
			return
		}
		writeJSON(w, 200, map[string]string{"token": token})
		return
	}
	m, err := readRequestObject(r, 2048, []string{"action", "installationId", "requestKey", "subject"}, nil)
	if err != nil {
		b.bad(w, err)
		return
	}
	action, e1 := stringField(m, "action")
	installation, e2 := stringField(m, "installationId")
	key, e3 := stringField(m, "requestKey")
	subject, e4 := stringField(m, "subject")
	_, e5 := protocol.DecodeCanonicalBase64url(installation, 16)
	_, e6 := protocol.DecodeCanonicalBase64url(key, 32)
	validSubject := len(subject) > 0 && len(subject) <= 254 && !strings.ContainsAny(subject, "\x00\r\n")
	if b.service == CommunityService {
		validSubject = privateUsername.MatchString(subject)
	}
	if e1 != nil || e2 != nil || e3 != nil || e4 != nil || e5 != nil || e6 != nil || !validSubject || !captchaAction(b.service, action) {
		b.bad(w, errMalformed)
		return
	}
	challenge, err := b.captcha.Create(r.Context(), authcaptcha.Binding(action, installation, key, subject))
	if err != nil {
		b.captchaFailure(w, err)
		return
	}
	writeJSON(w, 200, challenge)
}

// Enforced at the public HTTP boundary, before password work or mail admission.
// Existing handlers still perform all parsing, idempotency and final Gate checks.
func (b *boundary) requireCaptcha(w http.ResponseWriter, r *http.Request) bool {
	if b.captcha == nil || r.Method != http.MethodPost || !captchaAction(b.service, r.URL.Path) {
		return true
	}
	token, err := singleHeader(r.Header, "Captcha-Token")
	if err != nil || token == "" {
		b.fail(w, 428, "CAPTCHA_REQUIRED", 0)
		return false
	}
	fields := []string{"email"}
	installation := r.Header.Get("V-Installation-ID")
	subjectField := "email"
	if b.service == CommunityService {
		fields = []string{"username", "password", "installationId"}
		subjectField = "username"
	}
	m, err := readRequestObject(r, 8192, fields, nil)
	if err != nil {
		b.bad(w, err)
		return false
	}
	if b.service == CommunityService {
		installation, _ = stringField(m, "installationId")
	}
	subject, e1 := stringField(m, subjectField)
	key, e2 := singleHeader(r.Header, "Idempotency-Key")
	if _, e := protocol.DecodeCanonicalBase64url(installation, 16); e != nil {
		b.bad(w, errMalformed)
		return false
	}
	if _, e := protocol.DecodeCanonicalBase64url(key, 32); e != nil || e1 != nil || e2 != nil {
		b.bad(w, errMalformed)
		return false
	}
	if err = b.captcha.Verify(r.Context(), token, authcaptcha.Binding(r.URL.Path, installation, key, subject)); err != nil {
		b.captchaFailure(w, err)
		return false
	}
	raw, err := json.Marshal(m)
	if err != nil {
		b.bad(w, err)
		return false
	}
	r.Body = io.NopCloser(bytes.NewReader(raw))
	r.ContentLength = int64(len(raw))
	return true
}
