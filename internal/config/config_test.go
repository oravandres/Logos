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
	if cfg.ImageUploadDir != "" {
		t.Fatalf("ImageUploadDir = %q want empty (disabled)", cfg.ImageUploadDir)
	}
	if cfg.ImageMaxUploadBytes != 10<<20 {
		t.Fatalf("ImageMaxUploadBytes = %d want %d", cfg.ImageMaxUploadBytes, 10<<20)
	}
	if cfg.ImageUploadsEnabled() {
		t.Fatal("ImageUploadsEnabled = true with empty dir, want false")
	}
}

func TestLoadImageUploadConfig(t *testing.T) {
	t.Setenv("LOGOS_IMAGE_UPLOAD_DIR", "/var/lib/logos/images")
	t.Setenv("LOGOS_IMAGE_MAX_UPLOAD_BYTES", "5242880")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.ImageUploadDir != "/var/lib/logos/images" {
		t.Fatalf("ImageUploadDir = %q", cfg.ImageUploadDir)
	}
	if cfg.ImageMaxUploadBytes != 5242880 {
		t.Fatalf("ImageMaxUploadBytes = %d", cfg.ImageMaxUploadBytes)
	}
	if !cfg.ImageUploadsEnabled() {
		t.Fatal("ImageUploadsEnabled = false, want true")
	}
}

func TestLoadRejectsNonPositiveMaxUpload(t *testing.T) {
	t.Setenv("LOGOS_IMAGE_MAX_UPLOAD_BYTES", "0")
	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want non-nil")
	}
}

func TestLoadRejectsInvalidMaxUpload(t *testing.T) {
	t.Setenv("LOGOS_IMAGE_MAX_UPLOAD_BYTES", "not-a-number")
	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want non-nil")
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
