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

// scanQuoteRow returns nil from Scan, simulating a found quote row.
type scanQuoteRow struct{}

func (scanQuoteRow) Scan(_ ...any) error { return nil }

// quoteExistsStub returns a DBTX where GetQuote (QueryRow) succeeds
// but Exec can be overridden for the subsequent AddTagToQuote call.
func quoteExistsExecStub(execFn func(context.Context, string, ...any) (pgconn.CommandTag, error)) *execDBTX {
	return &execDBTX{
		execFn: execFn,
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return scanQuoteRow{}
		},
	}
}

func TestQuoteTagListTags_InvalidUUID(t *testing.T) {
	t.Parallel()
	router := quoteTagRouter(&handler.QuoteTagHandler{Q: nilQ()})
	rec := getRequest(t, router, "/quotes/not-a-uuid/tags")
	assertStatus(t, rec, http.StatusBadRequest)
	assertErrorMsg(t, rec, "invalid UUID")
}

func TestQuoteTagListTags_QuoteNotFound(t *testing.T) {
	t.Parallel()
	router := quoteTagRouter(&handler.QuoteTagHandler{Q: nilQ()})
	rec := getRequest(t, router, "/quotes/00000000-0000-0000-0000-000000000001/tags")
	assertStatus(t, rec, http.StatusNotFound)
	assertErrorMsg(t, rec, "quote not found")
}

func TestQuoteTagAddTag_InvalidQuoteUUID(t *testing.T) {
	t.Parallel()
	router := quoteTagRouter(&handler.QuoteTagHandler{Q: nilQ()})
	rec := postJSON(t, router, "/quotes/not-a-uuid/tags", map[string]any{
		"tag_id": "00000000-0000-0000-0000-000000000001",
	})
	assertStatus(t, rec, http.StatusBadRequest)
	assertErrorMsg(t, rec, "invalid UUID")
}

func TestQuoteTagAddTag_QuoteNotFound(t *testing.T) {
	t.Parallel()
	router := quoteTagRouter(&handler.QuoteTagHandler{Q: nilQ()})
	rec := postJSON(t, router, "/quotes/00000000-0000-0000-0000-000000000001/tags", map[string]any{
		"tag_id": "00000000-0000-0000-0000-000000000002",
	})
	assertStatus(t, rec, http.StatusNotFound)
	assertErrorMsg(t, rec, "quote not found")
}

func TestQuoteTagAddTag_MissingTagID(t *testing.T) {
	t.Parallel()
	stub := quoteExistsExecStub(nil)
	router := quoteTagRouter(&handler.QuoteTagHandler{Q: dbq.New(stub)})
	rec := postJSON(t, router, "/quotes/00000000-0000-0000-0000-000000000001/tags", map[string]any{})
	assertStatus(t, rec, http.StatusBadRequest)
	assertErrorMsg(t, rec, "tag_id is required")
}

func TestQuoteTagAddTag_FKViolation(t *testing.T) {
	t.Parallel()
	fkErr := &pgconn.PgError{Code: "23503", Message: "foreign key violation"}
	stub := quoteExistsExecStub(func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
		return pgconn.CommandTag{}, fkErr
	})
	router := quoteTagRouter(&handler.QuoteTagHandler{Q: dbq.New(stub)})

	rec := postJSON(t, router, "/quotes/00000000-0000-0000-0000-000000000001/tags", map[string]any{
		"tag_id": "00000000-0000-0000-0000-000000000099",
	})
	assertStatus(t, rec, http.StatusUnprocessableEntity)
	assertErrorMsg(t, rec, "referenced tag does not exist")
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

// execDBTX supports controllable Exec and QueryRow implementations.
type execDBTX struct {
	execFn     func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	queryRowFn func(ctx context.Context, sql string, args ...any) pgx.Row
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

func (s *execDBTX) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if s.queryRowFn != nil {
		return s.queryRowFn(ctx, sql, args...)
	}
	return errRow{err: pgx.ErrNoRows}
}
