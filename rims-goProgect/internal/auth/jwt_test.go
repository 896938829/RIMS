package auth

import "testing"

func TestTokenServiceGenerateAndParse(t *testing.T) {
	svc := NewTokenService("unit-test-secret", 1)
	token, err := svc.GenerateToken(42, "tester")
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
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
}

func TestTokenServiceParseRejectsInvalidToken(t *testing.T) {
	svc := NewTokenService("unit-test-secret", 1)
	if _, err := svc.ParseToken("invalid.token.value"); err == nil {
		t.Fatalf("expected parse error for invalid token")
	}
}
