// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package idempotency

import "testing"

func TestValidateKeyRejectsCompleteDotSegments(t *testing.T) {
	for _, key := range []string{".", ".."} {
		t.Run(key, func(t *testing.T) {
			if err := ValidateKey(key); err == nil {
				t.Fatalf("ValidateKey(%q) returned nil, want error", key)
			}
		})
	}
}

func TestValidateKeyAllowsDotsInsideLargerKeys(t *testing.T) {
	for _, key := range []string{".a", "a.."} {
		t.Run(key, func(t *testing.T) {
			if err := ValidateKey(key); err != nil {
				t.Fatalf("ValidateKey(%q) returned %v", key, err)
			}
		})
	}
}
