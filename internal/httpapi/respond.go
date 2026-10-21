package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/rbretschneider/ownership_board/internal/auth"
	"github.com/rbretschneider/ownership_board/internal/domain"
	"github.com/rbretschneider/ownership_board/internal/reposource"
)

// maxBodyBytes caps a request body; the largest legitimate one is a spec.
const maxBodyBytes = 1 << 20

// errorCode is the machine-readable part of an error response.
type errorCode string

const (
	codeBadRequest   errorCode = "bad_request"
	codeUnauthorized errorCode = "unauthorized"
	codeForbidden    errorCode = "forbidden"
	codeNotFound     errorCode = "not_found"
	codeConflict     errorCode = "conflict"
	codeRateLimited  errorCode = "rate_limited"
	codeInternal     errorCode = "internal"
	codeProvider     errorCode = "provider_error"
)

type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    errorCode `json:"code"`
	Message string    `json:"message"`
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeNoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

func writeErrorCode(w http.ResponseWriter, status int, code errorCode, message string) {
	writeJSON(w, status, errorBody{Error: errorDetail{Code: code, Message: message}})
}

// writeError maps an error to a response. Anything unexpected is logged here,
// where it is finally handled, and the caller sees a generic message.
func writeError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	if status, code, isKnown := classify(err); isKnown {
		if code == codeRateLimited {
			writeRetryAfter(w, err)
		}
		writeErrorCode(w, status, code, err.Error())
		return
	}
	logger.Error("request failed", "method", r.Method, "path", r.URL.Path, "error", err)
	writeErrorCode(w, http.StatusInternalServerError, codeInternal, "something went wrong on the server; try again")
}

func classify(err error) (int, errorCode, bool) {
	var throttled *auth.ThrottledError
	var limited *rateLimitedError
	switch {
	case errors.As(err, &throttled), errors.As(err, &limited):
		return http.StatusTooManyRequests, codeRateLimited, true
	case errors.Is(err, auth.ErrUnauthorized):
		return http.StatusUnauthorized, codeUnauthorized, true
	case reposource.IsProviderError(err):
		return http.StatusBadGateway, codeProvider, true
	}
	switch domain.KindOf(err) {
	case domain.KindInvalid:
		return http.StatusBadRequest, codeBadRequest, true
	case domain.KindNotFound:
		return http.StatusNotFound, codeNotFound, true
	case domain.KindConflict:
		return http.StatusConflict, codeConflict, true
	case domain.KindForbidden:
		return http.StatusForbidden, codeForbidden, true
	}
	return 0, "", false
}

func writeRetryAfter(w http.ResponseWriter, err error) {
	var throttled *auth.ThrottledError
	var limited *rateLimitedError
	if errors.As(err, &throttled) {
		w.Header().Set("Retry-After", strconv.Itoa(int(throttled.RetryAfter.Seconds())+1))
	}
	if errors.As(err, &limited) {
		w.Header().Set("Retry-After", strconv.Itoa(int(limited.retryAfter.Seconds())+1))
	}
}

// decodeJSON reads a JSON body into out, rejecting unknown fields so a typo in
// an agent's request fails loudly instead of being ignored.
func decodeJSON(w http.ResponseWriter, r *http.Request, out any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		if errors.Is(err, io.EOF) {
			return domain.Invalidf("request body is required")
		}
		return domain.Invalidf("request body is not valid: %v", err)
	}
	return nil
}

// decodeOptionalJSON is decodeJSON for endpoints whose body may be empty.
func decodeOptionalJSON(w http.ResponseWriter, r *http.Request, out any) error {
	if r.ContentLength == 0 {
		return nil
	}
	return decodeJSON(w, r, out)
}

func pathID(r *http.Request, name string) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil || id <= 0 {
		return 0, domain.Invalidf("%s must be a positive number", name)
	}
	return id, nil
}
