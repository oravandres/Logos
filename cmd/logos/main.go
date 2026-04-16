package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/oravandres/Logos/internal/config"
	"github.com/oravandres/Logos/internal/database"
	"github.com/oravandres/Logos/internal/router"
	"github.com/oravandres/Logos/migrations"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	setupLogger(cfg.LogLevel)

	slog.Info("starting logos", "addr", cfg.ListenAddr())

	slog.Info("running database migrations")
	if err := database.RunMigrations(cfg.DatabaseURL, migrations.FS); err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	poolCtx, poolCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer poolCancel()

	pool, err := database.NewPool(poolCtx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("database connection failed: %w", err)
	}
	defer pool.Close()
	slog.Info("database connected")

	srv := &http.Server{
		Addr:         cfg.ListenAddr(),
		Handler:      router.New(pool),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()

	slog.Info("server listening", "addr", cfg.ListenAddr())

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		slog.Info("shutting down", "signal", sig)
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("server failed: %w", err)
		}
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown error: %w", err)
	}

	slog.Info("server stopped")
	return nil
}

func setupLogger(level slog.Level) {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})))
}
