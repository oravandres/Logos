package router

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/oravandres/Logos/internal/blobstore"
	"github.com/oravandres/Logos/internal/config"
	"github.com/oravandres/Logos/internal/database/dbq"
	"github.com/oravandres/Logos/internal/handler"
	"github.com/oravandres/Logos/internal/imagegen"
	"github.com/oravandres/Logos/internal/middleware"
)

// New builds the chi router with all middleware and API routes wired up.
//
// The image upload / blob endpoints are conditional on the blobstore being
// configured (LOGOS_IMAGE_UPLOAD_DIR). When unset, the routes are NOT
// registered — a request to them produces a chi 404 rather than a runtime
// nil-pointer dereference. The handler itself also returns 503 if reached
// with a nil store, as a belt-and-braces guard for tests.
func New(pool *pgxpool.Pool, cfg config.Config) *chi.Mux {
	q := dbq.New(pool)

	var blobs blobstore.Store
	if cfg.ImageUploadsEnabled() {
		ls, err := blobstore.NewLocalStore(cfg.ImageUploadDir)
		if err != nil {
			// Misconfiguration is a fatal-at-startup signal; the
			// caller (cmd/logos) decides whether to exit. Logging
			// here gives the operator a clear breadcrumb in the
			// `kubectl logs` of the failed pod.
			slog.Error("failed to initialise image blobstore",
				"dir", cfg.ImageUploadDir, "error", err)
		} else {
			blobs = ls
		}
	}

	var gen imagegen.Generator
	if cfg.ImageGenEnabled() {
		switch cfg.ImageGenProvider {
		case "darkbase":
			gen = &imagegen.DarkbaseGenerator{
				BaseURL:    cfg.ImageGenBaseURL,
				AuthToken:  cfg.ImageGenAuthToken,
				HTTPClient: &http.Client{},
			}
		default:
			// Validation in `config.Load` keeps us out of this branch;
			// log loudly if the contract ever drifts.
			slog.Error("unknown image generation provider",
				"provider", cfg.ImageGenProvider)
		}
	}

	health := &handler.HealthHandler{Pinger: pool}
	categories := &handler.CategoryHandler{Q: q}
	images := &handler.ImageHandler{
		Q: q, Blobs: blobs, MaxUploadBytes: cfg.ImageMaxUploadBytes,
		ImageGen: gen, GenTimeout: cfg.ImageGenTimeout,
	}
	authors := &handler.AuthorHandler{Q: q}
	quotes := &handler.QuoteHandler{Q: q}
	tags := &handler.TagHandler{Q: q}
	quoteTags := &handler.QuoteTagHandler{Q: q, Pool: pool}

	r := chi.NewRouter()

	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)
	if len(cfg.CORSAllowedOrigins) > 0 {
		r.Use(cors.Handler(cors.Options{
			AllowedOrigins:   cfg.CORSAllowedOrigins,
			AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
			AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
			ExposedHeaders:   []string{"Link"},
			AllowCredentials: false,
			MaxAge:           300,
		}))
	}
	r.Use(middleware.Logging)
	r.Use(middleware.Metrics)

	r.Get("/livez", health.Live)
	r.Get("/readyz", health.Ready)
	r.Handle("/metrics", promhttp.Handler())

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/health", health.Compat)

		// Custom verb (Google AOIP-style) mounted at the API root so
		// the colon segment doesn't collide with `/images/{id}`. The
		// handler returns 503 when the generator or blobstore are not
		// configured.
		r.Post("/images:generate", images.Generate)

		r.Route("/categories", func(r chi.Router) {
			r.Get("/", categories.List)
			r.Post("/", categories.Create)
			r.Get("/{id}", categories.Get)
			r.Put("/{id}", categories.Update)
			r.Delete("/{id}", categories.Delete)
		})

		r.Route("/images", func(r chi.Router) {
			r.Get("/", images.List)
			r.Post("/", images.Create)
			r.Get("/{id}", images.Get)
			r.Put("/{id}", images.Update)
			r.Delete("/{id}", images.Delete)

			// Upload + blob serving. Registered unconditionally so
			// the surface is discoverable; the handler short-circuits
			// to 503 when the blobstore is nil. We deliberately do
			// not put `/uploads` and `/{id}/blob` behind a chi
			// sub-router gated on `blobs != nil` so OpenAPI / route
			// inspection always shows the same map.
			r.Post("/uploads", images.Upload)
			r.Get("/{id}/blob", images.Blob)
		})

		r.Route("/authors", func(r chi.Router) {
			r.Get("/", authors.List)
			r.Post("/", authors.Create)
			r.Get("/{id}", authors.Get)
			r.Put("/{id}", authors.Update)
			r.Delete("/{id}", authors.Delete)
		})

		r.Route("/quotes", func(r chi.Router) {
			r.Get("/", quotes.List)
			r.Post("/", quotes.Create)
			r.Get("/{id}", quotes.Get)
			r.Put("/{id}", quotes.Update)
			r.Delete("/{id}", quotes.Delete)

			r.Get("/{id}/tags", quoteTags.ListTags)
			r.Post("/{id}/tags", quoteTags.AddTag)
			r.Delete("/{id}/tags/{tagID}", quoteTags.RemoveTag)
		})

		r.Route("/tags", func(r chi.Router) {
			r.Get("/", tags.List)
			r.Post("/", tags.Create)
			r.Get("/{id}", tags.Get)
			r.Delete("/{id}", tags.Delete)
		})
	})

	return r
}
