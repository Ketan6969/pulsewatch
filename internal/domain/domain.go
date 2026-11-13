// Package domain defines the core types shared across Pulsewatch packages.
// It has no dependencies on storage or transport.
package domain

import (
	"errors"
	"time"
)

// Sentinel errors that the API layer maps to HTTP status codes.
var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
)

// HostStatus is the derived health of a host.
type HostStatus string

const (
	HostHealthy  HostStatus = "healthy"
	HostDegraded HostStatus = "degraded"
	HostOffline  HostStatus = "offline"
)

// Host is a machine reporting through an agent.
type Host struct {
	ID              string      `json:"id"`
	Name            string      `json:"name"`
	Hostname        string      `json:"hostname"`
	OS              string      `json:"os"`
	Platform        string      `json:"platform"`
	PlatformVersion string      `json:"platform_version"`
	KernelVersion   string      `json:"kernel_version"`
	Arch            string      `json:"arch"`
	CPUCount        int         `json:"cpu_count"`
	AgentVersion    string      `json:"agent_version"`
	BootTime        *time.Time  `json:"boot_time,omitempty"`
	ReportInterval  int         `json:"report_interval_s"`
	LastSeenAt      time.Time   `json:"last_seen_at"`
	CreatedAt       time.Time   `json:"created_at"`
	Status          HostStatus  `json:"status"`
	Latest          *HostMetric `json:"latest,omitempty"`
}

// HostMetric is a single point-in-time resource sample.
type HostMetric struct {
	Time      time.Time `json:"ts"`
	CPUPct    float64   `json:"cpu_pct"`
	MemPct    float64   `json:"mem_pct"`
	MemUsed   uint64    `json:"mem_used"`
	MemTotal  uint64    `json:"mem_total"`
	SwapPct   float64   `json:"swap_pct"`
	DiskPct   float64   `json:"disk_pct"`
	DiskUsed  uint64    `json:"disk_used"`
	DiskTotal uint64    `json:"disk_total"`
	NetRxBps  float64   `json:"net_rx_bps"`
	NetTxBps  float64   `json:"net_tx_bps"`
	Load1     float64   `json:"load1"`
	Load5     float64   `json:"load5"`
	Load15    float64   `json:"load15"`
	UptimeS   uint64    `json:"uptime_s"`
	ProcCount int       `json:"proc_count"`
}

// DiskUsage describes a mounted filesystem.
type DiskUsage struct {
	Mountpoint string  `json:"mountpoint"`
	Device     string  `json:"device"`
	FSType     string  `json:"fstype"`
	Total      uint64  `json:"total"`
	Used       uint64  `json:"used"`
	UsedPct    float64 `json:"used_pct"`
}

// NetInterface describes a network interface and its current throughput.
type NetInterface struct {
	Name      string   `json:"name"`
	Addrs     []string `json:"addrs"`
	MAC       string   `json:"mac"`
	Up        bool     `json:"up"`
	BytesRecv uint64   `json:"bytes_recv"`
	BytesSent uint64   `json:"bytes_sent"`
	RxBps     float64  `json:"rx_bps"`
	TxBps     float64  `json:"tx_bps"`
	Errin     uint64   `json:"errin"`
	Errout    uint64   `json:"errout"`
}

// Process is a snapshot of a running process.
type Process struct {
	PID        int32   `json:"pid"`
	Name       string  `json:"name"`
	User       string  `json:"user"`
	CPUPct     float64 `json:"cpu_pct"`
	MemPct     float32 `json:"mem_pct"`
	RSS        uint64  `json:"rss"`
	Status     string  `json:"status"`
	Cmdline    string  `json:"cmdline"`
	CreateTime int64   `json:"create_time"`
}

// HostInfo is the static description of a host sent by an agent.
type HostInfo struct {
	Hostname        string     `json:"hostname"`
	Name            string     `json:"name"`
	OS              string     `json:"os"`
	Platform        string     `json:"platform"`
	PlatformVersion string     `json:"platform_version"`
	KernelVersion   string     `json:"kernel_version"`
	Arch            string     `json:"arch"`
	CPUCount        int        `json:"cpu_count"`
	AgentVersion    string     `json:"agent_version"`
	BootTime        *time.Time `json:"boot_time,omitempty"`
}

// AgentReport is the payload an agent pushes on every collection cycle.
type AgentReport struct {
	Host       HostInfo       `json:"host"`
	IntervalS  int            `json:"interval_s"`
	Metric     HostMetric     `json:"metric"`
	Disks      []DiskUsage    `json:"disks"`
	Interfaces []NetInterface `json:"interfaces"`
	Processes  []Process      `json:"processes"`
}

// HostSnapshot is the latest detailed state of a host.
type HostSnapshot struct {
	CollectedAt time.Time      `json:"collected_at"`
	Disks       []DiskUsage    `json:"disks"`
	Interfaces  []NetInterface `json:"interfaces"`
	Processes   []Process      `json:"processes"`
}

// Monitor is an HTTP endpoint check definition.
type Monitor struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	URL            string            `json:"url"`
	Method         string            `json:"method"`
	IntervalS      int               `json:"interval_s"`
	TimeoutMS      int               `json:"timeout_ms"`
	ExpectedStatus int               `json:"expected_status"` // 0 = any 2xx/3xx
	Headers        map[string]string `json:"headers"`
	Enabled        bool              `json:"enabled"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
}

// CheckResult is the outcome of a single HTTP check.
type CheckResult struct {
	MonitorID  string    `json:"monitor_id"`
	CheckedAt  time.Time `json:"checked_at"`
	Success    bool      `json:"success"`
	StatusCode int       `json:"status_code"`
	ResponseMS int       `json:"response_ms"`
	Error      string    `json:"error,omitempty"`
}

// MonitorStatus is the derived current state of a monitor.
type MonitorStatus string

const (
	MonitorUp      MonitorStatus = "up"
	MonitorDown    MonitorStatus = "down"
	MonitorPending MonitorStatus = "pending"
	MonitorPaused  MonitorStatus = "paused"
)

// MonitorSummary is a monitor with its derived statistics.
type MonitorSummary struct {
	Monitor
	Status        MonitorStatus `json:"status"`
	LastCheck     *CheckResult  `json:"last_check,omitempty"`
	LastSuccessAt *time.Time    `json:"last_success_at,omitempty"`
	LastFailureAt *time.Time    `json:"last_failure_at,omitempty"`
	LastError     string        `json:"last_error,omitempty"`
	UptimePct     *float64      `json:"uptime_pct"`
	AvgResponseMS *float64      `json:"avg_response_ms"`
	ChecksTotal   int           `json:"checks_total"`
	ChecksFailed  int           `json:"checks_failed"`
}

// Alert rule kinds.
const (
	RuleHostMetric     = "host_metric"
	RuleHostOffline    = "host_offline"
	RuleMonitorDown    = "monitor_down"
	RuleMonitorLatency = "monitor_latency"
)

// Severities.
const (
	SeverityCritical = "critical"
	SeverityWarning  = "warning"
	SeverityInfo     = "info"
)

// AlertRule defines when an alert fires.
//
// For host_metric, Metric is one of cpu_pct, mem_pct, disk_pct, swap_pct,
// load1 and the alert fires when the last ForCount samples all satisfy
// "value Operator Threshold". For host_offline, Threshold is the number of
// seconds without a heartbeat. For monitor_down, ForCount consecutive failed
// checks fire the alert. For monitor_latency, Threshold is in milliseconds.
type AlertRule struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Kind      string    `json:"kind"`
	Metric    string    `json:"metric,omitempty"`
	Operator  string    `json:"operator"`
	Threshold float64   `json:"threshold"`
	ForCount  int       `json:"for_count"`
	Severity  string    `json:"severity"`
	HostID    *string   `json:"host_id,omitempty"`
	MonitorID *string   `json:"monitor_id,omitempty"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Alert states.
const (
	AlertFiring       = "firing"
	AlertAcknowledged = "acknowledged"
	AlertResolved     = "resolved"
)

// Alert is a single occurrence of a rule firing for a target.
type Alert struct {
	ID             string     `json:"id"`
	RuleID         string     `json:"rule_id"`
	RuleName       string     `json:"rule_name"`
	TargetType     string     `json:"target_type"` // host | monitor
	TargetID       string     `json:"target_id"`
	TargetName     string     `json:"target_name"`
	State          string     `json:"state"`
	Severity       string     `json:"severity"`
	Message        string     `json:"message"`
	Value          float64    `json:"value"`
	TriggeredAt    time.Time  `json:"triggered_at"`
	AcknowledgedAt *time.Time `json:"acknowledged_at,omitempty"`
	ResolvedAt     *time.Time `json:"resolved_at,omitempty"`
}

// Webhook is a notification destination.
type Webhook struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	URL       string    `json:"url"`
	Secret    string    `json:"-"`
	HasSecret bool      `json:"has_secret"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
}

// Event kinds.
const (
	EventAlertFiring   = "alert_firing"
	EventAlertResolved = "alert_resolved"
	EventAlertAck      = "alert_acknowledged"
	EventConfigChange  = "config_change"
	EventHostAdded     = "host_registered"
)

// Event is an entry in the operational event log.
type Event struct {
	ID         int64     `json:"id"`
	Time       time.Time `json:"ts"`
	Kind       string    `json:"kind"`
	Severity   string    `json:"severity"`
	Message    string    `json:"message"`
	TargetType string    `json:"target_type,omitempty"`
	TargetID   string    `json:"target_id,omitempty"`
}

// SeriesPoint is a time-bucketed aggregate used by charts.
type SeriesPoint struct {
	Time   time.Time          `json:"ts"`
	Values map[string]float64 `json:"v"`
}
