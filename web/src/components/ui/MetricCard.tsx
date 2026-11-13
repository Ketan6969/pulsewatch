import type { ReactNode } from "react";
import { Link } from "react-router-dom";
import { Meter } from "./primitives";
import { Skeleton } from "./states";

export interface MetricCardProps {
  label: string;
  value: ReactNode;
  unit?: string;
  icon?: ReactNode;
  tone?: "success" | "warning" | "danger";
  /** 0–100; renders a utilization meter under the value. */
  meter?: number | null;
  footer?: ReactNode;
  to?: string;
  loading?: boolean;
}

export function MetricCard({ label, value, unit, icon, tone, meter, footer, to, loading }: MetricCardProps) {
  const body = (
    <>
      <div className="metric__label">
        {icon}
        <span>{label}</span>
      </div>
      {loading ? (
        <Skeleton width="60%" height={26} />
      ) : (
        <div className="metric__value">
          {value}
          {unit && <span className="metric__unit">{unit}</span>}
        </div>
      )}
      {meter !== undefined && <Meter value={meter} tone={tone} label={label} />}
      {footer && <div className="metric__foot">{footer}</div>}
    </>
  );
  return to ? (
    <Link to={to} className="metric" data-tone={tone}>
      {body}
    </Link>
  ) : (
    <div className="metric" data-tone={tone}>
      {body}
    </div>
  );
}
