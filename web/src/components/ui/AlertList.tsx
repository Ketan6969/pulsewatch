// AlertList renders alerts; EventList renders the operational event log.
import { Activity, CheckCircle2, PlusCircle, Settings2, Bell } from "lucide-react";
import { Link } from "react-router-dom";
import type { Alert, PulseEvent } from "../../api/types";
import { duration, relative, timestamp } from "../../lib/format";
import { Button } from "./primitives";
import { SeverityIcon, StatusBadge } from "./status";

function targetLink(type?: string, id?: string) {
  if (!id) return undefined;
  return type === "host" ? `/hosts/${id}` : type === "monitor" ? `/monitors/${id}` : undefined;
}

export function AlertList({
  alerts,
  onAcknowledge,
  ackPending,
}: {
  alerts: Alert[];
  onAcknowledge?: (a: Alert) => void;
  ackPending?: string;
}) {
  return (
    <ul className="list">
      {alerts.map((a) => {
        const end = a.resolved_at ? Date.parse(a.resolved_at) : Date.now();
        const to = targetLink(a.target_type, a.target_id);
        return (
          <li key={a.id} className="list__item" data-state={a.state}>
            <span className="list__icon">
              <SeverityIcon severity={a.severity} resolved={a.state === "resolved"} />
            </span>
            <div style={{ minWidth: 0 }}>
              <div className="list__title truncate">
                {a.rule_name} · {to ? <Link to={to}>{a.target_name}</Link> : a.target_name}
              </div>
              <div className="list__meta">
                <StatusBadge status={a.state} />
                <span className="truncate" style={{ maxWidth: 520 }} title={a.message}>
                  {a.message}
                </span>
              </div>
            </div>
            <div style={{ textAlign: "right", display: "flex", flexDirection: "column", alignItems: "flex-end", gap: 4 }}>
              <span className="list__time" title={timestamp(a.triggered_at)}>
                {relative(a.triggered_at)} · {duration((end - Date.parse(a.triggered_at)) / 1000)}
              </span>
              {onAcknowledge && a.state === "firing" && (
                <Button size="sm" onClick={() => onAcknowledge(a)} disabled={ackPending === a.id}>
                  Acknowledge
                </Button>
              )}
            </div>
          </li>
        );
      })}
    </ul>
  );
}

const EVENT_ICON: Record<string, typeof Activity> = {
  alert_firing: Bell,
  alert_resolved: CheckCircle2,
  alert_acknowledged: Bell,
  config_change: Settings2,
  host_registered: PlusCircle,
};

const EVENT_LABEL: Record<string, string> = {
  alert_firing: "Alert",
  alert_resolved: "Recovery",
  alert_acknowledged: "Acknowledged",
  config_change: "Config change",
  host_registered: "Host added",
};

export function EventList({ events }: { events: PulseEvent[] }) {
  return (
    <ul className="list">
      {events.map((e) => {
        const Icon = EVENT_ICON[e.kind] ?? Activity;
        const color =
          e.kind === "alert_firing"
            ? e.severity === "critical"
              ? "var(--color-danger)"
              : "var(--color-warning)"
            : e.kind === "alert_resolved"
              ? "var(--color-success)"
              : "var(--color-text-muted)";
        const to = targetLink(e.target_type, e.target_id);
        return (
          <li key={e.id} className="list__item">
            <span className="list__icon">
              <Icon size={15} color={color} aria-hidden />
            </span>
            <div style={{ minWidth: 0 }}>
              <div className="truncate" title={e.message}>
                {to ? <Link to={to} style={{ color: "inherit" }}>{e.message}</Link> : e.message}
              </div>
              <div className="list__meta">{EVENT_LABEL[e.kind] ?? e.kind}</div>
            </div>
            <span className="list__time" title={timestamp(e.ts)}>
              {relative(e.ts)}
            </span>
          </li>
        );
      })}
    </ul>
  );
}
