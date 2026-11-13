package alert

import (
	"fmt"
	"time"

	"pulsewatch/internal/domain"
)

// Compare applies a rule operator.
func Compare(op string, v, threshold float64) bool {
	switch op {
	case ">":
		return v > threshold
	case ">=":
		return v >= threshold
	case "<":
		return v < threshold
	case "<=":
		return v <= threshold
	}
	return false
}

// MetricValue extracts a named metric from a sample.
func MetricValue(m domain.HostMetric, name string) (float64, bool) {
	switch name {
	case "cpu_pct":
		return m.CPUPct, true
	case "mem_pct":
		return m.MemPct, true
	case "disk_pct":
		return m.DiskPct, true
	case "swap_pct":
		return m.SwapPct, true
	case "load1":
		return m.Load1, true
	}
	return 0, false
}

var metricLabels = map[string]string{
	"cpu_pct": "CPU", "mem_pct": "memory", "disk_pct": "disk", "swap_pct": "swap", "load1": "load (1m)",
}

// ValidMetric reports whether name is a supported host metric.
func ValidMetric(name string) bool {
	_, ok := metricLabels[name]
	return ok
}

// EvalHostMetric fires when the newest ForCount samples all breach.
// samples must be ordered newest first.
func EvalHostMetric(r domain.AlertRule, samples []domain.HostMetric) verdict {
	if len(samples) == 0 {
		return verdict{}
	}
	latest, ok := MetricValue(samples[0], r.Metric)
	if !ok {
		return verdict{}
	}
	if !Compare(r.Operator, latest, r.Threshold) {
		// A single healthy sample is enough to recover.
		return verdict{known: true, value: latest}
	}
	if len(samples) < r.ForCount {
		return verdict{} // breaching but not yet for long enough to decide
	}
	for _, s := range samples[:r.ForCount] {
		v, _ := MetricValue(s, r.Metric)
		if !Compare(r.Operator, v, r.Threshold) {
			return verdict{known: true, value: latest}
		}
	}
	unit := "%"
	if r.Metric == "load1" {
		unit = ""
	}
	return verdict{
		known: true, breach: true, value: latest,
		detail: fmt.Sprintf("%s is %.1f%s (%s %g for %d samples)", metricLabels[r.Metric], latest, unit, r.Operator, r.Threshold, r.ForCount),
	}
}

// EvalOffline fires when no heartbeat has arrived for Threshold seconds.
func EvalOffline(r domain.AlertRule, lastSeen, now time.Time) verdict {
	silent := now.Sub(lastSeen)
	limit := time.Duration(r.Threshold * float64(time.Second))
	if silent > limit {
		return verdict{known: true, breach: true, value: silent.Seconds(),
			detail: fmt.Sprintf("has not reported for %s", silent.Round(time.Second))}
	}
	return verdict{known: true, value: silent.Seconds()}
}

// EvalMonitorDown fires after ForCount consecutive failed checks and
// recovers on the first success. results must be ordered newest first.
func EvalMonitorDown(r domain.AlertRule, results []domain.CheckResult) verdict {
	if len(results) == 0 {
		return verdict{}
	}
	if results[0].Success {
		return verdict{known: true}
	}
	if len(results) < r.ForCount {
		return verdict{}
	}
	for _, c := range results[:r.ForCount] {
		if c.Success {
			return verdict{known: true}
		}
	}
	return verdict{known: true, breach: true, value: float64(results[0].StatusCode),
		detail: fmt.Sprintf("is down (%d consecutive failures): %s", r.ForCount, results[0].Error)}
}

// EvalLatency fires when ForCount consecutive successful checks exceed
// Threshold milliseconds. Failed checks are left to the down rule.
func EvalLatency(r domain.AlertRule, results []domain.CheckResult) verdict {
	var ok []domain.CheckResult
	for _, c := range results {
		if c.Success {
			ok = append(ok, c)
		}
	}
	if len(ok) == 0 {
		return verdict{}
	}
	latest := float64(ok[0].ResponseMS)
	if !Compare(r.Operator, latest, r.Threshold) {
		return verdict{known: true, value: latest}
	}
	if len(ok) < r.ForCount || len(results) < r.ForCount {
		return verdict{}
	}
	for _, c := range ok[:r.ForCount] {
		if !Compare(r.Operator, float64(c.ResponseMS), r.Threshold) {
			return verdict{known: true, value: latest}
		}
	}
	return verdict{known: true, breach: true, value: latest,
		detail: fmt.Sprintf("response time is %.0fms (%s %gms for %d checks)", latest, r.Operator, r.Threshold, r.ForCount)}
}
