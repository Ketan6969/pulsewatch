// Package store implements PostgreSQL persistence for Pulsewatch.
package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"pulsewatch/internal/domain"
)

// Store wraps a pgx connection pool. It is safe for concurrent use.
type Store struct {
	pool *pgxpool.Pool
}

// Open connects to PostgreSQL and verifies the connection.
func Open(ctx context.Context, url string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	if cfg.MaxConns < 10 {
		cfg.MaxConns = 10
	}
	cfg.MaxConnIdleTime = 5 * time.Minute
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return &Store{pool: pool}, nil
}

// Close releases all connections.
func (s *Store) Close() { s.pool.Close() }

// Ping checks database connectivity; used by the readiness probe.
func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

// Prune deletes time-series data older than the cutoff. It deletes in
// batches to avoid long-running transactions and lock contention.
func (s *Store) Prune(ctx context.Context, before time.Time) (int64, error) {
	var total int64
	queries := []string{
		`DELETE FROM host_metrics WHERE ctid IN (SELECT ctid FROM host_metrics WHERE ts < $1 LIMIT 10000)`,
		`DELETE FROM check_results WHERE id IN (SELECT id FROM check_results WHERE checked_at < $1 LIMIT 10000)`,
		`DELETE FROM events WHERE id IN (SELECT id FROM events WHERE ts < $1 LIMIT 10000)`,
		`DELETE FROM alerts WHERE id IN (SELECT id FROM alerts WHERE state = 'resolved' AND resolved_at < $1 LIMIT 10000)`,
	}
	for _, q := range queries {
		for {
			tag, err := s.pool.Exec(ctx, q, before)
			if err != nil {
				return total, err
			}
			total += tag.RowsAffected()
			if tag.RowsAffected() < 10000 {
				break
			}
		}
	}
	return total, nil
}

// RecordEvent appends an entry to the event log.
func (s *Store) RecordEvent(ctx context.Context, e domain.Event) error {
	if e.Severity == "" {
		e.Severity = domain.SeverityInfo
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO events (kind, severity, message, target_type, target_id) VALUES ($1,$2,$3,$4,$5)`,
		e.Kind, e.Severity, e.Message, e.TargetType, e.TargetID)
	return err
}

// ListEvents returns the most recent events, newest first.
func (s *Store) ListEvents(ctx context.Context, since time.Time, limit int) ([]domain.Event, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, ts, kind, severity, message, target_type, target_id
		   FROM events WHERE ts >= $1 ORDER BY ts DESC, id DESC LIMIT $2`, since, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (domain.Event, error) {
		var e domain.Event
		err := r.Scan(&e.ID, &e.Time, &e.Kind, &e.Severity, &e.Message, &e.TargetType, &e.TargetID)
		return e, err
	})
}

// mapErr converts driver errors to domain sentinel errors.
func mapErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505": // unique_violation
			return fmt.Errorf("%w: %s", domain.ErrConflict, pgErr.Detail)
		case "23503": // foreign_key_violation
			return fmt.Errorf("%w: referenced object does not exist", domain.ErrNotFound)
		case "22P02": // invalid_text_representation (bad uuid)
			return domain.ErrNotFound
		}
	}
	return err
}

// BucketStep chooses a bucket width that yields roughly 120-240 points.
func BucketStep(rng time.Duration) time.Duration {
	steps := []time.Duration{
		15 * time.Second, 30 * time.Second, time.Minute, 2 * time.Minute, 5 * time.Minute,
		10 * time.Minute, 15 * time.Minute, 30 * time.Minute, time.Hour, 2 * time.Hour,
		6 * time.Hour, 12 * time.Hour, 24 * time.Hour,
	}
	for _, s := range steps {
		if rng/s <= 240 {
			return s
		}
	}
	return steps[len(steps)-1]
}
