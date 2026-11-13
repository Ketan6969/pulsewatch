// Package config loads runtime configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Server holds configuration for the pulsewatch server process.
type Server struct {
	HTTPAddr            string
	DatabaseURL         string
	LogLevel            string
	LogFormat           string
	WebDir              string // directory with the built dashboard; empty disables static serving
	AgentToken          string // shared secret agents must present; required
	APIToken            string // optional bearer token protecting the REST API
	ShutdownTimeout     time.Duration
	Retention           time.Duration // how long metrics and check results are kept
	AlertInterval       time.Duration // alert evaluation period
	MaxConcurrentChecks int
	WebhookTimeout      time.Duration
	CORSOrigin          string
}

// Agent holds configuration for the pulsewatch-agent process.
type Agent struct {
	ServerURL  string
	AgentToken string
	Interval   time.Duration
	Name       string // display name; defaults to hostname
	TopProcs   int
	LogLevel   string
	LogFormat  string
}

// LoadServer reads server configuration from the environment.
func LoadServer() (Server, error) {
	var errs []error
	c := Server{
		HTTPAddr:            str("PULSEWATCH_HTTP_ADDR", ":8080"),
		DatabaseURL:         str("PULSEWATCH_DATABASE_URL", ""),
		LogLevel:            str("PULSEWATCH_LOG_LEVEL", "info"),
		LogFormat:           str("PULSEWATCH_LOG_FORMAT", "json"),
		WebDir:              str("PULSEWATCH_WEB_DIR", ""),
		AgentToken:          str("PULSEWATCH_AGENT_TOKEN", ""),
		APIToken:            str("PULSEWATCH_API_TOKEN", ""),
		CORSOrigin:          str("PULSEWATCH_CORS_ORIGIN", ""),
		ShutdownTimeout:     dur("PULSEWATCH_SHUTDOWN_TIMEOUT", 15*time.Second, &errs),
		Retention:           dur("PULSEWATCH_RETENTION", 7*24*time.Hour, &errs),
		AlertInterval:       dur("PULSEWATCH_ALERT_INTERVAL", 15*time.Second, &errs),
		WebhookTimeout:      dur("PULSEWATCH_WEBHOOK_TIMEOUT", 10*time.Second, &errs),
		MaxConcurrentChecks: integer("PULSEWATCH_MAX_CONCURRENT_CHECKS", 64, &errs),
	}
	if c.DatabaseURL == "" {
		errs = append(errs, errors.New("PULSEWATCH_DATABASE_URL is required"))
	}
	if len(c.AgentToken) < 16 {
		errs = append(errs, errors.New("PULSEWATCH_AGENT_TOKEN is required and must be at least 16 characters"))
	}
	if c.Retention < time.Hour {
		errs = append(errs, errors.New("PULSEWATCH_RETENTION must be at least 1h"))
	}
	if c.AlertInterval < time.Second {
		errs = append(errs, errors.New("PULSEWATCH_ALERT_INTERVAL must be at least 1s"))
	}
	if c.MaxConcurrentChecks < 1 {
		errs = append(errs, errors.New("PULSEWATCH_MAX_CONCURRENT_CHECKS must be positive"))
	}
	return c, errors.Join(errs...)
}

// LoadAgent reads agent configuration from the environment.
func LoadAgent() (Agent, error) {
	var errs []error
	c := Agent{
		ServerURL:  strings.TrimRight(str("PULSEWATCH_SERVER_URL", "http://localhost:8080"), "/"),
		AgentToken: str("PULSEWATCH_AGENT_TOKEN", ""),
		Interval:   dur("PULSEWATCH_AGENT_INTERVAL", 15*time.Second, &errs),
		Name:       str("PULSEWATCH_AGENT_NAME", ""),
		TopProcs:   integer("PULSEWATCH_AGENT_TOP_PROCS", 25, &errs),
		LogLevel:   str("PULSEWATCH_LOG_LEVEL", "info"),
		LogFormat:  str("PULSEWATCH_LOG_FORMAT", "json"),
	}
	if c.AgentToken == "" {
		errs = append(errs, errors.New("PULSEWATCH_AGENT_TOKEN is required"))
	}
	if c.Interval < time.Second {
		errs = append(errs, errors.New("PULSEWATCH_AGENT_INTERVAL must be at least 1s"))
	}
	if c.TopProcs < 0 || c.TopProcs > 500 {
		errs = append(errs, errors.New("PULSEWATCH_AGENT_TOP_PROCS must be between 0 and 500"))
	}
	return c, errors.Join(errs...)
}

func str(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return def
}

func dur(key string, def time.Duration, errs *[]error) time.Duration {
	v := str(key, "")
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		*errs = append(*errs, fmt.Errorf("%s: invalid duration %q", key, v))
		return def
	}
	return d
}

func integer(key string, def int, errs *[]error) int {
	v := str(key, "")
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		*errs = append(*errs, fmt.Errorf("%s: invalid integer %q", key, v))
		return def
	}
	return n
}
