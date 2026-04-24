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

// usage is printed for `logos help`, `logos -h`, `logos --help`, and
// implicitly for any unknown subcommand before exit. It documents the three
// supported invocation shapes so Kubernetes operators have a local `-h`
// reference that matches the manifest wiring.
//
// The zero-arg shape exists for two callers that pre-date the split:
//   - Local dev via `go run ./cmd/logos` and `docker run ... logos-api`, where
//     there is no init container to own migrations.
//   - Older Deployment manifests whose main container invokes `/logos` with
//     no `args:` override. Keeping the zero-arg behaviour unchanged means
//     pinning a newer image digest is safe on its own; adopting the init
//     container split is a manifest-only follow-up.
const usage = `Logos API server.

Usage:
  logos             migrate, then start the HTTP server (default;
                    backward compatible with single-container deployments).
  logos migrate     run pending database migrations, then exit.
                    intended for a Kubernetes initContainer so schema
                    changes complete before the serving container starts.
  logos serve       start the HTTP server without running migrations.
                    intended for the main container when a separate
                    migrator has already advanced the schema to head.
  logos help        print this message and exit.

Environment:
  DATABASE_URL          PostgreSQL DSN (postgres://, postgresql://, and
                        pgx5:// schemes are all accepted).
  API_HOST, API_PORT    listen address for 'serve' and the default path
                        (default 0.0.0.0:8000). Ignored by 'migrate'.
  LOG_LEVEL             debug|info|warn|error (default info).
  CORS_ALLOWED_ORIGINS  comma-separated origin list (default: CORS disabled).
`

// mode is the dispatch result of parseMode. It is intentionally unexported:
// the CLI surface is a `cmd/logos` concern and callers that want programmatic
// access should use internal/database + internal/router directly.
type mode int

const (
	// modeAll runs migrations and then starts the HTTP server in the same
	// process. This is the zero-arg default and preserves the pre-split
	// single-container behaviour.
	modeAll mode = iota

	// modeMigrate runs migrations and exits. Suitable for a Kubernetes
	// initContainer: the pod only advances to the main container after this
	// command exits 0, so `readyz` in the serving container never answers
	// against a schema older than the embedded migrations.
	modeMigrate

	// modeServe starts the HTTP server without running migrations. It
	// assumes the schema has already been advanced by a separate migrator
	// (typically `logos migrate` in an initContainer, or an ops-run job).
	modeServe

	// modeHelp prints usage and exits 0.
	modeHelp
)

func main() {
	if err := run(os.Args); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

// run is the testable entrypoint. Tests drive parseMode directly; this
// function is covered by CI's `go build` and by integration usage.
func run(args []string) error {
	m, err := parseMode(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		fmt.Fprint(os.Stderr, usage)
		return fmt.Errorf("parse args: %w", err)
	}
	if m == modeHelp {
		if _, err := fmt.Fprint(os.Stdout, usage); err != nil {
			return fmt.Errorf("write usage: %w", err)
		}
		return nil
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	setupLogger(cfg.LogLevel)

	switch m {
	case modeMigrate:
		slog.Info("starting logos", "subcommand", "migrate")
		return runMigrate(cfg)
	case modeServe:
		slog.Info("starting logos", "subcommand", "serve", "addr", cfg.ListenAddr())
		return runServe(cfg)
	case modeAll:
		slog.Info("starting logos", "subcommand", "all", "addr", cfg.ListenAddr())
		if err := runMigrate(cfg); err != nil {
			return err
		}
		return runServe(cfg)
	}
	return fmt.Errorf("unreachable mode: %d", m)
}

// parseMode selects a dispatch mode from the command-line arguments. Exactly
// zero or one positional argument after the binary name is accepted so that
// typos or stray Kubernetes `args:` drift surface as a crash-loop with a
// clear error message rather than being silently reinterpreted as the
// backward-compatible "migrate + serve" path.
func parseMode(args []string) (mode, error) {
	if len(args) <= 1 {
		return modeAll, nil
	}
	if len(args) > 2 {
		return 0, fmt.Errorf("expected at most one subcommand, got %d extra argument(s): %v", len(args)-2, args[2:])
	}
	switch args[1] {
	case "migrate":
		return modeMigrate, nil
	case "serve":
		return modeServe, nil
	case "help", "-h", "--help":
		return modeHelp, nil
	default:
		return 0, fmt.Errorf("unknown subcommand %q", args[1])
	}
}

// runMigrate advances the database schema to the head of the embedded
// migrations and returns. A successful run logs `migrations complete
// version=N dirty=false` from internal/database.RunMigrations so operators
// inspecting initContainer logs can confirm progress without extra output
// here.
func runMigrate(cfg config.Config) error {
	slog.Info("running database migrations")
	if err := database.RunMigrations(cfg.DatabaseURL, migrations.FS); err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}
	return nil
}

// runServe starts the HTTP server and blocks until SIGINT / SIGTERM or an
// unexpected listener error. The database pool is created after `serve` is
// chosen so that `logos migrate` never opens long-lived connections it
// doesn't need.
func runServe(cfg config.Config) error {
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
		Handler:      router.New(pool, cfg),
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
