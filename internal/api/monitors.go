package api

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"pulsewatch/internal/domain"
)

// monitorInput is the create/update request body. Pointers distinguish
// "omitted" from zero values so defaults can be applied.
type monitorInput struct {
	Name           string            `json:"name"`
	URL            string            `json:"url"`
	Method         string            `json:"method"`
	IntervalS      *int              `json:"interval_s"`
	TimeoutMS      *int              `json:"timeout_ms"`
	ExpectedStatus *int              `json:"expected_status"`
	Headers        map[string]string `json:"headers"`
	Enabled        *bool             `json:"enabled"`
}

var allowedMethods = map[string]bool{"GET": true, "HEAD": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true, "OPTIONS": true}

func (in monitorInput) toMonitor() (domain.Monitor, error) {
	m := domain.Monitor{
		Name:      strings.TrimSpace(in.Name),
		URL:       strings.TrimSpace(in.URL),
		Method:    strings.ToUpper(strings.TrimSpace(in.Method)),
		IntervalS: 60,
		TimeoutMS: 10000,
		Headers:   in.Headers,
		Enabled:   true,
	}
	if m.Method == "" {
		m.Method = http.MethodGet
	}
	if in.IntervalS != nil {
		m.IntervalS = *in.IntervalS
	}
	if in.TimeoutMS != nil {
		m.TimeoutMS = *in.TimeoutMS
	}
	if in.ExpectedStatus != nil {
		m.ExpectedStatus = *in.ExpectedStatus
	}
	if in.Enabled != nil {
		m.Enabled = *in.Enabled
	}
	if m.Headers == nil {
		m.Headers = map[string]string{}
	}

	v := validationError{}
	v.check(m.Name != "" && len(m.Name) <= 100, "name", "required, at most 100 characters")
	u, err := url.Parse(m.URL)
	v.check(err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && len(m.URL) <= 2048,
		"url", "must be an absolute http(s) URL, at most 2048 characters")
	if err == nil && u.User != nil {
		v.check(false, "url", "credentials in the URL are not allowed; use an Authorization header")
	}
	v.check(allowedMethods[m.Method], "method", "must be one of GET, HEAD, POST, PUT, PATCH, DELETE, OPTIONS")
	v.check(m.IntervalS >= 10 && m.IntervalS <= 86400, "interval_s", "must be between 10 and 86400 seconds")
	v.check(m.TimeoutMS >= 100 && m.TimeoutMS <= 60000, "timeout_ms", "must be between 100 and 60000 milliseconds")
	v.check(m.TimeoutMS < m.IntervalS*1000, "timeout_ms", "must be shorter than the check interval")
	v.check(m.ExpectedStatus == 0 || (m.ExpectedStatus >= 100 && m.ExpectedStatus <= 599),
		"expected_status", "must be 0 (any 2xx/3xx) or a status code between 100 and 599")
	v.check(len(m.Headers) <= 20, "headers", "at most 20 headers")
	for k, val := range m.Headers {
		v.check(validHeaderName(k) && len(k) <= 128, "headers", fmt.Sprintf("invalid header name %q", k))
		v.check(!strings.ContainsAny(val, "\r\n\x00") && len(val) <= 4096, "headers", fmt.Sprintf("invalid value for header %q", k))
	}
	return m, v.err()
}

func validHeaderName(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", c)) {
			return false
		}
	}
	return true
}

func (s *Server) listMonitors(w http.ResponseWriter, r *http.Request) {
	from, _, err := timeWindow(r, 24*time.Hour)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	list, err := s.store.MonitorSummaries(r.Context(), from)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"monitors": nonNil(list)})
}

func (s *Server) getMonitor(w http.ResponseWriter, r *http.Request) {
	from, _, err := timeWindow(r, 24*time.Hour)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	m, err := s.store.MonitorSummary(r.Context(), r.PathValue("id"), from)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (s *Server) createMonitor(w http.ResponseWriter, r *http.Request) {
	var in monitorInput
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	m, err := in.toMonitor()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	m, err = s.store.CreateMonitor(r.Context(), m)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.scheduler.Refresh()
	s.event(r, domain.Event{Kind: domain.EventConfigChange, Message: fmt.Sprintf("HTTP monitor %q created", m.Name),
		TargetType: "monitor", TargetID: m.ID})
	writeJSON(w, http.StatusCreated, m)
}

func (s *Server) updateMonitor(w http.ResponseWriter, r *http.Request) {
	var in monitorInput
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	m, err := in.toMonitor()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	m.ID = r.PathValue("id")
	m, err = s.store.UpdateMonitor(r.Context(), m)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.scheduler.Refresh()
	s.event(r, domain.Event{Kind: domain.EventConfigChange, Message: fmt.Sprintf("HTTP monitor %q updated", m.Name),
		TargetType: "monitor", TargetID: m.ID})
	writeJSON(w, http.StatusOK, m)
}

func (s *Server) deleteMonitor(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	name, err := s.store.DeleteMonitor(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.scheduler.Refresh()
	s.event(r, domain.Event{Kind: domain.EventConfigChange, Message: fmt.Sprintf("HTTP monitor %q deleted", name),
		TargetType: "monitor", TargetID: id})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) monitorResults(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	from, to, err := timeWindow(r, 24*time.Hour)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	limit, err := intParam(r, "limit", 100, 1, 1000)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if _, err := s.store.GetMonitor(r.Context(), id); err != nil {
		s.fail(w, r, err)
		return
	}
	res, err := s.store.CheckResultsBetween(r.Context(), id, from, to, limit)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": nonNil(res)})
}

func (s *Server) monitorSeries(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := s.store.GetMonitor(r.Context(), id); err != nil {
		s.fail(w, r, err)
		return
	}
	s.seriesResponse(w, r, func(from, to time.Time) ([]domain.SeriesPoint, time.Duration, error) {
		return s.store.MonitorSeries(r.Context(), id, from, to)
	})
}

func (s *Server) fleetMonitorSeries(w http.ResponseWriter, r *http.Request) {
	s.seriesResponse(w, r, func(from, to time.Time) ([]domain.SeriesPoint, time.Duration, error) {
		return s.store.MonitorSeries(r.Context(), "", from, to)
	})
}
