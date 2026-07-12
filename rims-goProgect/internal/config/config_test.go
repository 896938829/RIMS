package config

import (
	"strings"
	"testing"
)

func TestLoadRejectsNonPositiveMaxAttachmentsPerObject(t *testing.T) {
	t.Setenv("DB_PASSWORD", "test-password")
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("MAX_ATTACHMENTS_PER_OBJECT", "0")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "MAX_ATTACHMENTS_PER_OBJECT must be > 0") {
		t.Fatalf("Load() error = %v, want MAX_ATTACHMENTS_PER_OBJECT validation", err)
	}
}
