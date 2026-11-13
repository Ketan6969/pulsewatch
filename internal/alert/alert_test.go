package alert

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"pulsewatch/internal/domain"
	"pulsewatch/internal/notify"
)

func cpuSamples(vals ...float64) []domain.HostMetric {
	now := time.Now()
	out := make([]domain.HostMetric, len(vals))
	for i, v := range vals {
		out[i] = domain.HostMetric{Time: now.Add(-time.Duration(i) * 15 * time.Second), CPUPct: v}
	}
	return out
}

func TestEvalHostMetric(t *testing.T) {
	r := domain.AlertRule{Metric: "cpu_pct", Operator: ">", Threshold: 90, ForCount: 3}
	cases := []struct {
		name          string
		samples       []domain.HostMetric
		known, breach bool
	}{
		{"no data", nil, false, false},
		{"all breach", cpuSamples(95, 96, 97), true, true},
		{"latest healthy", cpuSamples(50, 96, 97), true, false},
		{"one dip", cpuSamples(95, 50, 97), true, false},
		{"not enough samples", cpuSamples(95, 96), false, false},
	}
	for _, c := range cases {
		v := EvalHostMetric(r, c.samples)
		if v.known != c.known || v.breach != c.breach {
			t.Errorf("%s: got known=%v breach=%v", c.name, v.known, v.breach)
		}
	}
}

func TestEvalMonitorDown(t *testing.T) {
	r := domain.AlertRule{ForCount: 2}
	fail := domain.CheckResult{Success: false, Error: "boom"}
	ok := domain.CheckResult{Success: true}
	if v := EvalMonitorDown(r, []domain.CheckResult{fail, fail}); !v.breach {
		t.Error("expected breach after 2 failures")
	}
	if v := EvalMonitorDown(r, []domain.CheckResult{fail, ok}); v.breach || !v.known {
		t.Error("expected known non-breach after single failure")
	}
	if v := EvalMonitorDown(r, []domain.CheckResult{fail}); v.known {
		t.Error("expected unknown with insufficient history")
	}
	if v := EvalMonitorDown(r, []domain.CheckResult{ok}); !v.known || v.breach {
		t.Error("success should be known healthy")
	}
}

func TestEvalLatency(t *testing.T) {
	r := domain.AlertRule{Operator: ">", Threshold: 500, ForCount: 2}
	slow := domain.CheckResult{Success: true, ResponseMS: 900}
	fast := domain.CheckResult{Success: true, ResponseMS: 100}
	if v := EvalLatency(r, []domain.CheckResult{slow, slow}); !v.breach {
		t.Error("expected latency breach")
	}
	if v := EvalLatency(r, []domain.CheckResult{fast, slow}); v.breach || !v.known {
		t.Error("expected recovery on fast check")
	}
}

func TestEvalOffline(t *testing.T) {
	r := domain.AlertRule{Threshold: 60}
	now := time.Now()
	if v := EvalOffline(r, now.Add(-2*time.Minute), now); !v.breach {
		t.Error("expected offline")
	}
	if v := EvalOffline(r, now.Add(-10*time.Second), now); v.breach {
		t.Error("expected online")
	}
}

func TestCompare(t *testing.T) {
	if !Compare(">=", 5, 5) || Compare(">", 5, 5) || !Compare("<", 1, 2) || Compare("??", 1, 0) {
		t.Error("compare mismatch")
	}
}

// fakeStore is an in-memory Store that enforces the one-open-alert invariant.
type fakeStore struct {
	mu      sync.Mutex
	rules   []domain.AlertRule
	results map[string][]domain.CheckResult
	alerts  []domain.Alert
	events  []domain.Event
}

func (f *fakeStore) ListRules(context.Context) ([]domain.AlertRule, error) { return f.rules, nil }
func (f *fakeStore) ListHosts(context.Context) ([]domain.Host, error)      { return nil, nil }
func (f *fakeStore) ListMonitors(context.Context) ([]domain.Monitor, error) {
	return []domain.Monitor{{ID: "m1", Name: "api", Enabled: true}}, nil
}
func (f *fakeStore) RecentHostMetrics(context.Context, string, int) ([]domain.HostMetric, error) {
	return nil, nil
}
func (f *fakeStore) RecentCheckResults(_ context.Context, id string, n int) ([]domain.CheckResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r := f.results[id]
	if len(r) > n {
		r = r[:n]
	}
	return r, nil
}
func (f *fakeStore) OpenAlertsForRule(_ context.Context, ruleID string) (map[string]domain.Alert, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]domain.Alert{}
	for _, a := range f.alerts {
		if a.RuleID == ruleID && a.State != domain.AlertResolved {
			out[a.TargetID] = a
		}
	}
	return out, nil
}
func (f *fakeStore) OpenAlert(_ context.Context, a domain.Alert) (domain.Alert, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, x := range f.alerts {
		if x.RuleID == a.RuleID && x.TargetID == a.TargetID && x.State != domain.AlertResolved {
			return x, false, nil
		}
	}
	a.ID = time.Now().String()
	a.State = domain.AlertFiring
	a.TriggeredAt = time.Now()
	f.alerts = append(f.alerts, a)
	return a, true, nil
}
func (f *fakeStore) ResolveAlert(_ context.Context, id string) (domain.Alert, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.alerts {
		if f.alerts[i].ID == id && f.alerts[i].State != domain.AlertResolved {
			now := time.Now()
			f.alerts[i].State, f.alerts[i].ResolvedAt = domain.AlertResolved, &now
			return f.alerts[i], true, nil
		}
	}
	return domain.Alert{}, false, nil
}
func (f *fakeStore) RecordEvent(_ context.Context, e domain.Event) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, e)
	return nil
}

type recorder struct {
	mu     sync.Mutex
	events []string
}

func (r *recorder) Notify(_ context.Context, p notify.Payload) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, p.Event)
}

func TestEngineFiresOnceAndRecovers(t *testing.T) {
	fs := &fakeStore{
		rules: []domain.AlertRule{{ID: "r1", Name: "Endpoint down", Kind: domain.RuleMonitorDown, ForCount: 2,
			Severity: domain.SeverityCritical, Enabled: true}},
		results: map[string][]domain.CheckResult{"m1": {{Success: false}, {Success: false}}},
	}
	rec := &recorder{}
	e := NewEngine(fs, rec, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx := context.Background()

	for range 3 { // repeated evaluations must not duplicate alerts or notifications
		if err := e.Evaluate(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if len(fs.alerts) != 1 || len(rec.events) != 1 || rec.events[0] != notify.EventFiring {
		t.Fatalf("alerts=%d notifications=%v", len(fs.alerts), rec.events)
	}

	fs.mu.Lock()
	fs.results["m1"] = append([]domain.CheckResult{{Success: true}}, fs.results["m1"]...)
	fs.mu.Unlock()
	if err := e.Evaluate(ctx); err != nil {
		t.Fatal(err)
	}
	if fs.alerts[0].State != domain.AlertResolved {
		t.Fatal("alert not resolved")
	}
	if len(rec.events) != 2 || rec.events[1] != notify.EventResolved {
		t.Fatalf("notifications = %v", rec.events)
	}
	if err := e.Evaluate(ctx); err != nil {
		t.Fatal(err)
	}
	if len(rec.events) != 2 {
		t.Fatal("duplicate recovery notification")
	}
}

func TestEngineDisabledRuleResolves(t *testing.T) {
	fs := &fakeStore{
		rules:   []domain.AlertRule{{ID: "r1", Kind: domain.RuleMonitorDown, ForCount: 1, Enabled: true}},
		results: map[string][]domain.CheckResult{"m1": {{Success: false}}},
	}
	e := NewEngine(fs, &recorder{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	_ = e.Evaluate(context.Background())
	fs.rules[0].Enabled = false
	_ = e.Evaluate(context.Background())
	if fs.alerts[0].State != domain.AlertResolved {
		t.Fatal("disabled rule should resolve open alerts")
	}
}
