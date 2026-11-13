package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"pulsewatch/internal/domain"
)

const ruleCols = `id, name, kind, metric, operator, threshold, for_count, severity, host_id, monitor_id, enabled, created_at, updated_at`

func scanRule(r pgx.Row) (domain.AlertRule, error) {
	var a domain.AlertRule
	err := r.Scan(&a.ID, &a.Name, &a.Kind, &a.Metric, &a.Operator, &a.Threshold, &a.ForCount, &a.Severity,
		&a.HostID, &a.MonitorID, &a.Enabled, &a.CreatedAt, &a.UpdatedAt)
	return a, err
}

// ListRules returns all alert rules.
func (s *Store) ListRules(ctx context.Context) ([]domain.AlertRule, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+ruleCols+` FROM alert_rules ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.AlertRule, error) { return scanRule(r) })
}

// GetRule returns one alert rule.
func (s *Store) GetRule(ctx context.Context, id string) (domain.AlertRule, error) {
	r, err := scanRule(s.pool.QueryRow(ctx, `SELECT `+ruleCols+` FROM alert_rules WHERE id = $1`, id))
	return r, mapErr(err)
}

// CreateRule inserts an alert rule.
func (s *Store) CreateRule(ctx context.Context, a domain.AlertRule) (domain.AlertRule, error) {
	out, err := scanRule(s.pool.QueryRow(ctx, `
		INSERT INTO alert_rules (name, kind, metric, operator, threshold, for_count, severity, host_id, monitor_id, enabled)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING `+ruleCols,
		a.Name, a.Kind, a.Metric, a.Operator, a.Threshold, a.ForCount, a.Severity, a.HostID, a.MonitorID, a.Enabled))
	return out, mapErr(err)
}

// UpdateRule replaces an alert rule. Open alerts of a changed rule are left
// to the engine, which resolves them on the next evaluation if they no
// longer breach.
func (s *Store) UpdateRule(ctx context.Context, a domain.AlertRule) (domain.AlertRule, error) {
	out, err := scanRule(s.pool.QueryRow(ctx, `
		UPDATE alert_rules SET name=$2, kind=$3, metric=$4, operator=$5, threshold=$6, for_count=$7,
		       severity=$8, host_id=$9, monitor_id=$10, enabled=$11, updated_at=now()
		 WHERE id = $1 RETURNING `+ruleCols,
		a.ID, a.Name, a.Kind, a.Metric, a.Operator, a.Threshold, a.ForCount, a.Severity, a.HostID, a.MonitorID, a.Enabled))
	return out, mapErr(err)
}

// DeleteRule removes a rule and its alerts.
func (s *Store) DeleteRule(ctx context.Context, id string) (string, error) {
	var name string
	err := s.pool.QueryRow(ctx, `DELETE FROM alert_rules WHERE id = $1 RETURNING name`, id).Scan(&name)
	return name, mapErr(err)
}

const alertSelect = `
	SELECT a.id, a.rule_id, r.name, a.target_type, a.target_id, a.target_name, a.state, a.severity,
	       a.message, a.value, a.triggered_at, a.acknowledged_at, a.resolved_at
	  FROM alerts a JOIN alert_rules r ON r.id = a.rule_id`

func scanAlert(r pgx.Row) (domain.Alert, error) {
	var a domain.Alert
	err := r.Scan(&a.ID, &a.RuleID, &a.RuleName, &a.TargetType, &a.TargetID, &a.TargetName, &a.State,
		&a.Severity, &a.Message, &a.Value, &a.TriggeredAt, &a.AcknowledgedAt, &a.ResolvedAt)
	return a, err
}

// AlertFilter narrows ListAlerts.
type AlertFilter struct {
	State    string // "", "open", or a concrete state
	TargetID string
	Since    time.Time
	Limit    int
}

// ListAlerts returns alerts, newest first.
func (s *Store) ListAlerts(ctx context.Context, f AlertFilter) ([]domain.Alert, error) {
	if f.Limit <= 0 {
		f.Limit = 200
	}
	rows, err := s.pool.Query(ctx, alertSelect+`
		WHERE ($1 = '' OR ($1 = 'open' AND a.state <> 'resolved') OR a.state = $1)
		  AND ($2 = '' OR a.target_id::text = $2)
		  AND (a.state <> 'resolved' OR a.triggered_at >= $3)
		ORDER BY (a.state = 'resolved'), a.triggered_at DESC LIMIT $4`,
		f.State, f.TargetID, f.Since, f.Limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.Alert, error) { return scanAlert(r) })
}

// OpenAlertsForRule returns non-resolved alerts for a rule keyed by target ID.
func (s *Store) OpenAlertsForRule(ctx context.Context, ruleID string) (map[string]domain.Alert, error) {
	rows, err := s.pool.Query(ctx, alertSelect+` WHERE a.rule_id = $1 AND a.state <> 'resolved'`, ruleID)
	if err != nil {
		return nil, err
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.Alert, error) { return scanAlert(r) })
	if err != nil {
		return nil, err
	}
	out := make(map[string]domain.Alert, len(list))
	for _, a := range list {
		out[a.TargetID] = a
	}
	return out, nil
}

// OpenAlert creates a firing alert. It returns created=false without error
// if an open alert for the same rule and target already exists; the partial
// unique index makes this race-free across concurrent evaluators.
func (s *Store) OpenAlert(ctx context.Context, a domain.Alert) (domain.Alert, bool, error) {
	var id string
	err := s.pool.QueryRow(ctx, `
		INSERT INTO alerts (rule_id, target_type, target_id, target_name, state, severity, message, value)
		VALUES ($1,$2,$3,$4,'firing',$5,$6,$7)
		ON CONFLICT (rule_id, target_id) WHERE state <> 'resolved' DO NOTHING
		RETURNING id`,
		a.RuleID, a.TargetType, a.TargetID, a.TargetName, a.Severity, a.Message, a.Value).Scan(&id)
	if err == pgx.ErrNoRows {
		return a, false, nil
	}
	if err != nil {
		return a, false, mapErr(err)
	}
	out, err := s.GetAlert(ctx, id)
	return out, true, err
}

// ResolveAlert marks an open alert resolved. resolved=false means it was
// already resolved by someone else, so no notification should be sent.
func (s *Store) ResolveAlert(ctx context.Context, id string) (domain.Alert, bool, error) {
	tag, err := s.pool.Exec(ctx,
		`UPDATE alerts SET state = 'resolved', resolved_at = now() WHERE id = $1 AND state <> 'resolved'`, id)
	if err != nil {
		return domain.Alert{}, false, mapErr(err)
	}
	a, err := s.GetAlert(ctx, id)
	return a, tag.RowsAffected() == 1, err
}

// AcknowledgeAlert moves a firing alert to acknowledged.
func (s *Store) AcknowledgeAlert(ctx context.Context, id string) (domain.Alert, error) {
	tag, err := s.pool.Exec(ctx,
		`UPDATE alerts SET state = 'acknowledged', acknowledged_at = now() WHERE id = $1 AND state = 'firing'`, id)
	if err != nil {
		return domain.Alert{}, mapErr(err)
	}
	a, err := s.GetAlert(ctx, id)
	if err != nil {
		return a, err
	}
	if tag.RowsAffected() == 0 {
		return a, domain.ErrConflict
	}
	return a, nil
}

// GetAlert returns one alert.
func (s *Store) GetAlert(ctx context.Context, id string) (domain.Alert, error) {
	a, err := scanAlert(s.pool.QueryRow(ctx, alertSelect+` WHERE a.id = $1`, id))
	return a, mapErr(err)
}

// ListWebhooks returns notification destinations, including secrets.
func (s *Store) ListWebhooks(ctx context.Context) ([]domain.Webhook, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, name, url, secret, enabled, created_at FROM webhooks ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.Webhook, error) {
		var w domain.Webhook
		err := r.Scan(&w.ID, &w.Name, &w.URL, &w.Secret, &w.Enabled, &w.CreatedAt)
		w.HasSecret = w.Secret != ""
		return w, err
	})
}

// GetWebhook returns one webhook.
func (s *Store) GetWebhook(ctx context.Context, id string) (domain.Webhook, error) {
	var w domain.Webhook
	err := s.pool.QueryRow(ctx, `SELECT id, name, url, secret, enabled, created_at FROM webhooks WHERE id = $1`, id).
		Scan(&w.ID, &w.Name, &w.URL, &w.Secret, &w.Enabled, &w.CreatedAt)
	w.HasSecret = w.Secret != ""
	return w, mapErr(err)
}

// CreateWebhook inserts a webhook.
func (s *Store) CreateWebhook(ctx context.Context, w domain.Webhook) (domain.Webhook, error) {
	err := s.pool.QueryRow(ctx,
		`INSERT INTO webhooks (name, url, secret, enabled) VALUES ($1,$2,$3,$4) RETURNING id, created_at`,
		w.Name, w.URL, w.Secret, w.Enabled).Scan(&w.ID, &w.CreatedAt)
	w.HasSecret = w.Secret != ""
	return w, mapErr(err)
}

// DeleteWebhook removes a webhook.
func (s *Store) DeleteWebhook(ctx context.Context, id string) (string, error) {
	var name string
	err := s.pool.QueryRow(ctx, `DELETE FROM webhooks WHERE id = $1 RETURNING name`, id).Scan(&name)
	return name, mapErr(err)
}
