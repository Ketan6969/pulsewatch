// React Query hooks: the only place pages touch the API.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTimeRange } from "../lib/timeRange";
import { api } from "./client";
import type {
  Alert,
  AlertRule,
  AlertState,
  Host,
  HostSnapshot,
  MonitorInput,
  MonitorSummary,
  Monitor,
  Overview,
  PulseEvent,
  RuleInput,
  Series,
  CheckResult,
  Webhook,
} from "./types";

/** Dashboard data refreshes on this cadence. */
export const REFRESH_MS = 15_000;


export function useOverview() {
  const { params } = useTimeRange();
  return useQuery({
    queryKey: ["overview", params],
    queryFn: ({ signal }) => api.get<Overview>("/api/v1/overview", params, signal),
    refetchInterval: REFRESH_MS,
    placeholderData: (prev) => prev,
  });
}

export function useHosts() {
  return useQuery({
    queryKey: ["hosts"],
    queryFn: ({ signal }) => api.get<{ hosts: Host[] }>("/api/v1/hosts", undefined, signal).then((r) => r.hosts),
    refetchInterval: REFRESH_MS,
    placeholderData: (prev) => prev,
  });
}

export function useHost(id: string) {
  return useQuery({
    queryKey: ["hosts", id],
    queryFn: ({ signal }) => api.get<{ host: Host; snapshot: HostSnapshot }>(`/api/v1/hosts/${encodeURIComponent(id)}`, undefined, signal),
    refetchInterval: REFRESH_MS,
    placeholderData: (prev) => prev,
  });
}

/** Host resource series; pass hostId=undefined for the fleet aggregate. */
export function useHostSeries(hostId?: string) {
  const { params } = useTimeRange();
  const path = hostId ? `/api/v1/hosts/${encodeURIComponent(hostId)}/metrics` : "/api/v1/metrics/hosts";
  return useQuery({
    queryKey: ["host-series", hostId ?? "all", params],
    queryFn: ({ signal }) => api.get<Series>(path, params, signal),
    refetchInterval: REFRESH_MS,
    placeholderData: (prev) => prev,
  });
}

export function useMonitors() {
  const { params } = useTimeRange();
  return useQuery({
    queryKey: ["monitors", params],
    queryFn: ({ signal }) => api.get<{ monitors: MonitorSummary[] }>("/api/v1/monitors", params, signal).then((r) => r.monitors),
    refetchInterval: REFRESH_MS,
    placeholderData: (prev) => prev,
  });
}

export function useMonitor(id: string) {
  const { params } = useTimeRange();
  return useQuery({
    queryKey: ["monitors", id, params],
    queryFn: ({ signal }) => api.get<MonitorSummary>(`/api/v1/monitors/${encodeURIComponent(id)}`, params, signal),
    refetchInterval: REFRESH_MS,
    placeholderData: (prev) => prev,
  });
}

export function useMonitorSeries(id?: string) {
  const { params } = useTimeRange();
  const path = id ? `/api/v1/monitors/${encodeURIComponent(id)}/series` : "/api/v1/metrics/monitors";
  return useQuery({
    queryKey: ["monitor-series", id ?? "all", params],
    queryFn: ({ signal }) => api.get<Series>(path, params, signal),
    refetchInterval: REFRESH_MS,
    placeholderData: (prev) => prev,
  });
}

export function useMonitorResults(id: string) {
  const { params } = useTimeRange();
  return useQuery({
    queryKey: ["monitor-results", id, params],
    queryFn: ({ signal }) =>
      api
        .get<{ results: CheckResult[] }>(`/api/v1/monitors/${encodeURIComponent(id)}/results`, { ...params, limit: 200 }, signal)
        .then((r) => r.results),
    refetchInterval: REFRESH_MS,
    placeholderData: (prev) => prev,
  });
}

export function useAlerts(state?: AlertState | "open", targetId?: string) {
  const { params } = useTimeRange();
  // Open alerts are shown regardless of range; resolved history follows the range.
  const q = { state, target_id: targetId, ...(state === "open" || state === "firing" || state === "acknowledged" ? {} : params) };
  return useQuery({
    queryKey: ["alerts", q],
    queryFn: ({ signal }) => api.get<{ alerts: Alert[] }>("/api/v1/alerts", q, signal).then((r) => r.alerts),
    refetchInterval: REFRESH_MS,
    placeholderData: (prev) => prev,
  });
}

export function useEvents(limit = 50) {
  const { params } = useTimeRange();
  return useQuery({
    queryKey: ["events", limit, params],
    queryFn: ({ signal }) => api.get<{ events: PulseEvent[] }>("/api/v1/events", { ...params, limit }, signal).then((r) => r.events),
    refetchInterval: REFRESH_MS,
    placeholderData: (prev) => prev,
  });
}

export function useRules() {
  return useQuery({
    queryKey: ["rules"],
    queryFn: ({ signal }) => api.get<{ rules: AlertRule[] }>("/api/v1/alert-rules", undefined, signal).then((r) => r.rules),
  });
}

export function useWebhooks() {
  return useQuery({
    queryKey: ["webhooks"],
    queryFn: ({ signal }) => api.get<{ webhooks: Webhook[] }>("/api/v1/webhooks", undefined, signal).then((r) => r.webhooks),
  });
}

// ---- Mutations ----

function useInvalidating<TArgs, TResult>(fn: (args: TArgs) => Promise<TResult>, keys: string[][]) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: fn,
    onSuccess: () => Promise.all(keys.map((k) => qc.invalidateQueries({ queryKey: k }))),
  });
}

export const useSaveMonitor = () =>
  useInvalidating(
    ({ id, input }: { id?: string; input: MonitorInput }) =>
      id ? api.put<Monitor>(`/api/v1/monitors/${encodeURIComponent(id)}`, input) : api.post<Monitor>("/api/v1/monitors", input),
    [["monitors"], ["overview"], ["events"]],
  );

export const useDeleteMonitor = () =>
  useInvalidating((id: string) => api.del(`/api/v1/monitors/${encodeURIComponent(id)}`), [["monitors"], ["overview"], ["events"], ["alerts"]]);

export const useDeleteHost = () =>
  useInvalidating((id: string) => api.del(`/api/v1/hosts/${encodeURIComponent(id)}`), [["hosts"], ["overview"], ["events"], ["alerts"]]);

export const useAckAlert = () =>
  useInvalidating((id: string) => api.post<Alert>(`/api/v1/alerts/${encodeURIComponent(id)}/acknowledge`), [["alerts"], ["overview"], ["events"]]);

export const useSaveRule = () =>
  useInvalidating(
    ({ id, input }: { id?: string; input: RuleInput }) =>
      id ? api.put<AlertRule>(`/api/v1/alert-rules/${encodeURIComponent(id)}`, input) : api.post<AlertRule>("/api/v1/alert-rules", input),
    [["rules"], ["events"]],
  );

export const useDeleteRule = () =>
  useInvalidating((id: string) => api.del(`/api/v1/alert-rules/${encodeURIComponent(id)}`), [["rules"], ["alerts"], ["events"]]);

export const useCreateWebhook = () =>
  useInvalidating(
    (input: { name: string; url: string; secret?: string }) => api.post<Webhook>("/api/v1/webhooks", input),
    [["webhooks"], ["events"]],
  );

export const useDeleteWebhook = () =>
  useInvalidating((id: string) => api.del(`/api/v1/webhooks/${encodeURIComponent(id)}`), [["webhooks"], ["events"]]);

export const useTestWebhook = () =>
  useMutation({ mutationFn: (id: string) => api.post<{ status: string }>(`/api/v1/webhooks/${encodeURIComponent(id)}/test`) });
