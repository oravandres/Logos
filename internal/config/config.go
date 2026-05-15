package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
)

// Config holds application configuration sourced from environment variables.
type Config struct {
	DatabaseURL        string
	APIHost            string
	APIPort            int
	LogLevel           slog.Level
	MigrationsPath     string
	CORSAllowedOrigins []string

	// ImageUploadDir is the on-disk root for the LocalStore-backed image
	// blobstore. In Kubernetes this is the mount path for the
	// `logos-images` PVC; in local dev the empty string disables the
	// upload / generate endpoints (they return 503).
	ImageUploadDir string

	// ImageMaxUploadBytes caps the per-request size of multipart image
	// uploads. Defaults to 10 MiB; mirrored on the Nginx body limit so
	// the proxy and the app agree on the boundary.
	ImageMaxUploadBytes int64
}

// imageUploadsEnabled is the source-of-truth predicate used by handlers
// to fail fast when the operator forgot to configure the blobstore.
const defaultImageMaxUploadBytes int64 = 10 << 20 // 10 MiB

// ImageUploadsEnabled reports whether the upload / generate code paths
// have a writable storage backend configured.
func (c Config) ImageUploadsEnabled() bool { return c.ImageUploadDir != "" }

// Load reads environment variables and returns a populated Config with defaults.
func Load() (Config, error) {
	port, err := envIntInRangeOrDefault("API_PORT", 8000, 0, 65535)
	if err != nil {
		return Config{}, err
	}
	logLevel, err := parseLogLevel(envOrDefault("LOG_LEVEL", "info"))
	if err != nil {
		return Config{}, err
	}

	maxUpload, err := envInt64OrDefault("LOGOS_IMAGE_MAX_UPLOAD_BYTES", defaultImageMaxUploadBytes)
	if err != nil {
		return Config{}, err
	}
	if maxUpload <= 0 {
		return Config{}, fmt.Errorf("LOGOS_IMAGE_MAX_UPLOAD_BYTES must be > 0")
	}

	return Config{
		DatabaseURL:         envOrDefault("DATABASE_URL", "postgres://logos:logos@localhost:5432/logos?sslmode=disable"),
		APIHost:             envOrDefault("API_HOST", "0.0.0.0"),
		APIPort:             port,
		LogLevel:            logLevel,
		MigrationsPath:      envOrDefault("MIGRATIONS_PATH", ""),
		CORSAllowedOrigins:  envSlice("CORS_ALLOWED_ORIGINS", nil),
		ImageUploadDir:      envOrDefault("LOGOS_IMAGE_UPLOAD_DIR", ""),
		ImageMaxUploadBytes: maxUpload,
	}, nil
}

// ListenAddr returns the host:port string the server should bind to.
func (c Config) ListenAddr() string {
	return fmt.Sprintf("%s:%d", c.APIHost, c.APIPort)
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envSlice(key string, fallback []string) []string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func envInt64OrDefault(key string, fallback int64) (int64, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", key, err)
	}
	return n, nil
}

func envIntInRangeOrDefault(key string, fallback, min, max int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}

	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", key, err)
	}
	if n < min || n > max {
		return 0, fmt.Errorf("%s must be between %d and %d", key, min, max)
	}
	return n, nil
}

func parseLogLevel(v string) (slog.Level, error) {
	switch v {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("LOG_LEVEL must be one of: debug, info, warn, error")
	}
}
