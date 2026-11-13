// Command pulsewatch runs the Pulsewatch server: REST API, dashboard,
// HTTP check scheduler, alert engine, webhook notifier and data retention.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"pulsewatch/internal/alert"
	"pulsewatch/internal/api"
	"pulsewatch/internal/config"
	"pulsewatch/internal/httpcheck"
	"pulsewatch/internal/notify"
	"pulsewatch/internal/store"
)

func main() {
	// "pulsewatch healthcheck" probes the local readiness endpoint; used by
	// container health checks in images that ship without curl or a shell.
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck())
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "pulsewatch:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.LoadServer()
	if err != nil {
		return fmt.Errorf("configuration: %w", err)
	}
	log := config.NewLogger(os.Stdout, cfg.LogLevel, cfg.LogFormat)
	slog.SetDefault(log)

	// ctx is cancelled on SIGINT/SIGTERM and drives every background worker.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := openStore(ctx, cfg.DatabaseURL, log)
	if err != nil {
		return err
	}
	defer st.Close()
	if err := st.Migrate(ctx, log); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	notifier := notify.New(st, cfg.WebhookTimeout, log.With("component", "notify"))
	scheduler := httpcheck.NewScheduler(st, httpcheck.NewChecker(), cfg.MaxConcurrentChecks, log.With("component", "httpcheck"))
	engine := alert.NewEngine(st, notifier, log.With("component", "alert"))

	srv := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: api.New(st, scheduler, notifier, log.With("component", "api"), api.Options{
			AgentToken: cfg.AgentToken, APIToken: cfg.APIToken, WebDir: cfg.WebDir, CORSOrigin: cfg.CORSOrigin,
		}).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// Background workers use their own context so they can be stopped after
	// the HTTP server has drained (handlers may still enqueue notifications).
	workCtx, stopWork := context.WithCancel(context.Background())
	defer stopWork()
	var wg sync.WaitGroup
	start := func(name string, fn func()) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fn()
			log.Info("worker stopped", "worker", name)
		}()
	}
	start("notifier", func() { notifier.Run(workCtx, 5*time.Second) })
	start("scheduler", func() { scheduler.Run(workCtx) })
	start("alert-engine", func() { engine.Run(workCtx, cfg.AlertInterval) })
	start("retention", func() { retention(workCtx, st, cfg.Retention, log) })

	errCh := make(chan error, 1)
	go func() {
		log.Info("http server listening", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		log.Info("shutdown signal received")
	case err := <-errCh:
		if err != nil {
			stopWork()
			wg.Wait()
			return fmt.Errorf("http server: %w", err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("http shutdown", "err", err)
	}
	stopWork()

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
		log.Info("shutdown complete")
	case <-shutdownCtx.Done():
		log.Warn("shutdown timed out waiting for workers")
	}
	return nil
}

func healthcheck() int {
	addr := os.Getenv("PULSEWATCH_HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck: bad PULSEWATCH_HTTP_ADDR:", err)
		return 1
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + port + "/readyz")
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		return 1
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "healthcheck: status", resp.StatusCode)
		return 1
	}
	return 0
}

// openStore retries the initial connection so the server tolerates the
// database starting after it (common with docker compose).
func openStore(ctx context.Context, url string, log *slog.Logger) (*store.Store, error) {
	delay := time.Second
	for attempt := 1; ; attempt++ {
		st, err := store.Open(ctx, url)
		if err == nil {
			return st, nil
		}
		if attempt >= 10 {
			return nil, fmt.Errorf("connect to database: %w", err)
		}
		log.Warn("database not ready; retrying", "attempt", attempt, "err", err)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
		delay = min(delay*2, 10*time.Second)
	}
}

func retention(ctx context.Context, st *store.Store, keep time.Duration, log *slog.Logger) {
	t := time.NewTicker(10 * time.Minute)
	defer t.Stop()
	for {
		pctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		n, err := st.Prune(pctx, time.Now().Add(-keep))
		cancel()
		if err != nil && ctx.Err() == nil {
			log.Error("retention prune", "err", err)
		} else if n > 0 {
			log.Info("retention pruned rows", "rows", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
