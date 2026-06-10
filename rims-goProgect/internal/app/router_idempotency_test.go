// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package app

import (
	"testing"
	"time"

	"rims-go/internal/config"
)

func TestIdempotencyTTLFromConfigUsesConfiguredHours(t *testing.T) {
	cfg := config.Config{IdempotencyKeyTTLHours: 6}

	got := idempotencyTTLFromConfig(cfg)

	if got != 6*time.Hour {
		t.Fatalf("idempotency TTL = %s, want 6h from config", got)
	}
}
