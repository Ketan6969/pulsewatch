// Package sysmon collects host resource metrics using gopsutil.
//
// When running inside a container, set HOST_PROC, HOST_SYS and HOST_ROOT
// (e.g. /host/proc, /host/sys, /hostfs) so the collector reads the host's
// view rather than the container's.
package sysmon

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"

	"pulsewatch/internal/domain"
)

// Collector produces AgentReports. Rates (network throughput, per-process
// CPU) are computed from the delta to the previous call, so a Collector
// must be reused across collections. It is safe for concurrent use but
// collections are serialized.
type Collector struct {
	mu       sync.Mutex
	name     string
	version  string
	topProcs int
	rootFS   string

	prevNetAt  time.Time
	prevNet    map[string]net.IOCountersStat
	prevProcAt time.Time
	prevProc   map[int32]procTimes
}

type procTimes struct {
	total  float64 // seconds of CPU time
	create int64
}

// NewCollector creates a collector. name overrides the reported display
// name; topProcs limits the number of processes included in each report.
func NewCollector(name, version string, topProcs int) *Collector {
	return &Collector{
		name:     name,
		version:  version,
		topProcs: topProcs,
		rootFS:   os.Getenv("HOST_ROOT"),
	}
}

// Collect gathers a full report. Individual subsystem failures are
// tolerated: the report is still returned along with a joined error so the
// caller can log what was missing.
func (c *Collector) Collect(ctx context.Context) (domain.AgentReport, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var (
		r    domain.AgentReport
		errs []error
	)
	now := time.Now()

	info, err := host.InfoWithContext(ctx)
	if err != nil {
		return r, fmt.Errorf("host info: %w", err)
	}
	r.Host = domain.HostInfo{
		Hostname: info.Hostname, Name: c.name, OS: info.OS, Platform: info.Platform,
		PlatformVersion: info.PlatformVersion, KernelVersion: info.KernelVersion,
		Arch: info.KernelArch, AgentVersion: c.version,
	}
	if r.Host.Name == "" {
		r.Host.Name = info.Hostname
	}
	if info.BootTime > 0 {
		bt := time.Unix(int64(info.BootTime), 0).UTC()
		r.Host.BootTime = &bt
	}
	r.Metric.UptimeS = info.Uptime
	r.Metric.ProcCount = int(info.Procs)

	if n, err := cpu.CountsWithContext(ctx, true); err == nil {
		r.Host.CPUCount = n
	} else {
		r.Host.CPUCount = runtime.NumCPU()
	}

	// Interval 0 compares against the previous call (or boot on the first call).
	if pct, err := cpu.PercentWithContext(ctx, 0, false); err == nil && len(pct) > 0 {
		r.Metric.CPUPct = clampPct(pct[0])
	} else if err != nil {
		errs = append(errs, fmt.Errorf("cpu: %w", err))
	}

	if vm, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		r.Metric.MemPct = clampPct(vm.UsedPercent)
		r.Metric.MemUsed = vm.Used
		r.Metric.MemTotal = vm.Total
	} else {
		errs = append(errs, fmt.Errorf("memory: %w", err))
	}
	if sw, err := mem.SwapMemoryWithContext(ctx); err == nil {
		r.Metric.SwapPct = clampPct(sw.UsedPercent)
	}

	if avg, err := load.AvgWithContext(ctx); err == nil {
		r.Metric.Load1, r.Metric.Load5, r.Metric.Load15 = avg.Load1, avg.Load5, avg.Load15
	}

	disks, err := c.disks(ctx)
	if err != nil {
		errs = append(errs, fmt.Errorf("disk: %w", err))
	}
	r.Disks = disks
	if primary := primaryDisk(disks); primary != nil {
		r.Metric.DiskPct = primary.UsedPct
		r.Metric.DiskUsed = primary.Used
		r.Metric.DiskTotal = primary.Total
	}

	ifaces, rx, tx, err := c.network(ctx, now)
	if err != nil {
		errs = append(errs, fmt.Errorf("network: %w", err))
	}
	r.Interfaces = ifaces
	r.Metric.NetRxBps, r.Metric.NetTxBps = rx, tx

	procs, err := c.processes(ctx, now, r.Metric.MemTotal)
	if err != nil {
		errs = append(errs, fmt.Errorf("processes: %w", err))
	}
	r.Processes = procs
	if r.Metric.ProcCount == 0 {
		r.Metric.ProcCount = len(c.prevProc)
	}

	return r, errors.Join(errs...)
}

var ignoredFS = map[string]bool{
	"squashfs": true, "tmpfs": true, "devtmpfs": true, "overlay": true, "iso9660": true,
	"nsfs": true, "tracefs": true, "autofs": true,
}

func (c *Collector) disks(ctx context.Context) ([]domain.DiskUsage, error) {
	parts, err := disk.PartitionsWithContext(ctx, false)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []domain.DiskUsage
	for _, p := range parts {
		if ignoredFS[p.Fstype] || seen[p.Device] || strings.HasPrefix(p.Mountpoint, "/snap/") {
			continue
		}
		path := p.Mountpoint
		if c.rootFS != "" {
			path = filepath.Join(c.rootFS, p.Mountpoint)
		}
		u, err := disk.UsageWithContext(ctx, path)
		if err != nil || u.Total == 0 {
			continue
		}
		seen[p.Device] = true
		out = append(out, domain.DiskUsage{
			Mountpoint: p.Mountpoint, Device: p.Device, FSType: p.Fstype,
			Total: u.Total, Used: u.Used, UsedPct: clampPct(u.UsedPercent),
		})
	}
	if len(out) == 0 {
		// Fall back to the root filesystem (common in minimal containers).
		root := "/"
		if c.rootFS != "" {
			root = c.rootFS
		}
		u, err := disk.UsageWithContext(ctx, root)
		if err != nil {
			return nil, err
		}
		out = append(out, domain.DiskUsage{Mountpoint: "/", FSType: u.Fstype, Total: u.Total, Used: u.Used, UsedPct: clampPct(u.UsedPercent)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Mountpoint < out[j].Mountpoint })
	return out, nil
}

// primaryDisk picks the root mount, or the largest filesystem otherwise.
func primaryDisk(disks []domain.DiskUsage) *domain.DiskUsage {
	var best *domain.DiskUsage
	for i := range disks {
		d := &disks[i]
		if d.Mountpoint == "/" {
			return d
		}
		if best == nil || d.Total > best.Total {
			best = d
		}
	}
	return best
}

func (c *Collector) network(ctx context.Context, now time.Time) ([]domain.NetInterface, float64, float64, error) {
	counters, err := net.IOCountersWithContext(ctx, true)
	if err != nil {
		return nil, 0, 0, err
	}
	ifs, _ := net.InterfacesWithContext(ctx) // addresses are best effort
	meta := make(map[string]net.InterfaceStat, len(ifs))
	for _, i := range ifs {
		meta[i.Name] = i
	}

	elapsed := now.Sub(c.prevNetAt).Seconds()
	cur := make(map[string]net.IOCountersStat, len(counters))
	var out []domain.NetInterface
	var totalRx, totalTx float64
	for _, ct := range counters {
		cur[ct.Name] = ct
		if ct.Name == "lo" {
			continue
		}
		ni := domain.NetInterface{
			Name: ct.Name, BytesRecv: ct.BytesRecv, BytesSent: ct.BytesSent,
			Errin: ct.Errin, Errout: ct.Errout,
		}
		if m, ok := meta[ct.Name]; ok {
			ni.MAC = m.HardwareAddr
			for _, f := range m.Flags {
				if f == "up" {
					ni.Up = true
				}
			}
			for _, a := range m.Addrs {
				ni.Addrs = append(ni.Addrs, a.Addr)
			}
		}
		if prev, ok := c.prevNet[ct.Name]; ok && elapsed > 0 {
			ni.RxBps = rate(prev.BytesRecv, ct.BytesRecv, elapsed)
			ni.TxBps = rate(prev.BytesSent, ct.BytesSent, elapsed)
		}
		totalRx += ni.RxBps
		totalTx += ni.TxBps
		out = append(out, ni)
	}
	c.prevNet, c.prevNetAt = cur, now
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, totalRx, totalTx, nil
}

// rate returns bytes/second; counter resets (e.g. interface re-creation) yield 0.
func rate(prev, cur uint64, seconds float64) float64 {
	if cur < prev || seconds <= 0 {
		return 0
	}
	return float64(cur-prev) / seconds
}

func (c *Collector) processes(ctx context.Context, now time.Time, memTotal uint64) ([]domain.Process, error) {
	procs, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return nil, err
	}
	elapsed := now.Sub(c.prevProcAt).Seconds()
	cur := make(map[int32]procTimes, len(procs))
	out := make([]domain.Process, 0, len(procs))
	for _, p := range procs {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		times, err := p.TimesWithContext(ctx)
		if err != nil {
			continue // process exited or is inaccessible
		}
		create, _ := p.CreateTimeWithContext(ctx)
		total := times.User + times.System
		cur[p.Pid] = procTimes{total: total, create: create}

		dp := domain.Process{PID: p.Pid, CreateTime: create}
		if prev, ok := c.prevProc[p.Pid]; ok && prev.create == create && elapsed > 0 && total >= prev.total {
			dp.CPUPct = 100 * (total - prev.total) / elapsed
		}
		if mi, err := p.MemoryInfoWithContext(ctx); err == nil {
			dp.RSS = mi.RSS
			if memTotal > 0 {
				dp.MemPct = float32(100 * float64(mi.RSS) / float64(memTotal))
			}
		}
		out = append(out, dp)
	}
	c.prevProc, c.prevProcAt = cur, now

	sort.Slice(out, func(i, j int) bool {
		if out[i].CPUPct != out[j].CPUPct {
			return out[i].CPUPct > out[j].CPUPct
		}
		return out[i].RSS > out[j].RSS
	})
	if len(out) > c.topProcs {
		out = out[:c.topProcs]
	}
	// Only enrich the processes we report; these lookups are comparatively expensive.
	for i := range out {
		p := &process.Process{Pid: out[i].PID}
		out[i].Name, _ = p.NameWithContext(ctx)
		out[i].User, _ = p.UsernameWithContext(ctx)
		if st, err := p.StatusWithContext(ctx); err == nil && len(st) > 0 {
			out[i].Status = st[0]
		}
		if cmd, err := p.CmdlineWithContext(ctx); err == nil {
			out[i].Cmdline = truncate(cmd, 512)
		}
	}
	return out, nil
}

func clampPct(v float64) float64 {
	switch {
	case v < 0:
		return 0
	case v > 100:
		return 100
	default:
		return v
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
