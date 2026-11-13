// Fleet-level view: a status grid of every host plus aggregate resource charts.
import { Server } from "lucide-react";
import { Link } from "react-router-dom";
import { useHostSeries, useHosts } from "../api/queries";
import type { Host } from "../api/types";
import { CPU_MEM, DISK, LOAD, NETWORK, fmtBitrate, fmtNum, fmtPct } from "../components/charts/lines";
import { TimeSeriesChart } from "../components/charts/TimeSeriesChart";
import { Meter, Panel } from "../components/ui/primitives";
import { EmptyState, ErrorState, Skeleton } from "../components/ui/states";
import { HealthIndicator } from "../components/ui/status";
import { Page, usePageMeta } from "../layout/AppShell";
import { relative, utilTone } from "../lib/format";

function HostTile({ host }: { host: Host }) {
  const m = host.latest;
  const rows: [string, number | undefined][] = [
    ["CPU", m?.cpu_pct],
    ["MEM", m?.mem_pct],
    ["DISK", m?.disk_pct],
  ];
  return (
    <Link
      to={`/hosts/${host.id}`}
      className="metric"
      style={{
        color: "inherit",
        borderTop: `3px solid var(--color-${host.status === "healthy" ? "success" : host.status === "degraded" ? "warning" : "danger"})`,
      }}
    >
      <div style={{ display: "flex", justifyContent: "space-between", gap: 8, alignItems: "center" }}>
        <span className="truncate" style={{ fontWeight: 600 }}>
          {host.name}
        </span>
        <HealthIndicator status={host.status} />
      </div>
      <div style={{ display: "grid", gap: 6, marginTop: 4 }}>
        {rows.map(([label, v]) => (
          <div key={label} style={{ display: "grid", gridTemplateColumns: "36px 1fr 48px", alignItems: "center", gap: 8 }}>
            <span className="text-muted" style={{ fontSize: "var(--text-xs)" }}>
              {label}
            </span>
            <Meter value={host.status === "offline" ? 0 : v} tone={utilTone(v)} label={`${host.name} ${label}`} />
            <span className="num" style={{ textAlign: "right", fontSize: "var(--text-sm)" }}>
              {v === undefined || host.status === "offline" ? "—" : `${v.toFixed(0)}%`}
            </span>
          </div>
        ))}
      </div>
      <div className="metric__foot">
        <span className="mono">{host.hostname}</span>
        <span>· seen {relative(host.last_seen_at)}</span>
      </div>
    </Link>
  );
}

export function InfrastructurePage() {
  usePageMeta({ title: "Infrastructure", crumbs: [{ label: "Pulsewatch", to: "/" }, { label: "Infrastructure" }] });
  const hosts = useHosts();
  const series = useHostSeries();

  const order = { offline: 0, degraded: 1, healthy: 2 };
  const sorted = hosts.data ? [...hosts.data].sort((a, b) => order[a.status] - order[b.status] || a.name.localeCompare(b.name)) : undefined;

  return (
    <Page>
      <Panel title="Host map" subtitle={hosts.data ? `${hosts.data.length} hosts · most critical first` : undefined}>
        {hosts.isPending ? (
          <div className="grid" style={{ gridTemplateColumns: "repeat(auto-fill, minmax(220px, 1fr))" }}>
            {Array.from({ length: 4 }, (_, i) => (
              <Skeleton key={i} height={128} />
            ))}
          </div>
        ) : hosts.error && !hosts.data ? (
          <ErrorState error={hosts.error} onRetry={() => hosts.refetch()} />
        ) : sorted!.length === 0 ? (
          <EmptyState icon={<Server size={28} strokeWidth={1.5} />} title="No hosts reporting yet">
            Deploy the agent to start collecting host metrics.
          </EmptyState>
        ) : (
          <div className="grid" style={{ gridTemplateColumns: "repeat(auto-fill, minmax(220px, 1fr))" }}>
            {sorted!.map((h) => (
              <HostTile key={h.id} host={h} />
            ))}
          </div>
        )}
      </Panel>
      <div className="grid grid--2">
        <Panel title="CPU & memory" subtitle="fleet average">
          <TimeSeriesChart data={series.data} loading={series.isPending} lines={CPU_MEM} format={fmtPct} yMax={100} label="Fleet CPU and memory" />
        </Panel>
        <Panel title="Network traffic" subtitle="sum across hosts">
          <TimeSeriesChart data={series.data} loading={series.isPending} lines={NETWORK} format={fmtBitrate} label="Fleet network throughput" />
        </Panel>
        <Panel title="Disk utilization" subtitle="fleet average, primary volume">
          <TimeSeriesChart data={series.data} loading={series.isPending} lines={DISK} format={fmtPct} yMax={100} label="Fleet disk utilization" />
        </Panel>
        <Panel title="Load average" subtitle="1 minute, fleet average">
          <TimeSeriesChart data={series.data} loading={series.isPending} lines={LOAD} format={fmtNum} label="Fleet load average" />
        </Panel>
      </div>
    </Page>
  );
}
