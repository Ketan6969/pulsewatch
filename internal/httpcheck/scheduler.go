package httpcheck

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"

	"pulsewatch/internal/domain"
)

// Store is the persistence the scheduler needs.
type Store interface {
	ListMonitors(ctx context.Context) ([]domain.Monitor, error)
	InsertCheckResult(ctx context.Context, r domain.CheckResult) error
}

// Scheduler runs one goroutine per enabled monitor and reconciles the set of
// running monitors with the database whenever Refresh is called and on a
// periodic resync (which also picks up changes made by other replicas).
type Scheduler struct {
	store   Store
	checker *Checker
	log     *slog.Logger
	sem     chan struct{} // bounds concurrent in-flight checks
	resync  time.Duration
	refresh chan struct{}

	mu      sync.Mutex
	runners map[string]*runner
	wg      sync.WaitGroup
}

type runner struct {
	monitor domain.Monitor
	cancel  context.CancelFunc
}

// NewScheduler creates a scheduler allowing maxConcurrent simultaneous checks.
func NewScheduler(store Store, checker *Checker, maxConcurrent int, log *slog.Logger) *Scheduler {
	return &Scheduler{
		store:   store,
		checker: checker,
		log:     log,
		sem:     make(chan struct{}, maxConcurrent),
		resync:  30 * time.Second,
		refresh: make(chan struct{}, 1),
		runners: map[string]*runner{},
	}
}

// Refresh asks the scheduler to reload monitors. It never blocks.
func (s *Scheduler) Refresh() {
	select {
	case s.refresh <- struct{}{}:
	default: // a refresh is already pending
	}
}

// Run blocks until ctx is cancelled, then stops all runners and waits for
// in-flight checks to finish.
func (s *Scheduler) Run(ctx context.Context) {
	ticker := time.NewTicker(s.resync)
	defer ticker.Stop()
	s.reconcile(ctx)
	for {
		select {
		case <-ctx.Done():
			s.stopAll()
			return
		case <-ticker.C:
			s.reconcile(ctx)
		case <-s.refresh:
			s.reconcile(ctx)
		}
	}
}

// Running returns the number of active monitor goroutines.
func (s *Scheduler) Running() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.runners)
}

func (s *Scheduler) reconcile(ctx context.Context) {
	lctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	monitors, err := s.store.ListMonitors(lctx)
	if err != nil {
		if ctx.Err() == nil {
			s.log.Error("load monitors", "err", err)
		}
		return
	}
	want := make(map[string]domain.Monitor, len(monitors))
	for _, m := range monitors {
		if m.Enabled {
			want[m.ID] = m
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for id, r := range s.runners {
		m, ok := want[id]
		if !ok || !m.UpdatedAt.Equal(r.monitor.UpdatedAt) {
			r.cancel()
			delete(s.runners, id)
		}
	}
	for id, m := range want {
		if _, ok := s.runners[id]; ok {
			continue
		}
		rctx, rcancel := context.WithCancel(ctx)
		s.runners[id] = &runner{monitor: m, cancel: rcancel}
		s.wg.Add(1)
		go s.loop(rctx, m)
	}
}

func (s *Scheduler) stopAll() {
	s.mu.Lock()
	for id, r := range s.runners {
		r.cancel()
		delete(s.runners, id)
	}
	s.mu.Unlock()
	s.wg.Wait()
}

func (s *Scheduler) loop(ctx context.Context, m domain.Monitor) {
	defer s.wg.Done()
	interval := time.Duration(m.IntervalS) * time.Second

	// Spread the first check so a restart doesn't fire every monitor at once.
	maxJitter := min(interval, 5*time.Second)
	first := time.NewTimer(time.Duration(rand.Int64N(int64(maxJitter) + 1)))
	select {
	case <-ctx.Done():
		first.Stop()
		return
	case <-first.C:
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		s.runCheck(ctx, m)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Scheduler) runCheck(ctx context.Context, m domain.Monitor) {
	select {
	case s.sem <- struct{}{}:
	case <-ctx.Done():
		return
	}
	res := s.checker.Check(ctx, m)
	<-s.sem

	if ctx.Err() != nil {
		// The monitor was stopped or reconfigured mid-check; the result
		// reflects a cancellation, not the endpoint, so discard it.
		return
	}
	sctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := s.store.InsertCheckResult(sctx, res); err != nil {
		if ctx.Err() == nil {
			s.log.Error("store check result", "monitor_id", m.ID, "err", err)
		}
		return
	}
	if !res.Success {
		s.log.Info("check failed", "monitor", m.Name, "url", m.URL, "status", res.StatusCode, "error", res.Error)
	}
}
