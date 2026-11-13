// The single time-series chart implementation used across the app.
// Built on uPlot (canvas) for performance with thousands of points.
// Features: crosshair tooltip, drag-to-zoom (updates the global time range),
// gap rendering for missing buckets, theme-aware colors, responsive width.
import { useEffect, useMemo, useRef } from "react";
import uPlot from "uplot";
import "uplot/dist/uPlot.min.css";
import type { Series } from "../../api/types";
import { cssVar, useTheme } from "../../lib/theme";
import { useTimeRange } from "../../lib/timeRange";
import { EmptyState, LoadingSkeleton } from "../ui/states";

export interface Line {
  key: string;
  label: string;
  /** CSS custom property holding the color, e.g. "--chart-1". */
  color: string;
}

export interface TimeSeriesChartProps {
  data: Series | undefined;
  lines: Line[];
  format: (v: number) => string;
  height?: number;
  yMax?: number;
  loading?: boolean;
  /** Accessible description of what the chart shows. */
  label: string;
}

/** Builds aligned uPlot data, inserting nulls for empty buckets so gaps render as gaps. */
export function toAligned(series: Series, keys: string[]): uPlot.AlignedData {
  const step = series.step_s;
  const byTs = new Map<number, Record<string, number>>();
  for (const p of series.points) byTs.set(Math.round(Date.parse(p.ts) / 1000), p.v);
  const start = Math.floor(series.from / 1000 / step) * step;
  const end = Math.floor(series.to / 1000);
  const xs: number[] = [];
  for (let t = start; t <= end; t += step) xs.push(t);
  const ys = keys.map((k) => xs.map((t) => byTs.get(t)?.[k] ?? null));
  return [xs, ...ys] as uPlot.AlignedData;
}

function lastValue(arr: (number | null | undefined)[]): number | null {
  for (let i = arr.length - 1; i >= 0; i--) if (arr[i] !== null && arr[i] !== undefined) return arr[i] as number;
  return null;
}

const timeFmt = (t: number) =>
  new Date(t * 1000).toLocaleString(undefined, { month: "short", day: "2-digit", hour: "2-digit", minute: "2-digit", second: "2-digit" });

export function TimeSeriesChart({ data, lines, format, height = 200, yMax, loading, label }: TimeSeriesChartProps) {
  const wrapRef = useRef<HTMLDivElement>(null);
  const tipRef = useRef<HTMLDivElement>(null);
  const plotRef = useRef<uPlot | null>(null);
  const { theme } = useTheme();
  const { setAbsolute } = useTimeRange();
  const formatRef = useRef(format);
  formatRef.current = format;
  const selectRef = useRef(setAbsolute);
  selectRef.current = setAbsolute;

  const keys = useMemo(() => lines.map((l) => l.key), [lines]);
  const aligned = useMemo(() => (data ? toAligned(data, keys) : null), [data, keys]);
  const hasData = !!aligned && aligned.slice(1).some((ys) => ys.some((v) => v !== null));

  // Create the plot when structure/theme changes; data updates go through setData.
  useEffect(() => {
    const el = wrapRef.current;
    if (!el || !aligned) return;
    const axis = cssVar("--chart-axis");
    const grid = cssVar("--chart-grid");
    const font = `11px ${cssVar("--font-mono") || "monospace"}`;

    const opts: uPlot.Options = {
      width: el.clientWidth || 600,
      height,
      padding: [8, 8, 0, 0],
      legend: { show: false },
      cursor: {
        drag: { x: true, y: false, setScale: false },
        points: { size: 7, width: 2 },
        focus: { prox: 24 },
      },
      focus: { alpha: 0.35 },
      scales: {
        x: { time: true },
        y: { range: (_u, _min, max) => [0, yMax ?? (max > 0 ? max * 1.15 : 1)] },
      },
      axes: [
        { stroke: axis, font, grid: { stroke: grid, width: 1 }, ticks: { stroke: grid, width: 1, size: 4 } },
        {
          stroke: axis,
          font,
          size: 64,
          grid: { stroke: grid, width: 1 },
          ticks: { show: false },
          values: (_u, vals) => vals.map((v) => formatRef.current(v)),
        },
      ],
      series: [
        {},
        ...lines.map((l) => ({
          label: l.label,
          stroke: cssVar(l.color),
          width: 1.75,
          spanGaps: false,
          points: { show: false },
        })),
      ],
      hooks: {
        setSelect: [
          (u) => {
            if (u.select.width > 4) {
              const from = u.posToVal(u.select.left, "x") * 1000;
              const to = u.posToVal(u.select.left + u.select.width, "x") * 1000;
              selectRef.current(from, to);
            }
            u.setSelect({ left: 0, top: 0, width: 0, height: 0 }, false);
          },
        ],
        setCursor: [
          (u) => {
            const tip = tipRef.current;
            if (!tip) return;
            const idx = u.cursor.idx;
            if (idx === null || idx === undefined || u.cursor.left === undefined || u.cursor.left < 0) {
              tip.style.display = "none";
              return;
            }
            const rows = lines
              .map((l, i) => {
                const v = u.data[i + 1][idx];
                return `<div class="chart__tooltip-row"><i class="chart__swatch" style="background:${cssVar(l.color)}"></i><span>${escapeHtml(
                  l.label,
                )}</span><span>${v === null || v === undefined ? "—" : escapeHtml(formatRef.current(v))}</span></div>`;
              })
              .join("");
            tip.innerHTML = `<div class="chart__tooltip-time">${escapeHtml(timeFmt(u.data[0][idx]))}</div>${rows}`;
            tip.style.display = "block";
            const over = u.over.getBoundingClientRect();
            const wrap = el.getBoundingClientRect();
            const x = over.left - wrap.left + (u.cursor.left ?? 0);
            const y = over.top - wrap.top + (u.cursor.top ?? 0);
            const w = tip.offsetWidth;
            tip.style.left = `${x + 12 + w > wrap.width ? x - w - 12 : x + 12}px`;
            tip.style.top = `${Math.max(0, y - tip.offsetHeight / 2)}px`;
          },
        ],
      },
    };
    const u = new uPlot(opts, aligned, el);
    plotRef.current = u;
    const hide = () => tipRef.current && (tipRef.current.style.display = "none");
    u.over.addEventListener("mouseleave", hide);

    const ro = new ResizeObserver(() => {
      if (el.clientWidth > 0) u.setSize({ width: el.clientWidth, height });
    });
    ro.observe(el);
    return () => {
      ro.disconnect();
      u.over.removeEventListener("mouseleave", hide);
      u.destroy();
      plotRef.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps -- data handled below
  }, [theme, height, yMax, lines, !!aligned]);

  useEffect(() => {
    if (plotRef.current && aligned) plotRef.current.setData(aligned, true);
  }, [aligned]);

  if (loading && !data) return <LoadingSkeleton variant="chart" />;

  return (
    <figure className="chart" aria-label={label} style={{ margin: 0 }}>
      <div style={{ position: "relative" }}>
        <div ref={wrapRef} className="chart__canvas" style={{ minHeight: height }} aria-hidden />
        <div ref={tipRef} className="chart__tooltip" role="presentation" />
        {aligned && !hasData && (
          <div style={{ position: "absolute", inset: 0, display: "grid", placeItems: "center", pointerEvents: "none" }}>
            <EmptyState title="No data in this time range" />
          </div>
        )}
      </div>
      {lines.length > 0 && aligned && (
        <figcaption className="chart__legend">
          {lines.map((l, i) => {
            const v = lastValue(aligned[i + 1] as (number | null)[]);
            return (
              <span key={l.key} className="chart__legend-item">
                <i className="chart__swatch" style={{ background: `var(${l.color})` }} />
                {l.label}
                <span className="chart__legend-value num">{v === null ? "—" : format(v)}</span>
              </span>
            );
          })}
          <span className="text-muted hide-mobile" style={{ marginLeft: "auto", fontSize: "var(--text-xs)" }}>
            Drag to zoom
          </span>
        </figcaption>
      )}
    </figure>
  );
}

function escapeHtml(s: string): string {
  return s.replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c]!);
}
