import { Globe, Plus } from "lucide-react";
import { useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useMonitors } from "../api/queries";
import type { MonitorSummary } from "../api/types";
import { DataTable, type Column } from "../components/ui/DataTable";
import { FilterBar, matches } from "../components/ui/FilterBar";
import { Button, Panel } from "../components/ui/primitives";
import { EmptyState, ErrorState } from "../components/ui/states";
import { HealthIndicator } from "../components/ui/status";
import { MonitorForm } from "../features/monitors/MonitorForm";
import { Page, usePageMeta } from "../layout/AppShell";
import { interval, ms, relative, timestamp } from "../lib/format";

const STATUS_ORDER = { down: 0, pending: 1, up: 2, paused: 3 } as const;

function uptimeColor(v: number | null) {
  if (v === null) return undefined;
  return v >= 99.9 ? "var(--color-success-text)" : v >= 99 ? "var(--color-warning-text)" : "var(--color-danger-text)";
}

export const monitorColumns: Column<MonitorSummary>[] = [
  {
    key: "name",
    header: "Endpoint",
    sort: (m) => m.name,
    render: (m) => (
      <div style={{ minWidth: 0 }}>
        <div className="truncate" style={{ fontWeight: 500 }}>{m.name}</div>
        <div className="mono text-muted truncate" style={{ fontSize: "var(--text-xs)", maxWidth: 360 }} title={m.url}>
          <span className="method">{m.method}</span>
          {m.url}
        </div>
      </div>
    ),
  },
  { key: "status", header: "Status", sort: (m) => STATUS_ORDER[m.status], render: (m) => <HealthIndicator status={m.status} /> },
  {
    key: "code",
    header: "HTTP",
    align: "right",
    hideOnMobile: true,
    sort: (m) => m.last_check?.status_code,
    render: (m) => <span className="num">{m.last_check?.status_code || "—"}</span>,
  },
  {
    key: "rt",
    header: "Response",
    align: "right",
    sort: (m) => m.last_check?.response_ms,
    render: (m) => <span className="num">{m.last_check ? ms(m.last_check.response_ms) : "—"}</span>,
  },
  {
    key: "avg",
    header: "Avg",
    align: "right",
    hideOnMobile: true,
    sort: (m) => m.avg_response_ms,
    render: (m) => <span className="num text-2">{ms(m.avg_response_ms)}</span>,
  },
  {
    key: "uptime",
    header: "Availability",
    align: "right",
    sort: (m) => m.uptime_pct,
    render: (m) => (
      <span className="num" style={{ color: uptimeColor(m.uptime_pct) }}>
        {m.uptime_pct === null ? "—" : `${m.uptime_pct.toFixed(2)}%`}
      </span>
    ),
  },
  {
    key: "ok",
    header: "Last success",
    align: "right",
    hideOnMobile: true,
    sort: (m) => (m.last_success_at ? Date.parse(m.last_success_at) : null),
    render: (m) => <span className="num text-2" title={timestamp(m.last_success_at)}>{relative(m.last_success_at)}</span>,
  },
  {
    key: "fail",
    header: "Last failure",
    align: "right",
    hideOnMobile: true,
    sort: (m) => (m.last_failure_at ? Date.parse(m.last_failure_at) : null),
    render: (m) => (
      <span className="num text-2" title={m.last_error ? `${timestamp(m.last_failure_at)} — ${m.last_error}` : undefined}>
        {relative(m.last_failure_at)}
      </span>
    ),
  },
  {
    key: "interval",
    header: "Interval",
    align: "right",
    hideOnMobile: true,
    sort: (m) => m.interval_s,
    render: (m) => <span className="num text-2">{interval(m.interval_s)}</span>,
  },
];

export function MonitorsPage() {
  usePageMeta({ title: "HTTP Monitors", crumbs: [{ label: "Pulsewatch", to: "/" }, { label: "HTTP Monitors" }] });
  const monitors = useMonitors();
  const navigate = useNavigate();
  const [q, setQ] = useState("");
  const [status, setStatus] = useState("");
  const [creating, setCreating] = useState(false);

  const rows = useMemo(
    () => monitors.data?.filter((m) => (!status || m.status === status) && matches(q, m.name, m.url, m.method)),
    [monitors.data, q, status],
  );

  return (
    <Page>
      <Panel
        title="Endpoints"
        flush
        actions={
          <Button variant="primary" size="sm" onClick={() => setCreating(true)}>
            <Plus size={14} /> New monitor
          </Button>
        }
      >
        <FilterBar
          query={q}
          onQuery={setQ}
          placeholder="Filter by name or URL…"
          filters={[
            {
              label: "Status",
              value: status,
              onChange: setStatus,
              options: [
                { value: "", label: "All" },
                { value: "up", label: "Up" },
                { value: "down", label: "Down" },
                { value: "pending", label: "Pending" },
                { value: "paused", label: "Paused" },
              ],
            },
          ]}
          count={rows && monitors.data ? { shown: rows.length, total: monitors.data.length } : undefined}
        />
        {monitors.error && !monitors.data ? (
          <ErrorState error={monitors.error} onRetry={() => monitors.refetch()} />
        ) : (
          <DataTable
            label="HTTP monitors"
            columns={monitorColumns}
            rows={rows}
            loading={monitors.isPending}
            rowKey={(m) => m.id}
            onRowClick={(m) => navigate(`/monitors/${m.id}`)}
            defaultSort={{ key: "status", dir: "asc" }}
            empty={
              monitors.data?.length === 0 ? (
                <EmptyState
                  icon={<Globe size={28} strokeWidth={1.5} />}
                  title="No HTTP monitors yet"
                  action={<Button variant="primary" onClick={() => setCreating(true)}><Plus size={14} /> Create your first monitor</Button>}
                >
                  Monitors periodically request an endpoint and record availability, status codes and response times.
                </EmptyState>
              ) : (
                <EmptyState title="No monitors match these filters" />
              )
            }
          />
        )}
      </Panel>
      <MonitorForm open={creating} onClose={() => setCreating(false)} />
    </Page>
  );
}
