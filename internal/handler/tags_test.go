package handler_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/oravandres/Logos/internal/database/dbq"
	"github.com/oravandres/Logos/internal/handler"
)

func tagRouter(h *handler.TagHandler) *chi.Mux {
	r := chi.NewRouter()
	r.Get("/tags", h.List)
	r.Post("/tags", h.Create)
	r.Get("/tags/{id}", h.Get)
	r.Delete("/tags/{id}", h.Delete)
	return r
}

func TestTagCreate_Validation(t *testing.T) {
	t.Parallel()
	router := tagRouter(&handler.TagHandler{Q: nilQ()})

	tests := []struct {
		name      string
		body      map[string]any
		wantCode  int
		wantError string
	}{
		{
			name:      "missing name",
			body:      map[string]any{},
			wantCode:  http.StatusBadRequest,
			wantError: "name is required",
		},
		{
			name:      "empty name",
			body:      map[string]any{"name": ""},
			wantCode:  http.StatusBadRequest,
			wantError: "name is required",
		},
		{
			name:      "name too long",
			body:      map[string]any{"name": strings.Repeat("a", 101)},
			wantCode:  http.StatusBadRequest,
			wantError: "name must be 100 characters or fewer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec := postJSON(t, router, "/tags", tt.body)
			assertStatus(t, rec, tt.wantCode)
			assertErrorMsg(t, rec, tt.wantError)
		})
	}
}

func TestTagCreate_UniqueViolation(t *testing.T) {
	t.Parallel()
	stub := &stubDBTX{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return errRow{err: &pgconn.PgError{Code: "23505", Message: "duplicate key"}}
		},
	}
	router := tagRouter(&handler.TagHandler{Q: dbq.New(stub)})

	rec := postJSON(t, router, "/tags", map[string]any{"name": "stoicism"})
	assertStatus(t, rec, http.StatusConflict)
	assertErrorMsg(t, rec, "tag with this name already exists")
}

func TestTagGet_InvalidUUID(t *testing.T) {
	t.Parallel()
	router := tagRouter(&handler.TagHandler{Q: nilQ()})
	rec := getRequest(t, router, "/tags/not-a-uuid")
	assertStatus(t, rec, http.StatusBadRequest)
	assertErrorMsg(t, rec, "invalid UUID")
}

func TestTagGet_NotFound(t *testing.T) {
	t.Parallel()
	router := tagRouter(&handler.TagHandler{Q: nilQ()})
	rec := getRequest(t, router, "/tags/00000000-0000-0000-0000-000000000001")
	assertStatus(t, rec, http.StatusNotFound)
	assertErrorMsg(t, rec, "tag not found")
}

func TestTagDelete_InvalidUUID(t *testing.T) {
	t.Parallel()
	router := tagRouter(&handler.TagHandler{Q: nilQ()})
	rec := deleteRequest(t, router, "/tags/not-a-uuid")
	assertStatus(t, rec, http.StatusBadRequest)
	assertErrorMsg(t, rec, "invalid UUID")
}

func TestTagDelete_Success(t *testing.T) {
	t.Parallel()
	router := tagRouter(&handler.TagHandler{Q: nilQ()})
	rec := deleteRequest(t, router, "/tags/00000000-0000-0000-0000-000000000001")
	assertStatus(t, rec, http.StatusNoContent)
}
