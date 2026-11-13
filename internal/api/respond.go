package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"pulsewatch/internal/domain"
)

// errorBody is the uniform error envelope:
//
//	{"error": {"code": "validation_failed", "message": "...", "fields": {"url": "must be http or https"}}}
type errorBody struct {
	Error apiError `json:"error"`
}

type apiError struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, errorBody{Error: apiError{Code: code, Message: msg}})
}

// fail maps an error from the store or a decoder to a response. Unexpected
// errors are logged and reported as 500 without leaking details.
func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	var verr validationError
	var mbe *http.MaxBytesError
	switch {
	case errors.As(err, &verr):
		writeJSON(w, http.StatusUnprocessableEntity, errorBody{Error: apiError{
			Code: "validation_failed", Message: "request validation failed", Fields: verr,
		}})
	case errors.As(err, &mbe):
		writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", "request body too large")
	case errors.Is(err, errBadJSON):
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "resource not found")
	case errors.Is(err, domain.ErrConflict):
		writeError(w, http.StatusConflict, "conflict", "the request conflicts with the current state")
	case errors.Is(err, r.Context().Err()) && r.Context().Err() != nil:
		// Client went away; nothing useful to send.
	default:
		s.log.ErrorContext(r.Context(), "request failed", "method", r.Method, "path", r.URL.Path,
			"request_id", requestID(r.Context()), "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal server error")
	}
}

var errBadJSON = errors.New("invalid JSON body")

// decode reads a single JSON object, rejecting unknown fields and trailing data.
func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return err
		}
		return fmt.Errorf("%w: %s", errBadJSON, sanitizeJSONErr(err))
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: body must contain a single JSON object", errBadJSON)
	}
	return nil
}

func sanitizeJSONErr(err error) string {
	var se *json.SyntaxError
	var te *json.UnmarshalTypeError
	switch {
	case errors.As(err, &se):
		return fmt.Sprintf("syntax error at offset %d", se.Offset)
	case errors.As(err, &te):
		return fmt.Sprintf("field %q must be %s", te.Field, te.Type.String())
	case errors.Is(err, io.EOF):
		return "body is empty"
	case strings.HasPrefix(err.Error(), "json: unknown field"):
		return strings.TrimPrefix(err.Error(), "json: ")
	}
	return "malformed body"
}

// validationError maps field names to messages.
type validationError map[string]string

func (v validationError) Error() string { return "validation failed" }

func (v validationError) check(ok bool, field, msg string) {
	if !ok {
		if _, exists := v[field]; !exists {
			v[field] = msg
		}
	}
}

func (v validationError) err() error {
	if len(v) == 0 {
		return nil
	}
	return v
}

// timeWindow resolves ?range=1h or ?from=&to= (unix milliseconds).
func timeWindow(r *http.Request, def time.Duration) (time.Time, time.Time, error) {
	q := r.URL.Query()
	now := time.Now()
	if f, t := q.Get("from"), q.Get("to"); f != "" || t != "" {
		from, err1 := strconv.ParseInt(f, 10, 64)
		to, err2 := strconv.ParseInt(t, 10, 64)
		if err1 != nil || err2 != nil || to <= from {
			return time.Time{}, time.Time{}, validationError{"from": "from and to must be unix milliseconds with from < to"}
		}
		ft, tt := time.UnixMilli(from), time.UnixMilli(to)
		if tt.Sub(ft) > 90*24*time.Hour {
			return time.Time{}, time.Time{}, validationError{"from": "window may not exceed 90 days"}
		}
		return ft, tt, nil
	}
	rng := def
	if v := q.Get("range"); v != "" {
		d, err := parseRange(v)
		if err != nil {
			return time.Time{}, time.Time{}, validationError{"range": err.Error()}
		}
		rng = d
	}
	return now.Add(-rng), now, nil
}

// parseRange accepts Go durations plus a "d" (day) suffix, e.g. 15m, 6h, 7d.
func parseRange(v string) (time.Duration, error) {
	var d time.Duration
	if n, ok := strings.CutSuffix(v, "d"); ok {
		days, err := strconv.Atoi(n)
		if err != nil {
			return 0, errors.New("invalid range")
		}
		d = time.Duration(days) * 24 * time.Hour
	} else {
		var err error
		if d, err = time.ParseDuration(v); err != nil {
			return 0, errors.New("invalid range")
		}
	}
	if d < time.Minute || d > 90*24*time.Hour {
		return 0, errors.New("range must be between 1m and 90d")
	}
	return d, nil
}

func intParam(r *http.Request, name string, def, min, max int) (int, error) {
	v := r.URL.Query().Get(name)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < min || n > max {
		return 0, validationError{name: fmt.Sprintf("must be an integer between %d and %d", min, max)}
	}
	return n, nil
}

// decodeLenient is like decode but tolerates unknown fields, so newer
// agents can talk to older servers.
func decodeLenient(r *http.Request, v any) error {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return err
		}
		return fmt.Errorf("%w: %s", errBadJSON, sanitizeJSONErr(err))
	}
	return nil
}
