package authprivacyruntime

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5/pgconn"
	"log"
	"log/slog"

	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/authprivacyhttp"
)

var (
	errDatabaseConnection = errors.New("database connection")
	errDatabaseSchema     = errors.New("database schema")
	errDatabasePrivileges = errors.New("database privileges")
	errListenerBind       = errors.New("listener bind")
)

func failureReason(err error) string {
	var pg *pgconn.PgError
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline_exceeded"
	case errors.Is(err, context.Canceled):
		return "cancelled"
	case errors.Is(err, errCommentSnapshotConfiguration):
		return "comment_snapshot_configuration"
	case errors.Is(err, errListenerBind):
		return "listener_bind"
	case errors.As(err, &pg):
		switch pg.Code {
		case "42501", "28000", "28P01":
			return "database_privileges"
		case "42P01", "42703", "3F000":
			return "database_schema"
		}
	}
	switch {
	case errors.Is(err, errDatabasePrivileges):
		return "database_privileges"
	case errors.Is(err, errDatabaseSchema):
		return "database_schema"
	case errors.Is(err, errDatabaseConnection):
		return "database_connection"
	default:
		return "failed"
	}
}

type FailureStage string

const (
	ConfigurationFailure FailureStage = "configuration"
	RuntimeFailure       FailureStage = "runtime"
)

func diagnosticService(service authprivacyhttp.Service) string {
	switch service {
	case authprivacyhttp.CommunityService, authprivacyhttp.VerifierService:
		return string(service)
	default:
		return "unknown"
	}
}

// LogFailure deliberately excludes error text, SQL details, URLs and paths.
// Driver/transport errors can include secrets or attacker-controlled bytes.
// The small fixed vocabulary still identifies the process and failure stage.
func LogFailure(logger *slog.Logger, service authprivacyhttp.Service, stage FailureStage, err error) {
	if err == nil {
		return
	}
	if stage != ConfigurationFailure && stage != RuntimeFailure {
		stage = "unknown"
	}
	reason := failureReason(err)
	logger.Error("authentication process failure", "service", diagnosticService(service), "stage", string(stage), "reason", reason)
}

type httpDiagnosticWriter struct {
	logger  *slog.Logger
	service authprivacyhttp.Service
}

func (w httpDiagnosticWriter) Write(message []byte) (int, error) {
	// net/http's default logger includes peer addresses and panic values. Never
	// parse or forward that message; report only that a transport event occurred.
	w.logger.Warn("HTTP transport event", "service", diagnosticService(w.service))
	return len(message), nil
}

func HTTPErrorLogger(logger *slog.Logger, service authprivacyhttp.Service) *log.Logger {
	return log.New(httpDiagnosticWriter{logger: logger, service: service}, "", 0)
}
