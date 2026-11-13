// Package agent runs the collection loop that pushes host metrics to the server.
package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"pulsewatch/internal/domain"
)

// Collector produces reports; implemented by sysmon.Collector.
type Collector interface {
	Collect(ctx context.Context) (domain.AgentReport, error)
}

// Agent periodically collects and ships reports.
type Agent struct {
	serverURL string
	token     string
	interval  time.Duration
	collector Collector
	client    *http.Client
	log       *slog.Logger
}

// New creates an agent.
func New(serverURL, token string, interval time.Duration, c Collector, log *slog.Logger) *Agent {
	return &Agent{
		serverURL: serverURL,
		token:     token,
		interval:  interval,
		collector: c,
		client:    &http.Client{Timeout: 10 * time.Second},
		log:       log,
	}
}

// Run blocks until ctx is cancelled. A failed send is logged and the next
// tick proceeds normally; metrics are point-in-time so we do not buffer.
func (a *Agent) Run(ctx context.Context) error {
	ticker := time.NewTicker(a.interval)
	defer ticker.Stop()
	a.tick(ctx) // report immediately on start
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			a.tick(ctx)
		}
	}
}

func (a *Agent) tick(ctx context.Context) {
	cctx, cancel := context.WithTimeout(ctx, a.interval)
	defer cancel()
	report, err := a.collector.Collect(cctx)
	if err != nil {
		if report.Host.Hostname == "" {
			a.log.Error("collection failed", "err", err)
			return
		}
		a.log.Warn("partial collection", "err", err)
	}
	report.IntervalS = int(a.interval / time.Second)
	if err := a.send(cctx, report); err != nil {
		if ctx.Err() == nil {
			a.log.Error("report failed", "err", err)
		}
		return
	}
	a.log.Debug("report sent", "cpu_pct", report.Metric.CPUPct, "mem_pct", report.Metric.MemPct)
}

func (a *Agent) send(ctx context.Context, r domain.AgentReport) error {
	body, err := json.Marshal(r)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.serverURL+"/api/v1/agent/report", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.token)
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, bytes.TrimSpace(msg))
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	return nil
}
