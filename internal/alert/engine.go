// Package alert evaluates alert rules against stored data and drives the
// alert lifecycle (firing -> acknowledged -> resolved) with notifications.
//
// The engine is stateless between evaluations: all state lives in
// PostgreSQL, and a partial unique index guarantees at most one open alert
// per (rule, target). This makes evaluation restart-safe and prevents
// duplicate alerts even if several server replicas evaluate concurrently.
package alert

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"pulsewatch/internal/domain"
	"pulsewatch/internal/notify"
	"pulsewatch/internal/store"
)

// Store is the persistence the engine needs.
type Store interface {
	ListRules(ctx context.Context) ([]domain.AlertRule, error)
	ListHosts(ctx context.Context) ([]domain.Host, error)
	ListMonitors(ctx context.Context) ([]domain.Monitor, error)
	RecentHostMetrics(ctx context.Context, hostID string, n int) ([]domain.HostMetric, error)
	RecentCheckResults(ctx context.Context, monitorID string, n int) ([]domain.CheckResult, error)
	OpenAlertsForRule(ctx context.Context, ruleID string) (map[string]domain.Alert, error)
	OpenAlert(ctx context.Context, a domain.Alert) (domain.Alert, bool, error)
	ResolveAlert(ctx context.Context, id string) (domain.Alert, bool, error)
	RecordEvent(ctx context.Context, e domain.Event) error
}

// Notifier sends notifications; implemented by notify.Notifier.
type Notifier interface {
	Notify(ctx context.Context, p notify.Payload)
}

// Engine periodically evaluates all rules.
type Engine struct {
	store    Store
	notifier Notifier
	log      *slog.Logger
	now      func() time.Time
}

// NewEngine creates an engine.
func NewEngine(s Store, n Notifier, log *slog.Logger) *Engine {
	return &Engine{store: s, notifier: n, log: log, now: time.Now}
}

// Run evaluates every interval until ctx is cancelled.
func (e *Engine) Run(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			ectx, cancel := context.WithTimeout(ctx, interval)
			if err := e.Evaluate(ectx); err != nil && ctx.Err() == nil {
				e.log.Error("alert evaluation", "err", err)
			}
			cancel()
		}
	}
}

// verdict is the outcome of evaluating a rule for one target.
type verdict struct {
	known  bool // false: not enough data to decide; leave state unchanged
	breach bool
	value  float64
	detail string
}

type target struct {
	kind, id, name string
	verdict        verdict
}

// Evaluate runs one evaluation pass over all rules.
func (e *Engine) Evaluate(ctx context.Context) error {
	rules, err := e.store.ListRules(ctx)
	if err != nil {
		return fmt.Errorf("list rules: %w", err)
	}
	var (
		hosts    []domain.Host
		monitors []domain.Monitor
	)
	for _, r := range rules {
		if !r.Enabled {
			// Disabling a rule clears its open alerts.
			e.reconcile(ctx, r, nil, true)
			continue
		}
		var targets []target
		switch r.Kind {
		case domain.RuleHostMetric, domain.RuleHostOffline:
			if hosts == nil {
				if hosts, err = e.store.ListHosts(ctx); err != nil {
					return fmt.Errorf("list hosts: %w", err)
				}
			}
			targets, err = e.hostTargets(ctx, r, hosts)
		case domain.RuleMonitorDown, domain.RuleMonitorLatency:
			if monitors == nil {
				if monitors, err = e.store.ListMonitors(ctx); err != nil {
					return fmt.Errorf("list monitors: %w", err)
				}
			}
			targets, err = e.monitorTargets(ctx, r, monitors)
		default:
			continue
		}
		if err != nil {
			e.log.Error("evaluate rule", "rule", r.Name, "err", err)
			continue
		}
		e.reconcile(ctx, r, targets, false)
	}
	return nil
}

func (e *Engine) hostTargets(ctx context.Context, r domain.AlertRule, hosts []domain.Host) ([]target, error) {
	now := e.now()
	var out []target
	for _, h := range hosts {
		if r.HostID != nil && *r.HostID != h.ID {
			continue
		}
		t := target{kind: "host", id: h.ID, name: h.Name}
		if r.Kind == domain.RuleHostOffline {
			t.verdict = EvalOffline(r, h.LastSeenAt, now)
		} else {
			samples, err := e.store.RecentHostMetrics(ctx, h.ID, r.ForCount)
			if err != nil {
				return nil, err
			}
			// Ignore stale data from offline hosts; the offline rule covers them.
			if len(samples) > 0 && now.Sub(samples[0].Time) > store.OfflineAfter(h.ReportInterval) {
				samples = nil
			}
			t.verdict = EvalHostMetric(r, samples)
		}
		out = append(out, t)
	}
	return out, nil
}

func (e *Engine) monitorTargets(ctx context.Context, r domain.AlertRule, monitors []domain.Monitor) ([]target, error) {
	var out []target
	for _, m := range monitors {
		if r.MonitorID != nil && *r.MonitorID != m.ID {
			continue
		}
		t := target{kind: "monitor", id: m.ID, name: m.Name}
		if !m.Enabled {
			// Paused monitors resolve their open alerts.
			t.verdict = verdict{known: true}
			out = append(out, t)
			continue
		}
		results, err := e.store.RecentCheckResults(ctx, m.ID, r.ForCount)
		if err != nil {
			return nil, err
		}
		if r.Kind == domain.RuleMonitorDown {
			t.verdict = EvalMonitorDown(r, results)
		} else {
			t.verdict = EvalLatency(r, results)
		}
		out = append(out, t)
	}
	return out, nil
}

// reconcile opens alerts for breaching targets and resolves recovered ones.
func (e *Engine) reconcile(ctx context.Context, r domain.AlertRule, targets []target, resolveAll bool) {
	open, err := e.store.OpenAlertsForRule(ctx, r.ID)
	if err != nil {
		e.log.Error("load open alerts", "rule", r.Name, "err", err)
		return
	}
	seen := make(map[string]bool, len(targets))
	for _, t := range targets {
		seen[t.id] = true
		existing, isOpen := open[t.id]
		switch {
		case !t.verdict.known:
			// Insufficient data: keep current state.
		case t.verdict.breach && !isOpen:
			e.fire(ctx, r, t)
		case !t.verdict.breach && isOpen:
			e.resolve(ctx, existing, "recovered")
		}
	}
	// Targets no longer in scope (rule narrowed, rule disabled) are resolved.
	for id, a := range open {
		if resolveAll || !seen[id] {
			e.resolve(ctx, a, "no longer evaluated")
		}
	}
}

func (e *Engine) fire(ctx context.Context, r domain.AlertRule, t target) {
	a, created, err := e.store.OpenAlert(ctx, domain.Alert{
		RuleID: r.ID, RuleName: r.Name, TargetType: t.kind, TargetID: t.id, TargetName: t.name,
		Severity: r.Severity, Value: t.verdict.value,
		Message: fmt.Sprintf("%s: %s %s", r.Name, t.name, t.verdict.detail),
	})
	if err != nil {
		e.log.Error("open alert", "rule", r.Name, "target", t.name, "err", err)
		return
	}
	if !created {
		return // another evaluator opened it first; it owns the notification
	}
	e.log.Warn("alert firing", "rule", r.Name, "target", t.name, "value", t.verdict.value)
	e.recordEvent(ctx, domain.Event{Kind: domain.EventAlertFiring, Severity: r.Severity, Message: a.Message,
		TargetType: t.kind, TargetID: t.id})
	e.notifier.Notify(ctx, notify.Payload{
		Event: notify.EventFiring, Alert: &a,
		Text: fmt.Sprintf("[FIRING][%s] %s", a.Severity, a.Message),
	})
}

func (e *Engine) resolve(ctx context.Context, open domain.Alert, why string) {
	a, changed, err := e.store.ResolveAlert(ctx, open.ID)
	if err != nil {
		e.log.Error("resolve alert", "alert", open.ID, "err", err)
		return
	}
	if !changed {
		return
	}
	dur := a.ResolvedAt.Sub(a.TriggeredAt).Round(time.Second)
	msg := fmt.Sprintf("%s: %s %s after %s", a.RuleName, a.TargetName, why, dur)
	e.log.Info("alert resolved", "rule", a.RuleName, "target", a.TargetName, "after", dur)
	e.recordEvent(ctx, domain.Event{Kind: domain.EventAlertResolved, Severity: domain.SeverityInfo, Message: msg,
		TargetType: a.TargetType, TargetID: a.TargetID})
	e.notifier.Notify(ctx, notify.Payload{
		Event: notify.EventResolved, Alert: &a, Text: "[RESOLVED] " + msg,
	})
}

func (e *Engine) recordEvent(ctx context.Context, ev domain.Event) {
	if err := e.store.RecordEvent(ctx, ev); err != nil {
		e.log.Error("record event", "err", err)
	}
}
