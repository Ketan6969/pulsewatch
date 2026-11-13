package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"

	"pulsewatch/internal/domain"
)

const monitorCols = `id, name, url, method, interval_s, timeout_ms, expected_status, headers, enabled, created_at, updated_at`

func scanMonitor(r pgx.Row) (domain.Monitor, error) {
	var (
		m       domain.Monitor
		headers []byte
	)
	err := r.Scan(&m.ID, &m.Name, &m.URL, &m.Method, &m.IntervalS, &m.TimeoutMS, &m.ExpectedStatus,
		&headers, &m.Enabled, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		return m, err
	}
	m.Headers = map[string]string{}
	if len(headers) > 0 {
		if err := json.Unmarshal(headers, &m.Headers); err != nil {
			return m, err
		}
	}
	return m, nil
}

// ListMonitors returns all monitor definitions.
func (s *Store) ListMonitors(ctx context.Context) ([]domain.Monitor, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+monitorCols+` FROM monitors ORDER BY name, id`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.Monitor, error) { return scanMonitor(r) })
}

// GetMonitor returns one monitor definition.
func (s *Store) GetMonitor(ctx context.Context, id string) (domain.Monitor, error) {
	m, err := scanMonitor(s.pool.QueryRow(ctx, `SELECT `+monitorCols+` FROM monitors WHERE id = $1`, id))
	return m, mapErr(err)
}

// CreateMonitor inserts a monitor and returns it.
func (s *Store) CreateMonitor(ctx context.Context, m domain.Monitor) (domain.Monitor, error) {
	headers, err := json.Marshal(m.Headers)
	if err != nil {
		return m, err
	}
	out, err := scanMonitor(s.pool.QueryRow(ctx, `
		INSERT INTO monitors (name, url, method, interval_s, timeout_ms, expected_status, headers, enabled)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING `+monitorCols,
		m.Name, m.URL, m.Method, m.IntervalS, m.TimeoutMS, m.ExpectedStatus, headers, m.Enabled))
	return out, mapErr(err)
}

// UpdateMonitor replaces a monitor's configuration.
func (s *Store) UpdateMonitor(ctx context.Context, m domain.Monitor) (domain.Monitor, error) {
	headers, err := json.Marshal(m.Headers)
	if err != nil {
		return m, err
	}
	out, err := scanMonitor(s.pool.QueryRow(ctx, `
		UPDATE monitors SET name=$2, url=$3, method=$4, interval_s=$5, timeout_ms=$6, expected_status=$7,
		       headers=$8, enabled=$9, updated_at=now()
		 WHERE id = $1 RETURNING `+monitorCols,
		m.ID, m.Name, m.URL, m.Method, m.IntervalS, m.TimeoutMS, m.ExpectedStatus, headers, m.Enabled))
	return out, mapErr(err)
}

// DeleteMonitor removes a monitor, its results and its alerts.
func (s *Store) DeleteMonitor(ctx context.Context, id string) (string, error) {
	var name string
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `DELETE FROM monitors WHERE id = $1 RETURNING name`, id).Scan(&name); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `DELETE FROM alerts WHERE target_type = 'monitor' AND target_id = $1`, id)
		return err
	})
	return name, mapErr(err)
}

// InsertCheckResult persists the outcome of one HTTP check.
func (s *Store) InsertCheckResult(ctx context.Context, r domain.CheckResult) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO check_results (monitor_id, checked_at, success, status_code, response_ms, error)
		VALUES ($1,$2,$3,$4,$5,$6)`,
		r.MonitorID, r.CheckedAt, r.Success, r.StatusCode, r.ResponseMS, r.Error)
	return mapErr(err)
}

// RecentCheckResults returns the newest n results for a monitor, newest first.
func (s *Store) RecentCheckResults(ctx context.Context, monitorID string, n int) ([]domain.CheckResult, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT monitor_id, checked_at, success, status_code, response_ms, error
		  FROM check_results WHERE monitor_id = $1 ORDER BY checked_at DESC LIMIT $2`, monitorID, n)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, scanResult)
}

func scanResult(r pgx.CollectableRow) (domain.CheckResult, error) {
	var c domain.CheckResult
	err := r.Scan(&c.MonitorID, &c.CheckedAt, &c.Success, &c.StatusCode, &c.ResponseMS, &c.Error)
	return c, err
}

// CheckResultsBetween returns results for a monitor in a window, newest first.
func (s *Store) CheckResultsBetween(ctx context.Context, monitorID string, from, to time.Time, limit int) ([]domain.CheckResult, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT monitor_id, checked_at, success, status_code, response_ms, error
		  FROM check_results WHERE monitor_id = $1 AND checked_at BETWEEN $2 AND $3
		 ORDER BY checked_at DESC LIMIT $4`, monitorID, from, to, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, scanResult)
}

// MonitorSummaries returns every monitor with statistics over [since, now].
func (s *Store) MonitorSummaries(ctx context.Context, since time.Time) ([]domain.MonitorSummary, error) {
	return s.monitorSummaries(ctx, since, "")
}

// MonitorSummary returns one monitor with statistics over [since, now].
func (s *Store) MonitorSummary(ctx context.Context, id string, since time.Time) (domain.MonitorSummary, error) {
	out, err := s.monitorSummaries(ctx, since, id)
	if err != nil {
		return domain.MonitorSummary{}, mapErr(err)
	}
	if len(out) == 0 {
		return domain.MonitorSummary{}, domain.ErrNotFound
	}
	return out[0], nil
}

func (s *Store) monitorSummaries(ctx context.Context, since time.Time, id string) ([]domain.MonitorSummary, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT m.id, m.name, m.url, m.method, m.interval_s, m.timeout_ms, m.expected_status, m.headers,
		       m.enabled, m.created_at, m.updated_at,
		       stats.total, stats.failed, stats.avg_ms,
		       last.checked_at, last.success, last.status_code, last.response_ms, last.error,
		       (SELECT max(checked_at) FROM check_results c WHERE c.monitor_id = m.id AND c.success),
		       lf.checked_at, lf.error
		  FROM monitors m
		  LEFT JOIN LATERAL (
		       SELECT count(*) total, count(*) FILTER (WHERE NOT success) failed,
		              (avg(response_ms) FILTER (WHERE success))::float8 avg_ms
		         FROM check_results c WHERE c.monitor_id = m.id AND c.checked_at >= $1
		  ) stats ON true
		  LEFT JOIN LATERAL (
		       SELECT * FROM check_results c WHERE c.monitor_id = m.id ORDER BY checked_at DESC LIMIT 1
		  ) last ON true
		  LEFT JOIN LATERAL (
		       SELECT checked_at, error FROM check_results c
		        WHERE c.monitor_id = m.id AND NOT success ORDER BY checked_at DESC LIMIT 1
		  ) lf ON true
		 WHERE ($2 = '' OR m.id::text = $2)
		 ORDER BY m.name, m.id`, since, id)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.MonitorSummary, error) {
		var (
			ms       domain.MonitorSummary
			headers  []byte
			avg      *float64
			lastAt   *time.Time
			lastOK   *bool
			lastCode *int
			lastMS   *int
			lastErr  *string
			lfErr    *string
		)
		err := r.Scan(&ms.ID, &ms.Name, &ms.URL, &ms.Method, &ms.IntervalS, &ms.TimeoutMS, &ms.ExpectedStatus,
			&headers, &ms.Enabled, &ms.CreatedAt, &ms.UpdatedAt,
			&ms.ChecksTotal, &ms.ChecksFailed, &avg,
			&lastAt, &lastOK, &lastCode, &lastMS, &lastErr,
			&ms.LastSuccessAt, &ms.LastFailureAt, &lfErr)
		if err != nil {
			return ms, err
		}
		ms.Headers = map[string]string{}
		_ = json.Unmarshal(headers, &ms.Headers)
		ms.AvgResponseMS = avg
		if ms.ChecksTotal > 0 {
			up := 100 * float64(ms.ChecksTotal-ms.ChecksFailed) / float64(ms.ChecksTotal)
			ms.UptimePct = &up
		}
		if lfErr != nil {
			ms.LastError = *lfErr
		}
		switch {
		case !ms.Enabled:
			ms.Status = domain.MonitorPaused
		case lastAt == nil:
			ms.Status = domain.MonitorPending
		case *lastOK:
			ms.Status = domain.MonitorUp
		default:
			ms.Status = domain.MonitorDown
		}
		if lastAt != nil {
			ms.LastCheck = &domain.CheckResult{
				MonitorID: ms.ID, CheckedAt: *lastAt, Success: *lastOK, StatusCode: *lastCode,
				ResponseMS: *lastMS, Error: *lastErr,
			}
		}
		return ms, nil
	})
}

// MonitorSeries returns bucketed response time and availability for one
// monitor, or across all monitors when monitorID is empty.
func (s *Store) MonitorSeries(ctx context.Context, monitorID string, from, to time.Time) ([]domain.SeriesPoint, time.Duration, error) {
	step := BucketStep(to.Sub(from))
	rows, err := s.pool.Query(ctx, `
		SELECT date_bin($1::interval, checked_at, 'epoch'::timestamptz) AS bucket,
		       coalesce(avg(response_ms) FILTER (WHERE success), 0)::float8,
		       coalesce(max(response_ms) FILTER (WHERE success), 0)::float8,
		       (100.0 * count(*) FILTER (WHERE success) / count(*))::float8,
		       (count(*) FILTER (WHERE NOT success))::float8
		  FROM check_results
		 WHERE checked_at >= $2 AND checked_at <= $3 AND ($4 = '' OR monitor_id::text = $4)
		 GROUP BY bucket ORDER BY bucket`, step, from, to, monitorID)
	if err != nil {
		return nil, step, err
	}
	pts, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.SeriesPoint, error) {
		var (
			p                    domain.SeriesPoint
			avg, mx, avail, fail float64
		)
		err := r.Scan(&p.Time, &avg, &mx, &avail, &fail)
		p.Values = map[string]float64{"avg_ms": avg, "max_ms": mx, "availability": avail, "failures": fail}
		return p, err
	})
	return pts, step, err
}
