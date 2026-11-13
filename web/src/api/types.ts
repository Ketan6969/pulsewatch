// Types mirror the Go domain package (internal/domain).

export type HostStatus = "healthy" | "degraded" | "offline";
export type MonitorStatus = "up" | "down" | "pending" | "paused";
export type AlertState = "firing" | "acknowledged" | "resolved";
export type Severity = "critical" | "warning" | "info";

export interface HostMetric {
  ts: string;
  cpu_pct: number;
  mem_pct: number;
  mem_used: number;
  mem_total: number;
  swap_pct: number;
  disk_pct: number;
  disk_used: number;
  disk_total: number;
  net_rx_bps: number;
  net_tx_bps: number;
  load1: number;
  load5: number;
  load15: number;
  uptime_s: number;
  proc_count: number;
}

export interface Host {
  id: string;
  name: string;
  hostname: string;
  os: string;
  platform: string;
  platform_version: string;
  kernel_version: string;
  arch: string;
  cpu_count: number;
  agent_version: string;
  boot_time?: string;
  report_interval_s: number;
  last_seen_at: string;
  created_at: string;
  status: HostStatus;
  latest?: HostMetric;
}

export interface DiskUsage {
  mountpoint: string;
  device: string;
  fstype: string;
  total: number;
  used: number;
  used_pct: number;
}

export interface NetInterface {
  name: string;
  addrs: string[] | null;
  mac: string;
  up: boolean;
  bytes_recv: number;
  bytes_sent: number;
  rx_bps: number;
  tx_bps: number;
  errin: number;
  errout: number;
}

export interface Process {
  pid: number;
  name: string;
  user: string;
  cpu_pct: number;
  mem_pct: number;
  rss: number;
  status: string;
  cmdline: string;
  create_time: number;
}

export interface HostSnapshot {
  collected_at: string;
  disks: DiskUsage[] | null;
  interfaces: NetInterface[] | null;
  processes: Process[] | null;
}

export interface CheckResult {
  monitor_id: string;
  checked_at: string;
  success: boolean;
  status_code: number;
  response_ms: number;
  error?: string;
}

export interface Monitor {
  id: string;
  name: string;
  url: string;
  method: string;
  interval_s: number;
  timeout_ms: number;
  expected_status: number;
  headers: Record<string, string>;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface MonitorSummary extends Monitor {
  status: MonitorStatus;
  last_check?: CheckResult;
  last_success_at?: string;
  last_failure_at?: string;
  last_error?: string;
  uptime_pct: number | null;
  avg_response_ms: number | null;
  checks_total: number;
  checks_failed: number;
}

export type MonitorInput = Pick<
  Monitor,
  "name" | "url" | "method" | "interval_s" | "timeout_ms" | "expected_status" | "headers" | "enabled"
>;

export type RuleKind = "host_metric" | "host_offline" | "monitor_down" | "monitor_latency";

export interface AlertRule {
  id: string;
  name: string;
  kind: RuleKind;
  metric?: string;
  operator: string;
  threshold: number;
  for_count: number;
  severity: Severity;
  host_id?: string | null;
  monitor_id?: string | null;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export type RuleInput = Omit<AlertRule, "id" | "created_at" | "updated_at">;

export interface Alert {
  id: string;
  rule_id: string;
  rule_name: string;
  target_type: "host" | "monitor";
  target_id: string;
  target_name: string;
  state: AlertState;
  severity: Severity;
  message: string;
  value: number;
  triggered_at: string;
  acknowledged_at?: string;
  resolved_at?: string;
}

export interface Webhook {
  id: string;
  name: string;
  url: string;
  has_secret: boolean;
  enabled: boolean;
  created_at: string;
}

export interface PulseEvent {
  id: number;
  ts: string;
  kind: string;
  severity: Severity;
  message: string;
  target_type?: string;
  target_id?: string;
}

export interface SeriesPoint {
  ts: string;
  v: Record<string, number>;
}

export interface Series {
  from: number;
  to: number;
  step_s: number;
  points: SeriesPoint[];
}

export interface Overview {
  hosts: { total: number; healthy: number; degraded: number; offline: number };
  system: {
    reporting: number;
    cpu_pct: number | null;
    mem_pct: number | null;
    disk_pct: number | null;
    net_rx_bps: number;
    net_tx_bps: number;
  };
  http: {
    total: number;
    up: number;
    down: number;
    paused: number;
    uptime_pct: number | null;
    avg_response_ms: number | null;
    failed_checks: number;
    total_checks: number;
  };
  alerts: { firing: number; acknowledged: number; critical: number };
  from: number;
  to: number;
}
