// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package auth

import "testing"

func TestTokenServiceGenerateAndParse(t *testing.T) {
	svc := NewTokenService("unit-test-secret", 1)
	token, expiresAt, err := svc.GenerateToken(42, "tester", 1, "admin")
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}
	if expiresAt == 0 {
		t.Fatalf("expiresAt should be non-zero")
	}

	claims, err := svc.ParseToken(token)
	if err != nil {
		t.Fatalf("ParseToken() error = %v", err)
	}

	if claims.UserID != 42 {
		t.Fatalf("UserID = %d, want %d", claims.UserID, 42)
	}
	if claims.Username != "tester" {
		t.Fatalf("Username = %q, want %q", claims.Username, "tester")
	}
	if claims.RoleID != 1 {
		t.Fatalf("RoleID = %d, want %d", claims.RoleID, 1)
	}
	if claims.RoleCode != "admin" {
		t.Fatalf("RoleCode = %q, want %q", claims.RoleCode, "admin")
	}
}

func TestTokenServiceParseRejectsInvalidToken(t *testing.T) {
	svc := NewTokenService("unit-test-secret", 1)
	if _, err := svc.ParseToken("invalid.token.value"); err == nil {
		t.Fatalf("expected parse error for invalid token")
	}
}
