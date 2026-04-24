package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
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
	//   ListQuotes:  [limit, offset, FilterAuthorID, FilterCategoryID, SearchTitle, FilterTagID, SearchQ]
	//   CountQuotes: [FilterAuthorID, FilterCategoryID, SearchTitle, FilterTagID, SearchQ]
	// SearchQ was appended in the FTS migration (queries/quotes.sql, migration
	// 000007) — preserving FilterTagID's position was deliberate so this pin
	// did not need to move.
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

// textArgAt extracts a pgtype.Text at positional args[idx], surfacing type
// drift and Valid-flag drift with a clear failure message. Mirror of
// uuidArgAt for the text-valued full-text-search parameter.
func textArgAt(t *testing.T, queryName string, args []any, idx int) pgtype.Text {
	t.Helper()
	if idx >= len(args) {
		t.Fatalf("%s args=%+v has no index %d", queryName, args, idx)
	}
	pg, ok := args[idx].(pgtype.Text)
	if !ok {
		t.Fatalf("%s args[%d] = %T (%v), want pgtype.Text", queryName, idx, args[idx], args[idx])
	}
	return pg
}

// TestQuoteList_SearchQIsThreadedToBothQueries pins a regression that the
// `?q=` query parameter has to be threaded into BOTH ListQuotes and
// CountQuotes. Same shape as the ?tag_id= test above: without this, items and
// total silently drift under a search and pagination breaks for the searching
// user. We assert that (a) both code paths receive a valid pgtype.Text, (b)
// they carry the same bytes, and (c) the bytes are what the URL sent after
// Go's net/http URL-decoded them.
func TestQuoteList_SearchQIsThreadedToBothQueries(t *testing.T) {
	t.Parallel()

	// Include a quoted phrase and a negation operator so the test exercises
	// the websearch_to_tsquery syntax Logos supports. The handler is a pure
	// pass-through — no parsing, no validation — so whatever the URL carries
	// must show up byte-for-byte as the SearchQ parameter value.
	const wantQ = `"know thyself" -fortune`

	rec := &recordingDBTX{}
	router := quoteRouter(&handler.QuoteHandler{Q: dbq.New(rec)})

	resp := getRequest(t, router, "/quotes?q="+url.QueryEscape(wantQ))
	assertStatus(t, resp, http.StatusOK)

	if len(rec.queryCalls) != 1 {
		t.Fatalf("Query calls = %d, want 1; calls=%+v", len(rec.queryCalls), rec.queryCalls)
	}
	if len(rec.queryRowCalls) != 1 {
		t.Fatalf("QueryRow calls = %d, want 1; calls=%+v", len(rec.queryRowCalls), rec.queryRowCalls)
	}

	if !strings.Contains(rec.queryCalls[0].sql, "name: ListQuotes") {
		t.Fatalf("Query SQL = %q, want ListQuotes", rec.queryCalls[0].sql)
	}
	if !strings.Contains(rec.queryRowCalls[0].sql, "name: CountQuotes") {
		t.Fatalf("QueryRow SQL = %q, want CountQuotes", rec.queryRowCalls[0].sql)
	}

	// Argument layout, set by sqlc and asserted here so a sqlc regeneration
	// that reorders params fails loudly:
	//   ListQuotes:  [limit, offset, FilterAuthorID, FilterCategoryID, SearchTitle, FilterTagID, SearchQ]
	//   CountQuotes: [FilterAuthorID, FilterCategoryID, SearchTitle, FilterTagID, SearchQ]
	listQArg := textArgAt(t, "ListQuotes", rec.queryCalls[0].args, 6)
	countQArg := textArgAt(t, "CountQuotes", rec.queryRowCalls[0].args, 4)

	if !listQArg.Valid {
		t.Fatalf("ListQuotes SearchQ = invalid, want valid string")
	}
	if !countQArg.Valid {
		t.Fatalf("CountQuotes SearchQ = invalid, want valid string")
	}
	if listQArg.String != countQArg.String {
		t.Fatalf("SearchQ drift: List=%q, Count=%q", listQArg.String, countQArg.String)
	}
	if listQArg.String != wantQ {
		t.Fatalf("SearchQ = %q, want %q", listQArg.String, wantQ)
	}
}

// TestQuoteList_QIgnoresDualSentTitle pins that when both ?q= and ?title= are
// present, SearchTitle is forced to NULL so LogosUI can dual-send the same
// string for pre-FTS API pods without AND-ing ILIKE(title) onto @@ tsvector.
func TestQuoteList_QIgnoresDualSentTitle(t *testing.T) {
	t.Parallel()

	const wantQ = "virtue"

	rec := &recordingDBTX{}
	router := quoteRouter(&handler.QuoteHandler{Q: dbq.New(rec)})

	resp := getRequest(t, router, "/quotes?q="+url.QueryEscape(wantQ)+"&title="+url.QueryEscape(wantQ))
	assertStatus(t, resp, http.StatusOK)

	if len(rec.queryCalls) != 1 || len(rec.queryRowCalls) != 1 {
		t.Fatalf("calls: Query=%d QueryRow=%d, want 1 each", len(rec.queryCalls), len(rec.queryRowCalls))
	}

	listTitleArg := textArgAt(t, "ListQuotes", rec.queryCalls[0].args, 4)
	countTitleArg := textArgAt(t, "CountQuotes", rec.queryRowCalls[0].args, 2)
	if listTitleArg.Valid {
		t.Fatalf("ListQuotes SearchTitle = %+v, want invalid when ?q= is set", listTitleArg)
	}
	if countTitleArg.Valid {
		t.Fatalf("CountQuotes SearchTitle = %+v, want invalid when ?q= is set", countTitleArg)
	}

	listQArg := textArgAt(t, "ListQuotes", rec.queryCalls[0].args, 6)
	if !listQArg.Valid || listQArg.String != wantQ {
		t.Fatalf("ListQuotes SearchQ = %+v, want {%q, true}", listQArg, wantQ)
	}
}

// TestQuoteList_EmptyQIsTreatedAsAbsent pins the empty-string => NULL contract
// for ?q=. This matches the ?title= convention and is the hinge that lets the
// ORDER BY CASE in queries/quotes.sql short-circuit to (created_at, id) when
// the user has not typed anything. If a future refactor maps "" to a Valid
// pgtype.Text, the ORDER BY rank expression starts firing against an empty
// tsquery and the plan changes (and ranks degenerate to 0 across the board,
// silently killing the historic newest-first ordering for the unfiltered
// list).
func TestQuoteList_EmptyQIsTreatedAsAbsent(t *testing.T) {
	t.Parallel()

	rec := &recordingDBTX{}
	router := quoteRouter(&handler.QuoteHandler{Q: dbq.New(rec)})

	resp := getRequest(t, router, "/quotes?q=")
	assertStatus(t, resp, http.StatusOK)

	if len(rec.queryCalls) != 1 || len(rec.queryRowCalls) != 1 {
		t.Fatalf("calls: Query=%d QueryRow=%d, want 1 each", len(rec.queryCalls), len(rec.queryRowCalls))
	}

	listQArg := textArgAt(t, "ListQuotes", rec.queryCalls[0].args, 6)
	countQArg := textArgAt(t, "CountQuotes", rec.queryRowCalls[0].args, 4)

	if listQArg.Valid {
		t.Fatalf("ListQuotes SearchQ = %+v, want invalid (empty ?q= -> NULL)", listQArg)
	}
	if countQArg.Valid {
		t.Fatalf("CountQuotes SearchQ = %+v, want invalid (empty ?q= -> NULL)", countQArg)
	}
}

// TestQuoteList_QAbsentWhenParamMissing pins that a request with no ?q= at
// all produces the same NULL-valued SearchQ as an explicit ?q=. Without this
// pin the two could diverge (e.g. missing-param producing an absent arg and
// empty-string producing a valid empty string) and only one would exercise
// the WHERE short-circuit in the SQL.
func TestQuoteList_QAbsentWhenParamMissing(t *testing.T) {
	t.Parallel()

	rec := &recordingDBTX{}
	router := quoteRouter(&handler.QuoteHandler{Q: dbq.New(rec)})

	resp := getRequest(t, router, "/quotes")
	assertStatus(t, resp, http.StatusOK)

	if len(rec.queryCalls) != 1 || len(rec.queryRowCalls) != 1 {
		t.Fatalf("calls: Query=%d QueryRow=%d, want 1 each", len(rec.queryCalls), len(rec.queryRowCalls))
	}

	if listQArg := textArgAt(t, "ListQuotes", rec.queryCalls[0].args, 6); listQArg.Valid {
		t.Fatalf("ListQuotes SearchQ = %+v, want invalid (no ?q= -> NULL)", listQArg)
	}
	if countQArg := textArgAt(t, "CountQuotes", rec.queryRowCalls[0].args, 4); countQArg.Valid {
		t.Fatalf("CountQuotes SearchQ = %+v, want invalid (no ?q= -> NULL)", countQArg)
	}
}

// TestQuoteList_QComposesWithFacets pins that ?q= does not swallow or replace
// the other filter facets: all four (?author_id, ?category_id, ?tag_id, ?q)
// must be threaded on the same request. Without this test, a refactor that
// early-returns on ?q= could silently drop the facet filters and search
// across the whole corpus instead of the facet-restricted subset the UI
// asked for.
func TestQuoteList_QComposesWithFacets(t *testing.T) {
	t.Parallel()

	const (
		authorID = "11111111-1111-1111-1111-111111111111"
		catID    = "22222222-2222-2222-2222-222222222222"
		tagID    = "33333333-3333-3333-3333-333333333333"
		q        = "virtue"
	)

	rec := &recordingDBTX{}
	router := quoteRouter(&handler.QuoteHandler{Q: dbq.New(rec)})

	resp := getRequest(t, router,
		"/quotes?author_id="+authorID+
			"&category_id="+catID+
			"&tag_id="+tagID+
			"&q="+url.QueryEscape(q))
	assertStatus(t, resp, http.StatusOK)

	if len(rec.queryCalls) != 1 || len(rec.queryRowCalls) != 1 {
		t.Fatalf("calls: Query=%d QueryRow=%d, want 1 each", len(rec.queryCalls), len(rec.queryRowCalls))
	}

	// ListQuotes positions: [limit, offset, AuthorID, CategoryID, SearchTitle, TagID, SearchQ]
	gotAuthor := uuidArgAt(t, "ListQuotes", rec.queryCalls[0].args, 2)
	gotCat := uuidArgAt(t, "ListQuotes", rec.queryCalls[0].args, 3)
	gotTag := uuidArgAt(t, "ListQuotes", rec.queryCalls[0].args, 5)
	gotQ := textArgAt(t, "ListQuotes", rec.queryCalls[0].args, 6)

	if gotAuthor.String() != authorID {
		t.Fatalf("FilterAuthorID = %s, want %s", gotAuthor, authorID)
	}
	if gotCat.String() != catID {
		t.Fatalf("FilterCategoryID = %s, want %s", gotCat, catID)
	}
	if gotTag.String() != tagID {
		t.Fatalf("FilterTagID = %s, want %s", gotTag, tagID)
	}
	if !gotQ.Valid || gotQ.String != q {
		t.Fatalf("SearchQ = %+v, want {%q, true}", gotQ, q)
	}
}
