package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/oravandres/Logos/internal/database/dbq"
	"github.com/oravandres/Logos/internal/handler"
)

// stubDBTX is a minimal DBTX implementation for testing.
// queryRowFn controls the response for every QueryRow call.
type stubDBTX struct {
	queryRowFn func(ctx context.Context, sql string, args ...any) pgx.Row
}

func (s *stubDBTX) Exec(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (s *stubDBTX) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	return nil, nil
}

func (s *stubDBTX) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if s.queryRowFn != nil {
		return s.queryRowFn(ctx, sql, args...)
	}
	return errRow{err: pgx.ErrNoRows}
}

// errRow returns a fixed error from Scan.
type errRow struct{ err error }

func (r errRow) Scan(_ ...any) error { return r.err }

// dummyDeleteRow simulates a successful DELETE ... RETURNING scan in handler tests.
type dummyDeleteRow struct{}

func (dummyDeleteRow) Scan(_ ...any) error { return nil }

func authorRouter(h *handler.AuthorHandler) *chi.Mux {
	r := chi.NewRouter()
	r.Post("/authors", h.Create)
	r.Get("/authors/{id}", h.Get)
	r.Put("/authors/{id}", h.Update)
	return r
}

func postJSON(t *testing.T, router http.Handler, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func putJSON(t *testing.T, router http.Handler, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPut, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func getRequest(t *testing.T, router http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func deleteRequest(t *testing.T, router http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, path, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func assertStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Errorf("status = %d, want %d; body: %s", rec.Code, want, rec.Body.String())
	}
}

func assertErrorMsg(t *testing.T, rec *httptest.ResponseRecorder, want string) {
	t.Helper()
	var resp struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Error != want {
		t.Errorf("error = %q, want %q", resp.Error, want)
	}
}

// nilQ returns a Queries backed by a nil-ish DBTX that panics on DB access.
// Safe for tests that only exercise pre-DB validation paths.
func nilQ() *dbq.Queries { return dbq.New(&stubDBTX{}) }

func TestAuthorCreate_Validation(t *testing.T) {
	t.Parallel()
	router := authorRouter(&handler.AuthorHandler{Q: nilQ()})

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
			name:      "invalid born_date",
			body:      map[string]any{"name": "Socrates", "born_date": "not-a-date"},
			wantCode:  http.StatusBadRequest,
			wantError: "born_date must be YYYY-MM-DD",
		},
		{
			name:      "invalid died_date",
			body:      map[string]any{"name": "Socrates", "died_date": "12/31/1999"},
			wantCode:  http.StatusBadRequest,
			wantError: "died_date must be YYYY-MM-DD",
		},
		{
			name: "died_date before born_date",
			body: map[string]any{
				"name":      "Socrates",
				"born_date": "0470-01-01",
				"died_date": "0469-01-01",
			},
			wantCode:  http.StatusBadRequest,
			wantError: "died_date must not be earlier than born_date",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec := postJSON(t, router, "/authors", tt.body)
			assertStatus(t, rec, tt.wantCode)
			assertErrorMsg(t, rec, tt.wantError)
		})
	}
}

func TestAuthorUpdate_Validation(t *testing.T) {
	t.Parallel()
	router := authorRouter(&handler.AuthorHandler{Q: nilQ()})

	validUUID := "00000000-0000-0000-0000-000000000001"

	tests := []struct {
		name      string
		path      string
		body      map[string]any
		wantCode  int
		wantError string
	}{
		{
			name:      "invalid UUID",
			path:      "/authors/not-a-uuid",
			body:      map[string]any{"name": "X"},
			wantCode:  http.StatusBadRequest,
			wantError: "invalid UUID",
		},
		{
			name:      "missing name",
			path:      "/authors/" + validUUID,
			body:      map[string]any{},
			wantCode:  http.StatusBadRequest,
			wantError: "name is required",
		},
		{
			name: "died_date before born_date",
			path: "/authors/" + validUUID,
			body: map[string]any{
				"name":      "Plato",
				"born_date": "0428-01-01",
				"died_date": "0427-01-01",
			},
			wantCode:  http.StatusBadRequest,
			wantError: "died_date must not be earlier than born_date",
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

func TestAuthorGet_InvalidUUID(t *testing.T) {
	t.Parallel()
	router := authorRouter(&handler.AuthorHandler{Q: nilQ()})
	rec := getRequest(t, router, "/authors/not-a-uuid")
	assertStatus(t, rec, http.StatusBadRequest)
	assertErrorMsg(t, rec, "invalid UUID")
}

func TestAuthorCreate_FKViolation(t *testing.T) {
	t.Parallel()
	fkErr := &pgconn.PgError{Code: "23503", Message: "insert violates foreign key constraint"}
	stub := &stubDBTX{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return errRow{err: fkErr}
		},
	}
	router := authorRouter(&handler.AuthorHandler{Q: dbq.New(stub)})

	rec := postJSON(t, router, "/authors", map[string]any{
		"name":     "Socrates",
		"image_id": "00000000-0000-0000-0000-000000000099",
	})
	assertStatus(t, rec, http.StatusUnprocessableEntity)
	assertErrorMsg(t, rec, "referenced image or category does not exist")
}

func TestAuthorCreate_CategoryTypeMismatch(t *testing.T) {
	t.Parallel()
	// GetCategory is the first QueryRow call when category_id is provided.
	// Return a category with type "image" instead of the required "author".
	stub := &stubDBTX{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &scanCategoryRow{name: "portraits", catType: "image"}
		},
	}
	router := authorRouter(&handler.AuthorHandler{Q: dbq.New(stub)})

	rec := postJSON(t, router, "/authors", map[string]any{
		"name":        "Socrates",
		"category_id": "00000000-0000-0000-0000-000000000001",
	})
	assertStatus(t, rec, http.StatusUnprocessableEntity)
	assertErrorMsg(t, rec, `category type must be "author"`)
}

func TestAuthorCreate_CheckViolation(t *testing.T) {
	t.Parallel()
	// Simulates the trigger catching a category type mismatch that slipped
	// past the handler pre-check (race condition). The first QueryRow call
	// is GetCategory (pre-check passes with correct type), the second is
	// CreateAuthor which fails with a check_violation from the trigger.
	callCount := 0
	stub := &stubDBTX{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			callCount++
			if callCount == 1 {
				return &scanCategoryRow{name: "philosopher", catType: "author"}
			}
			return errRow{err: &pgconn.PgError{Code: "23514", ConstraintName: "chk_authors_category_type"}}
		},
	}
	router := authorRouter(&handler.AuthorHandler{Q: dbq.New(stub)})

	rec := postJSON(t, router, "/authors", map[string]any{
		"name":        "Socrates",
		"category_id": "00000000-0000-0000-0000-000000000001",
	})
	assertStatus(t, rec, http.StatusUnprocessableEntity)
	assertErrorMsg(t, rec, `category type must be "author"`)
}

// scanCategoryRow populates a Category row on Scan, matching the GetCategory column order.
type scanCategoryRow struct {
	name    string
	catType string
}

func (r *scanCategoryRow) Scan(dest ...any) error {
	// GetCategory column order: id, name, type, created_at
	if len(dest) < 4 {
		return pgx.ErrNoRows
	}
	// name
	if p, ok := dest[1].(*string); ok {
		*p = r.name
	}
	// type
	if p, ok := dest[2].(*string); ok {
		*p = r.catType
	}
	return nil
}
