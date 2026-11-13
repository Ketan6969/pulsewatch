import { Search } from "lucide-react";
import { useId, type ReactNode } from "react";

export interface FilterOption {
  value: string;
  label: string;
}

/** Single row of filter controls above a table: text search + select filters. */
export function FilterBar({
  query,
  onQuery,
  placeholder = "Filter…",
  filters = [],
  count,
  actions,
}: {
  query: string;
  onQuery: (q: string) => void;
  placeholder?: string;
  filters?: { label: string; value: string; options: FilterOption[]; onChange: (v: string) => void }[];
  count?: { shown: number; total: number };
  actions?: ReactNode;
}) {
  const id = useId();
  return (
    <div className="filterbar" role="search">
      <div className="filterbar__search">
        <Search size={14} />
        <label htmlFor={id} className="sr-only">
          {placeholder}
        </label>
        <input id={id} className="input" type="search" value={query} placeholder={placeholder} onChange={(e) => onQuery(e.target.value)} />
      </div>
      {filters.map((f) => (
        <label key={f.label} className="toolbar" style={{ gap: 6 }}>
          <span className="text-2" style={{ fontSize: "var(--text-sm)" }}>
            {f.label}
          </span>
          <select className="select" value={f.value} onChange={(e) => f.onChange(e.target.value)}>
            {f.options.map((o) => (
              <option key={o.value} value={o.value}>
                {o.label}
              </option>
            ))}
          </select>
        </label>
      ))}
      {count && (
        <span className="filterbar__count num">
          {count.shown === count.total ? count.total : `${count.shown} of ${count.total}`}
        </span>
      )}
      {actions}
    </div>
  );
}

/** Case-insensitive match of q against any of the given fields. */
export function matches(q: string, ...fields: (string | undefined | null)[]): boolean {
  const needle = q.trim().toLowerCase();
  if (!needle) return true;
  return fields.some((f) => f?.toLowerCase().includes(needle));
}
