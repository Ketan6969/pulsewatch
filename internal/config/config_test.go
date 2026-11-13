package config

import (
	"testing"
	"time"
)

func TestLoadServerRequiresSecrets(t *testing.T) {
	t.Setenv("PULSEWATCH_DATABASE_URL", "")
	t.Setenv("PULSEWATCH_AGENT_TOKEN", "short")
	if _, err := LoadServer(); err == nil {
		t.Fatal("expected error for missing database URL and short token")
	}
}

func TestLoadServerDefaultsAndOverrides(t *testing.T) {
	t.Setenv("PULSEWATCH_DATABASE_URL", "postgres://x")
	t.Setenv("PULSEWATCH_AGENT_TOKEN", "0123456789abcdef")
	t.Setenv("PULSEWATCH_RETENTION", "48h")
	c, err := LoadServer()
	if err != nil {
		t.Fatal(err)
	}
	if c.HTTPAddr != ":8080" {
		t.Errorf("HTTPAddr = %q", c.HTTPAddr)
	}
	if c.Retention != 48*time.Hour {
		t.Errorf("Retention = %v", c.Retention)
	}
}

func TestLoadServerBadDuration(t *testing.T) {
	t.Setenv("PULSEWATCH_DATABASE_URL", "postgres://x")
	t.Setenv("PULSEWATCH_AGENT_TOKEN", "0123456789abcdef")
	t.Setenv("PULSEWATCH_ALERT_INTERVAL", "soon")
	if _, err := LoadServer(); err == nil {
		t.Fatal("expected error for invalid duration")
	}
}

func TestLoadAgent(t *testing.T) {
	t.Setenv("PULSEWATCH_AGENT_TOKEN", "tok")
	t.Setenv("PULSEWATCH_SERVER_URL", "http://server:8080/")
	c, err := LoadAgent()
	if err != nil {
		t.Fatal(err)
	}
	if c.ServerURL != "http://server:8080" {
		t.Errorf("ServerURL = %q", c.ServerURL)
	}
}
