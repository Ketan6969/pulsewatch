import { Clock, Cpu, HardDrive, MemoryStick, Network, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useAlerts, useDeleteHost, useHost, useHostSeries } from "../api/queries";
import type { DiskUsage, NetInterface, Process } from "../api/types";
import { CPU_MEM, DISK, LOAD, NETWORK, fmtBitrate, fmtNum, fmtPct } from "../components/charts/lines";
import { TimeSeriesChart } from "../components/charts/TimeSeriesChart";
import { AlertList } from "../components/ui/AlertList";
import { DataTable, type Column } from "../components/ui/DataTable";
import { ConfirmationDialog } from "../components/ui/Dialog";
import { FilterBar, matches } from "../components/ui/FilterBar";
import { MetricCard } from "../components/ui/MetricCard";
import { Button, Panel } from "../components/ui/primitives";
import { EmptyState, ErrorState, LoadingSkeleton } from "../components/ui/states";
import { StatusBadge } from "../components/ui/status";
import { UtilCell } from "../features/hosts/hostColumns";
import { Page, usePageMeta } from "../layout/AppShell";
import { bitrate, bytes, duration, relative, timestamp, utilTone } from "../lib/format";

const diskCols: Column<DiskUsage>[] = [
  { key: "mount", header: "Mount", sort: (d) => d.mountpoint, render: (d) => <span className="mono">{d.mountpoint}</span> },
  { key: "dev", header: "Device", hideOnMobile: true, sort: (d) => d.device, render: (d) => <span className="mono text-2">{d.device || "—"}</span> },
  { key: "fs", header: "FS", hideOnMobile: true, sort: (d) => d.fstype, render: (d) => d.fstype },
  { key: "used", header: "Used", align: "right", sort: (d) => d.used, render: (d) => <span className="num">{bytes(d.used)}</span> },
  { key: "total", header: "Size", align: "right", sort: (d) => d.total, render: (d) => <span className="num">{bytes(d.total)}</span> },
  { key: "pct", header: "Usage", width: 140, sort: (d) => d.used_pct, render: (d) => <UtilCell value={d.used_pct} label={`${d.mountpoint} usage`} /> },
];

const ifaceCols: Column<NetInterface>[] = [
  { key: "name", header: "Interface", sort: (n) => n.name, render: (n) => <span className="mono">{n.name}</span> },
  { key: "state", header: "State", sort: (n) => (n.up ? 1 : 0), render: (n) => <StatusBadge status={n.up ? "up" : "down"} /> },
  {
    key: "addr",
    header: "Addresses",
    hideOnMobile: true,
    render: (n) => <span className="mono text-2 truncate" style={{ display: "block" }} title={(n.addrs ?? []).join(", ")}>{(n.addrs ?? []).join(", ") || "—"}</span>,
  },
  { key: "rx", header: "In", align: "right", sort: (n) => n.rx_bps, render: (n) => <span className="num">{bitrate(n.rx_bps)}</span> },
  { key: "tx", header: "Out", align: "right", sort: (n) => n.tx_bps, render: (n) => <span className="num">{bitrate(n.tx_bps)}</span> },
  { key: "total", header: "Total in / out", align: "right", hideOnMobile: true, render: (n) => <span className="num text-2">{bytes(n.bytes_recv)} / {bytes(n.bytes_sent)}</span> },
  { key: "err", header: "Errors", align: "right", hideOnMobile: true, sort: (n) => n.errin + n.errout, render: (n) => <span className="num">{n.errin + n.errout}</span> },
];

const procCols: Column<Process>[] = [
  { key: "pid", header: "PID", align: "right", width: 70, sort: (p) => p.pid, render: (p) => <span className="num">{p.pid}</span> },
  {
    key: "name",
    header: "Process",
    sort: (p) => p.name,
    render: (p) => (
      <div style={{ minWidth: 0 }}>
        <div className="truncate" style={{ fontWeight: 500 }}>{p.name || "—"}</div>
        <div className="mono text-muted truncate" style={{ fontSize: "var(--text-xs)", maxWidth: 380 }} title={p.cmdline}>{p.cmdline}</div>
      </div>
    ),
  },
  { key: "user", header: "User", hideOnMobile: true, sort: (p) => p.user, render: (p) => <span className="text-2">{p.user || "—"}</span> },
  { key: "status", header: "State", hideOnMobile: true, sort: (p) => p.status, render: (p) => <span className="mono text-2">{p.status || "—"}</span> },
  { key: "cpu", header: "CPU", align: "right", sort: (p) => p.cpu_pct, render: (p) => <span className="num">{p.cpu_pct.toFixed(1)}%</span> },
  { key: "mem", header: "Mem", align: "right", sort: (p) => p.mem_pct, render: (p) => <span className="num">{p.mem_pct.toFixed(1)}%</span> },
  { key: "rss", header: "RSS", align: "right", hideOnMobile: true, sort: (p) => p.rss, render: (p) => <span className="num">{bytes(p.rss)}</span> },
];

const ICON = { size: 14, strokeWidth: 1.75 } as const;

export function HostDetailPage() {
  const { id = "" } = useParams();
  const q = useHost(id);
  const series = useHostSeries(id);
  const alerts = useAlerts(undefined, id);
  const del = useDeleteHost();
  const navigate = useNavigate();
  const [confirm, setConfirm] = useState(false);
  const [procQ, setProcQ] = useState("");
  const host = q.data?.host;
  const snap = q.data?.snapshot;

  usePageMeta({
    title: host?.name ?? "Host",
    crumbs: [{ label: "Pulsewatch", to: "/" }, { label: "Hosts", to: "/hosts" }, { label: host?.hostname ?? id }],
  });

  const procs = useMemo(() => (snap?.processes ?? []).filter((p) => matches(procQ, p.name, p.cmdline, p.user, String(p.pid))), [snap, procQ]);

  if (q.isPending) return <Page><LoadingSkeleton rows={8} /></Page>;
  if (!host) return <Page><ErrorState error={q.error} onRetry={() => q.refetch()} /></Page>;
  const m = host.latest;
  const offline = host.status === "offline";

  return (
    <Page>
      <Panel
        title={
          <span className="toolbar">
            {host.name} <StatusBadge status={host.status} />
          </span>
        }
        actions={
          <Button variant="ghost" size="sm" onClick={() => setConfirm(true)}>
            <Trash2 size={14} /> Remove host
          </Button>
        }
      >
        <div className="grid grid--3" style={{ gap: 24 }}>
          <dl className="kv">
            <dt>Hostname</dt>
            <dd className="mono">{host.hostname}</dd>
            <dt>Operating system</dt>
            <dd>{host.platform || host.os} {host.platform_version}</dd>
            <dt>Kernel</dt>
            <dd className="mono">{host.kernel_version || "—"} ({host.arch})</dd>
          </dl>
          <dl className="kv">
            <dt>Agent</dt>
            <dd>
              <span className="mono">v{host.agent_version}</span> · <span style={{ color: offline ? "var(--color-danger-text)" : "var(--color-success-text)" }}>{offline ? "not reporting" : "reporting"}</span>
            </dd>
            <dt>Last heartbeat</dt>
            <dd className="num" title={timestamp(host.last_seen_at)}>{relative(host.last_seen_at)}</dd>
            <dt>Report interval</dt>
            <dd className="num">{host.report_interval_s}s</dd>
          </dl>
          <dl className="kv">
            <dt>CPU cores</dt>
            <dd className="num">{host.cpu_count}</dd>
            <dt>Booted</dt>
            <dd className="num">{host.boot_time ? timestamp(host.boot_time) : "—"}</dd>
            <dt>Registered</dt>
            <dd className="num">{timestamp(host.created_at)}</dd>
          </dl>
        </div>
      </Panel>

      <div className="grid grid--stats">
        <MetricCard label="CPU" icon={<Cpu {...ICON} />} value={m ? m.cpu_pct.toFixed(1) : "—"} unit={m ? "%" : undefined} meter={m?.cpu_pct} tone={utilTone(m?.cpu_pct)}
          footer={m && <span className="num">load {m.load1.toFixed(2)} / {m.load5.toFixed(2)} / {m.load15.toFixed(2)}</span>} />
        <MetricCard label="Memory" icon={<MemoryStick {...ICON} />} value={m ? m.mem_pct.toFixed(1) : "—"} unit={m ? "%" : undefined} meter={m?.mem_pct} tone={utilTone(m?.mem_pct)}
          footer={m && <span className="num">{bytes(m.mem_used)} of {bytes(m.mem_total)} · swap {m.swap_pct.toFixed(0)}%</span>} />
        <MetricCard label="Disk (primary)" icon={<HardDrive {...ICON} />} value={m ? m.disk_pct.toFixed(1) : "—"} unit={m ? "%" : undefined} meter={m?.disk_pct} tone={utilTone(m?.disk_pct)}
          footer={m && <span className="num">{bytes(m.disk_used)} of {bytes(m.disk_total)}</span>} />
        <MetricCard label="Uptime" icon={<Clock {...ICON} />} value={duration(m?.uptime_s)}
          footer={m && <><span className="num">{m.proc_count} processes</span><span className="num"><Network size={11} /> {bitrate(m.net_rx_bps + m.net_tx_bps)}</span></>} />
      </div>

      <div className="grid grid--2">
        <Panel title="CPU & memory">
          <TimeSeriesChart data={series.data} loading={series.isPending} lines={CPU_MEM} format={fmtPct} yMax={100} label={`${host.name} CPU and memory`} />
        </Panel>
        <Panel title="Network throughput">
          <TimeSeriesChart data={series.data} loading={series.isPending} lines={NETWORK} format={fmtBitrate} label={`${host.name} network`} />
        </Panel>
        <Panel title="Disk utilization">
          <TimeSeriesChart data={series.data} loading={series.isPending} lines={DISK} format={fmtPct} yMax={100} label={`${host.name} disk`} />
        </Panel>
        <Panel title="Load average">
          <TimeSeriesChart data={series.data} loading={series.isPending} lines={LOAD} format={fmtNum} label={`${host.name} load`} />
        </Panel>
      </div>

      <div className="grid grid--2">
        <Panel title="Filesystems" flush>
          <DataTable label="Filesystems" columns={diskCols} rows={snap?.disks ?? []} rowKey={(d) => d.mountpoint} defaultSort={{ key: "mount", dir: "asc" }} empty={<EmptyState title="No filesystem data" />} />
        </Panel>
        <Panel title="Network interfaces" flush>
          <DataTable label="Network interfaces" columns={ifaceCols} rows={snap?.interfaces ?? []} rowKey={(n) => n.name} defaultSort={{ key: "name", dir: "asc" }} empty={<EmptyState title="No interface data" />} />
        </Panel>
      </div>

      <div className="grid grid--main-side">
        <Panel title="Processes" subtitle={snap ? `top ${snap.processes?.length ?? 0} by CPU · ${relative(snap.collected_at)}` : undefined} flush>
          <FilterBar query={procQ} onQuery={setProcQ} placeholder="Filter by name, command, user, PID…" count={{ shown: procs.length, total: snap?.processes?.length ?? 0 }} />
          <DataTable label="Processes" columns={procCols} rows={procs} rowKey={(p) => String(p.pid)} defaultSort={{ key: "cpu", dir: "desc" }} pageSize={15} empty={<EmptyState title="No processes" />} />
        </Panel>
        <Panel title="Alerts" subtitle="this host" flush>
          {alerts.data && alerts.data.length > 0 ? <AlertList alerts={alerts.data} /> : <EmptyState title="No alerts for this host" />}
        </Panel>
      </div>

      <ConfirmationDialog
        open={confirm}
        title="Remove host?"
        message={
          <>
            This deletes <strong>{host.name}</strong> and all of its historical metrics and alerts. If the agent is still running, the host will
            re-register on its next report.
          </>
        }
        confirmLabel="Remove host"
        onCancel={() => setConfirm(false)}
        onConfirm={async () => {
          await del.mutateAsync(host.id);
          navigate("/hosts");
        }}
      />
    </Page>
  );
}
