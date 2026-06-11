// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package middleware

import (
	"crypto/rand"
	"errors"
	"io"
	"strings"
	"testing"
)

type failingReader struct{}

func (failingReader) Read(p []byte) (int, error) {
	return 0, errors.New("random reader failed")
}

func TestGenerateIDFallsBackWhenRandomReaderFails(t *testing.T) {
	originalReader := rand.Reader
	rand.Reader = failingReader{}
	t.Cleanup(func() {
		rand.Reader = originalReader
	})

	id := generateID()

	if id == "" {
		t.Fatal("expected fallback ID to be non-empty")
	}
	if strings.Trim(id, "0") == "" {
		t.Fatalf("expected fallback ID to be non-zero, got %q", id)
	}
}

var _ io.Reader = failingReader{}
