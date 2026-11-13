// Shared series definitions. Defined once at module scope so chart identity
// is stable across renders (uPlot is only rebuilt when these change).
import { bitrate, ms, pct } from "../../lib/format";
import type { Line } from "./TimeSeriesChart";

export const CPU_MEM: Line[] = [
  { key: "cpu_pct", label: "CPU", color: "--chart-1" },
  { key: "mem_pct", label: "Memory", color: "--chart-2" },
];
export const DISK: Line[] = [{ key: "disk_pct", label: "Disk used", color: "--chart-1" }];
export const NETWORK: Line[] = [
  { key: "net_rx_bps", label: "Inbound", color: "--chart-1" },
  { key: "net_tx_bps", label: "Outbound", color: "--chart-2" },
];
export const LOAD: Line[] = [{ key: "load1", label: "Load (1m)", color: "--chart-1" }];
export const RESPONSE: Line[] = [
  { key: "avg_ms", label: "Avg response", color: "--chart-1" },
  { key: "max_ms", label: "Max response", color: "--chart-2" },
];
export const AVAILABILITY: Line[] = [{ key: "availability", label: "Availability", color: "--chart-3" }];

export const fmtPct = (v: number) => (v === 0 ? "0%" : pct(v, v >= 10 || Number.isInteger(v) ? 0 : 1));
export const fmtBitrate = (v: number) => bitrate(v);
export const fmtMs = (v: number) => (v > 0 && v < 10 && !Number.isInteger(v) ? `${v.toFixed(1)} ms` : ms(v));
export const fmtNum = (v: number) => v.toFixed(2);
