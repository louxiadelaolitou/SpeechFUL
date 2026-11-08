package authprivacyruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/authprivacyhttp"
)

func TestProcessDiagnosticsExcludeUntrustedErrors(t *testing.T) {
	const secret = "sentinel-email@example.invalid/password/otp/bearer/recovery/private-key"
	for _, err := range []error{
		errors.New(secret),
		&pgconn.PgError{Message: secret, Detail: secret, Where: secret},
		fmt.Errorf("%s: %w", secret, context.DeadlineExceeded),
		fmt.Errorf("%s: %w", secret, context.Canceled),
		fmt.Errorf("%s: %w", secret, errCommentSnapshotConfiguration),
	} {
		var output bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&output, nil))
		LogFailure(logger, authprivacyhttp.Service(secret), FailureStage(secret), err)
		if strings.Contains(output.String(), secret) {
			t.Fatal("diagnostic exposed error or caller-provided labels")
		}
		var event map[string]any
		if err := json.Unmarshal(output.Bytes(), &event); err != nil {
			t.Fatal(err)
		}
		if event["service"] != "unknown" || event["stage"] != "unknown" || len(event) != 6 {
			t.Fatalf("unexpected diagnostic fields: %v", event)
		}
	}
}

func TestProcessDiagnosticsStableCategories(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{errors.Join(errDatabaseConnection, errors.New("secret endpoint")), "database_connection"},
		{errors.Join(errDatabaseSchema, errors.New("private table")), "database_schema"},
		{errors.Join(errDatabasePrivileges, errors.New("private role")), "database_privileges"},
		{errors.Join(errListenerBind, errors.New("private address")), "listener_bind"},
		{fmt.Errorf("secret: %w", &pgconn.PgError{Code: "42501", Message: "secret"}), "database_privileges"},
		{fmt.Errorf("secret: %w", &pgconn.PgError{Code: "42P01", Message: "secret"}), "database_schema"},
	} {
		if got := failureReason(tc.err); got != tc.want {
			t.Fatalf("got %s want %s", got, tc.want)
		}
	}
}

func TestHTTPPanicDiagnosticExcludesRequestAndPanicMaterial(t *testing.T) {
	const secret = "sentinel@example.invalid-secret-capability"
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic(secret)
	}))
	server.Config.ErrorLog = HTTPErrorLogger(logger, authprivacyhttp.CommunityService)
	server.Start()
	defer server.Close()
	request, _ := http.NewRequest(http.MethodGet, server.URL+"/"+secret, nil)
	request.Header.Set("Authorization", "Bearer "+secret)
	response, err := server.Client().Do(request)
	if response != nil {
		response.Body.Close()
	}
	// Close waits for the panic handler and its diagnostic writer to finish.
	// Reading the plain test buffer before that wait races with net/http.
	server.Close()
	if err == nil {
		t.Fatal("panic did not close the HTTP connection")
	}
	if strings.Contains(output.String(), secret) || strings.Contains(output.String(), "127.0.0.1") || strings.Contains(output.String(), "goroutine") {
		t.Fatal("HTTP diagnostics exposed panic, request, peer or stack")
	}
	if !strings.Contains(output.String(), "HTTP transport event") {
		t.Fatal("transport failure was not recorded")
	}
}
