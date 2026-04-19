package config

import (
	"log/slog"
	"strings"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("API_HOST", "")
	t.Setenv("API_PORT", "")
	t.Setenv("LOG_LEVEL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.DatabaseURL != "postgres://logos:logos@localhost:5432/logos?sslmode=disable" {
		t.Fatalf("DatabaseURL = %q", cfg.DatabaseURL)
	}
	if cfg.APIHost != "0.0.0.0" {
		t.Fatalf("APIHost = %q", cfg.APIHost)
	}
	if cfg.APIPort != 8000 {
		t.Fatalf("APIPort = %d", cfg.APIPort)
	}
	if cfg.LogLevel != slog.LevelInfo {
		t.Fatalf("LogLevel = %v", cfg.LogLevel)
	}
}

func TestLoadRejectsInvalidPort(t *testing.T) {
	t.Setenv("API_PORT", "not-a-number")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "API_PORT must be an integer") {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestLoadRejectsInvalidLogLevel(t *testing.T) {
	t.Setenv("LOG_LEVEL", "verbose")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "LOG_LEVEL must be one of") {
		t.Fatalf("Load() error = %v", err)
	}
}
