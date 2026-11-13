package agent

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"pulsewatch/internal/domain"
)

type fakeCollector struct{}

func (fakeCollector) Collect(context.Context) (domain.AgentReport, error) {
	return domain.AgentReport{Host: domain.HostInfo{Hostname: "h1"}, Metric: domain.HostMetric{CPUPct: 12}}, nil
}

func TestAgentSendsAuthenticatedReports(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("missing auth header")
		}
		var rep domain.AgentReport
		if err := json.NewDecoder(r.Body).Decode(&rep); err != nil {
			t.Errorf("decode: %v", err)
		}
		if rep.Host.Hostname != "h1" || rep.IntervalS != 1 {
			t.Errorf("unexpected report %+v", rep)
		}
		calls.Add(1)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	a := New(srv.URL, "secret", time.Second, fakeCollector{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	if err := a.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if n := calls.Load(); n < 2 {
		t.Fatalf("expected at least 2 reports, got %d", n)
	}
}
