import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from "react";

export const PRESETS = [
  { key: "15m", label: "15m", ms: 15 * 60_000 },
  { key: "1h", label: "1h", ms: 60 * 60_000 },
  { key: "6h", label: "6h", ms: 6 * 3600_000 },
  { key: "24h", label: "24h", ms: 24 * 3600_000 },
  { key: "7d", label: "7d", ms: 7 * 86400_000 },
  { key: "30d", label: "30d", ms: 30 * 86400_000 },
] as const;

export type PresetKey = (typeof PRESETS)[number]["key"];

/** Either a relative preset or an absolute window (from chart zoom). */
export type TimeRange = { kind: "preset"; key: PresetKey } | { kind: "absolute"; from: number; to: number };

interface Ctx {
  range: TimeRange;
  setPreset: (k: PresetKey) => void;
  setAbsolute: (from: number, to: number) => void;
  /** Query params for the API: {range} or {from,to}. */
  params: { range?: string; from?: number; to?: number };
  label: string;
}

const TimeRangeContext = createContext<Ctx | null>(null);
const STORAGE_KEY = "pulsewatch.range";

function initial(): TimeRange {
  try {
    const k = localStorage.getItem(STORAGE_KEY);
    if (k && PRESETS.some((p) => p.key === k)) return { kind: "preset", key: k as PresetKey };
  } catch {
    /* ignore */
  }
  return { kind: "preset", key: "1h" };
}

export function TimeRangeProvider({ children }: { children: ReactNode }) {
  const [range, setRange] = useState<TimeRange>(initial);

  const setPreset = useCallback((key: PresetKey) => {
    setRange({ kind: "preset", key });
    try {
      localStorage.setItem(STORAGE_KEY, key);
    } catch {
      /* ignore */
    }
  }, []);
  const setAbsolute = useCallback((from: number, to: number) => {
    if (to - from >= 60_000) setRange({ kind: "absolute", from: Math.round(from), to: Math.round(to) });
  }, []);

  const value = useMemo<Ctx>(() => {
    if (range.kind === "preset") return { range, setPreset, setAbsolute, params: { range: range.key }, label: `Last ${range.key}` };
    const fmt = (t: number) => new Date(t).toLocaleString(undefined, { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" });
    return { range, setPreset, setAbsolute, params: { from: range.from, to: range.to }, label: `${fmt(range.from)} – ${fmt(range.to)}` };
  }, [range, setPreset, setAbsolute]);

  return <TimeRangeContext.Provider value={value}>{children}</TimeRangeContext.Provider>;
}

export function useTimeRange(): Ctx {
  const ctx = useContext(TimeRangeContext);
  if (!ctx) throw new Error("useTimeRange must be used within TimeRangeProvider");
  return ctx;
}
