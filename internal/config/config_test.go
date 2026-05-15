package config

import (
	"log/slog"
	"strings"
	"testing"
	"time"
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
	if cfg.ImageGenEnabled() {
		t.Fatal("ImageGenEnabled = true with no provider/base URL, want false")
	}
	if cfg.ImageGenTimeout != 120*time.Second {
		t.Fatalf("ImageGenTimeout = %v, want 120s", cfg.ImageGenTimeout)
	}
}

func TestLoadImageGenConfig(t *testing.T) {
	t.Setenv("LOGOS_IMAGEGEN_PROVIDER", "darkbase")
	t.Setenv("LOGOS_IMAGEGEN_BASE_URL", "http://image-adapter.darkbase.svc:8081")
	t.Setenv("LOGOS_IMAGEGEN_AUTH_TOKEN", "shh")
	t.Setenv("LOGOS_IMAGEGEN_TIMEOUT_SECONDS", "300")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.ImageGenProvider != "darkbase" {
		t.Fatalf("ImageGenProvider = %q", cfg.ImageGenProvider)
	}
	if cfg.ImageGenBaseURL != "http://image-adapter.darkbase.svc:8081" {
		t.Fatalf("ImageGenBaseURL = %q", cfg.ImageGenBaseURL)
	}
	if cfg.ImageGenAuthToken != "shh" {
		t.Fatalf("ImageGenAuthToken = %q", cfg.ImageGenAuthToken)
	}
	if cfg.ImageGenTimeout != 300*time.Second {
		t.Fatalf("ImageGenTimeout = %v", cfg.ImageGenTimeout)
	}
	if !cfg.ImageGenEnabled() {
		t.Fatal("ImageGenEnabled = false, want true")
	}
}

func TestLoadImageGenEnabledRequiresBaseURL(t *testing.T) {
	t.Setenv("LOGOS_IMAGEGEN_PROVIDER", "darkbase")
	t.Setenv("LOGOS_IMAGEGEN_BASE_URL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.ImageGenEnabled() {
		t.Fatal("ImageGenEnabled = true with empty base URL, want false")
	}
}

func TestLoadRejectsUnknownImageGenProvider(t *testing.T) {
	t.Setenv("LOGOS_IMAGEGEN_PROVIDER", "magic-pony")
	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want non-nil")
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
