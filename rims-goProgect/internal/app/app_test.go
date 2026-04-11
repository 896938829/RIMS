// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

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
		CORSOrigins:    "*",
		ReadTimeout:    30,
		WriteTimeout:   30,
		UploadDir:      t.TempDir(),
		MaxUploadMB:    10,
		AllowedExts:    ".jpg,.png,.pdf",
	}
	r := buildRouter(cfg, nil)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}
