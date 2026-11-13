// Formatting helpers. Every metric renders through these so units and
// precision stay consistent across the app.

export function pct(v: number | null | undefined, digits = 1): string {
  if (v === null || v === undefined || Number.isNaN(v)) return "—";
  return `${v.toFixed(digits)}%`;
}

const BYTE_UNITS = ["B", "KB", "MB", "GB", "TB", "PB"];

export function bytes(v: number | null | undefined, digits = 1): string {
  if (v === null || v === undefined || Number.isNaN(v)) return "—";
  let i = 0;
  let n = Math.abs(v);
  while (n >= 1024 && i < BYTE_UNITS.length - 1) {
    n /= 1024;
    i++;
  }
  return `${(Math.sign(v) * n).toFixed(i === 0 ? 0 : digits)} ${BYTE_UNITS[i]}`;
}

/** Network throughput in bits per second (input is bytes/second). */
export function bitrate(bytesPerSec: number | null | undefined): string {
  if (bytesPerSec === null || bytesPerSec === undefined || Number.isNaN(bytesPerSec)) return "—";
  const units = ["bps", "Kbps", "Mbps", "Gbps", "Tbps"];
  let n = bytesPerSec * 8;
  let i = 0;
  while (n >= 1000 && i < units.length - 1) {
    n /= 1000;
    i++;
  }
  return `${n.toFixed(n >= 100 || i === 0 ? 0 : 1)} ${units[i]}`;
}

export function ms(v: number | null | undefined): string {
  if (v === null || v === undefined || Number.isNaN(v)) return "—";
  if (v >= 10_000) return `${(v / 1000).toFixed(1)} s`;
  return `${Math.round(v)} ms`;
}

export function duration(seconds: number | null | undefined): string {
  if (seconds === null || seconds === undefined || Number.isNaN(seconds)) return "—";
  const s = Math.max(0, Math.floor(seconds));
  const d = Math.floor(s / 86400);
  const h = Math.floor((s % 86400) / 3600);
  const m = Math.floor((s % 3600) / 60);
  if (d > 0) return `${d}d ${h}h`;
  if (h > 0) return `${h}h ${m}m`;
  if (m > 0) return `${m}m ${s % 60}s`;
  return `${s}s`;
}

export function relative(iso: string | number | null | undefined, now = Date.now()): string {
  if (iso === null || iso === undefined || iso === "") return "—";
  const t = typeof iso === "number" ? iso : Date.parse(iso);
  if (Number.isNaN(t)) return "—";
  const diff = Math.round((now - t) / 1000);
  if (diff < 5) return "just now";
  if (diff < 60) return `${diff}s ago`;
  if (diff < 3600) return `${Math.floor(diff / 60)}m ago`;
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`;
  return `${Math.floor(diff / 86400)}d ago`;
}

export function timestamp(iso: string | number | null | undefined): string {
  if (iso === null || iso === undefined || iso === "") return "—";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "—";
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
}

export function interval(seconds: number): string {
  if (seconds % 3600 === 0) return `${seconds / 3600}h`;
  if (seconds % 60 === 0) return `${seconds / 60}m`;
  return `${seconds}s`;
}

/** Severity tone for a utilization percentage. */
export function utilTone(v: number | null | undefined): "success" | "warning" | "danger" | undefined {
  if (v === null || v === undefined) return undefined;
  if (v >= 90) return "danger";
  if (v >= 75) return "warning";
  return undefined;
}
