package api

import (
	"fmt"
	"math"
	"net/http"
	"time"

	"pulsewatch/internal/domain"
)

func (s *Server) agentReport(w http.ResponseWriter, r *http.Request) {
	var rep domain.AgentReport
	if err := decodeLenient(r, &rep); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := validateReport(&rep); err != nil {
		s.fail(w, r, err)
		return
	}
	id, created, err := s.store.IngestReport(r.Context(), rep)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if created {
		s.log.Info("host registered", "host_id", id, "hostname", rep.Host.Hostname)
		s.event(r, domain.Event{Kind: domain.EventHostAdded, Message: fmt.Sprintf("Host %s registered", rep.Host.Hostname),
			TargetType: "host", TargetID: id})
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"host_id": id})
}

func validateReport(rep *domain.AgentReport) error {
	v := validationError{}
	v.check(rep.Host.Hostname != "" && len(rep.Host.Hostname) <= 255, "host.hostname", "required, at most 255 characters")
	v.check(len(rep.Host.Name) <= 255, "host.name", "at most 255 characters")
	v.check(rep.IntervalS >= 1 && rep.IntervalS <= 3600, "interval_s", "must be between 1 and 3600")
	v.check(len(rep.Processes) <= 500, "processes", "at most 500 entries")
	v.check(len(rep.Disks) <= 256, "disks", "at most 256 entries")
	v.check(len(rep.Interfaces) <= 256, "interfaces", "at most 256 entries")
	m := rep.Metric
	for name, val := range map[string]float64{"cpu_pct": m.CPUPct, "mem_pct": m.MemPct, "disk_pct": m.DiskPct, "swap_pct": m.SwapPct} {
		v.check(!math.IsNaN(val) && val >= 0 && val <= 100, "metric."+name, "must be between 0 and 100")
	}
	for name, val := range map[string]float64{"net_rx_bps": m.NetRxBps, "net_tx_bps": m.NetTxBps, "load1": m.Load1} {
		v.check(!math.IsNaN(val) && !math.IsInf(val, 0) && val >= 0, "metric."+name, "must be a non-negative number")
	}
	// Stored as bigint.
	for name, val := range map[string]uint64{"mem_total": m.MemTotal, "mem_used": m.MemUsed, "disk_total": m.DiskTotal, "disk_used": m.DiskUsed, "uptime_s": m.UptimeS} {
		v.check(val <= math.MaxInt64, "metric."+name, "out of range")
	}
	return v.err()
}

func (s *Server) listHosts(w http.ResponseWriter, r *http.Request) {
	hosts, err := s.store.ListHosts(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"hosts": nonNil(hosts)})
}

func (s *Server) getHost(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	h, err := s.store.GetHost(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	snap, err := s.store.GetHostSnapshot(r.Context(), id)
	if err != nil && err != domain.ErrNotFound {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"host": h, "snapshot": snap})
}

func (s *Server) deleteHost(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	h, err := s.store.GetHost(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.store.DeleteHost(r.Context(), id); err != nil {
		s.fail(w, r, err)
		return
	}
	s.event(r, domain.Event{Kind: domain.EventConfigChange, Message: fmt.Sprintf("Host %s removed", h.Name),
		TargetType: "host", TargetID: id})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) hostMetrics(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := s.store.GetHost(r.Context(), id); err != nil {
		s.fail(w, r, err)
		return
	}
	s.seriesResponse(w, r, func(from, to time.Time) ([]domain.SeriesPoint, time.Duration, error) {
		return s.store.HostSeries(r.Context(), id, from, to)
	})
}

func (s *Server) fleetMetrics(w http.ResponseWriter, r *http.Request) {
	s.seriesResponse(w, r, func(from, to time.Time) ([]domain.SeriesPoint, time.Duration, error) {
		return s.store.HostSeries(r.Context(), "", from, to)
	})
}

func (s *Server) seriesResponse(w http.ResponseWriter, r *http.Request, q func(from, to time.Time) ([]domain.SeriesPoint, time.Duration, error)) {
	from, to, err := timeWindow(r, time.Hour)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	pts, step, err := q(from, to)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"from": from.UnixMilli(), "to": to.UnixMilli(), "step_s": int(step.Seconds()), "points": nonNil(pts),
	})
}

func (s *Server) event(r *http.Request, e domain.Event) {
	if err := s.store.RecordEvent(r.Context(), e); err != nil {
		s.log.Error("record event", "err", err)
	}
}

func nonNil[T any](v []T) []T {
	if v == nil {
		return []T{}
	}
	return v
}
