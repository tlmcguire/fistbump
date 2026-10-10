// Package api serves the HTTP routes under /v1.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/tlmcguire/fistbump/backend/internal/store"
)

// apiErr is returned by handlers and rendered as the documented error shape.
type apiErr struct {
	Status  int
	Code    string
	Message string
	Details map[string]any
}

func (e *apiErr) Error() string { return e.Message }

func errf(status int, code, format string, a ...any) *apiErr {
	return &apiErr{Status: status, Code: code, Message: fmt.Sprintf(format, a...)}
}

func badRequest(format string, a ...any) *apiErr { return errf(400, "bad_request", format, a...) }
func notFound(what string) *apiErr               { return errf(404, "not_found", "%s not found", what) }
func conflict(format string, a ...any) *apiErr   { return errf(409, "conflict", format, a...) }
func invalid(field, format string, a ...any) *apiErr {
	e := errf(422, "validation_failed", format, a...)
	if field != "" {
		e.Details = map[string]any{"field": field}
	}
	return e
}
func upstream(format string, a ...any) *apiErr { return errf(502, "upstream_error", format, a...) }

func (e *apiErr) with(k string, v any) *apiErr {
	if e.Details == nil {
		e.Details = map[string]any{}
	}
	e.Details[k] = v
	return e
}

// fromStore maps store sentinel errors to API errors. what names the missing resource.
func fromStore(err error, what string) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, store.ErrNotFound):
		return notFound(what)
	case errors.Is(err, store.ErrConflict):
		return conflict("%s conflicts with existing data", what)
	}
	return err
}

// needsProfile maps a store error from a write that needs the profile row to exist.
func needsProfile(err error) error {
	if errors.Is(err, store.ErrConflict) {
		return conflict("save your resume first")
	}
	return fromStore(err, "profile")
}

type handler func(w http.ResponseWriter, r *http.Request) error

// wrap turns a handler into an http.HandlerFunc that renders returned errors.
func wrap(h handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
			writeError(w, err)
		}
	}
}

func writeError(w http.ResponseWriter, err error) {
	var ae *apiErr
	if !errors.As(err, &ae) {
		ae = errf(500, "internal", "internal error")
		logf("internal error: %v", err)
	}
	body := map[string]any{"code": ae.Code, "message": ae.Message}
	if ae.Details != nil {
		body["details"] = ae.Details
	}
	writeJSON(w, ae.Status, map[string]any{"error": body})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

func noContent(w http.ResponseWriter) error {
	w.WriteHeader(http.StatusNoContent)
	return nil
}

const maxBody = 8 << 20

// readBody reads and size-limits a request body.
func readBody(r *http.Request) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r.Body, maxBody+1))
	if err != nil {
		return nil, badRequest("could not read body")
	}
	if len(data) > maxBody {
		return nil, badRequest("body too large")
	}
	return data, nil
}

// decodeBytes decodes JSON into v. Malformed JSON is 400. Unknown fields are 422.
func decodeBytes(data []byte, v any) error {
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		if strings.HasPrefix(err.Error(), "json: unknown field") {
			return invalid(strings.Trim(strings.TrimPrefix(err.Error(), "json: unknown field "), `"`), "%s", err.Error())
		}
		var te *json.UnmarshalTypeError
		if errors.As(err, &te) {
			return invalid(te.Field, "field %s has the wrong type", te.Field)
		}
		return badRequest("malformed JSON")
	}
	if dec.More() {
		return badRequest("malformed JSON")
	}
	return nil
}

func decode(r *http.Request, v any) error {
	data, err := readBody(r)
	if err != nil {
		return err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return badRequest("request body is required")
	}
	return decodeBytes(data, v)
}

// decodeOptional is like decode but treats an empty body as {}.
func decodeOptional(r *http.Request, v any) error {
	data, err := readBody(r)
	if err != nil {
		return err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil
	}
	return decodeBytes(data, v)
}

func pathID(r *http.Request, name string) (int64, error) {
	var id int64
	if _, err := fmt.Sscanf(r.PathValue(name), "%d", &id); err != nil || id <= 0 {
		return 0, notFound("resource")
	}
	return id, nil
}

func queryInt(r *http.Request, key string) int {
	var n int
	_, _ = fmt.Sscanf(r.URL.Query().Get(key), "%d", &n)
	return n
}

var dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
var partialDateRe = regexp.MustCompile(`^\d{4}(-\d{2}(-\d{2})?)?$`)

func validDate(field string, p *string) error {
	if p != nil && !dateRe.MatchString(*p) {
		return invalid(field, "%s must be YYYY-MM-DD", field)
	}
	return nil
}

// cleanList trims, drops empties, and de-duplicates case-insensitively. lower also lower-cases values.
func cleanList(in []string, lower bool) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, s := range in {
		s = strings.TrimSpace(s)
		if lower {
			s = strings.ToLower(s)
		}
		k := strings.ToLower(s)
		if s == "" || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, s)
	}
	return out
}

func blankToNil(p **string) {
	if *p != nil && strings.TrimSpace(**p) == "" {
		*p = nil
	}
}
