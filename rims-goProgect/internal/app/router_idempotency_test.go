// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package app

import (
	"testing"
	"time"

	"gorm.io/gorm"

	"rims-go/internal/config"
	"rims-go/internal/idempotency"
)

func TestIdempotencyTTLFromConfigUsesConfiguredHours(t *testing.T) {
	cfg := config.Config{IdempotencyKeyTTLHours: 6}

	got := idempotencyTTLFromConfig(cfg)

	if got != 6*time.Hour {
		t.Fatalf("idempotency TTL = %s, want 6h from config", got)
	}
}

func TestBuildRouterMutationRoutesMatchIdempotencyRegistry(t *testing.T) {
	router := buildRouter(config.Config{UploadDir: t.TempDir()}, &gorm.DB{})
	actual := make(map[string]bool)
	for _, route := range router.Routes() {
		actual[route.Method+" "+route.Path] = true
	}

	registered := idempotency.RegisteredMutationRoutes()
	if len(registered) != 5 {
		t.Fatalf("registered idempotent mutation routes = %d, want 5", len(registered))
	}
	for _, route := range registered {
		if !actual[route.Scope()] {
			t.Fatalf("registered idempotent mutation route %q is not mounted", route.Scope())
		}
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
