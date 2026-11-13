import { Server } from "lucide-react";
import { useMemo, useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { useHosts } from "../api/queries";
import { DataTable } from "../components/ui/DataTable";
import { FilterBar, matches } from "../components/ui/FilterBar";
import { Panel } from "../components/ui/primitives";
import { EmptyState, ErrorState } from "../components/ui/states";
import { hostColumns } from "../features/hosts/hostColumns";
import { Page, usePageMeta } from "../layout/AppShell";

export function HostsPage() {
  usePageMeta({ title: "Hosts", crumbs: [{ label: "Pulsewatch", to: "/" }, { label: "Hosts" }], noTimeRange: true });
  const hosts = useHosts();
  const navigate = useNavigate();
  const [params, setParams] = useSearchParams();
  const [q, setQ] = useState("");
  const status = params.get("status") ?? "";

  const rows = useMemo(
    () => hosts.data?.filter((h) => (!status || h.status === status) && matches(q, h.name, h.hostname, h.platform, h.os)),
    [hosts.data, q, status],
  );

  return (
    <Page>
      <Panel title="Host inventory" flush>
        <FilterBar
          query={q}
          onQuery={setQ}
          placeholder="Filter by name, hostname, OS…"
          filters={[
            {
              label: "Status",
              value: status,
              onChange: (v) => setParams(v ? { status: v } : {}),
              options: [
                { value: "", label: "All" },
                { value: "healthy", label: "Healthy" },
                { value: "degraded", label: "Degraded" },
                { value: "offline", label: "Offline" },
              ],
            },
          ]}
          count={rows && hosts.data ? { shown: rows.length, total: hosts.data.length } : undefined}
        />
        {hosts.error && !hosts.data ? (
          <ErrorState error={hosts.error} onRetry={() => hosts.refetch()} />
        ) : (
          <DataTable
            label="Hosts"
            columns={hostColumns}
            rows={rows}
            loading={hosts.isPending}
            rowKey={(h) => h.id}
            onRowClick={(h) => navigate(`/hosts/${h.id}`)}
            defaultSort={{ key: "status", dir: "asc" }}
            empty={
              hosts.data?.length === 0 ? (
                <EmptyState icon={<Server size={28} strokeWidth={1.5} />} title="No hosts reporting yet">
                  Install <code>pulsewatch-agent</code> on a Linux machine and point it at this server with{" "}
                  <code>PULSEWATCH_SERVER_URL</code> and <code>PULSEWATCH_AGENT_TOKEN</code>.
                </EmptyState>
              ) : (
                <EmptyState title="No hosts match these filters" />
              )
            }
          />
        )}
      </Panel>
    </Page>
  );
}
