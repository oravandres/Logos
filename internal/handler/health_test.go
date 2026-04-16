package handler

import (
	"context"
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

func TestHealthHandlerLive(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/livez", nil)
	rec := httptest.NewRecorder()

	(&HealthHandler{}).Live(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestHealthHandlerReady(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		handler  HealthHandler
		wantCode int
	}{
		{
			name:     "ready",
			handler:  HealthHandler{Pinger: stubPinger{}},
			wantCode: http.StatusOK,
		},
		{
			name:     "dependency failure",
			handler:  HealthHandler{Pinger: stubPinger{pingErr: errors.New("db unavailable")}},
			wantCode: http.StatusServiceUnavailable,
		},
		{
			name:     "missing pinger",
			handler:  HealthHandler{},
			wantCode: http.StatusServiceUnavailable,
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
		})
	}
}
