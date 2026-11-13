package sysmon

import (
	"context"
	"runtime"
	"testing"
	"time"

	"pulsewatch/internal/domain"
)

func TestRate(t *testing.T) {
	if got := rate(100, 300, 2); got != 100 {
		t.Errorf("rate = %v, want 100", got)
	}
	if got := rate(300, 100, 2); got != 0 {
		t.Errorf("counter reset should yield 0, got %v", got)
	}
	if got := rate(0, 100, 0); got != 0 {
		t.Errorf("zero elapsed should yield 0, got %v", got)
	}
}

func TestPrimaryDisk(t *testing.T) {
	disks := []domain.DiskUsage{{Mountpoint: "/data", Total: 500}, {Mountpoint: "/", Total: 100}}
	if p := primaryDisk(disks); p.Mountpoint != "/" {
		t.Errorf("primary = %s, want /", p.Mountpoint)
	}
	disks = []domain.DiskUsage{{Mountpoint: "/a", Total: 5}, {Mountpoint: "/b", Total: 50}}
	if p := primaryDisk(disks); p.Mountpoint != "/b" {
		t.Errorf("primary = %s, want /b", p.Mountpoint)
	}
	if primaryDisk(nil) != nil {
		t.Error("expected nil for no disks")
	}
}

func TestClampPct(t *testing.T) {
	for in, want := range map[float64]float64{-1: 0, 50: 50, 101: 100} {
		if got := clampPct(in); got != want {
			t.Errorf("clampPct(%v) = %v", in, got)
		}
	}
}

func TestCollectLive(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("live collection is only exercised on linux")
	}
	c := NewCollector("", "test", 5)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	r, err := c.Collect(ctx)
	if err != nil {
		t.Logf("partial collection: %v", err)
	}
	if r.Host.Hostname == "" {
		t.Fatal("hostname missing")
	}
	if r.Metric.MemTotal == 0 {
		t.Error("memory total missing")
	}
	if len(r.Processes) == 0 || len(r.Processes) > 5 {
		t.Errorf("got %d processes, want 1..5", len(r.Processes))
	}
	// Second collection exercises the rate paths.
	time.Sleep(200 * time.Millisecond)
	if _, err := c.Collect(ctx); err != nil {
		t.Logf("partial collection: %v", err)
	}
}
