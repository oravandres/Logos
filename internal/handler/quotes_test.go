package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
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
		{
			name:      "invalid tag_id",
			query:     "?tag_id=not-a-uuid",
			wantCode:  http.StatusBadRequest,
			wantError: "invalid tag_id",
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

// recordedCall captures the raw args sqlc passed to the database driver for a
// single Query or QueryRow invocation.
type recordedCall struct {
	sql  string
	args []any
}

// recordingDBTX is a dbq.DBTX that captures the SQL text and the positional
// args of every Query / QueryRow call without actually executing anything.
// It returns empty pgx.Rows from Query (so generated *.sql.go for :many
// queries returns an empty slice) and a Scan-target-zeroing row from QueryRow
// (so :one queries that count return 0).
type recordingDBTX struct {
	queryCalls    []recordedCall
	queryRowCalls []recordedCall
}

func (s *recordingDBTX) Exec(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (s *recordingDBTX) Query(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
	s.queryCalls = append(s.queryCalls, recordedCall{sql: sql, args: args})
	return emptyRows{}, nil
}

func (s *recordingDBTX) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	s.queryRowCalls = append(s.queryRowCalls, recordedCall{sql: sql, args: args})
	return scanZeroRow{}
}

// emptyRows is a minimal pgx.Rows that yields no rows. The embedded interface
// is intentionally nil — the generated ListQuotes loop only invokes
// Close/Next/Err, so any other method call would surface as a clear test panic.
type emptyRows struct{ pgx.Rows }

func (emptyRows) Close()                                       {}
func (emptyRows) Err() error                                   { return nil }
func (emptyRows) Next() bool                                   { return false }
func (emptyRows) Scan(_ ...any) error                          { return nil }
func (emptyRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (emptyRows) FieldDescriptions() []pgconn.FieldDescription { return nil }

// scanZeroRow satisfies pgx.Row by writing 0 to a single *int64 destination,
// which is what sqlc-generated COUNT queries scan into.
type scanZeroRow struct{}

func (scanZeroRow) Scan(dest ...any) error {
	for _, d := range dest {
		if p, ok := d.(*int64); ok {
			*p = 0
		}
	}
	return nil
}

// TestQuoteList_TagFilterIsThreadedToBothQueries pins a regression that the
// `?tag_id=` query parameter has to be threaded into BOTH ListQuotes and
// CountQuotes. If a future refactor drops it from one but keeps it on the
// other, items and total drift silently and pagination breaks. We assert here
// that (a) both code paths receive a parsed UUID, (b) it's the same UUID, and
// (c) it's the one we sent on the wire.
func TestQuoteList_TagFilterIsThreadedToBothQueries(t *testing.T) {
	t.Parallel()

	const tagIDStr = "11111111-2222-3333-4444-555555555555"
	wantUUID, err := uuid.Parse(tagIDStr)
	if err != nil {
		t.Fatalf("parse tag uuid: %v", err)
	}

	rec := &recordingDBTX{}
	router := quoteRouter(&handler.QuoteHandler{Q: dbq.New(rec)})

	resp := getRequest(t, router, "/quotes?tag_id="+tagIDStr)
	assertStatus(t, resp, http.StatusOK)

	if len(rec.queryCalls) != 1 {
		t.Fatalf("Query calls = %d, want 1; calls=%+v", len(rec.queryCalls), rec.queryCalls)
	}
	if len(rec.queryRowCalls) != 1 {
		t.Fatalf("QueryRow calls = %d, want 1; calls=%+v", len(rec.queryRowCalls), rec.queryRowCalls)
	}

	// Sanity-check we recorded the right SQL on each side; without this it
	// would be possible for an unrelated regression to swap routes and still
	// satisfy the per-arg assertions below.
	if !strings.Contains(rec.queryCalls[0].sql, "name: ListQuotes") {
		t.Fatalf("Query SQL = %q, want ListQuotes", rec.queryCalls[0].sql)
	}
	if !strings.Contains(rec.queryRowCalls[0].sql, "name: CountQuotes") {
		t.Fatalf("QueryRow SQL = %q, want CountQuotes", rec.queryRowCalls[0].sql)
	}

	// Argument layout, set by sqlc and asserted here so a sqlc regeneration
	// that reorders params fails loudly:
	//   ListQuotes:  [limit, offset, FilterAuthorID, FilterCategoryID, SearchTitle, FilterTagID]
	//   CountQuotes: [FilterAuthorID, FilterCategoryID, SearchTitle, FilterTagID]
	listTagArg := uuidArgAt(t, "ListQuotes", rec.queryCalls[0].args, 5)
	countTagArg := uuidArgAt(t, "CountQuotes", rec.queryRowCalls[0].args, 3)

	if listTagArg != countTagArg {
		t.Fatalf("FilterTagID drift: List=%s, Count=%s", listTagArg, countTagArg)
	}
	if listTagArg != wantUUID {
		t.Fatalf("FilterTagID = %s, want %s", listTagArg, wantUUID)
	}

	var page struct {
		Items  []any `json:"items"`
		Total  int64 `json:"total"`
		Limit  int32 `json:"limit"`
		Offset int32 `json:"offset"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode response: %v (raw=%s)", err, resp.Body.String())
	}
	if len(page.Items) != 0 || page.Total != 0 {
		t.Fatalf("expected empty result for stubbed DB, got items=%d total=%d", len(page.Items), page.Total)
	}
}

func uuidArgAt(t *testing.T, queryName string, args []any, idx int) uuid.UUID {
	t.Helper()
	if idx >= len(args) {
		t.Fatalf("%s args=%+v has no index %d", queryName, args, idx)
	}
	pg, ok := args[idx].(pgtype.UUID)
	if !ok {
		t.Fatalf("%s args[%d] = %T (%v), want pgtype.UUID", queryName, idx, args[idx], args[idx])
	}
	if !pg.Valid {
		t.Fatalf("%s args[%d] = invalid pgtype.UUID, want valid", queryName, idx)
	}
	return uuid.UUID(pg.Bytes)
}
