package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"pulsewatch/internal/domain"
)

// OfflineAfter returns how long a host may be silent before it is considered
// offline: three missed reports, never less than 30 seconds.
func OfflineAfter(intervalS int) time.Duration {
	d := 3 * time.Duration(intervalS) * time.Second
	if d < 30*time.Second {
		d = 30 * time.Second
	}
	return d
}

// IngestReport upserts the host, appends a metric sample and replaces the
// latest snapshot, atomically. It returns the host ID and whether the host
// was newly registered.
func (s *Store) IngestReport(ctx context.Context, r domain.AgentReport) (string, bool, error) {
	disks, err := json.Marshal(nonNil(r.Disks))
	if err != nil {
		return "", false, err
	}
	ifaces, err := json.Marshal(nonNil(r.Interfaces))
	if err != nil {
		return "", false, err
	}
	procs, err := json.Marshal(nonNil(r.Processes))
	if err != nil {
		return "", false, err
	}
	name := r.Host.Name
	if name == "" {
		name = r.Host.Hostname
	}
	var (
		id      string
		created bool
	)
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		// xmax = 0 identifies a freshly inserted row in an upsert.
		err := tx.QueryRow(ctx, `
			INSERT INTO hosts (name, hostname, os, platform, platform_version, kernel_version, arch,
			                   cpu_count, agent_version, boot_time, report_interval_s, last_seen_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11, now())
			ON CONFLICT (hostname) DO UPDATE SET
				name = EXCLUDED.name, os = EXCLUDED.os, platform = EXCLUDED.platform,
				platform_version = EXCLUDED.platform_version, kernel_version = EXCLUDED.kernel_version,
				arch = EXCLUDED.arch, cpu_count = EXCLUDED.cpu_count, agent_version = EXCLUDED.agent_version,
				boot_time = EXCLUDED.boot_time, report_interval_s = EXCLUDED.report_interval_s,
				last_seen_at = now()
			RETURNING id, (xmax = 0)`,
			name, r.Host.Hostname, r.Host.OS, r.Host.Platform, r.Host.PlatformVersion,
			r.Host.KernelVersion, r.Host.Arch, r.Host.CPUCount, r.Host.AgentVersion,
			r.Host.BootTime, r.IntervalS,
		).Scan(&id, &created)
		if err != nil {
			return fmt.Errorf("upsert host: %w", err)
		}
		m := r.Metric
		// The server's clock is authoritative to avoid agent clock skew.
		_, err = tx.Exec(ctx, `
			INSERT INTO host_metrics (host_id, ts, cpu_pct, mem_pct, mem_used, mem_total, swap_pct,
			    disk_pct, disk_used, disk_total, net_rx_bps, net_tx_bps, load1, load5, load15, uptime_s, proc_count)
			VALUES ($1, now(), $2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
			ON CONFLICT (host_id, ts) DO NOTHING`,
			id, m.CPUPct, m.MemPct, int64(m.MemUsed), int64(m.MemTotal), m.SwapPct, m.DiskPct,
			int64(m.DiskUsed), int64(m.DiskTotal), m.NetRxBps, m.NetTxBps, m.Load1, m.Load5, m.Load15,
			int64(m.UptimeS), m.ProcCount)
		if err != nil {
			return fmt.Errorf("insert metric: %w", err)
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO host_snapshots (host_id, collected_at, disks, interfaces, processes)
			VALUES ($1, now(), $2, $3, $4)
			ON CONFLICT (host_id) DO UPDATE SET collected_at = EXCLUDED.collected_at,
				disks = EXCLUDED.disks, interfaces = EXCLUDED.interfaces, processes = EXCLUDED.processes`,
			id, disks, ifaces, procs)
		if err != nil {
			return fmt.Errorf("upsert snapshot: %w", err)
		}
		return nil
	})
	return id, created, err
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

const hostSelect = `
	SELECT h.id, h.name, h.hostname, h.os, h.platform, h.platform_version, h.kernel_version, h.arch,
	       h.cpu_count, h.agent_version, h.boot_time, h.report_interval_s, h.last_seen_at, h.created_at,
	       EXISTS (SELECT 1 FROM alerts a WHERE a.target_id = h.id AND a.state <> 'resolved') AS has_alert,
	       m.ts, m.cpu_pct, m.mem_pct, m.mem_used, m.mem_total, m.swap_pct, m.disk_pct, m.disk_used,
	       m.disk_total, m.net_rx_bps, m.net_tx_bps, m.load1, m.load5, m.load15, m.uptime_s, m.proc_count
	  FROM hosts h
	  LEFT JOIN LATERAL (
	       SELECT * FROM host_metrics hm WHERE hm.host_id = h.id ORDER BY hm.ts DESC LIMIT 1
	  ) m ON true`

func scanHost(r pgx.Row, now time.Time) (domain.Host, error) {
	var (
		h                                              domain.Host
		hasAlert                                       bool
		ts                                             *time.Time
		cpu, mem, swap, disk, rx, tx, l1, l5, l15      *float64
		memUsed, memTotal, diskUsed, diskTotal, uptime *int64
		procs                                          *int
	)
	err := r.Scan(&h.ID, &h.Name, &h.Hostname, &h.OS, &h.Platform, &h.PlatformVersion, &h.KernelVersion,
		&h.Arch, &h.CPUCount, &h.AgentVersion, &h.BootTime, &h.ReportInterval, &h.LastSeenAt, &h.CreatedAt,
		&hasAlert, &ts, &cpu, &mem, &memUsed, &memTotal, &swap, &disk, &diskUsed, &diskTotal, &rx, &tx,
		&l1, &l5, &l15, &uptime, &procs)
	if err != nil {
		return h, err
	}
	if ts != nil {
		h.Latest = &domain.HostMetric{
			Time: *ts, CPUPct: *cpu, MemPct: *mem, MemUsed: uint64(*memUsed), MemTotal: uint64(*memTotal),
			SwapPct: *swap, DiskPct: *disk, DiskUsed: uint64(*diskUsed), DiskTotal: uint64(*diskTotal),
			NetRxBps: *rx, NetTxBps: *tx, Load1: *l1, Load5: *l5, Load15: *l15, UptimeS: uint64(*uptime),
			ProcCount: *procs,
		}
	}
	h.Status = DeriveHostStatus(h.LastSeenAt, h.ReportInterval, hasAlert, now)
	return h, nil
}

// DeriveHostStatus computes host health from heartbeat freshness and open alerts.
func DeriveHostStatus(lastSeen time.Time, intervalS int, hasOpenAlert bool, now time.Time) domain.HostStatus {
	switch {
	case now.Sub(lastSeen) > OfflineAfter(intervalS):
		return domain.HostOffline
	case hasOpenAlert:
		return domain.HostDegraded
	default:
		return domain.HostHealthy
	}
}

// ListHosts returns all hosts with their latest sample and derived status.
func (s *Store) ListHosts(ctx context.Context) ([]domain.Host, error) {
	rows, err := s.pool.Query(ctx, hostSelect+` ORDER BY h.name`)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.Host, error) { return scanHost(r, now) })
}

// GetHost returns a single host.
func (s *Store) GetHost(ctx context.Context, id string) (domain.Host, error) {
	h, err := scanHost(s.pool.QueryRow(ctx, hostSelect+` WHERE h.id = $1`, id), time.Now())
	return h, mapErr(err)
}

// GetHostSnapshot returns the latest detailed snapshot for a host.
func (s *Store) GetHostSnapshot(ctx context.Context, id string) (domain.HostSnapshot, error) {
	var (
		snap                 domain.HostSnapshot
		disks, ifaces, procs []byte
	)
	err := s.pool.QueryRow(ctx,
		`SELECT collected_at, disks, interfaces, processes FROM host_snapshots WHERE host_id = $1`, id,
	).Scan(&snap.CollectedAt, &disks, &ifaces, &procs)
	if err != nil {
		return snap, mapErr(err)
	}
	if err := json.Unmarshal(disks, &snap.Disks); err != nil {
		return snap, err
	}
	if err := json.Unmarshal(ifaces, &snap.Interfaces); err != nil {
		return snap, err
	}
	if err := json.Unmarshal(procs, &snap.Processes); err != nil {
		return snap, err
	}
	return snap, nil
}

// DeleteHost removes a host and all of its data.
func (s *Store) DeleteHost(ctx context.Context, id string) error {
	var name string
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `DELETE FROM hosts WHERE id = $1 RETURNING name`, id).Scan(&name); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `DELETE FROM alerts WHERE target_type = 'host' AND target_id = $1`, id)
		return err
	})
	return mapErr(err)
}

// HostSeries returns bucketed averages for one host, or all hosts when hostID is empty.
func (s *Store) HostSeries(ctx context.Context, hostID string, from, to time.Time) ([]domain.SeriesPoint, time.Duration, error) {
	step := BucketStep(to.Sub(from))
	// Network throughput is summed across hosts within a bucket; percentages are averaged.
	q := `
		WITH per_host AS (
			SELECT host_id, date_bin($1::interval, ts, 'epoch'::timestamptz) AS bucket,
			       avg(cpu_pct) cpu, avg(mem_pct) mem, avg(disk_pct) disk, avg(swap_pct) swap,
			       avg(net_rx_bps) rx, avg(net_tx_bps) tx, avg(load1) load1
			  FROM host_metrics
			 WHERE ts >= $2 AND ts <= $3 AND ($4 = '' OR host_id::text = $4)
			 GROUP BY host_id, bucket
		)
		SELECT bucket, avg(cpu)::float8, avg(mem)::float8, avg(disk)::float8, avg(swap)::float8, sum(rx)::float8, sum(tx)::float8, avg(load1)::float8
		  FROM per_host GROUP BY bucket ORDER BY bucket`
	rows, err := s.pool.Query(ctx, q, step, from, to, hostID)
	if err != nil {
		return nil, step, err
	}
	pts, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.SeriesPoint, error) {
		var (
			p                                  domain.SeriesPoint
			cpu, mem, disk, swap, rx, tx, load float64
		)
		err := r.Scan(&p.Time, &cpu, &mem, &disk, &swap, &rx, &tx, &load)
		p.Values = map[string]float64{
			"cpu_pct": cpu, "mem_pct": mem, "disk_pct": disk, "swap_pct": swap,
			"net_rx_bps": rx, "net_tx_bps": tx, "load1": load,
		}
		return p, err
	})
	return pts, step, err
}

// RecentHostMetrics returns the newest n samples for a host, newest first.
func (s *Store) RecentHostMetrics(ctx context.Context, hostID string, n int) ([]domain.HostMetric, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT ts, cpu_pct, mem_pct, swap_pct, disk_pct, load1
		  FROM host_metrics WHERE host_id = $1 ORDER BY ts DESC LIMIT $2`, hostID, n)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.HostMetric, error) {
		var m domain.HostMetric
		err := r.Scan(&m.Time, &m.CPUPct, &m.MemPct, &m.SwapPct, &m.DiskPct, &m.Load1)
		return m, err
	})
}
