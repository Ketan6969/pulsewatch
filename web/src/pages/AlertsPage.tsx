import { BellOff, Pencil, Plus, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { useAckAlert, useAlerts, useDeleteRule, useRules } from "../api/queries";
import type { Alert, AlertRule } from "../api/types";
import { DataTable, type Column } from "../components/ui/DataTable";
import { ConfirmationDialog } from "../components/ui/Dialog";
import { FilterBar, matches } from "../components/ui/FilterBar";
import { Button, Panel } from "../components/ui/primitives";
import { EmptyState, ErrorState } from "../components/ui/states";
import { SeverityLabel, StatusBadge } from "../components/ui/status";
import { KIND_LABEL, RuleForm, describeRule } from "../features/alerts/RuleForm";
import { Page, usePageMeta } from "../layout/AppShell";
import { duration, relative, timestamp } from "../lib/format";

const SEV_ORDER = { critical: 0, warning: 1, info: 2 } as const;
const STATE_ORDER = { firing: 0, acknowledged: 1, resolved: 2 } as const;

function alertColumns(onAck: (a: Alert) => void, pending?: string): Column<Alert>[] {
  return [
    { key: "sev", header: "Severity", width: 110, sort: (a) => SEV_ORDER[a.severity], render: (a) => <SeverityLabel severity={a.severity} /> },
    {
      key: "name",
      header: "Alert",
      sort: (a) => a.rule_name,
      render: (a) => (
        <div style={{ minWidth: 0 }}>
          <div style={{ fontWeight: 500 }}>{a.rule_name}</div>
          <div className="text-muted truncate" style={{ fontSize: "var(--text-xs)", maxWidth: 420 }} title={a.message}>{a.message}</div>
        </div>
      ),
    },
    {
      key: "target",
      header: "Affected",
      sort: (a) => a.target_name,
      render: (a) => (
        <Link to={a.target_type === "host" ? `/hosts/${a.target_id}` : `/monitors/${a.target_id}`} onClick={(e) => e.stopPropagation()}>
          {a.target_name}
        </Link>
      ),
    },
    { key: "state", header: "State", sort: (a) => STATE_ORDER[a.state], render: (a) => <StatusBadge status={a.state} /> },
    {
      key: "triggered",
      header: "Triggered",
      align: "right",
      sort: (a) => Date.parse(a.triggered_at),
      render: (a) => <span className="num" title={relative(a.triggered_at)}>{timestamp(a.triggered_at)}</span>,
    },
    {
      key: "resolved",
      header: "Resolved",
      align: "right",
      hideOnMobile: true,
      sort: (a) => (a.resolved_at ? Date.parse(a.resolved_at) : null),
      render: (a) => <span className="num text-2">{a.resolved_at ? timestamp(a.resolved_at) : "—"}</span>,
    },
    {
      key: "dur",
      header: "Duration",
      align: "right",
      hideOnMobile: true,
      sort: (a) => (a.resolved_at ? Date.parse(a.resolved_at) : Date.now()) - Date.parse(a.triggered_at),
      render: (a) => <span className="num">{duration(((a.resolved_at ? Date.parse(a.resolved_at) : Date.now()) - Date.parse(a.triggered_at)) / 1000)}</span>,
    },
    {
      key: "act",
      header: "",
      align: "right",
      render: (a) =>
        a.state === "firing" ? (
          <Button size="sm" disabled={pending === a.id} onClick={() => onAck(a)}>Acknowledge</Button>
        ) : a.acknowledged_at ? (
          <span className="text-muted" style={{ fontSize: "var(--text-xs)" }} title={timestamp(a.acknowledged_at)}>ack'd {relative(a.acknowledged_at)}</span>
        ) : null,
    },
  ];
}

function AlertsTab({ state }: { state: "open" | "resolved" | "" }) {
  const alerts = useAlerts(state || undefined);
  const ack = useAckAlert();
  const [q, setQ] = useState("");
  const [sev, setSev] = useState("");
  const cols = useMemo(() => alertColumns((a) => ack.mutate(a.id), ack.isPending ? ack.variables : undefined), [ack]);
  const rows = alerts.data?.filter((a) => (!sev || a.severity === sev) && matches(q, a.rule_name, a.target_name, a.message));

  return (
    <>
      <FilterBar
        query={q}
        onQuery={setQ}
        placeholder="Filter by alert, host or endpoint…"
        filters={[{ label: "Severity", value: sev, onChange: setSev, options: [{ value: "", label: "All" }, { value: "critical", label: "Critical" }, { value: "warning", label: "Warning" }, { value: "info", label: "Info" }] }]}
        count={rows && alerts.data ? { shown: rows.length, total: alerts.data.length } : undefined}
      />
      {ack.error && <div style={{ padding: 12 }}><ErrorState compact error={ack.error} /></div>}
      {alerts.error && !alerts.data ? (
        <ErrorState error={alerts.error} onRetry={() => alerts.refetch()} />
      ) : (
        <DataTable
          label="Alerts"
          columns={cols}
          rows={rows}
          loading={alerts.isPending}
          rowKey={(a) => a.id}
          defaultSort={{ key: state === "open" ? "state" : "triggered", dir: state === "open" ? "asc" : "desc" }}
          empty={<EmptyState icon={<BellOff size={28} strokeWidth={1.5} />} title={state === "open" ? "No active alerts" : "No alerts in this range"} />}
        />
      )}
    </>
  );
}

function RulesTab() {
  const rules = useRules();
  const del = useDeleteRule();
  const [editing, setEditing] = useState<AlertRule | "new" | null>(null);
  const [deleting, setDeleting] = useState<AlertRule | null>(null);

  const cols: Column<AlertRule>[] = [
    { key: "name", header: "Rule", sort: (r) => r.name, render: (r) => <span style={{ fontWeight: 500 }}>{r.name}</span> },
    { key: "kind", header: "Type", hideOnMobile: true, sort: (r) => r.kind, render: (r) => <span className="text-2">{KIND_LABEL[r.kind]}</span> },
    { key: "cond", header: "Condition", render: (r) => <span className="mono" style={{ fontSize: "var(--text-sm)" }}>{describeRule(r)}</span> },
    { key: "scope", header: "Scope", hideOnMobile: true, render: (r) => <span className="text-2">{r.host_id || r.monitor_id ? "Single target" : r.kind.startsWith("host") ? "All hosts" : "All endpoints"}</span> },
    { key: "sev", header: "Severity", sort: (r) => SEV_ORDER[r.severity], render: (r) => <SeverityLabel severity={r.severity} /> },
    { key: "enabled", header: "State", sort: (r) => (r.enabled ? 0 : 1), render: (r) => <StatusBadge status={r.enabled ? "enabled" : "disabled"} /> },
    {
      key: "act",
      header: "",
      align: "right",
      render: (r) => (
        <div className="toolbar" style={{ justifyContent: "flex-end", gap: 2 }}>
          <Button size="sm" variant="ghost" icon aria-label={`Edit ${r.name}`} onClick={() => setEditing(r)}><Pencil size={13} /></Button>
          <Button size="sm" variant="ghost" icon aria-label={`Delete ${r.name}`} onClick={() => setDeleting(r)}><Trash2 size={13} /></Button>
        </div>
      ),
    },
  ];

  return (
    <>
      <div className="filterbar">
        <span className="text-2" style={{ fontSize: "var(--text-sm)" }}>Rules are evaluated continuously. Each rule opens at most one alert per host or endpoint.</span>
        <span className="spacer" />
        <Button variant="primary" size="sm" onClick={() => setEditing("new")}><Plus size={14} /> New rule</Button>
      </div>
      {rules.error && !rules.data ? (
        <ErrorState error={rules.error} onRetry={() => rules.refetch()} />
      ) : (
        <DataTable label="Alert rules" columns={cols} rows={rules.data} loading={rules.isPending} rowKey={(r) => r.id} empty={<EmptyState title="No alert rules" />} />
      )}
      <RuleForm open={editing !== null} onClose={() => setEditing(null)} rule={editing && editing !== "new" ? editing : undefined} />
      <ConfirmationDialog
        open={deleting !== null}
        title="Delete alert rule?"
        message={<>Deleting <strong>{deleting?.name}</strong> also removes its alert history.</>}
        confirmLabel="Delete rule"
        onCancel={() => setDeleting(null)}
        onConfirm={async () => {
          await del.mutateAsync(deleting!.id);
          setDeleting(null);
        }}
      />
    </>
  );
}

const TABS = [
  { key: "active", label: "Active" },
  { key: "history", label: "History" },
  { key: "rules", label: "Rules" },
] as const;

export function AlertsPage() {
  const [params, setParams] = useSearchParams();
  const tab = (params.get("tab") ?? "active") as (typeof TABS)[number]["key"];
  usePageMeta({ title: "Alerts", crumbs: [{ label: "Pulsewatch", to: "/" }, { label: "Alerts" }], noTimeRange: tab !== "history" });
  const firing = useAlerts("open").data?.length;

  return (
    <Page>
      <Panel flush>
        <div className="tabs" role="tablist" style={{ padding: "0 12px" }}>
          {TABS.map((t) => (
            <button key={t.key} role="tab" aria-selected={tab === t.key} onClick={() => setParams({ tab: t.key })}>
              {t.label}
              {t.key === "active" && firing ? <span className="badge" data-tone="danger" style={{ marginLeft: 6 }}>{firing}</span> : null}
            </button>
          ))}
        </div>
        <div role="tabpanel">
          {tab === "active" && <AlertsTab state="open" />}
          {tab === "history" && <AlertsTab state="" />}
          {tab === "rules" && <RulesTab />}
        </div>
      </Panel>
    </Page>
  );
}
