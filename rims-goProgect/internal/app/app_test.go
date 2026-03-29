package app

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"rims-go/internal/config"
)

func TestBuildRouterHealthz(t *testing.T) {
	cfg := config.Config{
		JWTSecret:      "test-secret",
		JWTExpireHours: 1,
		DemoUser:       "admin",
		DemoPassword:   "admin123",
	}
	r := buildRouter(cfg, nil)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}
