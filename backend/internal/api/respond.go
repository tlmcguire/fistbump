// respond.go

package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
)

// apiError is an error
type apiError struct {
	Status  int
	Code    string
	Message string
	Details map[string]any
}

func (e *apiError) Error() string {
	return e.Code + ": " + e.Message
}

func unauthorized() *apiError {
	return &apiError{
		Status:  http.StatusUnauthorized,
		Code:    "unauthorized",
		Message: "Authentication is required",
	}
}
func badRequest(msg string) *apiError {
	return &apiError{
		Status:  http.StatusBadRequest,
		Code:    "bad_request",
		Message: msg,
	}
}

func notFound(msg string) *apiError {
	return &apiError{
		Status:  http.StatusNotFound,
		Code:    "not_found",
		Message: msg,
	}
}

func validationFailed(msg string) *apiError {
	return &apiError{
		Status:  http.StatusUnprocessableEntity,
		Code:    "validation_failed",
		Message: msg,
	}
}

func notImplemented() *apiError {
	return &apiError{
		Status:  http.StatusNotImplemented,
		Code:    "not_implemented",
		Message: "This feature is not yet available",
	}
}

func upstreamError(msg string) *apiError {
	return &apiError{
		Status:  http.StatusBadGateway,
		Code:    "upstream_error",
		Message: msg,
	}
}

// errorBody is the error shape in docs/api.md.
type errorBody struct {
	Error struct {
		Code    string         `json:"code"`
		Message string         `json:"message"`
		Details map[string]any `json:"details,omitempty"`
	} `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	// set content type
	w.Header().Set("Content-Type", "application/json")

	// write status header
	w.WriteHeader(status)

	// encode; on error log
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// The response may already be partially written; log the error
		slog.Error("encode response", "err", err)
	}
}

func internal() *apiError {
	return &apiError{
		Status:  http.StatusInternalServerError,
		Code:    "internal",
		Message: "An unexpected error occurred",
	}
}

func writeError(w http.ResponseWriter, err error) {
	var apiErr *apiError
	if !errors.As(err, &apiErr) {
		slog.Error("unhandled error", "err", err)
		apiErr = internal()
	}

	var body errorBody
	body.Error.Code = apiErr.Code
	body.Error.Message = apiErr.Message
	body.Error.Details = apiErr.Details
	writeJSON(w, apiErr.Status, body)
}

type handlerFunc func(w http.ResponseWriter, r *http.Request) error

// handle adapts h: nil means h wrote the response; *apiError
// picks the response; anything else is a 500
func handle(h handlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
			writeError(w, err)
		}
	})
}
