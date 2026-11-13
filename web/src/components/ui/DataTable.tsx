// Generic compact table with client-side sorting and pagination.
// Filtering is done by the caller (see FilterBar) so filter state can live
// alongside page-specific controls.
import { ArrowDown, ArrowUp, ChevronLeft, ChevronRight } from "lucide-react";
import { useEffect, useMemo, useState, type ReactNode } from "react";
import { Button } from "./primitives";
import { EmptyState, LoadingSkeleton } from "./states";

export interface Column<T> {
  key: string;
  header: string;
  render: (row: T) => ReactNode;
  /** Value used for sorting; omit to make the column unsortable. */
  sort?: (row: T) => string | number | null | undefined;
  align?: "right";
  width?: number | string;
  /** Hide on narrow screens to keep mobile tables readable. */
  hideOnMobile?: boolean;
}

export interface DataTableProps<T> {
  columns: Column<T>[];
  rows: T[] | undefined;
  rowKey: (row: T) => string;
  loading?: boolean;
  onRowClick?: (row: T) => void;
  defaultSort?: { key: string; dir: "asc" | "desc" };
  pageSize?: number;
  empty?: ReactNode;
  label: string;
}

export function DataTable<T>({
  columns,
  rows,
  rowKey,
  loading,
  onRowClick,
  defaultSort,
  pageSize = 25,
  empty,
  label,
}: DataTableProps<T>) {
  const [sort, setSort] = useState(defaultSort);
  const [page, setPage] = useState(0);

  const sorted = useMemo(() => {
    if (!rows) return [];
    const col = columns.find((c) => c.key === sort?.key);
    if (!col?.sort || !sort) return rows;
    const get = col.sort;
    const dir = sort.dir === "asc" ? 1 : -1;
    return [...rows].sort((a, b) => {
      const va = get(a);
      const vb = get(b);
      if (va === vb) return 0;
      if (va === null || va === undefined) return 1; // missing values last
      if (vb === null || vb === undefined) return -1;
      if (typeof va === "number" && typeof vb === "number") return (va - vb) * dir;
      return String(va).localeCompare(String(vb), undefined, { numeric: true }) * dir;
    });
  }, [rows, columns, sort]);

  const pages = Math.max(1, Math.ceil(sorted.length / pageSize));
  useEffect(() => {
    if (page >= pages) setPage(pages - 1);
  }, [page, pages]);
  const visible = sorted.slice(page * pageSize, page * pageSize + pageSize);

  const toggleSort = (key: string) =>
    setSort((s) => (s?.key === key ? { key, dir: s.dir === "asc" ? "desc" : "asc" } : { key, dir: "asc" }));

  if (loading && !rows) return <LoadingSkeleton rows={6} />;
  if (rows && rows.length === 0) return <>{empty ?? <EmptyState title="No results" />}</>;

  return (
    <>
      <div className="table-wrap">
        <table className="table" aria-label={label}>
          <thead>
            <tr>
              {columns.map((c) => {
                const active = sort?.key === c.key;
                return (
                  <th
                    key={c.key}
                    scope="col"
                    style={{ width: c.width }}
                    className={[c.align === "right" && "align-right", c.hideOnMobile && "hide-mobile"].filter(Boolean).join(" ")}
                    aria-sort={active ? (sort!.dir === "asc" ? "ascending" : "descending") : undefined}
                  >
                    {c.sort ? (
                      <button onClick={() => toggleSort(c.key)}>
                        {c.header}
                        {active && (sort!.dir === "asc" ? <ArrowUp size={12} /> : <ArrowDown size={12} />)}
                      </button>
                    ) : (
                      c.header
                    )}
                  </th>
                );
              })}
            </tr>
          </thead>
          <tbody>
            {visible.map((row) => (
              <tr
                key={rowKey(row)}
                data-clickable={!!onRowClick}
                tabIndex={onRowClick ? 0 : undefined}
                onClick={onRowClick ? () => onRowClick(row) : undefined}
                onKeyDown={
                  onRowClick
                    ? (e) => {
                        if (e.key === "Enter" && e.target === e.currentTarget) onRowClick(row);
                      }
                    : undefined
                }
              >
                {columns.map((c) => (
                  <td
                    key={c.key}
                    className={[c.align === "right" && "align-right", c.hideOnMobile && "hide-mobile"].filter(Boolean).join(" ")}
                  >
                    {c.render(row)}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {sorted.length > pageSize && (
        <div className="table-footer">
          <span className="num">
            {page * pageSize + 1}–{Math.min(sorted.length, (page + 1) * pageSize)} of {sorted.length}
          </span>
          <div className="pager">
            <Button size="sm" icon variant="ghost" aria-label="Previous page" disabled={page === 0} onClick={() => setPage(page - 1)}>
              <ChevronLeft size={14} />
            </Button>
            <span className="num">
              {page + 1} / {pages}
            </span>
            <Button size="sm" icon variant="ghost" aria-label="Next page" disabled={page >= pages - 1} onClick={() => setPage(page + 1)}>
              <ChevronRight size={14} />
            </Button>
          </div>
        </div>
      )}
    </>
  );
}
