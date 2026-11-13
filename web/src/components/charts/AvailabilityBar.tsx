// Historical availability as a strip of buckets (one cell per time bucket).
import type { Series } from "../../api/types";
import { timestamp } from "../../lib/format";
import { toAligned } from "./TimeSeriesChart";
import { LoadingSkeleton } from "../ui/states";

const MAX_CELLS = 90;

export function AvailabilityBar({ data, loading }: { data: Series | undefined; loading?: boolean }) {
  if (loading && !data) return <LoadingSkeleton variant="chart" />;
  if (!data) return null;
  const [xs, avail] = toAligned(data, ["availability"]) as [number[], (number | null)[]];

  // Merge buckets so the strip stays readable for long ranges.
  const group = Math.max(1, Math.ceil(xs.length / MAX_CELLS));
  const cells: { t: number; v: number | null }[] = [];
  for (let i = 0; i < xs.length; i += group) {
    const vals = avail.slice(i, i + group).filter((v): v is number => v !== null);
    cells.push({ t: xs[i], v: vals.length ? vals.reduce((a, b) => a + b, 0) / vals.length : null });
  }
  const known = avail.filter((v): v is number => v !== null);
  const overall = known.length ? known.reduce((a, b) => a + b, 0) / known.length : null;

  return (
    <div>
      <div className="avail" role="img" aria-label={`Availability ${overall === null ? "unknown" : overall.toFixed(2) + "%"} over the selected range`}>
        {cells.map((c) => {
          const tone = c.v === null ? undefined : c.v >= 99.9 ? "success" : c.v >= 95 ? "warning" : "danger";
          return (
            <span
              key={c.t}
              className="avail__cell"
              data-tone={tone}
              title={`${timestamp(c.t * 1000)} — ${c.v === null ? "no data" : c.v.toFixed(2) + "% available"}`}
            />
          );
        })}
      </div>
      <div className="avail__axis num">
        <span>{timestamp(data.from)}</span>
        <span>{timestamp(data.to)}</span>
      </div>
    </div>
  );
}
