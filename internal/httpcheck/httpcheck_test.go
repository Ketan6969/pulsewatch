package httpcheck

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"pulsewatch/internal/domain"
)

func monitor(url string) domain.Monitor {
	return domain.Monitor{ID: "m1", Name: "test", URL: url, Method: http.MethodGet, IntervalS: 10, TimeoutMS: 500}
}

func TestCheckSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Token") != "abc" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = io.WriteString(w, "ok")
	}))
	defer srv.Close()
	m := monitor(srv.URL)
	m.Headers = map[string]string{"X-Token": "abc"}
	res := NewChecker().Check(context.Background(), m)
	if !res.Success || res.StatusCode != 200 || res.Error != "" {
		t.Fatalf("unexpected result %+v", res)
	}
}

func TestCheckUnexpectedStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	res := NewChecker().Check(context.Background(), monitor(srv.URL))
	if res.Success || res.StatusCode != 503 || !strings.Contains(res.Error, "503") {
		t.Fatalf("unexpected result %+v", res)
	}
}

func TestCheckExpectedStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	m := monitor(srv.URL)
	m.ExpectedStatus = 204
	if res := NewChecker().Check(context.Background(), m); !res.Success {
		t.Fatalf("expected success, got %+v", res)
	}
	m.ExpectedStatus = 200
	if res := NewChecker().Check(context.Background(), m); res.Success {
		t.Fatalf("expected failure, got %+v", res)
	}
}

func TestCheckTimeout(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(release)
	m := monitor(srv.URL)
	m.TimeoutMS = 100
	start := time.Now()
	res := NewChecker().Check(context.Background(), m)
	if res.Success || !strings.Contains(res.Error, "timeout") {
		t.Fatalf("expected timeout, got %+v", res)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("timeout not enforced")
	}
}

func TestCheckConnectionRefused(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	res := NewChecker().Check(context.Background(), monitor(url))
	if res.Success || res.Error == "" {
		t.Fatalf("expected failure, got %+v", res)
	}
}

type memStore struct {
	mu       sync.Mutex
	monitors []domain.Monitor
	results  []domain.CheckResult
}

func (m *memStore) ListMonitors(context.Context) ([]domain.Monitor, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]domain.Monitor(nil), m.monitors...), nil
}

func (m *memStore) InsertCheckResult(_ context.Context, r domain.CheckResult) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.results = append(m.results, r)
	return nil
}

func (m *memStore) count(id string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, r := range m.results {
		if r.MonitorID == id {
			n++
		}
	}
	return n
}

func TestSchedulerReconcile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	st := &memStore{monitors: []domain.Monitor{
		{ID: "a", URL: srv.URL, Method: "GET", IntervalS: 1, TimeoutMS: 500, Enabled: true},
		{ID: "b", URL: srv.URL, Method: "GET", IntervalS: 1, TimeoutMS: 500, Enabled: false},
	}}
	s := NewScheduler(st, NewChecker(), 4, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()

	waitFor(t, func() bool { return st.count("a") >= 1 })
	if st.count("b") != 0 {
		t.Fatal("disabled monitor was checked")
	}
	if s.Running() != 1 {
		t.Fatalf("running = %d, want 1", s.Running())
	}

	// Enable b, disable a.
	st.mu.Lock()
	st.monitors[0].Enabled = false
	st.monitors[1].Enabled = true
	st.mu.Unlock()
	s.Refresh()
	waitFor(t, func() bool { return st.count("b") >= 1 })

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("scheduler did not stop")
	}
	if s.Running() != 0 {
		t.Fatal("runners left after shutdown")
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("condition not met before deadline")
}
