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

// stubTx implements pgx.Tx backed by a DBTX stub.
type stubTx struct {
	pgx.Tx
	dbtx dbq.DBTX
}

func (s *stubTx) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return s.dbtx.Exec(ctx, sql, args...)
}

func (s *stubTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return s.dbtx.Query(ctx, sql, args...)
}

func (s *stubTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return s.dbtx.QueryRow(ctx, sql, args...)
}

func (s *stubTx) Commit(_ context.Context) error   { return nil }
func (s *stubTx) Rollback(_ context.Context) error { return nil }
func (s *stubTx) Conn() *pgx.Conn                  { return nil }

// stubTxBeginner implements handler.TxBeginner, returning a stubTx backed
// by the provided DBTX.
type stubTxBeginner struct {
	dbtx dbq.DBTX
}

func (b *stubTxBeginner) Begin(_ context.Context) (pgx.Tx, error) {
	return &stubTx{dbtx: b.dbtx}, nil
}

// quoteNotFoundHandler returns a QuoteTagHandler where GetQuoteForKeyShare
// returns pgx.ErrNoRows (quote does not exist).
func quoteNotFoundHandler() *handler.QuoteTagHandler {
	stub := &stubDBTX{}
	return &handler.QuoteTagHandler{
		Q:    dbq.New(stub),
		Pool: &stubTxBeginner{dbtx: stub},
	}
}

// quoteExistsHandler returns a QuoteTagHandler where GetQuoteForKeyShare
// succeeds but Exec can be overridden.
func quoteExistsHandler(execFn func(context.Context, string, ...any) (pgconn.CommandTag, error)) *handler.QuoteTagHandler {
	dbtx := &execDBTX{
		execFn: execFn,
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return scanQuoteRow{}
		},
	}
	return &handler.QuoteTagHandler{
		Q:    dbq.New(dbtx),
		Pool: &stubTxBeginner{dbtx: dbtx},
	}
}

// scanQuoteRow returns nil from Scan, simulating a found quote row.
type scanQuoteRow struct{}

func (scanQuoteRow) Scan(_ ...any) error { return nil }

func TestQuoteTagListTags_InvalidUUID(t *testing.T) {
	t.Parallel()
	router := quoteTagRouter(quoteNotFoundHandler())
	rec := getRequest(t, router, "/quotes/not-a-uuid/tags")
	assertStatus(t, rec, http.StatusBadRequest)
	assertErrorMsg(t, rec, "invalid UUID")
}

func TestQuoteTagListTags_QuoteNotFound(t *testing.T) {
	t.Parallel()
	router := quoteTagRouter(quoteNotFoundHandler())
	rec := getRequest(t, router, "/quotes/00000000-0000-0000-0000-000000000001/tags")
	assertStatus(t, rec, http.StatusNotFound)
	assertErrorMsg(t, rec, "quote not found")
}

func TestQuoteTagAddTag_InvalidQuoteUUID(t *testing.T) {
	t.Parallel()
	router := quoteTagRouter(quoteNotFoundHandler())
	rec := postJSON(t, router, "/quotes/not-a-uuid/tags", map[string]any{
		"tag_id": "00000000-0000-0000-0000-000000000001",
	})
	assertStatus(t, rec, http.StatusBadRequest)
	assertErrorMsg(t, rec, "invalid UUID")
}

func TestQuoteTagAddTag_QuoteNotFound(t *testing.T) {
	t.Parallel()
	router := quoteTagRouter(quoteNotFoundHandler())
	rec := postJSON(t, router, "/quotes/00000000-0000-0000-0000-000000000001/tags", map[string]any{
		"tag_id": "00000000-0000-0000-0000-000000000002",
	})
	assertStatus(t, rec, http.StatusNotFound)
	assertErrorMsg(t, rec, "quote not found")
}

func TestQuoteTagAddTag_MissingTagID(t *testing.T) {
	t.Parallel()
	router := quoteTagRouter(quoteExistsHandler(nil))
	rec := postJSON(t, router, "/quotes/00000000-0000-0000-0000-000000000001/tags", map[string]any{})
	assertStatus(t, rec, http.StatusBadRequest)
	assertErrorMsg(t, rec, "tag_id is required")
}

func TestQuoteTagAddTag_FKViolation(t *testing.T) {
	t.Parallel()
	fkErr := &pgconn.PgError{Code: "23503", Message: "foreign key violation"}
	h := quoteExistsHandler(func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
		return pgconn.CommandTag{}, fkErr
	})
	router := quoteTagRouter(h)

	rec := postJSON(t, router, "/quotes/00000000-0000-0000-0000-000000000001/tags", map[string]any{
		"tag_id": "00000000-0000-0000-0000-000000000099",
	})
	assertStatus(t, rec, http.StatusUnprocessableEntity)
	assertErrorMsg(t, rec, "referenced tag does not exist")
}

func TestQuoteTagRemoveTag_InvalidUUIDs(t *testing.T) {
	t.Parallel()
	router := quoteTagRouter(quoteNotFoundHandler())

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
	router := quoteTagRouter(quoteNotFoundHandler())
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
