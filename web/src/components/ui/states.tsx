// EmptyState, ErrorState and LoadingSkeleton.
import { AlertCircle, Inbox } from "lucide-react";
import type { CSSProperties, ReactNode } from "react";
import { ApiError } from "../../api/client";
import { Button } from "./primitives";

export function Skeleton({ width = "100%", height = 14, style }: { width?: number | string; height?: number | string; style?: CSSProperties }) {
  return <span className="skeleton" style={{ width, height, ...style }} aria-hidden />;
}

/** Placeholder sized like the content it replaces, to avoid layout jumps. */
export function LoadingSkeleton({ rows = 5, variant = "table" }: { rows?: number; variant?: "table" | "chart" | "list" }) {
  if (variant === "chart") {
    return (
      <div role="status" aria-label="Loading chart">
        <Skeleton height={200} />
      </div>
    );
  }
  return (
    <div role="status" aria-label="Loading" style={{ display: "flex", flexDirection: "column" }}>
      {Array.from({ length: rows }, (_, i) => (
        <div key={i} style={{ display: "flex", gap: 12, padding: "10px 12px", borderBottom: "1px solid var(--color-border)" }}>
          <Skeleton width={variant === "list" ? 16 : "22%"} />
          <Skeleton width="38%" />
          <Skeleton width="14%" />
          <Skeleton width="12%" />
        </div>
      ))}
    </div>
  );
}

export function EmptyState({ icon, title, children, action }: { icon?: ReactNode; title: string; children?: ReactNode; action?: ReactNode }) {
  return (
    <div className="empty">
      <div className="empty__icon">{icon ?? <Inbox size={28} strokeWidth={1.5} />}</div>
      <div className="empty__title">{title}</div>
      {children && <div className="empty__body">{children}</div>}
      {action}
    </div>
  );
}

export function errorMessage(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.status === 401) return "The API requires a token. Set it under Settings → API access.";
    return err.message;
  }
  return err instanceof Error ? err.message : "Unexpected error";
}

export function ErrorState({ error, onRetry, compact }: { error: unknown; onRetry?: () => void; compact?: boolean }) {
  if (compact) {
    return (
      <div className="error-banner" role="alert">
        <AlertCircle size={16} />
        <span style={{ flex: 1 }}>{errorMessage(error)}</span>
        {onRetry && (
          <Button size="sm" onClick={onRetry}>
            Retry
          </Button>
        )}
      </div>
    );
  }
  return (
    <div className="error-state" role="alert">
      <AlertCircle className="error-state__icon" size={28} strokeWidth={1.5} />
      <div className="error-state__title">Couldn't load data</div>
      <div>{errorMessage(error)}</div>
      {onRetry && <Button onClick={onRetry}>Retry</Button>}
    </div>
  );
}

/** Renders loading / error / content for a query in one place. */
export function QueryBoundary<T>({
  query,
  skeleton,
  children,
}: {
  query: { data: T | undefined; error: unknown; isPending: boolean; refetch: () => unknown };
  skeleton?: ReactNode;
  children: (data: T) => ReactNode;
}) {
  if (query.isPending) return <>{skeleton ?? <LoadingSkeleton />}</>;
  if (query.data === undefined) return <ErrorState error={query.error} onRetry={() => query.refetch()} />;
  return (
    <>
      {query.error ? <ErrorState compact error={query.error} onRetry={() => query.refetch()} /> : null}
      {children(query.data)}
    </>
  );
}
