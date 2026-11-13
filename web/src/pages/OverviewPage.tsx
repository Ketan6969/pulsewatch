import { Activity, AlertTriangle, CheckCircle2, Clock, Cpu, Globe, HardDrive, MemoryStick, Network, Server, WifiOff, XCircle } from "lucide-react";
import { Link } from "react-router-dom";
import { useAckAlert, useAlerts, useEvents, useHostSeries, useMonitorSeries, useOverview } from "../api/queries";
import { AvailabilityBar } from "../components/charts/AvailabilityBar";
import { CPU_MEM, NETWORK, RESPONSE, fmtBitrate, fmtMs, fmtPct } from "../components/charts/lines";
import { TimeSeriesChart } from "../components/charts/TimeSeriesChart";
import { AlertList, EventList } from "../components/ui/AlertList";
import { MetricCard } from "../components/ui/MetricCard";
import { Panel } from "../components/ui/primitives";
import { EmptyState, ErrorState, LoadingSkeleton } from "../components/ui/states";
import { Page, usePageMeta } from "../layout/AppShell";
import { bitrate, ms, utilTone } from "../lib/format";
import { useTimeRange } from "../lib/timeRange";

const ICON = { size: 14, strokeWidth: 1.75 } as const;

export function OverviewPage() {
  usePageMeta({ title: "Overview", crumbs: [{ label: "Pulsewatch" }] });
  const { label } = useTimeRange();
  const overview = useOverview();
  const hostSeries = useHostSeries();
  const monSeries = useMonitorSeries();
  const openAlerts = useAlerts("open");
  const events = useEvents(30);
  const ack = useAckAlert();

  const o = overview.data;
  const loading = overview.isPending;

  if (overview.error && !o) return <ErrorState error={overview.error} onRetry={() => overview.refetch()} />;

  return (
    <Page>
      {overview.error && <ErrorState compact error={overview.error} onRetry={() => overview.refetch()} />}

      <div className="section-title m-order-1">Infrastructure</div>
      <div className="grid grid--stats m-order-1">
        <MetricCard label="Monitored hosts" icon={<Server {...ICON} />} value={o?.hosts.total ?? "—"} loading={loading} to="/hosts" footer={<span>{o?.system.reporting ?? 0} reporting</span>} />
        <MetricCard label="Healthy" icon={<CheckCircle2 {...ICON} color="var(--color-success)" />} value={o?.hosts.healthy ?? "—"} loading={loading} to="/hosts?status=healthy" />
        <MetricCard label="Degraded" icon={<AlertTriangle {...ICON} color="var(--color-warning)" />} value={o?.hosts.degraded ?? "—"} tone={o?.hosts.degraded ? "warning" : undefined} loading={loading} to="/hosts?status=degraded" />
        <MetricCard label="Offline" icon={<WifiOff {...ICON} color="var(--color-danger)" />} value={o?.hosts.offline ?? "—"} tone={o?.hosts.offline ? "danger" : undefined} loading={loading} to="/hosts?status=offline" />
      </div>

      <div className="section-title">System resources <span className="text-muted" style={{ textTransform: "none", letterSpacing: 0, fontWeight: 400 }}>· fleet average, current</span></div>
      <div className="grid grid--stats">
        <MetricCard label="CPU utilization" icon={<Cpu {...ICON} />} value={o?.system.cpu_pct?.toFixed(1) ?? "—"} unit={o?.system.cpu_pct != null ? "%" : undefined} meter={o?.system.cpu_pct} tone={utilTone(o?.system.cpu_pct)} loading={loading} />
        <MetricCard label="Memory utilization" icon={<MemoryStick {...ICON} />} value={o?.system.mem_pct?.toFixed(1) ?? "—"} unit={o?.system.mem_pct != null ? "%" : undefined} meter={o?.system.mem_pct} tone={utilTone(o?.system.mem_pct)} loading={loading} />
        <MetricCard label="Disk utilization" icon={<HardDrive {...ICON} />} value={o?.system.disk_pct?.toFixed(1) ?? "—"} unit={o?.system.disk_pct != null ? "%" : undefined} meter={o?.system.disk_pct} tone={utilTone(o?.system.disk_pct)} loading={loading} />
        <MetricCard
          label="Network throughput"
          icon={<Network {...ICON} />}
          value={o ? bitrate(o.system.net_rx_bps + o.system.net_tx_bps) : "—"}
          loading={loading}
          footer={
            o && (
              <>
                <span className="num">↓ {bitrate(o.system.net_rx_bps)}</span>
                <span className="num">↑ {bitrate(o.system.net_tx_bps)}</span>
              </>
            )
          }
        />
      </div>

      <div className="section-title">HTTP monitoring <span className="text-muted" style={{ textTransform: "none", letterSpacing: 0, fontWeight: 400 }}>· {label.toLowerCase()}</span></div>
      <div className="grid grid--stats">
        <MetricCard
          label="Monitored endpoints"
          icon={<Globe {...ICON} />}
          value={o?.http.total ?? "—"}
          loading={loading}
          to="/monitors"
          footer={o && <><span>{o.http.up} up</span><span>{o.http.down} down</span>{o.http.paused > 0 && <span>{o.http.paused} paused</span>}</>}
        />
        <MetricCard
          label="Uptime"
          icon={<Activity {...ICON} />}
          value={o?.http.uptime_pct != null ? o.http.uptime_pct.toFixed(2) : "—"}
          unit={o?.http.uptime_pct != null ? "%" : undefined}
          tone={o?.http.uptime_pct == null ? undefined : o.http.uptime_pct >= 99.9 ? "success" : o.http.uptime_pct >= 99 ? "warning" : "danger"}
          loading={loading}
        />
        <MetricCard label="Avg response time" icon={<Clock {...ICON} />} value={ms(o?.http.avg_response_ms)} loading={loading} />
        <MetricCard
          label="Failed checks"
          icon={<XCircle {...ICON} />}
          value={o?.http.failed_checks ?? "—"}
          tone={o?.http.failed_checks ? "danger" : undefined}
          loading={loading}
          footer={o && <span className="num">of {o.http.total_checks.toLocaleString()} checks</span>}
        />
      </div>

      <div className="grid grid--2">
        <Panel title="CPU & memory" subtitle="fleet average">
          <TimeSeriesChart data={hostSeries.data} loading={hostSeries.isPending} lines={CPU_MEM} format={fmtPct} yMax={100} label="CPU and memory utilization over time" />
        </Panel>
        <Panel title="Network traffic" subtitle="all hosts">
          <TimeSeriesChart data={hostSeries.data} loading={hostSeries.isPending} lines={NETWORK} format={fmtBitrate} label="Network throughput over time" />
        </Panel>
        <Panel title="HTTP response time" subtitle="all endpoints">
          <TimeSeriesChart data={monSeries.data} loading={monSeries.isPending} lines={RESPONSE} format={fmtMs} label="HTTP response time over time" />
        </Panel>
        <Panel title="Availability" subtitle="all endpoints, per interval">
          <div style={{ display: "flex", flexDirection: "column", justifyContent: "center", height: "100%", gap: 12 }}>
            <AvailabilityBar data={monSeries.data} loading={monSeries.isPending} />
            <div className="toolbar text-2" style={{ fontSize: "var(--text-xs)", gap: 16 }}>
              <span className="health"><span className="dot" data-tone="success" /> ≥ 99.9%</span>
              <span className="health"><span className="dot" data-tone="warning" /> ≥ 95%</span>
              <span className="health"><span className="dot" data-tone="danger" /> &lt; 95%</span>
              <span className="health"><span className="dot" /> No data</span>
            </div>
          </div>
        </Panel>
      </div>

      <div className="grid grid--2 m-order-2">
        <Panel
          title="Active alerts"
          subtitle={openAlerts.data ? `${openAlerts.data.length} open` : undefined}
          flush
          actions={<Link to="/alerts" style={{ fontSize: "var(--text-sm)" }}>View all</Link>}
        >
          {openAlerts.isPending ? (
            <LoadingSkeleton rows={4} variant="list" />
          ) : openAlerts.error && !openAlerts.data ? (
            <ErrorState error={openAlerts.error} onRetry={() => openAlerts.refetch()} />
          ) : openAlerts.data!.length === 0 ? (
            <EmptyState icon={<CheckCircle2 size={28} strokeWidth={1.5} color="var(--color-success)" />} title="All clear">
              No alerts are firing.
            </EmptyState>
          ) : (
            <div style={{ maxHeight: 360, overflowY: "auto" }}>
              <AlertList alerts={openAlerts.data!} onAcknowledge={(a) => ack.mutate(a.id)} ackPending={ack.isPending ? ack.variables : undefined} />
            </div>
          )}
        </Panel>
        <Panel title="Recent events" flush>
          {events.isPending ? (
            <LoadingSkeleton rows={4} variant="list" />
          ) : events.error && !events.data ? (
            <ErrorState error={events.error} onRetry={() => events.refetch()} />
          ) : events.data!.length === 0 ? (
            <EmptyState title="No events in this range" />
          ) : (
            <div style={{ maxHeight: 360, overflowY: "auto" }}>
              <EventList events={events.data!} />
            </div>
          )}
        </Panel>
      </div>
    </Page>
  );
}
