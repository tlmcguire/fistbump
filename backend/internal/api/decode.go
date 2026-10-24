package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// maxBody caps a request body at 1 MB.
const maxBody = 1 << 20

// decodeJSON reads one JSON value from the body into v. Malformed JSON is 400 bad_request; an
// unknown field or a wrong type is 422 validation_failed naming the field; trailing data is 400.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) {
			field := typeErr.Field
			return validationError(field, field+" has the wrong type")
		}
		if name, ok := strings.CutPrefix(err.Error(), "json: unknown field "); ok {
			name, _ = strconv.Unquote(name)
			return validationError(name, "unknown field "+name)
		}
		return bodyError(err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return badRequest("Request must contain a single JSON value")
		}
		return bodyError(err)
	}
	return nil
}

// pathID reads the numeric {id} from the path; a value that is not a positive integer is a 404.
func pathID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		return 0, notFound("Not found")
	}
	return id, nil
}
