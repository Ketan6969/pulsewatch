// Modal dialog built on native <dialog>: focus trapping, Esc to close and
// inert background come from the platform.
import { X } from "lucide-react";
import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import { Button } from "./primitives";

export function Dialog({
  open,
  onClose,
  title,
  children,
  footer,
  size,
}: {
  open: boolean;
  onClose: () => void;
  title: string;
  children: ReactNode;
  footer?: ReactNode;
  size?: "sm";
}) {
  const ref = useRef<HTMLDialogElement>(null);
  const titleId = useId();
  useEffect(() => {
    const d = ref.current;
    if (!d) return;
    if (open && !d.open) d.showModal?.();
    if (!open && d.open) d.close();
  }, [open]);

  if (!open) return null;
  return (
    <dialog
      ref={ref}
      className={`dialog ${size === "sm" ? "dialog--sm" : ""}`}
      aria-labelledby={titleId}
      onCancel={(e) => {
        e.preventDefault();
        onClose();
      }}
      onMouseDown={(e) => {
        if (e.target === ref.current) onClose(); // backdrop click
      }}
    >
      <div className="dialog__header">
        <h2 className="dialog__title" id={titleId}>
          {title}
        </h2>
        <Button variant="ghost" icon onClick={onClose} aria-label="Close dialog">
          <X size={16} />
        </Button>
      </div>
      <div className="dialog__body">{children}</div>
      {footer && <div className="dialog__footer">{footer}</div>}
    </dialog>
  );
}

/** Confirms destructive actions. The confirm button shows progress and errors. */
export function ConfirmationDialog({
  open,
  title,
  message,
  confirmLabel = "Delete",
  onConfirm,
  onCancel,
}: {
  open: boolean;
  title: string;
  message: ReactNode;
  confirmLabel?: string;
  onConfirm: () => Promise<unknown>;
  onCancel: () => void;
}) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();
  useEffect(() => {
    if (open) {
      setBusy(false);
      setError(undefined);
    }
  }, [open]);

  return (
    <Dialog
      open={open}
      onClose={busy ? () => {} : onCancel}
      title={title}
      size="sm"
      footer={
        <>
          <Button onClick={onCancel} disabled={busy} autoFocus>
            Cancel
          </Button>
          <Button
            variant="danger"
            disabled={busy}
            onClick={async () => {
              setBusy(true);
              setError(undefined);
              try {
                await onConfirm();
              } catch (e) {
                setError(e instanceof Error ? e.message : "Action failed");
                setBusy(false);
              }
            }}
          >
            {busy ? "Working…" : confirmLabel}
          </Button>
        </>
      }
    >
      <p>{message}</p>
      {error && (
        <p className="field__error" role="alert" style={{ marginTop: 8 }}>
          {error}
        </p>
      )}
    </Dialog>
  );
}
