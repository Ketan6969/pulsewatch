import { X } from "lucide-react";
import { PRESETS, useTimeRange } from "../../lib/timeRange";
import { Button } from "./primitives";

export function TimeRangeSelector() {
  const { range, setPreset, label } = useTimeRange();
  if (range.kind === "absolute") {
    return (
      <div className="toolbar" style={{ gap: 4 }}>
        <span className="badge" data-tone="primary" title="Custom range selected on a chart">
          <span className="num" style={{ textTransform: "none" }}>
            {label}
          </span>
        </span>
        <Button size="sm" variant="ghost" icon aria-label="Reset time range" onClick={() => setPreset("1h")}>
          <X size={14} />
        </Button>
      </div>
    );
  }
  return (
    <div className="segmented" role="group" aria-label="Time range">
      {PRESETS.map((p) => (
        <button key={p.key} aria-pressed={range.key === p.key} onClick={() => setPreset(p.key)} title={`Last ${p.label}`}>
          {p.label}
        </button>
      ))}
    </div>
  );
}
