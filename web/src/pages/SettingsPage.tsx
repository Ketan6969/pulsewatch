import { useQueryClient } from "@tanstack/react-query";
import { Moon, Send, Sun, Trash2, Webhook as WebhookIcon } from "lucide-react";
import { useState, type FormEvent } from "react";
import { ApiError, getApiToken, setApiToken } from "../api/client";
import { useCreateWebhook, useDeleteWebhook, useTestWebhook, useWebhooks } from "../api/queries";
import type { Webhook } from "../api/types";
import { DataTable, type Column } from "../components/ui/DataTable";
import { ConfirmationDialog } from "../components/ui/Dialog";
import { Button, Field, Panel } from "../components/ui/primitives";
import { EmptyState, ErrorState } from "../components/ui/states";
import { StatusBadge } from "../components/ui/status";
import { Page, usePageMeta } from "../layout/AppShell";
import { timestamp } from "../lib/format";
import { useTheme } from "../lib/theme";

function WebhookSection() {
  const hooks = useWebhooks();
  const create = useCreateWebhook();
  const del = useDeleteWebhook();
  const test = useTestWebhook();
  const [form, setForm] = useState({ name: "", url: "", secret: "" });
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [deleting, setDeleting] = useState<Webhook | null>(null);
  const [testResult, setTestResult] = useState<{ id: string; ok: boolean; msg: string }>();

  async function submit(e: FormEvent) {
    e.preventDefault();
    const errs: Record<string, string> = {};
    if (!form.name.trim()) errs.name = "Name is required";
    if (!/^https?:\/\/.+/.test(form.url.trim())) errs.url = "Must be an http(s) URL";
    setErrors(errs);
    if (Object.keys(errs).length) return;
    try {
      await create.mutateAsync({ name: form.name.trim(), url: form.url.trim(), secret: form.secret || undefined });
      setForm({ name: "", url: "", secret: "" });
    } catch (err) {
      setErrors(err instanceof ApiError && err.fields ? err.fields : { form: err instanceof Error ? err.message : "Save failed" });
    }
  }

  const cols: Column<Webhook>[] = [
    { key: "name", header: "Name", sort: (w) => w.name, render: (w) => <span style={{ fontWeight: 500 }}>{w.name}</span> },
    { key: "url", header: "URL", render: (w) => <span className="mono text-2 truncate" style={{ display: "block", maxWidth: 360 }} title={w.url}>{w.url}</span> },
    { key: "signed", header: "Signing", hideOnMobile: true, render: (w) => (w.has_secret ? <StatusBadge status="enabled" label="HMAC" /> : <span className="text-muted">None</span>) },
    { key: "created", header: "Added", hideOnMobile: true, render: (w) => <span className="num text-2">{timestamp(w.created_at)}</span> },
    {
      key: "act",
      header: "",
      align: "right",
      render: (w) => (
        <div className="toolbar" style={{ justifyContent: "flex-end", gap: 4 }}>
          {testResult?.id === w.id && (
            <span style={{ fontSize: "var(--text-xs)", color: testResult.ok ? "var(--color-success-text)" : "var(--color-danger-text)" }} role="status">
              {testResult.msg}
            </span>
          )}
          <Button
            size="sm"
            disabled={test.isPending}
            onClick={() =>
              test.mutate(w.id, {
                onSuccess: () => setTestResult({ id: w.id, ok: true, msg: "Delivered" }),
                onError: (e) => setTestResult({ id: w.id, ok: false, msg: e.message }),
              })
            }
          >
            <Send size={12} /> Test
          </Button>
          <Button size="sm" variant="ghost" icon aria-label={`Delete ${w.name}`} onClick={() => setDeleting(w)}>
            <Trash2 size={13} />
          </Button>
        </div>
      ),
    },
  ];

  return (
    <Panel title="Notification webhooks" subtitle="alert firing and recovery events" flush>
      <form onSubmit={submit} noValidate className="filterbar" style={{ alignItems: "flex-start" }}>
        <Field label="Name" htmlFor="wh-name" error={errors.name}>
          <input id="wh-name" className="input" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="ops-slack" />
        </Field>
        <Field label="URL" htmlFor="wh-url" error={errors.url} className="filterbar__search" >
          <input id="wh-url" className="input mono" style={{ paddingLeft: 8 }} value={form.url} onChange={(e) => setForm({ ...form, url: e.target.value })} placeholder="https://hooks.example.com/…" />
        </Field>
        <Field label="Signing secret" htmlFor="wh-secret" error={errors.secret} hint="Optional · HMAC-SHA256">
          <input id="wh-secret" className="input mono" type="password" autoComplete="off" value={form.secret} onChange={(e) => setForm({ ...form, secret: e.target.value })} />
        </Field>
        <div className="field">
          <span className="field__label" aria-hidden>&nbsp;</span>
          <Button variant="primary" type="submit" disabled={create.isPending}>Add webhook</Button>
        </div>
      </form>
      {errors.form && <div style={{ padding: 12 }}><ErrorState compact error={new Error(errors.form)} /></div>}
      {hooks.error && !hooks.data ? (
        <ErrorState error={hooks.error} onRetry={() => hooks.refetch()} />
      ) : (
        <DataTable
          label="Webhooks"
          columns={cols}
          rows={hooks.data}
          loading={hooks.isPending}
          rowKey={(w) => w.id}
          empty={
            <EmptyState icon={<WebhookIcon size={28} strokeWidth={1.5} />} title="No webhooks configured">
              Alerts are still recorded and shown in the dashboard. Add a webhook to push them to Slack, PagerDuty, or your own service.
            </EmptyState>
          }
        />
      )}
      <ConfirmationDialog
        open={deleting !== null}
        title="Remove webhook?"
        message={<>Notifications will no longer be sent to <strong>{deleting?.name}</strong>.</>}
        confirmLabel="Remove"
        onCancel={() => setDeleting(null)}
        onConfirm={async () => {
          await del.mutateAsync(deleting!.id);
          setDeleting(null);
        }}
      />
    </Panel>
  );
}

function ApiAccessSection() {
  const [token, setToken] = useState(getApiToken());
  const [saved, setSaved] = useState(false);
  const qc = useQueryClient();
  return (
    <Panel title="API access">
      <form
        className="toolbar"
        style={{ alignItems: "flex-end" }}
        onSubmit={(e) => {
          e.preventDefault();
          setApiToken(token.trim());
          setSaved(true);
          qc.invalidateQueries();
        }}
      >
        <Field label="API token" htmlFor="api-token" hint="Required only when the server sets PULSEWATCH_API_TOKEN. Stored in this browser.">
          <input id="api-token" className="input mono" type="password" autoComplete="off" style={{ width: 340 }} value={token} onChange={(e) => { setToken(e.target.value); setSaved(false); }} />
        </Field>
        <Button type="submit">Save</Button>
        {saved && <span className="text-2" role="status" style={{ fontSize: "var(--text-sm)" }}>Saved</span>}
      </form>
    </Panel>
  );
}

export function SettingsPage() {
  usePageMeta({ title: "Settings", crumbs: [{ label: "Pulsewatch", to: "/" }, { label: "Settings" }], noTimeRange: true });
  const { theme, toggle } = useTheme();
  return (
    <Page>
      <WebhookSection />
      <div className="grid grid--2">
        <ApiAccessSection />
        <Panel title="Appearance">
          <div className="toolbar">
            <span className="text-2">Theme</span>
            <div className="segmented" role="group" aria-label="Theme">
              <button aria-pressed={theme === "light"} onClick={() => theme !== "light" && toggle()}>
                <Sun size={12} style={{ verticalAlign: -1 }} /> Light
              </button>
              <button aria-pressed={theme === "dark"} onClick={() => theme !== "dark" && toggle()}>
                <Moon size={12} style={{ verticalAlign: -1 }} /> Dark
              </button>
            </div>
          </div>
        </Panel>
      </div>
    </Page>
  );
}
