import { Activity, Clock, Pause, Pencil, Play, Timer, Trash2, XCircle } from "lucide-react";
import { useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useAlerts, useDeleteMonitor, useMonitor, useMonitorResults, useMonitorSeries, useSaveMonitor } from "../api/queries";
import type { CheckResult } from "../api/types";
import { AvailabilityBar } from "../components/charts/AvailabilityBar";
import { AVAILABILITY, RESPONSE, fmtMs, fmtPct } from "../components/charts/lines";
import { TimeSeriesChart } from "../components/charts/TimeSeriesChart";
import { AlertList } from "../components/ui/AlertList";
import { DataTable, type Column } from "../components/ui/DataTable";
import { ConfirmationDialog } from "../components/ui/Dialog";
import { MetricCard } from "../components/ui/MetricCard";
import { Button, Panel } from "../components/ui/primitives";
import { EmptyState, ErrorState, LoadingSkeleton } from "../components/ui/states";
import { StatusBadge } from "../components/ui/status";
import { MonitorForm } from "../features/monitors/MonitorForm";
import { Page, usePageMeta } from "../layout/AppShell";
import { interval, ms, relative, timestamp } from "../lib/format";
import { useTimeRange } from "../lib/timeRange";

const resultCols: Column<CheckResult>[] = [
  { key: "time", header: "Checked at", sort: (r) => Date.parse(r.checked_at), render: (r) => <span className="num">{timestamp(r.checked_at)}</span> },
  { key: "result", header: "Result", sort: (r) => (r.success ? 1 : 0), render: (r) => <StatusBadge status={r.success ? "up" : "down"} label={r.success ? "OK" : "Failed"} /> },
  { key: "code", header: "HTTP", align: "right", sort: (r) => r.status_code, render: (r) => <span className="num">{r.status_code || "—"}</span> },
  { key: "rt", header: "Response", align: "right", sort: (r) => r.response_ms, render: (r) => <span className="num">{ms(r.response_ms)}</span> },
  { key: "err", header: "Error", hideOnMobile: true, render: (r) => <span className="text-2 truncate" style={{ display: "block" }} title={r.error}>{r.error || ""}</span> },
];

const ICON = { size: 14, strokeWidth: 1.75 } as const;

export function MonitorDetailPage() {
  const { id = "" } = useParams();
  const q = useMonitor(id);
  const series = useMonitorSeries(id);
  const results = useMonitorResults(id);
  const alerts = useAlerts(undefined, id);
  const save = useSaveMonitor();
  const del = useDeleteMonitor();
  const navigate = useNavigate();
  const { label } = useTimeRange();
  const [editing, setEditing] = useState(false);
  const [confirm, setConfirm] = useState(false);
  const m = q.data;

  usePageMeta({
    title: m?.name ?? "Monitor",
    crumbs: [{ label: "Pulsewatch", to: "/" }, { label: "HTTP Monitors", to: "/monitors" }, { label: m?.name ?? id }],
  });

  if (q.isPending) return <Page><LoadingSkeleton rows={8} /></Page>;
  if (!m) return <Page><ErrorState error={q.error} onRetry={() => q.refetch()} /></Page>;

  const togglePause = () =>
    save.mutate({
      id: m.id,
      input: { name: m.name, url: m.url, method: m.method, interval_s: m.interval_s, timeout_ms: m.timeout_ms, expected_status: m.expected_status, headers: m.headers, enabled: !m.enabled },
    });

  return (
    <Page>
      <Panel
        title={
          <span className="toolbar">
            {m.name} <StatusBadge status={m.status} />
          </span>
        }
        actions={
          <div className="toolbar">
            <Button size="sm" onClick={togglePause} disabled={save.isPending}>
              {m.enabled ? <><Pause size={13} /> Pause</> : <><Play size={13} /> Resume</>}
            </Button>
            <Button size="sm" onClick={() => setEditing(true)}>
              <Pencil size={13} /> Edit
            </Button>
            <Button size="sm" variant="ghost" onClick={() => setConfirm(true)} aria-label="Delete monitor">
              <Trash2 size={13} />
            </Button>
          </div>
        }
      >
        <dl className="kv">
          <dt>URL</dt>
          <dd className="mono"><span className="method">{m.method}</span>{m.url}</dd>
          <dt>Check interval</dt>
          <dd className="num">every {interval(m.interval_s)} · timeout {ms(m.timeout_ms)} · expects {m.expected_status || "2xx/3xx"}</dd>
          {m.last_error && (
            <>
              <dt>Last error</dt>
              <dd style={{ color: "var(--color-danger-text)" }}>{m.last_error} <span className="text-muted num">({relative(m.last_failure_at)})</span></dd>
            </>
          )}
        </dl>
      </Panel>

      <div className="grid grid--stats">
        <MetricCard label={`Availability · ${label.toLowerCase()}`} icon={<Activity {...ICON} />} value={m.uptime_pct === null ? "—" : m.uptime_pct.toFixed(2)} unit={m.uptime_pct === null ? undefined : "%"}
          tone={m.uptime_pct === null ? undefined : m.uptime_pct >= 99.9 ? "success" : m.uptime_pct >= 99 ? "warning" : "danger"}
          footer={<span className="num">{m.checks_total.toLocaleString()} checks</span>} />
        <MetricCard label="Current response" icon={<Timer {...ICON} />} value={m.last_check ? ms(m.last_check.response_ms) : "—"}
          footer={m.last_check && <span className="num">HTTP {m.last_check.status_code || "—"} · {relative(m.last_check.checked_at)}</span>} />
        <MetricCard label="Avg response" icon={<Clock {...ICON} />} value={ms(m.avg_response_ms)} footer={<span>successful checks</span>} />
        <MetricCard label="Failed checks" icon={<XCircle {...ICON} />} value={m.checks_failed} tone={m.checks_failed ? "danger" : undefined}
          footer={<span className="num">last success {relative(m.last_success_at)}</span>} />
      </div>

      <Panel title="Availability timeline">
        <AvailabilityBar data={series.data} loading={series.isPending} />
      </Panel>

      <div className="grid grid--2">
        <Panel title="Response time">
          <TimeSeriesChart data={series.data} loading={series.isPending} lines={RESPONSE} format={fmtMs} label={`${m.name} response time`} />
        </Panel>
        <Panel title="Uptime">
          <TimeSeriesChart data={series.data} loading={series.isPending} lines={AVAILABILITY} format={fmtPct} yMax={100} label={`${m.name} availability`} />
        </Panel>
      </div>

      <div className="grid grid--main-side">
        <Panel title="Recent checks" flush>
          {results.error && !results.data ? (
            <ErrorState error={results.error} onRetry={() => results.refetch()} />
          ) : (
            <DataTable label="Recent checks" columns={resultCols} rows={results.data} loading={results.isPending} rowKey={(r) => r.checked_at}
              defaultSort={{ key: "time", dir: "desc" }} pageSize={20} empty={<EmptyState title="No checks in this range" />} />
          )}
        </Panel>
        <Panel title="Alerts" subtitle="this endpoint" flush>
          {alerts.data && alerts.data.length > 0 ? <AlertList alerts={alerts.data} /> : <EmptyState title="No alerts for this endpoint" />}
        </Panel>
      </div>

      <MonitorForm open={editing} onClose={() => setEditing(false)} monitor={m} />
      <ConfirmationDialog
        open={confirm}
        title="Delete monitor?"
        message={<>This permanently deletes <strong>{m.name}</strong>, its check history and its alerts.</>}
        confirmLabel="Delete monitor"
        onCancel={() => setConfirm(false)}
        onConfirm={async () => {
          await del.mutateAsync(m.id);
          navigate("/monitors");
        }}
      />
    </Page>
  );
}
