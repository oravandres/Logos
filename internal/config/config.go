package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds application configuration sourced from environment variables.
type Config struct {
	DatabaseURL        string
	APIHost            string
	APIPort            int
	LogLevel           string
	MigrationsPath     string
	CORSAllowedOrigins []string
}

// Load reads environment variables and returns a populated Config with defaults.
func Load() Config {
	return Config{
		DatabaseURL:        envOrDefault("DATABASE_URL", "postgres://logos:logos@localhost:5432/logos?sslmode=disable"),
		APIHost:            envOrDefault("API_HOST", "0.0.0.0"),
		APIPort:            envIntOrDefault("API_PORT", 8000),
		LogLevel:           envOrDefault("LOG_LEVEL", "info"),
		MigrationsPath:     envOrDefault("MIGRATIONS_PATH", ""),
		CORSAllowedOrigins: envSlice("CORS_ALLOWED_ORIGINS", []string{"*"}),
	}
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
	return strings.Split(v, ",")
}

func envIntOrDefault(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
