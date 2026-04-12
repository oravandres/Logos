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

func quoteTagRouter(h *handler.QuoteTagHandler) *chi.Mux {
	r := chi.NewRouter()
	r.Get("/quotes/{id}/tags", h.ListTags)
	r.Post("/quotes/{id}/tags", h.AddTag)
	r.Delete("/quotes/{id}/tags/{tagID}", h.RemoveTag)
	return r
}

func TestQuoteTagListTags_InvalidUUID(t *testing.T) {
	t.Parallel()
	router := quoteTagRouter(&handler.QuoteTagHandler{Q: nilQ()})
	rec := getRequest(t, router, "/quotes/not-a-uuid/tags")
	assertStatus(t, rec, http.StatusBadRequest)
	assertErrorMsg(t, rec, "invalid UUID")
}

func TestQuoteTagAddTag_Validation(t *testing.T) {
	t.Parallel()
	router := quoteTagRouter(&handler.QuoteTagHandler{Q: nilQ()})

	tests := []struct {
		name      string
		path      string
		body      map[string]any
		wantCode  int
		wantError string
	}{
		{
			name:      "invalid quote UUID",
			path:      "/quotes/not-a-uuid/tags",
			body:      map[string]any{"tag_id": "00000000-0000-0000-0000-000000000001"},
			wantCode:  http.StatusBadRequest,
			wantError: "invalid UUID",
		},
		{
			name:      "missing tag_id",
			path:      "/quotes/00000000-0000-0000-0000-000000000001/tags",
			body:      map[string]any{},
			wantCode:  http.StatusBadRequest,
			wantError: "tag_id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec := postJSON(t, router, tt.path, tt.body)
			assertStatus(t, rec, tt.wantCode)
			assertErrorMsg(t, rec, tt.wantError)
		})
	}
}

func TestQuoteTagAddTag_FKViolation(t *testing.T) {
	t.Parallel()
	fkErr := &pgconn.PgError{Code: "23503", Message: "foreign key violation"}
	stub := &stubDBTX{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return errRow{err: fkErr}
		},
	}

	execStub := &execDBTX{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, fkErr
		},
	}
	_ = stub // keep for reference

	router := quoteTagRouter(&handler.QuoteTagHandler{Q: dbq.New(execStub)})

	rec := postJSON(t, router, "/quotes/00000000-0000-0000-0000-000000000001/tags", map[string]any{
		"tag_id": "00000000-0000-0000-0000-000000000099",
	})
	assertStatus(t, rec, http.StatusUnprocessableEntity)
	assertErrorMsg(t, rec, "referenced quote or tag does not exist")
}

func TestQuoteTagRemoveTag_InvalidUUIDs(t *testing.T) {
	t.Parallel()
	router := quoteTagRouter(&handler.QuoteTagHandler{Q: nilQ()})

	tests := []struct {
		name string
		path string
	}{
		{name: "invalid quote UUID", path: "/quotes/not-a-uuid/tags/00000000-0000-0000-0000-000000000001"},
		{name: "invalid tag UUID", path: "/quotes/00000000-0000-0000-0000-000000000001/tags/not-a-uuid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec := deleteRequest(t, router, tt.path)
			assertStatus(t, rec, http.StatusBadRequest)
			assertErrorMsg(t, rec, "invalid UUID")
		})
	}
}

func TestQuoteTagRemoveTag_Success(t *testing.T) {
	t.Parallel()
	router := quoteTagRouter(&handler.QuoteTagHandler{Q: nilQ()})
	rec := deleteRequest(t, router, "/quotes/00000000-0000-0000-0000-000000000001/tags/00000000-0000-0000-0000-000000000002")
	assertStatus(t, rec, http.StatusNoContent)
}

// execDBTX extends stubDBTX with a controllable Exec implementation.
type execDBTX struct {
	execFn func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func (s *execDBTX) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if s.execFn != nil {
		return s.execFn(ctx, sql, args...)
	}
	return pgconn.CommandTag{}, nil
}

func (s *execDBTX) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	return nil, nil
}

func (s *execDBTX) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row {
	return errRow{err: pgx.ErrNoRows}
}
