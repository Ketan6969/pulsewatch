// Metrics explorer: pick a scope (fleet or one host / endpoint) and view
// every series for it at a larger size.
import { useSearchParams } from "react-router-dom";
import { useHostSeries, useHosts, useMonitorSeries, useMonitors } from "../api/queries";
import { AVAILABILITY, CPU_MEM, DISK, LOAD, NETWORK, RESPONSE, fmtBitrate, fmtMs, fmtNum, fmtPct } from "../components/charts/lines";
import { TimeSeriesChart } from "../components/charts/TimeSeriesChart";
import { Panel } from "../components/ui/primitives";
import { Page, usePageMeta } from "../layout/AppShell";

export function MetricsPage() {
  usePageMeta({ title: "Metrics", crumbs: [{ label: "Pulsewatch", to: "/" }, { label: "Metrics" }] });
  const [params, setParams] = useSearchParams();
  const source = params.get("source") === "http" ? "http" : "system";
  const target = params.get("target") ?? "";
  const hosts = useHosts().data ?? [];
  const monitors = useMonitors().data ?? [];
  const hostSeries = useHostSeries(source === "system" && target ? target : undefined);
  const monSeries = useMonitorSeries(source === "http" && target ? target : undefined);
  const scopeLabel = source === "system" ? hosts.find((h) => h.id === target)?.name ?? "All hosts" : monitors.find((m) => m.id === target)?.name ?? "All endpoints";

  return (
    <Page>
      <div className="panel">
        <div className="filterbar" style={{ borderBottom: 0 }}>
          <label className="toolbar" style={{ gap: 6 }}>
            <span className="text-2" style={{ fontSize: "var(--text-sm)" }}>Source</span>
            <select className="select" value={source} onChange={(e) => setParams({ source: e.target.value })}>
              <option value="system">System metrics</option>
              <option value="http">HTTP checks</option>
            </select>
          </label>
          <label className="toolbar" style={{ gap: 6 }}>
            <span className="text-2" style={{ fontSize: "var(--text-sm)" }}>Scope</span>
            <select className="select" value={target} onChange={(e) => setParams({ source, ...(e.target.value ? { target: e.target.value } : {}) })} style={{ minWidth: 200 }}>
              {source === "system" ? (
                <>
                  <option value="">All hosts (aggregate)</option>
                  {hosts.map((h) => <option key={h.id} value={h.id}>{h.name}</option>)}
                </>
              ) : (
                <>
                  <option value="">All endpoints (aggregate)</option>
                  {monitors.map((m) => <option key={m.id} value={m.id}>{m.name}</option>)}
                </>
              )}
            </select>
          </label>
          <span className="filterbar__count">
            {(source === "system" ? hostSeries.data : monSeries.data)?.step_s ?? "—"}s resolution
          </span>
        </div>
      </div>

      {source === "system" ? (
        <div className="grid grid--2">
          <Panel title="CPU & memory utilization" subtitle={scopeLabel}>
            <TimeSeriesChart data={hostSeries.data} loading={hostSeries.isPending} lines={CPU_MEM} format={fmtPct} yMax={100} height={260} label="CPU and memory" />
          </Panel>
          <Panel title="Network throughput" subtitle={scopeLabel}>
            <TimeSeriesChart data={hostSeries.data} loading={hostSeries.isPending} lines={NETWORK} format={fmtBitrate} height={260} label="Network" />
          </Panel>
          <Panel title="Disk utilization" subtitle={scopeLabel}>
            <TimeSeriesChart data={hostSeries.data} loading={hostSeries.isPending} lines={DISK} format={fmtPct} yMax={100} height={260} label="Disk" />
          </Panel>
          <Panel title="Load average (1m)" subtitle={scopeLabel}>
            <TimeSeriesChart data={hostSeries.data} loading={hostSeries.isPending} lines={LOAD} format={fmtNum} height={260} label="Load" />
          </Panel>
        </div>
      ) : (
        <div className="grid grid--2">
          <Panel title="Response time" subtitle={scopeLabel}>
            <TimeSeriesChart data={monSeries.data} loading={monSeries.isPending} lines={RESPONSE} format={fmtMs} height={260} label="Response time" />
          </Panel>
          <Panel title="Availability" subtitle={scopeLabel}>
            <TimeSeriesChart data={monSeries.data} loading={monSeries.isPending} lines={AVAILABILITY} format={fmtPct} yMax={100} height={260} label="Availability" />
          </Panel>
        </div>
      )}
    </Page>
  );
}
