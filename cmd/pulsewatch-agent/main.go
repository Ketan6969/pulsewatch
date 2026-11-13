// Command pulsewatch-agent collects host metrics and pushes them to a
// Pulsewatch server.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"pulsewatch/internal/agent"
	"pulsewatch/internal/config"
	"pulsewatch/internal/sysmon"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	cfg, err := config.LoadAgent()
	if err != nil {
		fmt.Fprintln(os.Stderr, "pulsewatch-agent: configuration:", err)
		os.Exit(1)
	}
	log := config.NewLogger(os.Stdout, cfg.LogLevel, cfg.LogFormat)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Info("agent starting", "server", cfg.ServerURL, "interval", cfg.Interval.String(), "version", version)
	a := agent.New(cfg.ServerURL, cfg.AgentToken, cfg.Interval, sysmon.NewCollector(cfg.Name, version, cfg.TopProcs), log)
	if err := a.Run(ctx); err != nil {
		log.Error("agent stopped", "err", err)
		os.Exit(1)
	}
	log.Info("agent stopped")
}
