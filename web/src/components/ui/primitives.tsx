// Low-level design primitives: Panel, Button, Field, Meter.
import { forwardRef, type ButtonHTMLAttributes, type ReactNode } from "react";

export function Panel({
  title,
  subtitle,
  actions,
  children,
  flush,
  className,
}: {
  title?: ReactNode;
  subtitle?: ReactNode;
  actions?: ReactNode;
  children: ReactNode;
  flush?: boolean;
  className?: string;
}) {
  return (
    <section className={`panel ${className ?? ""}`} aria-label={typeof title === "string" ? title : undefined}>
      {(title || actions) && (
        <header className="panel__header">
          <h2 className="panel__title">
            {title}
            {subtitle && <span className="panel__subtitle">{subtitle}</span>}
          </h2>
          {actions}
        </header>
      )}
      <div className={`panel__body ${flush ? "panel__body--flush" : ""}`}>{children}</div>
    </section>
  );
}

type Variant = "default" | "primary" | "danger" | "ghost";

export const Button = forwardRef<
  HTMLButtonElement,
  ButtonHTMLAttributes<HTMLButtonElement> & { variant?: Variant; size?: "sm"; icon?: boolean }
>(function Button({ variant = "default", size, icon, className, type = "button", ...rest }, ref) {
  const cls = ["btn", variant !== "default" && `btn--${variant}`, size && `btn--${size}`, icon && "btn--icon", className]
    .filter(Boolean)
    .join(" ");
  return <button ref={ref} type={type} className={cls} {...rest} />;
});

export function Field({
  label,
  htmlFor,
  hint,
  error,
  children,
  className,
}: {
  label: string;
  htmlFor: string;
  hint?: ReactNode;
  error?: string;
  children: ReactNode;
  className?: string;
}) {
  return (
    <div className={`field ${className ?? ""}`}>
      <label className="field__label" htmlFor={htmlFor}>
        {label}
      </label>
      {children}
      {error ? (
        <span className="field__error" id={`${htmlFor}-error`} role="alert">
          {error}
        </span>
      ) : (
        hint && <span className="field__hint">{hint}</span>
      )}
    </div>
  );
}

export function Meter({ value, tone, label }: { value: number | null | undefined; tone?: string; label: string }) {
  const v = Math.max(0, Math.min(100, value ?? 0));
  return (
    <div className="meter" role="meter" aria-label={label} aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.round(v)}>
      <div className="meter__fill" data-tone={tone} style={{ width: `${v}%` }} />
    </div>
  );
}
