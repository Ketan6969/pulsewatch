package api

import (
	"net/http"
	"time"

	"pulsewatch/internal/domain"
	"pulsewatch/internal/store"
)

type overviewResponse struct {
	Hosts struct {
		Total    int `json:"total"`
		Healthy  int `json:"healthy"`
		Degraded int `json:"degraded"`
		Offline  int `json:"offline"`
	} `json:"hosts"`
	System struct {
		Reporting int      `json:"reporting"`
		CPUPct    *float64 `json:"cpu_pct"`
		MemPct    *float64 `json:"mem_pct"`
		DiskPct   *float64 `json:"disk_pct"`
		NetRxBps  float64  `json:"net_rx_bps"`
		NetTxBps  float64  `json:"net_tx_bps"`
	} `json:"system"`
	HTTP struct {
		Total         int      `json:"total"`
		Up            int      `json:"up"`
		Down          int      `json:"down"`
		Paused        int      `json:"paused"`
		UptimePct     *float64 `json:"uptime_pct"`
		AvgResponseMS *float64 `json:"avg_response_ms"`
		FailedChecks  int      `json:"failed_checks"`
		TotalChecks   int      `json:"total_checks"`
	} `json:"http"`
	Alerts struct {
		Firing       int `json:"firing"`
		Acknowledged int `json:"acknowledged"`
		Critical     int `json:"critical"`
	} `json:"alerts"`
	From int64 `json:"from"`
	To   int64 `json:"to"`
}

func (s *Server) overview(w http.ResponseWriter, r *http.Request) {
	from, to, err := timeWindow(r, time.Hour)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	ctx := r.Context()
	hosts, err := s.store.ListHosts(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	monitors, err := s.store.MonitorSummaries(ctx, from)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	alerts, err := s.store.ListAlerts(ctx, store.AlertFilter{State: "open", Limit: 1000})
	if err != nil {
		s.fail(w, r, err)
		return
	}

	var out overviewResponse
	out.From, out.To = from.UnixMilli(), to.UnixMilli()

	out.Hosts.Total = len(hosts)
	var cpu, mem, disk float64
	for _, h := range hosts {
		switch h.Status {
		case domain.HostHealthy:
			out.Hosts.Healthy++
		case domain.HostDegraded:
			out.Hosts.Degraded++
		case domain.HostOffline:
			out.Hosts.Offline++
		}
		if h.Status != domain.HostOffline && h.Latest != nil {
			out.System.Reporting++
			cpu += h.Latest.CPUPct
			mem += h.Latest.MemPct
			disk += h.Latest.DiskPct
			out.System.NetRxBps += h.Latest.NetRxBps
			out.System.NetTxBps += h.Latest.NetTxBps
		}
	}
	if n := float64(out.System.Reporting); n > 0 {
		cpu, mem, disk = cpu/n, mem/n, disk/n
		out.System.CPUPct, out.System.MemPct, out.System.DiskPct = &cpu, &mem, &disk
	}

	var weightedMS float64
	var okChecks int
	for _, m := range monitors {
		out.HTTP.Total++
		switch m.Status {
		case domain.MonitorUp:
			out.HTTP.Up++
		case domain.MonitorDown:
			out.HTTP.Down++
		case domain.MonitorPaused:
			out.HTTP.Paused++
		}
		out.HTTP.TotalChecks += m.ChecksTotal
		out.HTTP.FailedChecks += m.ChecksFailed
		if m.AvgResponseMS != nil {
			n := m.ChecksTotal - m.ChecksFailed
			weightedMS += *m.AvgResponseMS * float64(n)
			okChecks += n
		}
	}
	if out.HTTP.TotalChecks > 0 {
		up := 100 * float64(out.HTTP.TotalChecks-out.HTTP.FailedChecks) / float64(out.HTTP.TotalChecks)
		out.HTTP.UptimePct = &up
	}
	if okChecks > 0 {
		avg := weightedMS / float64(okChecks)
		out.HTTP.AvgResponseMS = &avg
	}

	for _, a := range alerts {
		switch a.State {
		case domain.AlertFiring:
			out.Alerts.Firing++
		case domain.AlertAcknowledged:
			out.Alerts.Acknowledged++
		}
		if a.Severity == domain.SeverityCritical {
			out.Alerts.Critical++
		}
	}
	writeJSON(w, http.StatusOK, out)
}
