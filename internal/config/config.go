package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
)

// Config holds application configuration sourced from environment variables.
type Config struct {
	DatabaseURL string
	APIHost     string
	APIPort     int
	LogLevel    slog.Level
}

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

	return Config{
		DatabaseURL: envOrDefault("DATABASE_URL", "postgres://logos:logos@localhost:5432/logos?sslmode=disable"),
		APIHost:     envOrDefault("API_HOST", "0.0.0.0"),
		APIPort:     port,
		LogLevel:    logLevel,
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
