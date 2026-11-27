package authprivacyhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/authprivacy"
	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/posts"
	rm "github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/recommendation/roundmodel"
	rw "github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/recommendation/transport"
)

type RecommendationBackend interface {
	StartRecommendationPublic(context.Context, [32]byte, rw.Start) (authprivacy.PostResponse, error)
	ReadRecommendationPublic(context.Context, [32]byte, string) (authprivacy.PostResponse, error)
	FeedbackRecommendationPublic(context.Context, [32]byte, rw.Feedback) (authprivacy.PostResponse, error)
}

const recommendationPrefix = "/api/v1/feed/recommendation"

func isRecommendationPath(path string) bool {
	return path == recommendationPrefix || strings.HasPrefix(path, recommendationPrefix+"/")
}
func recommendationOperation(path string) string {
	for _, operation := range []string{"start", "page", "feedback"} {
		if path == recommendationPrefix+"/"+operation {
			return operation
		}
	}
	return ""
}

func (b *boundary) communityRecommendations(w http.ResponseWriter, r *http.Request, backend RecommendationBackend) {
	operation := recommendationOperation(r.URL.Path)
	if operation == "" || r.URL.RawPath != "" {
		b.postBad(w, errMalformed)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		b.postFail(w, 405, "MALFORMED_REQUEST", 0)
		return
	}
	for _, header := range []string{"Idempotency-Key", "V-Installation-ID", "OTP-Flow-ID", "Reset-Intent-ID", "Credential-Change-ID", "Content-Encoding", "X-Post-Preview"} {
		if hasHeader(r.Header, header) {
			b.postBad(w, errMalformed)
			return
		}
	}
	bearer, err := sessionCapability(r.Header, "Bearer")
	if err != nil {
		if errors.Is(err, errMalformed) {
			b.postBad(w, err)
		} else {
			b.postFail(w, 401, "AUTHENTICATION_FAILED", 0)
		}
		return
	}
	if backend == nil {
		b.postFail(w, 503, "SERVICE_UNAVAILABLE", 0)
		return
	}
	var response authprivacy.PostResponse
	switch operation {
	case "start":
		var object map[string]any
		object, err = readRequestObject(r, rw.MaxBody, []string{"version", "nonce", "createdAt", "context"}, []string{"channelId"})
		if err != nil {
			break
		}
		var input rw.Start
		var version int
		version, err = integerField(object, "version", 1, 1)
		input.Version = version
		if err != nil {
			break
		}
		input.Nonce, _ = object["nonce"].(string)
		input.Context, _ = object["context"].(string)
		created, ok := object["createdAt"].(string)
		if !ok {
			err = errMalformed
			break
		}
		input.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
		if err != nil || !rw.ValidTime(input.CreatedAt) || input.CreatedAt.Format(time.RFC3339Nano) != created || !rw.ValidID(input.Nonce) || !rw.ValidToken(input.Context, rw.MaxContext) {
			err = errMalformed
			break
		}
		if value, exists := object["channelId"]; exists {
			input.ChannelID, ok = value.(string)
			if !ok || !rw.ValidID(input.ChannelID) {
				err = errMalformed
				break
			}
		}
		response, err = backend.StartRecommendationPublic(r.Context(), bearer, input)
	case "page":
		var object map[string]any
		object, err = readRequestObject(r, rw.MaxBody, []string{"cursor"}, nil)
		if err != nil {
			break
		}
		cursor, ok := object["cursor"].(string)
		if !ok || !rw.ValidToken(cursor, rw.MaxReference) {
			err = errMalformed
			break
		}
		response, err = backend.ReadRecommendationPublic(r.Context(), bearer, cursor)
	case "feedback":
		var object map[string]any
		object, err = readRequestObject(r, rw.MaxBody, []string{"receipt", "postIds"}, nil)
		if err != nil {
			break
		}
		input := rw.Feedback{PostIDs: []string{}}
		input.Receipt, _ = object["receipt"].(string)
		ids, ok := object["postIds"].([]any)
		if !ok {
			err = errMalformed
			break
		}
		for _, value := range ids {
			id, ok := value.(string)
			if !ok {
				err = errMalformed
				break
			}
			input.PostIDs = append(input.PostIDs, id)
		}
		if err != nil {
			break
		}
		if !rw.ValidFeedback(input) {
			err = errMalformed
			break
		}
		response, err = backend.FeedbackRecommendationPublic(r.Context(), bearer, input)
	}
	if err != nil {
		b.recommendationFailure(w, err)
		return
	}
	if !validPostTime(response.ServerTime) || !validPostTime(response.ExpiresAt) || !response.ExpiresAt.After(response.ServerTime) {
		b.postFail(w, 503, "SERVICE_UNAVAILABLE", 0)
		return
	}
	var value any
	if operation == "feedback" {
		ack, ok := response.Value.(rw.ACK)
		if !ok || !ack.Accepted {
			b.postFail(w, 503, "SERVICE_UNAVAILABLE", 0)
			return
		}
		value = ack
	} else {
		page, ok := response.Value.(rw.Page)
		if !ok || page.Version != rw.Version || page.Items == nil || len(page.Items) > 10 || !rw.ValidTime(page.ExpiresAt) || !rw.ValidTime(page.FeedbackUntil) || !page.ExpiresAt.After(response.ServerTime) || !validNullablePostCursor(page.NextCursor) || !validNullablePostCursor(page.FeedbackReceipt) {
			b.postFail(w, 503, "SERVICE_UNAVAILABLE", 0)
			return
		}
		if page.FeedbackReceipt != nil && !response.ServerTime.Before(page.FeedbackUntil) {
			b.postFail(w, 503, "SERVICE_UNAVAILABLE", 0)
			return
		}
		checked, e := strictPostValue(posts.CrossChannelFeedPage{Items: page.Items}, postRoute{kind: authprivacy.PostFeed, limit: 10, preview: true})
		if e != nil {
			b.postFail(w, 503, "SERVICE_UNAVAILABLE", 0)
			return
		}
		page.Items = checked.(posts.CrossChannelFeedPage).Items
		value = page
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		b.postFail(w, 503, "SERVICE_UNAVAILABLE", 0)
		return
	}
	w.Header().Set("Session-Expires-At", utc(response.ExpiresAt))
	w.Header().Set("Server-Time", utc(response.ServerTime))
	w.WriteHeader(200)
	_, _ = w.Write(append(encoded, '\n'))
}

func (b *boundary) recommendationFailure(w http.ResponseWriter, err error) {
	var constraint *pgconn.PgError
	if errors.As(err, &constraint) && constraint.Code == "23514" && constraint.TableName == "control" && (constraint.ConstraintName == "control_check" || constraint.ConstraintName == "control_check1" || constraint.ConstraintName == "control_check2") {
		b.postFail(w, 429, "RATE_LIMITED", 1)
		return
	}
	switch {
	case errors.Is(err, rw.ErrInvalid), errors.Is(err, rm.ErrInvalid):
		b.postFail(w, 400, "RECOMMENDATION_TOKEN_INVALID", 0)
	case errors.Is(err, rw.ErrExpired), errors.Is(err, rm.ErrExpired):
		b.postFail(w, 409, "RECOMMENDATION_EXPIRED", 0)
	case errors.Is(err, rw.ErrScope), errors.Is(err, rm.ErrScope):
		b.postFail(w, 409, "RECOMMENDATION_SCOPE_CHANGED", 0)
	case errors.Is(err, rm.ErrCapacity):
		b.postFail(w, 429, "RATE_LIMITED", 1)
	case errors.Is(err, rw.ErrUnavailable):
		b.postFail(w, 503, "SERVICE_UNAVAILABLE", 0)
	case errors.Is(err, errMalformed), errors.Is(err, errTooLarge), errors.Is(err, errMediaType):
		b.postBad(w, err)
	default:
		b.postFailure(w, err)
	}
}

const recommendationExposeHeaders = "Recommendation-Version, Recommendation-Available, Recommendation-Context, Recommendation-Issued-At, Recommendation-Create-Before"

func writeRecommendationHeaders(w http.ResponseWriter, capability *rw.Capability) error {
	if capability == nil {
		return nil
	}
	if !capability.Available && (capability.Context != "" || !capability.IssuedAt.IsZero() || !capability.CreateBefore.IsZero()) || capability.Available && (!rw.ValidToken(capability.Context, rw.MaxContext) || !rw.ValidTime(capability.IssuedAt) || !rw.ValidTime(capability.CreateBefore) || capability.CreateBefore.Sub(capability.IssuedAt) != rw.ContextWindow) {
		return errMalformed
	}
	w.Header().Set("Recommendation-Version", strconv.Itoa(rw.Version))
	w.Header().Set("Recommendation-Available", strconv.FormatBool(capability.Available))
	if capability.Available {
		w.Header().Set("Recommendation-Context", capability.Context)
		w.Header().Set("Recommendation-Issued-At", utc(capability.IssuedAt))
		w.Header().Set("Recommendation-Create-Before", utc(capability.CreateBefore))
	}
	return nil
}
