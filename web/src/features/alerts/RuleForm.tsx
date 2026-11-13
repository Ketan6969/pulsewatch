import { useEffect, useState, type FormEvent } from "react";
import { ApiError } from "../../api/client";
import { useHosts, useMonitors, useSaveRule } from "../../api/queries";
import type { AlertRule, RuleInput, RuleKind } from "../../api/types";
import { Dialog } from "../../components/ui/Dialog";
import { Button, Field } from "../../components/ui/primitives";

export const KIND_LABEL: Record<RuleKind, string> = {
  host_metric: "Host metric threshold",
  host_offline: "Host offline",
  monitor_down: "Endpoint down",
  monitor_latency: "Endpoint latency",
};

export const METRIC_LABEL: Record<string, string> = {
  cpu_pct: "CPU %",
  mem_pct: "Memory %",
  disk_pct: "Disk %",
  swap_pct: "Swap %",
  load1: "Load (1m)",
};

const blank: RuleInput = {
  name: "",
  kind: "host_metric",
  metric: "cpu_pct",
  operator: ">",
  threshold: 90,
  for_count: 3,
  severity: "warning",
  host_id: null,
  monitor_id: null,
  enabled: true,
};

/** Human-readable condition, e.g. "CPU % > 90 for 3 samples". */
export function describeRule(r: Pick<AlertRule, "kind" | "metric" | "operator" | "threshold" | "for_count">): string {
  switch (r.kind) {
    case "host_metric":
      return `${METRIC_LABEL[r.metric ?? ""] ?? r.metric} ${r.operator} ${r.threshold} for ${r.for_count} sample${r.for_count > 1 ? "s" : ""}`;
    case "host_offline":
      return `No heartbeat for ${r.threshold}s`;
    case "monitor_down":
      return `${r.for_count} consecutive failed check${r.for_count > 1 ? "s" : ""}`;
    case "monitor_latency":
      return `Response ${r.operator} ${r.threshold} ms for ${r.for_count} check${r.for_count > 1 ? "s" : ""}`;
  }
}

export function RuleForm({ open, onClose, rule }: { open: boolean; onClose: () => void; rule?: AlertRule }) {
  const [v, setV] = useState<RuleInput>(blank);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [formError, setFormError] = useState<string>();
  const save = useSaveRule();
  const hosts = useHosts().data ?? [];
  const monitors = useMonitors().data ?? [];

  useEffect(() => {
    if (!open) return;
    setV(rule ? { ...blank, ...rule, metric: rule.metric || "cpu_pct" } : blank);
    setErrors({});
    setFormError(undefined);
  }, [open, rule]);

  const set = <K extends keyof RuleInput>(k: K, val: RuleInput[K]) => setV((s) => ({ ...s, [k]: val }));
  const isHost = v.kind === "host_metric" || v.kind === "host_offline";

  async function submit(e: FormEvent) {
    e.preventDefault();
    const input: RuleInput = {
      ...v,
      name: v.name.trim(),
      metric: v.kind === "host_metric" ? v.metric : "",
      host_id: isHost ? v.host_id || null : null,
      monitor_id: isHost ? null : v.monitor_id || null,
    };
    const errs: Record<string, string> = {};
    if (!input.name) errs.name = "Name is required";
    if (!Number.isFinite(input.threshold)) errs.threshold = "Enter a number";
    if (input.for_count < 1 || input.for_count > 100) errs.for_count = "Between 1 and 100";
    setErrors(errs);
    if (Object.keys(errs).length) return;
    try {
      await save.mutateAsync({ id: rule?.id, input });
      onClose();
    } catch (err) {
      if (err instanceof ApiError && err.fields) setErrors(err.fields);
      setFormError(err instanceof Error ? err.message : "Save failed");
    }
  }

  const id = (k: string) => `rule-${k}`;
  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={rule ? "Edit alert rule" : "New alert rule"}
      footer={
        <>
          <Button onClick={onClose}>Cancel</Button>
          <Button variant="primary" type="submit" form="rule-form" disabled={save.isPending}>
            {save.isPending ? "Saving…" : rule ? "Save changes" : "Create rule"}
          </Button>
        </>
      }
    >
      <form id="rule-form" onSubmit={submit} noValidate>
        {formError && <div className="error-banner" role="alert" style={{ marginBottom: 12 }}>{formError}</div>}
        <div className="form-grid">
          <Field label="Name" htmlFor={id("name")} error={errors.name} className="span-2">
            <input id={id("name")} className="input" value={v.name} onChange={(e) => set("name", e.target.value)} autoFocus aria-invalid={!!errors.name} />
          </Field>
          <Field label="Type" htmlFor={id("kind")} error={errors.kind}>
            <select id={id("kind")} className="select" value={v.kind}
              onChange={(e) => {
                const kind = e.target.value as RuleKind;
                const defaults: Partial<RuleInput> =
                  kind === "host_offline" ? { threshold: 90, for_count: 1 } : kind === "monitor_down" ? { threshold: 0, for_count: 3 } : kind === "monitor_latency" ? { threshold: 2000, for_count: 3 } : { threshold: 90, for_count: 3 };
                setV((s) => ({ ...s, kind, ...defaults }));
              }}>
              {Object.entries(KIND_LABEL).map(([k, l]) => <option key={k} value={k}>{l}</option>)}
            </select>
          </Field>
          <Field label="Severity" htmlFor={id("severity")} error={errors.severity}>
            <select id={id("severity")} className="select" value={v.severity} onChange={(e) => set("severity", e.target.value as RuleInput["severity"])}>
              <option value="critical">Critical</option>
              <option value="warning">Warning</option>
              <option value="info">Info</option>
            </select>
          </Field>
          {v.kind === "host_metric" && (
            <Field label="Metric" htmlFor={id("metric")} error={errors.metric}>
              <select id={id("metric")} className="select" value={v.metric} onChange={(e) => set("metric", e.target.value)}>
                {Object.entries(METRIC_LABEL).map(([k, l]) => <option key={k} value={k}>{l}</option>)}
              </select>
            </Field>
          )}
          {(v.kind === "host_metric" || v.kind === "monitor_latency") && (
            <Field label="Operator" htmlFor={id("operator")}>
              <select id={id("operator")} className="select" value={v.operator} onChange={(e) => set("operator", e.target.value)}>
                {[">", ">=", "<", "<="].map((o) => <option key={o}>{o}</option>)}
              </select>
            </Field>
          )}
          {v.kind !== "monitor_down" && (
            <Field label={v.kind === "host_offline" ? "Silence threshold (s)" : v.kind === "monitor_latency" ? "Threshold (ms)" : "Threshold"} htmlFor={id("threshold")} error={errors.threshold}>
              <input id={id("threshold")} className="input num" type="number" value={v.threshold} onChange={(e) => set("threshold", Number(e.target.value))} aria-invalid={!!errors.threshold} />
            </Field>
          )}
          {v.kind !== "host_offline" && (
            <Field label="Consecutive evaluations" htmlFor={id("for_count")} error={errors.for_count} hint="How many samples/checks in a row must breach">
              <input id={id("for_count")} className="input num" type="number" min={1} max={100} value={v.for_count} onChange={(e) => set("for_count", Number(e.target.value))} aria-invalid={!!errors.for_count} />
            </Field>
          )}
          <Field label="Scope" htmlFor={id("scope")} error={errors.host_id ?? errors.monitor_id} className="span-2">
            {isHost ? (
              <select id={id("scope")} className="select" value={v.host_id ?? ""} onChange={(e) => set("host_id", e.target.value || null)}>
                <option value="">All hosts</option>
                {hosts.map((h) => <option key={h.id} value={h.id}>{h.name}</option>)}
              </select>
            ) : (
              <select id={id("scope")} className="select" value={v.monitor_id ?? ""} onChange={(e) => set("monitor_id", e.target.value || null)}>
                <option value="">All endpoints</option>
                {monitors.map((m) => <option key={m.id} value={m.id}>{m.name}</option>)}
              </select>
            )}
          </Field>
          <label className="checkbox span-2">
            <input type="checkbox" checked={v.enabled} onChange={(e) => set("enabled", e.target.checked)} /> Enabled
          </label>
          <p className="span-2 text-2" style={{ fontSize: "var(--text-sm)" }}>
            Condition: <strong>{describeRule(v)}</strong>
          </p>
        </div>
      </form>
    </Dialog>
  );
}
