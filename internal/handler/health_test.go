package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type stubPinger struct {
	pingErr error
}

func (s stubPinger) Ping(context.Context) error {
	return s.pingErr
}

func decodeStatus(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v (raw=%q)", err, rec.Body.String())
	}
	return body.Status
}

func TestHealthHandlerLive(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/livez", nil)
	rec := httptest.NewRecorder()

	(&HealthHandler{}).Live(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := decodeStatus(t, rec); got != "ok" {
		t.Fatalf("status field = %q, want %q", got, "ok")
	}
}

func TestHealthHandlerReady(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		handler    HealthHandler
		wantCode   int
		wantStatus string
	}{
		{
			name:       "ready",
			handler:    HealthHandler{Pinger: stubPinger{}},
			wantCode:   http.StatusOK,
			wantStatus: "ready",
		},
		{
			name:       "dependency failure",
			handler:    HealthHandler{Pinger: stubPinger{pingErr: errors.New("db unavailable")}},
			wantCode:   http.StatusServiceUnavailable,
			wantStatus: "unready",
		},
		{
			name:       "missing pinger",
			handler:    HealthHandler{},
			wantCode:   http.StatusServiceUnavailable,
			wantStatus: "unready",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/readyz", nil)
			rec := httptest.NewRecorder()

			tt.handler.Ready(rec, req)

			if rec.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantCode)
			}
			if got := decodeStatus(t, rec); got != tt.wantStatus {
				t.Fatalf("status field = %q, want %q", got, tt.wantStatus)
			}
		})
	}
}

// TestHealthHandlerCompat locks the legacy /api/v1/health body shape
// ("healthy" / "unhealthy") so consumers that depend on those exact strings
// (LogosUI dashboard, downstream monitoring) keep working when /readyz starts
// emitting the new "ready" / "unready" labels.
func TestHealthHandlerCompat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		handler    HealthHandler
		wantCode   int
		wantStatus string
	}{
		{
			name:       "healthy",
			handler:    HealthHandler{Pinger: stubPinger{}},
			wantCode:   http.StatusOK,
			wantStatus: "healthy",
		},
		{
			name:       "dependency failure",
			handler:    HealthHandler{Pinger: stubPinger{pingErr: errors.New("db unavailable")}},
			wantCode:   http.StatusServiceUnavailable,
			wantStatus: "unhealthy",
		},
		{
			name:       "missing pinger",
			handler:    HealthHandler{},
			wantCode:   http.StatusServiceUnavailable,
			wantStatus: "unhealthy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/health", nil)
			rec := httptest.NewRecorder()

			tt.handler.Compat(rec, req)

			if rec.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantCode)
			}
			if got := decodeStatus(t, rec); got != tt.wantStatus {
				t.Fatalf("status field = %q, want %q", got, tt.wantStatus)
			}
		})
	}
}
