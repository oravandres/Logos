package router

import (
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/oravandres/Logos/internal/config"
	"github.com/oravandres/Logos/internal/database/dbq"
	"github.com/oravandres/Logos/internal/handler"
	"github.com/oravandres/Logos/internal/middleware"
)

// New builds the chi router with all middleware and API routes wired up.
func New(pool *pgxpool.Pool, cfg config.Config) *chi.Mux {
	q := dbq.New(pool)

	health := &handler.HealthHandler{Pinger: pool}
	categories := &handler.CategoryHandler{Q: q}
	images := &handler.ImageHandler{Q: q}
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
		r.Get("/health", health.Ready)

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
