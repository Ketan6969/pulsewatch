// StatusBadge and HealthIndicator: status is always conveyed by text + icon/shape, never color alone.
import { AlertOctagon, AlertTriangle, CheckCircle2, Info } from "lucide-react";
import type { AlertState, HostStatus, MonitorStatus, Severity } from "../../api/types";

export type Tone = "success" | "warning" | "danger" | "info" | "neutral" | "primary";

const STATUS: Record<string, { tone: Tone; label: string }> = {
  healthy: { tone: "success", label: "Healthy" },
  degraded: { tone: "warning", label: "Degraded" },
  offline: { tone: "danger", label: "Offline" },
  up: { tone: "success", label: "Up" },
  down: { tone: "danger", label: "Down" },
  pending: { tone: "neutral", label: "Pending" },
  paused: { tone: "neutral", label: "Paused" },
  firing: { tone: "danger", label: "Firing" },
  acknowledged: { tone: "warning", label: "Acknowledged" },
  resolved: { tone: "success", label: "Resolved" },
  enabled: { tone: "success", label: "Enabled" },
  disabled: { tone: "neutral", label: "Disabled" },
};

export function statusTone(status: string): Tone {
  return STATUS[status]?.tone ?? "neutral";
}

export function StatusBadge({ status, label }: { status: HostStatus | MonitorStatus | AlertState | string; label?: string }) {
  const s = STATUS[status] ?? { tone: "neutral" as Tone, label: status };
  return (
    <span className="badge" data-tone={s.tone}>
      {label ?? s.label}
    </span>
  );
}

/** Dot + label; used inline in tables and headers. */
export function HealthIndicator({ status, label }: { status: HostStatus | MonitorStatus | AlertState | string; label?: string }) {
  const s = STATUS[status] ?? { tone: "neutral" as Tone, label: status };
  return (
    <span className="health">
      <span className="dot" data-tone={s.tone} data-pulse={status === "down" || status === "offline" || status === "firing"} aria-hidden />
      <span>{label ?? s.label}</span>
    </span>
  );
}

const SEV: Record<Severity, { tone: Tone; label: string; Icon: typeof Info }> = {
  critical: { tone: "danger", label: "Critical", Icon: AlertOctagon },
  warning: { tone: "warning", label: "Warning", Icon: AlertTriangle },
  info: { tone: "info", label: "Info", Icon: Info },
};

export function SeverityLabel({ severity }: { severity: Severity }) {
  const s = SEV[severity] ?? SEV.info;
  return (
    <span className="sev" style={{ color: `var(--color-${s.tone === "info" ? "info" : s.tone}-text)` }}>
      <s.Icon size={14} aria-hidden />
      {s.label}
    </span>
  );
}

export function SeverityIcon({ severity, resolved }: { severity: Severity; resolved?: boolean }) {
  if (resolved) return <CheckCircle2 size={16} color="var(--color-success)" aria-label="Resolved" />;
  const s = SEV[severity] ?? SEV.info;
  return <s.Icon size={16} color={`var(--color-${s.tone})`} aria-label={s.label} />;
}
