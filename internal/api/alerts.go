package api

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"pulsewatch/internal/alert"
	"pulsewatch/internal/domain"
	"pulsewatch/internal/notify"
	"pulsewatch/internal/store"
)

func (s *Server) listAlerts(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	switch state {
	case "", "open", domain.AlertFiring, domain.AlertAcknowledged, domain.AlertResolved:
	default:
		s.fail(w, r, validationError{"state": "must be one of open, firing, acknowledged, resolved"})
		return
	}
	from, _, err := timeWindow(r, 7*24*time.Hour)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	limit, err := intParam(r, "limit", 200, 1, 1000)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	list, err := s.store.ListAlerts(r.Context(), store.AlertFilter{
		State: state, TargetID: r.URL.Query().Get("target_id"), Since: from, Limit: limit,
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"alerts": nonNil(list)})
}

func (s *Server) ackAlert(w http.ResponseWriter, r *http.Request) {
	a, err := s.store.AcknowledgeAlert(r.Context(), r.PathValue("id"))
	if errors.Is(err, domain.ErrConflict) {
		writeError(w, http.StatusConflict, "invalid_state", "only firing alerts can be acknowledged (current state: "+a.State+")")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.event(r, domain.Event{Kind: domain.EventAlertAck, Severity: domain.SeverityInfo,
		Message: fmt.Sprintf("Alert acknowledged: %s", a.Message), TargetType: a.TargetType, TargetID: a.TargetID})
	writeJSON(w, http.StatusOK, a)
}

var validOperators = map[string]bool{">": true, ">=": true, "<": true, "<=": true}

type ruleInput struct {
	Name      string  `json:"name"`
	Kind      string  `json:"kind"`
	Metric    string  `json:"metric"`
	Operator  string  `json:"operator"`
	Threshold float64 `json:"threshold"`
	ForCount  int     `json:"for_count"`
	Severity  string  `json:"severity"`
	HostID    *string `json:"host_id"`
	MonitorID *string `json:"monitor_id"`
	Enabled   *bool   `json:"enabled"`
}

func (s *Server) toRule(ctx context.Context, in ruleInput) (domain.AlertRule, error) {
	rule := domain.AlertRule{
		Name: strings.TrimSpace(in.Name), Kind: in.Kind, Metric: in.Metric, Operator: in.Operator,
		Threshold: in.Threshold, ForCount: in.ForCount, Severity: in.Severity, Enabled: true,
	}
	if in.Enabled != nil {
		rule.Enabled = *in.Enabled
	}
	if rule.Operator == "" {
		rule.Operator = ">"
	}
	if rule.ForCount == 0 {
		rule.ForCount = 1
	}
	if in.HostID != nil && *in.HostID != "" {
		rule.HostID = in.HostID
	}
	if in.MonitorID != nil && *in.MonitorID != "" {
		rule.MonitorID = in.MonitorID
	}

	v := validationError{}
	v.check(rule.Name != "" && len(rule.Name) <= 100, "name", "required, at most 100 characters")
	v.check(rule.Severity == domain.SeverityCritical || rule.Severity == domain.SeverityWarning || rule.Severity == domain.SeverityInfo,
		"severity", "must be critical, warning or info")
	v.check(validOperators[rule.Operator], "operator", "must be one of >, >=, <, <=")
	v.check(!math.IsNaN(rule.Threshold) && !math.IsInf(rule.Threshold, 0), "threshold", "must be a finite number")
	v.check(rule.ForCount >= 1 && rule.ForCount <= 100, "for_count", "must be between 1 and 100")

	switch rule.Kind {
	case domain.RuleHostMetric:
		v.check(alert.ValidMetric(rule.Metric), "metric", "must be one of cpu_pct, mem_pct, disk_pct, swap_pct, load1")
		v.check(rule.MonitorID == nil, "monitor_id", "not applicable to host rules")
	case domain.RuleHostOffline:
		rule.Metric = ""
		v.check(rule.Threshold >= 10, "threshold", "offline threshold must be at least 10 seconds")
		v.check(rule.MonitorID == nil, "monitor_id", "not applicable to host rules")
	case domain.RuleMonitorDown:
		rule.Metric = ""
		v.check(rule.HostID == nil, "host_id", "not applicable to monitor rules")
	case domain.RuleMonitorLatency:
		rule.Metric = ""
		v.check(rule.Threshold > 0, "threshold", "latency threshold must be positive (milliseconds)")
		v.check(rule.HostID == nil, "host_id", "not applicable to monitor rules")
	default:
		v.check(false, "kind", "must be one of host_metric, host_offline, monitor_down, monitor_latency")
	}
	if len(v) == 0 && rule.HostID != nil {
		if _, err := s.store.GetHost(ctx, *rule.HostID); err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				v["host_id"] = "host does not exist"
			} else {
				return rule, err
			}
		}
	}
	if len(v) == 0 && rule.MonitorID != nil {
		if _, err := s.store.GetMonitor(ctx, *rule.MonitorID); err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				v["monitor_id"] = "monitor does not exist"
			} else {
				return rule, err
			}
		}
	}
	return rule, v.err()
}

func (s *Server) listRules(w http.ResponseWriter, r *http.Request) {
	rules, err := s.store.ListRules(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rules": nonNil(rules)})
}

func (s *Server) createRule(w http.ResponseWriter, r *http.Request) {
	var in ruleInput
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	rule, err := s.toRule(r.Context(), in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	rule, err = s.store.CreateRule(r.Context(), rule)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.event(r, domain.Event{Kind: domain.EventConfigChange, Message: fmt.Sprintf("Alert rule %q created", rule.Name)})
	writeJSON(w, http.StatusCreated, rule)
}

func (s *Server) updateRule(w http.ResponseWriter, r *http.Request) {
	var in ruleInput
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	rule, err := s.toRule(r.Context(), in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	rule.ID = r.PathValue("id")
	rule, err = s.store.UpdateRule(r.Context(), rule)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.event(r, domain.Event{Kind: domain.EventConfigChange, Message: fmt.Sprintf("Alert rule %q updated", rule.Name)})
	writeJSON(w, http.StatusOK, rule)
}

func (s *Server) deleteRule(w http.ResponseWriter, r *http.Request) {
	name, err := s.store.DeleteRule(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.event(r, domain.Event{Kind: domain.EventConfigChange, Message: fmt.Sprintf("Alert rule %q deleted", name)})
	w.WriteHeader(http.StatusNoContent)
}

type webhookInput struct {
	Name    string `json:"name"`
	URL     string `json:"url"`
	Secret  string `json:"secret"`
	Enabled *bool  `json:"enabled"`
}

func (s *Server) listWebhooks(w http.ResponseWriter, r *http.Request) {
	hooks, err := s.store.ListWebhooks(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"webhooks": nonNil(hooks)})
}

func (s *Server) createWebhook(w http.ResponseWriter, r *http.Request) {
	var in webhookInput
	if err := decode(r, &in); err != nil {
		s.fail(w, r, err)
		return
	}
	hook := domain.Webhook{Name: strings.TrimSpace(in.Name), URL: strings.TrimSpace(in.URL), Secret: in.Secret, Enabled: true}
	if in.Enabled != nil {
		hook.Enabled = *in.Enabled
	}
	v := validationError{}
	v.check(hook.Name != "" && len(hook.Name) <= 100, "name", "required, at most 100 characters")
	u, err := url.Parse(hook.URL)
	v.check(err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && len(hook.URL) <= 2048,
		"url", "must be an absolute http(s) URL")
	v.check(len(hook.Secret) <= 256, "secret", "at most 256 characters")
	if err := v.err(); err != nil {
		s.fail(w, r, err)
		return
	}
	hook, err = s.store.CreateWebhook(r.Context(), hook)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.event(r, domain.Event{Kind: domain.EventConfigChange, Message: fmt.Sprintf("Webhook %q added", hook.Name)})
	writeJSON(w, http.StatusCreated, hook)
}

func (s *Server) deleteWebhook(w http.ResponseWriter, r *http.Request) {
	name, err := s.store.DeleteWebhook(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.event(r, domain.Event{Kind: domain.EventConfigChange, Message: fmt.Sprintf("Webhook %q removed", name)})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) testWebhook(w http.ResponseWriter, r *http.Request) {
	hook, err := s.store.GetWebhook(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	err = s.notifier.Send(ctx, hook, notify.Payload{Event: notify.EventTest, Text: "Pulsewatch test notification"})
	if err != nil {
		// Delivery errors describe the remote endpoint and are useful to the operator.
		writeError(w, http.StatusBadGateway, "delivery_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "delivered"})
}

func (s *Server) listEvents(w http.ResponseWriter, r *http.Request) {
	from, _, err := timeWindow(r, 7*24*time.Hour)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	limit, err := intParam(r, "limit", 100, 1, 1000)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	events, err := s.store.ListEvents(r.Context(), from, limit)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": nonNil(events)})
}
