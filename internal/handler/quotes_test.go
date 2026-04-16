package handler_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/oravandres/Logos/internal/database/dbq"
	"github.com/oravandres/Logos/internal/handler"
)

func quoteRouter(h *handler.QuoteHandler) *chi.Mux {
	r := chi.NewRouter()
	r.Get("/quotes", h.List)
	r.Post("/quotes", h.Create)
	r.Get("/quotes/{id}", h.Get)
	r.Put("/quotes/{id}", h.Update)
	r.Delete("/quotes/{id}", h.Delete)
	return r
}

var validAuthorID = "00000000-0000-0000-0000-000000000001"

func TestQuoteCreate_Validation(t *testing.T) {
	t.Parallel()
	router := quoteRouter(&handler.QuoteHandler{Q: nilQ()})

	tests := []struct {
		name      string
		body      map[string]any
		wantCode  int
		wantError string
	}{
		{
			name:      "missing title",
			body:      map[string]any{"text": "Know thyself", "author_id": validAuthorID},
			wantCode:  http.StatusBadRequest,
			wantError: "title is required",
		},
		{
			name:      "missing text",
			body:      map[string]any{"title": "Famous", "author_id": validAuthorID},
			wantCode:  http.StatusBadRequest,
			wantError: "text is required",
		},
		{
			name:      "missing author_id",
			body:      map[string]any{"title": "Famous", "text": "Know thyself"},
			wantCode:  http.StatusBadRequest,
			wantError: "author_id is required",
		},
		{
			name:      "null author_id",
			body:      map[string]any{"title": "Famous", "text": "Know thyself", "author_id": "00000000-0000-0000-0000-000000000000"},
			wantCode:  http.StatusBadRequest,
			wantError: "author_id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec := postJSON(t, router, "/quotes", tt.body)
			assertStatus(t, rec, tt.wantCode)
			assertErrorMsg(t, rec, tt.wantError)
		})
	}
}

func TestQuoteUpdate_Validation(t *testing.T) {
	t.Parallel()
	router := quoteRouter(&handler.QuoteHandler{Q: nilQ()})

	validUUID := "00000000-0000-0000-0000-000000000002"

	tests := []struct {
		name      string
		path      string
		body      map[string]any
		wantCode  int
		wantError string
	}{
		{
			name:      "invalid UUID",
			path:      "/quotes/not-a-uuid",
			body:      map[string]any{"title": "X", "text": "Y", "author_id": validAuthorID},
			wantCode:  http.StatusBadRequest,
			wantError: "invalid UUID",
		},
		{
			name:      "missing title",
			path:      "/quotes/" + validUUID,
			body:      map[string]any{"text": "Y", "author_id": validAuthorID},
			wantCode:  http.StatusBadRequest,
			wantError: "title is required",
		},
		{
			name:      "missing text",
			path:      "/quotes/" + validUUID,
			body:      map[string]any{"title": "X", "author_id": validAuthorID},
			wantCode:  http.StatusBadRequest,
			wantError: "text is required",
		},
		{
			name:      "missing author_id",
			path:      "/quotes/" + validUUID,
			body:      map[string]any{"title": "X", "text": "Y"},
			wantCode:  http.StatusBadRequest,
			wantError: "author_id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec := putJSON(t, router, tt.path, tt.body)
			assertStatus(t, rec, tt.wantCode)
			assertErrorMsg(t, rec, tt.wantError)
		})
	}
}

func TestQuoteGet_InvalidUUID(t *testing.T) {
	t.Parallel()
	router := quoteRouter(&handler.QuoteHandler{Q: nilQ()})
	rec := getRequest(t, router, "/quotes/not-a-uuid")
	assertStatus(t, rec, http.StatusBadRequest)
	assertErrorMsg(t, rec, "invalid UUID")
}

func TestQuoteCreate_FKViolation(t *testing.T) {
	t.Parallel()
	fkErr := &pgconn.PgError{Code: "23503", Message: "insert violates foreign key constraint"}
	stub := &stubDBTX{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return errRow{err: fkErr}
		},
	}
	router := quoteRouter(&handler.QuoteHandler{Q: dbq.New(stub)})

	rec := postJSON(t, router, "/quotes", map[string]any{
		"title":     "Famous",
		"text":      "Know thyself",
		"author_id": validAuthorID,
	})
	assertStatus(t, rec, http.StatusUnprocessableEntity)
	assertErrorMsg(t, rec, "referenced author, image, or category does not exist")
}

func TestQuoteCreate_CategoryTypeMismatch(t *testing.T) {
	t.Parallel()
	stub := &stubDBTX{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &scanCategoryRow{name: "portraits", catType: "image"}
		},
	}
	router := quoteRouter(&handler.QuoteHandler{Q: dbq.New(stub)})

	rec := postJSON(t, router, "/quotes", map[string]any{
		"title":       "Famous",
		"text":        "Know thyself",
		"author_id":   validAuthorID,
		"category_id": "00000000-0000-0000-0000-000000000001",
	})
	assertStatus(t, rec, http.StatusUnprocessableEntity)
	assertErrorMsg(t, rec, `category type must be "quote"`)
}

func TestQuoteList_InvalidFilters(t *testing.T) {
	t.Parallel()
	router := quoteRouter(&handler.QuoteHandler{Q: nilQ()})

	tests := []struct {
		name      string
		query     string
		wantCode  int
		wantError string
	}{
		{
			name:      "invalid author_id",
			query:     "?author_id=not-a-uuid",
			wantCode:  http.StatusBadRequest,
			wantError: "invalid author_id",
		},
		{
			name:      "invalid category_id",
			query:     "?category_id=not-a-uuid",
			wantCode:  http.StatusBadRequest,
			wantError: "invalid category_id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec := getRequest(t, router, "/quotes"+tt.query)
			assertStatus(t, rec, tt.wantCode)
			assertErrorMsg(t, rec, tt.wantError)
		})
	}
}

func TestQuoteDelete_InvalidUUID(t *testing.T) {
	t.Parallel()
	router := quoteRouter(&handler.QuoteHandler{Q: nilQ()})
	rec := deleteRequest(t, router, "/quotes/not-a-uuid")
	assertStatus(t, rec, http.StatusBadRequest)
	assertErrorMsg(t, rec, "invalid UUID")
}

func TestQuoteDelete_Success(t *testing.T) {
	t.Parallel()
	stub := &stubDBTX{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return dummyDeleteRow{}
		},
	}
	router := quoteRouter(&handler.QuoteHandler{Q: dbq.New(stub)})
	rec := deleteRequest(t, router, "/quotes/00000000-0000-0000-0000-000000000001")
	assertStatus(t, rec, http.StatusNoContent)
}

func TestQuoteCreate_CheckViolation(t *testing.T) {
	t.Parallel()
	callCount := 0
	stub := &stubDBTX{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			callCount++
			if callCount == 1 {
				return &scanCategoryRow{name: "philosophy", catType: "quote"}
			}
			return errRow{err: &pgconn.PgError{Code: "23514", ConstraintName: "chk_quotes_category_type"}}
		},
	}
	router := quoteRouter(&handler.QuoteHandler{Q: dbq.New(stub)})

	rec := postJSON(t, router, "/quotes", map[string]any{
		"title":       "Famous",
		"text":        "Know thyself",
		"author_id":   validAuthorID,
		"category_id": "00000000-0000-0000-0000-000000000001",
	})
	assertStatus(t, rec, http.StatusUnprocessableEntity)
	assertErrorMsg(t, rec, `category type must be "quote"`)
}
