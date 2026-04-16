package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeHandler(t *testing.T) {
	t.Parallel()

	type requestBody struct {
		Name string `json:"name"`
	}

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req requestBody
		if err := decode(w, r, &req); err != nil {
			writeDecodeError(w, err)
			return
		}
		respondJSON(w, http.StatusOK, req)
	})

	tests := []struct {
		name        string
		contentType string
		body        string
		wantCode    int
		wantError   string
	}{
		{
			name:        "valid json",
			contentType: "application/json",
			body:        `{"name":"logos"}`,
			wantCode:    http.StatusOK,
		},
		{
			name:        "unsupported media type",
			contentType: "text/plain",
			body:        `{"name":"logos"}`,
			wantCode:    http.StatusUnsupportedMediaType,
			wantError:   "content type must be application/json",
		},
		{
			name:        "empty body",
			contentType: "application/json",
			body:        "",
			wantCode:    http.StatusBadRequest,
			wantError:   "request body is empty",
		},
		{
			name:        "multiple json values",
			contentType: "application/json",
			body:        `{"name":"logos"}{"name":"extra"}`,
			wantCode:    http.StatusBadRequest,
			wantError:   "request body must contain a single JSON object",
		},
		{
			name:        "body too large",
			contentType: "application/json",
			body:        `{"name":"` + strings.Repeat("a", maxRequestBodyBytes) + `"}`,
			wantCode:    http.StatusRequestEntityTooLarge,
			wantError:   "request body must be at most 1048576 bytes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/test", strings.NewReader(tt.body))
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}
			rec := httptest.NewRecorder()

			testHandler.ServeHTTP(rec, req)

			if rec.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tt.wantCode, rec.Body.String())
			}

			if tt.wantError == "" {
				return
			}

			var body struct {
				Error string `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}
			if body.Error != tt.wantError {
				t.Fatalf("error = %q, want %q", body.Error, tt.wantError)
			}
		})
	}
}
