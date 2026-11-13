import { Plus, X } from "lucide-react";
import { useEffect, useState, type FormEvent } from "react";
import { ApiError } from "../../api/client";
import { useSaveMonitor } from "../../api/queries";
import type { Monitor, MonitorInput } from "../../api/types";
import { Dialog } from "../../components/ui/Dialog";
import { Button, Field } from "../../components/ui/primitives";

const METHODS = ["GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"];
const INTERVALS = [10, 30, 60, 120, 300, 600, 1800, 3600];

const blank: MonitorInput = {
  name: "",
  url: "https://",
  method: "GET",
  interval_s: 60,
  timeout_ms: 10000,
  expected_status: 0,
  headers: {},
  enabled: true,
};

/** Client-side checks mirror the API's; the server remains authoritative. */
export function validateMonitor(v: MonitorInput): Record<string, string> {
  const e: Record<string, string> = {};
  if (!v.name.trim()) e.name = "Name is required";
  try {
    const u = new URL(v.url);
    if (u.protocol !== "http:" && u.protocol !== "https:") e.url = "Must be an http or https URL";
  } catch {
    e.url = "Enter a valid absolute URL";
  }
  if (v.timeout_ms < 100 || v.timeout_ms > 60000) e.timeout_ms = "Between 100 and 60000 ms";
  else if (v.timeout_ms >= v.interval_s * 1000) e.timeout_ms = "Must be shorter than the check interval";
  if (v.expected_status !== 0 && (v.expected_status < 100 || v.expected_status > 599)) e.expected_status = "0 or 100–599";
  return e;
}

export function MonitorForm({ open, onClose, monitor }: { open: boolean; onClose: () => void; monitor?: Monitor }) {
  const [v, setV] = useState<MonitorInput>(blank);
  const [headers, setHeaders] = useState<{ k: string; v: string }[]>([]);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [formError, setFormError] = useState<string>();
  const save = useSaveMonitor();

  useEffect(() => {
    if (!open) return;
    const init = monitor ? { ...blank, ...monitor } : blank;
    setV(init);
    setHeaders(Object.entries(init.headers ?? {}).map(([k, val]) => ({ k, v: val })));
    setErrors({});
    setFormError(undefined);
  }, [open, monitor]);

  const set = <K extends keyof MonitorInput>(k: K, val: MonitorInput[K]) => setV((s) => ({ ...s, [k]: val }));

  async function submit(e: FormEvent) {
    e.preventDefault();
    const input: MonitorInput = {
      ...v,
      name: v.name.trim(),
      url: v.url.trim(),
      headers: Object.fromEntries(headers.filter((h) => h.k.trim()).map((h) => [h.k.trim(), h.v])),
    };
    const errs = validateMonitor(input);
    setErrors(errs);
    if (Object.keys(errs).length) return;
    try {
      await save.mutateAsync({ id: monitor?.id, input });
      onClose();
    } catch (err) {
      if (err instanceof ApiError && err.fields) setErrors(err.fields);
      setFormError(err instanceof Error ? err.message : "Save failed");
    }
  }

  const fid = (k: string) => `monitor-${k}`;
  const inv = (k: string) => (errors[k] ? { "aria-invalid": true as const, "aria-describedby": `${fid(k)}-error` } : {});

  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={monitor ? "Edit HTTP monitor" : "New HTTP monitor"}
      footer={
        <>
          <Button onClick={onClose}>Cancel</Button>
          <Button variant="primary" type="submit" form="monitor-form" disabled={save.isPending}>
            {save.isPending ? "Saving…" : monitor ? "Save changes" : "Create monitor"}
          </Button>
        </>
      }
    >
      <form id="monitor-form" onSubmit={submit} noValidate>
        {formError && (
          <div className="error-banner" role="alert" style={{ marginBottom: 12 }}>
            {formError}
          </div>
        )}
        <div className="form-grid">
          <Field label="Name" htmlFor={fid("name")} error={errors.name} className="span-2">
            <input id={fid("name")} className="input" value={v.name} onChange={(e) => set("name", e.target.value)} autoFocus {...inv("name")} />
          </Field>
          <Field label="URL" htmlFor={fid("url")} error={errors.url} className="span-2">
            <div style={{ display: "flex", gap: 8 }}>
              <select aria-label="HTTP method" className="select" value={v.method} onChange={(e) => set("method", e.target.value)} style={{ width: 100 }}>
                {METHODS.map((m) => (
                  <option key={m}>{m}</option>
                ))}
              </select>
              <input id={fid("url")} className="input mono" style={{ flex: 1 }} value={v.url} onChange={(e) => set("url", e.target.value)} {...inv("url")} />
            </div>
          </Field>
          <Field label="Check interval" htmlFor={fid("interval_s")}>
            <select id={fid("interval_s")} className="select" value={v.interval_s} onChange={(e) => set("interval_s", Number(e.target.value))}>
              {INTERVALS.map((s) => (
                <option key={s} value={s}>
                  {s < 60 ? `${s} seconds` : s < 3600 ? `${s / 60} minute${s > 60 ? "s" : ""}` : "1 hour"}
                </option>
              ))}
            </select>
          </Field>
          <Field label="Timeout (ms)" htmlFor={fid("timeout_ms")} error={errors.timeout_ms}>
            <input id={fid("timeout_ms")} className="input num" type="number" min={100} max={60000} step={100} value={v.timeout_ms} onChange={(e) => set("timeout_ms", Number(e.target.value))} {...inv("timeout_ms")} />
          </Field>
          <Field label="Expected status" htmlFor={fid("expected_status")} error={errors.expected_status} hint="0 accepts any 2xx or 3xx">
            <input id={fid("expected_status")} className="input num" type="number" min={0} max={599} value={v.expected_status} onChange={(e) => set("expected_status", Number(e.target.value))} {...inv("expected_status")} />
          </Field>
          <Field label="State" htmlFor={fid("enabled")}>
            <label className="checkbox" style={{ height: "var(--control-h)" }}>
              <input id={fid("enabled")} type="checkbox" checked={v.enabled} onChange={(e) => set("enabled", e.target.checked)} />
              Enabled
            </label>
          </Field>
          <div className="field span-2">
            <span className="field__label">Request headers</span>
            {headers.map((h, i) => (
              <div key={i} style={{ display: "flex", gap: 8 }}>
                <input aria-label="Header name" className="input mono" placeholder="Header" value={h.k} style={{ flex: 1 }}
                  onChange={(e) => setHeaders((hs) => hs.map((x, j) => (j === i ? { ...x, k: e.target.value } : x)))} />
                <input aria-label="Header value" className="input mono" placeholder="Value" value={h.v} style={{ flex: 2 }}
                  onChange={(e) => setHeaders((hs) => hs.map((x, j) => (j === i ? { ...x, v: e.target.value } : x)))} />
                <Button icon variant="ghost" aria-label="Remove header" onClick={() => setHeaders((hs) => hs.filter((_, j) => j !== i))}>
                  <X size={14} />
                </Button>
              </div>
            ))}
            {errors.headers && <span className="field__error">{errors.headers}</span>}
            <div>
              <Button size="sm" variant="ghost" onClick={() => setHeaders((hs) => [...hs, { k: "", v: "" }])} disabled={headers.length >= 20}>
                <Plus size={13} /> Add header
              </Button>
            </div>
          </div>
        </div>
      </form>
    </Dialog>
  );
}
