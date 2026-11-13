package api_test

// Integration tests exercising the HTTP API against a real PostgreSQL.
// Set PULSEWATCH_TEST_DATABASE_URL to run them, e.g.
//   PULSEWATCH_TEST_DATABASE_URL=postgres://pulsewatch:pulsewatch@localhost:5432/pulsewatch_test?sslmode=disable
// The target database is wiped.

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"pulsewatch/internal/alert"
	"pulsewatch/internal/api"
	"pulsewatch/internal/domain"
	"pulsewatch/internal/notify"
	"pulsewatch/internal/store"
)

const agentToken = "test-agent-token-123456"

type refresher struct{ n int }

func (r *refresher) Refresh() { r.n++ }

type env struct {
	t     *testing.T
	srv   *httptest.Server
	store *store.Store
	ref   *refresher
}

func setup(t *testing.T, apiToken string) *env {
	t.Helper()
	url := os.Getenv("PULSEWATCH_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("PULSEWATCH_TEST_DATABASE_URL not set; skipping integration test")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`); err != nil {
		t.Fatal(err)
	}
	conn.Close(ctx)

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	st, err := store.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Migrate(ctx, log); err != nil {
		t.Fatal(err)
	}
	// Migrations must be idempotent.
	if err := st.Migrate(ctx, log); err != nil {
		t.Fatal(err)
	}
	ref := &refresher{}
	n := notify.New(st, time.Second, log)
	h := api.New(st, ref, n, log, api.Options{AgentToken: agentToken, APIToken: apiToken}).Handler()
	srv := httptest.NewServer(h)
	t.Cleanup(func() { srv.Close(); st.Close() })
	return &env{t: t, srv: srv, store: st, ref: ref}
}

func (e *env) do(method, path, token string, body any) (int, map[string]any) {
	e.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, e.srv.URL+path, rd)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func report(hostname string, cpu float64) domain.AgentReport {
	return domain.AgentReport{
		Host:      domain.HostInfo{Hostname: hostname, OS: "linux", CPUCount: 4},
		IntervalS: 15,
		Metric:    domain.HostMetric{CPUPct: cpu, MemPct: 40, DiskPct: 50, MemTotal: 8 << 30, DiskTotal: 100 << 30, ProcCount: 120},
		Disks:     []domain.DiskUsage{{Mountpoint: "/", Total: 100 << 30, UsedPct: 50}},
		Processes: []domain.Process{{PID: 1, Name: "init"}},
	}
}

func TestHealthAndReadiness(t *testing.T) {
	e := setup(t, "")
	if code, _ := e.do("GET", "/healthz", "", nil); code != 200 {
		t.Fatalf("healthz = %d", code)
	}
	if code, body := e.do("GET", "/readyz", "", nil); code != 200 || body["database"] != "ok" {
		t.Fatalf("readyz = %d %v", code, body)
	}
}

func TestAgentIngestAndHosts(t *testing.T) {
	e := setup(t, "")
	if code, _ := e.do("POST", "/api/v1/agent/report", "wrong", report("web-1", 10)); code != 401 {
		t.Fatalf("bad token accepted: %d", code)
	}
	bad := report("", 10)
	if code, body := e.do("POST", "/api/v1/agent/report", agentToken, bad); code != 422 {
		t.Fatalf("invalid report = %d %v", code, body)
	}
	code, body := e.do("POST", "/api/v1/agent/report", agentToken, report("web-1", 10))
	if code != 202 {
		t.Fatalf("ingest = %d %v", code, body)
	}
	hostID := body["host_id"].(string)
	time.Sleep(5 * time.Millisecond)
	e.do("POST", "/api/v1/agent/report", agentToken, report("web-1", 20))

	_, body = e.do("GET", "/api/v1/hosts", "", nil)
	hosts := body["hosts"].([]any)
	if len(hosts) != 1 {
		t.Fatalf("hosts = %v", hosts)
	}
	h := hosts[0].(map[string]any)
	if h["status"] != "healthy" || h["latest"].(map[string]any)["cpu_pct"].(float64) != 20 {
		t.Fatalf("unexpected host %v", h)
	}

	code, body = e.do("GET", "/api/v1/hosts/"+hostID, "", nil)
	if code != 200 || len(body["snapshot"].(map[string]any)["processes"].([]any)) != 1 {
		t.Fatalf("host detail = %d %v", code, body)
	}
	code, body = e.do("GET", "/api/v1/hosts/"+hostID+"/metrics?range=1h", "", nil)
	if code != 200 || len(body["points"].([]any)) == 0 {
		t.Fatalf("metrics = %d %v", code, body)
	}
	if code, _ := e.do("GET", "/api/v1/hosts/not-a-uuid", "", nil); code != 404 {
		t.Fatalf("bad id = %d", code)
	}
	if code, _ := e.do("GET", "/api/v1/hosts/"+hostID+"/metrics?range=banana", "", nil); code != 422 {
		t.Fatalf("bad range = %d", code)
	}

	_, body = e.do("GET", "/api/v1/events", "", nil)
	if evs := body["events"].([]any); len(evs) != 1 || evs[0].(map[string]any)["kind"] != "host_registered" {
		t.Fatalf("events = %v", evs)
	}

	if code, _ := e.do("DELETE", "/api/v1/hosts/"+hostID, "", nil); code != 204 {
		t.Fatalf("delete = %d", code)
	}
	if code, _ := e.do("GET", "/api/v1/hosts/"+hostID, "", nil); code != 404 {
		t.Fatalf("deleted host = %d", code)
	}
}

func TestMonitorCRUDAndValidation(t *testing.T) {
	e := setup(t, "")
	code, body := e.do("POST", "/api/v1/monitors", "", map[string]any{
		"name": "", "url": "ftp://x", "interval_s": 5, "timeout_ms": 50, "method": "BREW",
	})
	if code != 422 {
		t.Fatalf("validation = %d %v", code, body)
	}
	fields := body["error"].(map[string]any)["fields"].(map[string]any)
	for _, f := range []string{"name", "url", "interval_s", "timeout_ms", "method"} {
		if _, ok := fields[f]; !ok {
			t.Errorf("expected field error for %s: %v", f, fields)
		}
	}
	if code, _ := e.do("POST", "/api/v1/monitors", "", map[string]any{"name": "x", "url": "http://a", "bogus": 1}); code != 400 {
		t.Fatalf("unknown field = %d", code)
	}

	code, body = e.do("POST", "/api/v1/monitors", "", map[string]any{
		"name": "API", "url": "https://example.com/health", "interval_s": 30, "timeout_ms": 2000,
		"headers": map[string]string{"X-Probe": "1"},
	})
	if code != 201 || e.ref.n != 1 {
		t.Fatalf("create = %d %v refresh=%d", code, body, e.ref.n)
	}
	id := body["id"].(string)

	ctx := context.Background()
	for i, ok := range []bool{true, true, false} {
		r := domain.CheckResult{MonitorID: id, CheckedAt: time.Now().Add(time.Duration(i-3) * time.Second), Success: ok, StatusCode: 200, ResponseMS: 100 * (i + 1)}
		if !ok {
			r.StatusCode, r.Error = 503, "unexpected status 503"
		}
		if err := e.store.InsertCheckResult(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	code, body = e.do("GET", "/api/v1/monitors/"+id, "", nil)
	if code != 200 || body["status"] != "down" || body["checks_total"].(float64) != 3 || body["last_error"] != "unexpected status 503" {
		t.Fatalf("summary = %d %v", code, body)
	}
	if up := body["uptime_pct"].(float64); up < 66 || up > 67 {
		t.Fatalf("uptime = %v", up)
	}

	code, body = e.do("PUT", "/api/v1/monitors/"+id, "", map[string]any{
		"name": "API v2", "url": "https://example.com/health", "interval_s": 60, "timeout_ms": 2000, "enabled": false,
	})
	if code != 200 || body["enabled"] != false {
		t.Fatalf("update = %d %v", code, body)
	}
	_, body = e.do("GET", "/api/v1/monitors", "", nil)
	if m := body["monitors"].([]any)[0].(map[string]any); m["status"] != "paused" {
		t.Fatalf("expected paused, got %v", m["status"])
	}
	if code, body = e.do("GET", "/api/v1/monitors/"+id+"/series?range=1h", "", nil); code != 200 {
		t.Fatalf("series = %d %v", code, body)
	}
	if code, _ = e.do("DELETE", "/api/v1/monitors/"+id, "", nil); code != 204 {
		t.Fatalf("delete = %d", code)
	}
	if code, _ = e.do("DELETE", "/api/v1/monitors/"+id, "", nil); code != 404 {
		t.Fatalf("second delete = %d", code)
	}
}

type captured struct {
	mu   sync.Mutex
	list []notify.Payload
}

func (c *captured) Notify(_ context.Context, p notify.Payload) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.list = append(c.list, p)
}

func TestAlertLifecycleAgainstPostgres(t *testing.T) {
	e := setup(t, "")
	ctx := context.Background()
	code, body := e.do("POST", "/api/v1/monitors", "", map[string]any{"name": "svc", "url": "http://svc.local", "interval_s": 10, "timeout_ms": 1000})
	if code != 201 {
		t.Fatal(body)
	}
	monID := body["id"].(string)
	for i := range 3 {
		_ = e.store.InsertCheckResult(ctx, domain.CheckResult{MonitorID: monID, CheckedAt: time.Now().Add(time.Duration(i-5) * time.Second), Error: "down"})
	}
	cap := &captured{}
	eng := alert.NewEngine(e.store, cap, slog.New(slog.NewTextHandler(io.Discard, nil)))

	// Evaluate concurrently to exercise the database-level dedup guarantee.
	var wg sync.WaitGroup
	for range 5 {
		wg.Add(1)
		go func() { defer wg.Done(); _ = eng.Evaluate(ctx) }()
	}
	wg.Wait()

	_, body = e.do("GET", "/api/v1/alerts?state=open", "", nil)
	alerts := body["alerts"].([]any)
	if len(alerts) != 1 || len(cap.list) != 1 {
		t.Fatalf("open alerts = %d, notifications = %d", len(alerts), len(cap.list))
	}
	a := alerts[0].(map[string]any)
	if a["rule_name"] != "Endpoint down" || a["state"] != "firing" {
		t.Fatalf("alert = %v", a)
	}
	alertID := a["id"].(string)

	if code, body = e.do("POST", "/api/v1/alerts/"+alertID+"/acknowledge", "", nil); code != 200 || body["state"] != "acknowledged" {
		t.Fatalf("ack = %d %v", code, body)
	}
	if code, _ = e.do("POST", "/api/v1/alerts/"+alertID+"/acknowledge", "", nil); code != 409 {
		t.Fatalf("second ack = %d", code)
	}

	_ = e.store.InsertCheckResult(ctx, domain.CheckResult{MonitorID: monID, CheckedAt: time.Now(), Success: true, StatusCode: 200, ResponseMS: 10})
	if err := eng.Evaluate(ctx); err != nil {
		t.Fatal(err)
	}
	_, body = e.do("GET", "/api/v1/alerts?state=resolved", "", nil)
	if len(body["alerts"].([]any)) != 1 || len(cap.list) != 2 || cap.list[1].Event != notify.EventResolved {
		t.Fatalf("resolved = %v notifications = %d", body, len(cap.list))
	}

	code, body = e.do("GET", "/api/v1/overview?range=1h", "", nil)
	if code != 200 || body["http"].(map[string]any)["total"].(float64) != 1 {
		t.Fatalf("overview = %d %v", code, body)
	}
}

func TestAlertRuleValidation(t *testing.T) {
	e := setup(t, "")
	code, body := e.do("POST", "/api/v1/alert-rules", "", map[string]any{"name": "x", "kind": "host_metric", "metric": "nope", "severity": "meh"})
	if code != 422 {
		t.Fatalf("validation = %d %v", code, body)
	}
	code, body = e.do("POST", "/api/v1/alert-rules", "", map[string]any{
		"name": "x", "kind": "host_metric", "metric": "cpu_pct", "threshold": 80, "severity": "warning",
		"host_id": "00000000-0000-0000-0000-000000000000",
	})
	if code != 422 || !strings.Contains(body["error"].(map[string]any)["fields"].(map[string]any)["host_id"].(string), "does not exist") {
		t.Fatalf("missing host = %d %v", code, body)
	}
	code, body = e.do("POST", "/api/v1/alert-rules", "", map[string]any{
		"name": "Load", "kind": "host_metric", "metric": "load1", "operator": ">=", "threshold": 8, "for_count": 2, "severity": "info",
	})
	if code != 201 {
		t.Fatalf("create = %d %v", code, body)
	}
	_, body = e.do("GET", "/api/v1/alert-rules", "", nil)
	if n := len(body["rules"].([]any)); n != 7 { // 6 seeded + 1
		t.Fatalf("rules = %d", n)
	}
}

func TestWebhookTestEndpoint(t *testing.T) {
	e := setup(t, "")
	got := make(chan string, 1)
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- r.Header.Get("X-Pulsewatch-Signature")
	}))
	defer hook.Close()
	code, body := e.do("POST", "/api/v1/webhooks", "", map[string]any{"name": "ops", "url": hook.URL, "secret": "s"})
	if code != 201 || body["has_secret"] != true || body["secret"] != nil {
		t.Fatalf("create = %d %v", code, body)
	}
	if code, body = e.do("POST", "/api/v1/webhooks/"+body["id"].(string)+"/test", "", nil); code != 200 {
		t.Fatalf("test = %d %v", code, body)
	}
	if sig := <-got; !strings.HasPrefix(sig, "sha256=") {
		t.Fatalf("signature = %q", sig)
	}
}

func TestAPITokenEnforced(t *testing.T) {
	e := setup(t, "api-secret")
	if code, _ := e.do("GET", "/api/v1/hosts", "", nil); code != 401 {
		t.Fatalf("unauthenticated = %d", code)
	}
	if code, _ := e.do("GET", "/api/v1/hosts", "api-secret", nil); code != 200 {
		t.Fatalf("authenticated = %d", code)
	}
	// Probes stay open for orchestrators.
	if code, _ := e.do("GET", "/healthz", "", nil); code != 200 {
		t.Fatalf("healthz = %d", code)
	}
}
