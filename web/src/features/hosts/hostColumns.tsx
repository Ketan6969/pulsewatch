// Column definitions shared by the host tables.
import type { Host } from "../../api/types";
import type { Column } from "../../components/ui/DataTable";
import { Meter } from "../../components/ui/primitives";
import { HealthIndicator } from "../../components/ui/status";
import { bitrate, duration, relative, timestamp, utilTone } from "../../lib/format";

export function UtilCell({ value, label }: { value: number | undefined; label: string }) {
  if (value === undefined) return <span className="text-muted">—</span>;
  return (
    <div className="meter-cell">
      <span className="num" style={{ textAlign: "right" }}>
        {value.toFixed(1)}%
      </span>
      <Meter value={value} tone={utilTone(value)} label={label} />
    </div>
  );
}

const STATUS_ORDER = { offline: 0, degraded: 1, healthy: 2 } as const;

export const hostColumns: Column<Host>[] = [
  {
    key: "name",
    header: "Host",
    sort: (h) => h.name,
    render: (h) => (
      <div style={{ minWidth: 0 }}>
        <div className="truncate" style={{ fontWeight: 500 }}>
          {h.name}
        </div>
        {h.name !== h.hostname && <div className="mono text-muted truncate" style={{ fontSize: "var(--text-xs)" }}>{h.hostname}</div>}
      </div>
    ),
  },
  { key: "status", header: "Status", sort: (h) => STATUS_ORDER[h.status], render: (h) => <HealthIndicator status={h.status} /> },
  {
    key: "os",
    header: "OS",
    hideOnMobile: true,
    sort: (h) => `${h.platform} ${h.platform_version}`,
    render: (h) => (
      <span className="truncate text-2">
        {h.platform || h.os} {h.platform_version}
      </span>
    ),
  },
  { key: "cpu", header: "CPU", width: 130, sort: (h) => h.latest?.cpu_pct, render: (h) => <UtilCell value={h.latest?.cpu_pct} label="CPU" /> },
  { key: "mem", header: "Memory", width: 130, sort: (h) => h.latest?.mem_pct, render: (h) => <UtilCell value={h.latest?.mem_pct} label="Memory" /> },
  { key: "disk", header: "Disk", width: 130, hideOnMobile: true, sort: (h) => h.latest?.disk_pct, render: (h) => <UtilCell value={h.latest?.disk_pct} label="Disk" /> },
  {
    key: "net",
    header: "Network",
    align: "right",
    hideOnMobile: true,
    sort: (h) => (h.latest ? h.latest.net_rx_bps + h.latest.net_tx_bps : undefined),
    render: (h) => <span className="num">{h.latest ? bitrate(h.latest.net_rx_bps + h.latest.net_tx_bps) : "—"}</span>,
  },
  {
    key: "uptime",
    header: "Uptime",
    align: "right",
    hideOnMobile: true,
    sort: (h) => h.latest?.uptime_s,
    render: (h) => <span className="num">{duration(h.latest?.uptime_s)}</span>,
  },
  {
    key: "seen",
    header: "Last heartbeat",
    align: "right",
    sort: (h) => Date.parse(h.last_seen_at),
    render: (h) => (
      <span className="num text-2" title={timestamp(h.last_seen_at)}>
        {relative(h.last_seen_at)}
      </span>
    ),
  },
];
