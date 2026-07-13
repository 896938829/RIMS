// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package app

import (
	"testing"
	"time"

	"gorm.io/gorm"

	"rims-go/internal/config"
)

func TestIdempotencyTTLFromConfigUsesConfiguredHours(t *testing.T) {
	cfg := config.Config{IdempotencyKeyTTLHours: 6}

	got := idempotencyTTLFromConfig(cfg)

	if got != 6*time.Hour {
		t.Fatalf("idempotency TTL = %s, want 6h from config", got)
	}
}

func TestBuildRouterRegistersIdempotencyStatusRoute(t *testing.T) {
	router := buildRouter(config.Config{UploadDir: t.TempDir()}, &gorm.DB{})

	for _, route := range router.Routes() {
		if route.Method == "GET" && route.Path == "/api/v1/operations/idempotency/:key" {
			return
		}
	}

	t.Fatal("GET /api/v1/operations/idempotency/:key route is not registered")
}
