// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package deploy_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to locate test file")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

func TestDockerComposeInitializesAllMigrations(t *testing.T) {
	root := repoRoot(t)
	composePath := filepath.Join(root, "deploy", "docker-compose.yml")
	composeBytes, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("read docker-compose.yml: %v", err)
	}

	compose := strings.ReplaceAll(string(composeBytes), "\\", "/")
	expectedMount := "./rims-goProgect/migrations:/docker-entrypoint-initdb.d:ro"
	if !strings.Contains(compose, expectedMount) {
		t.Fatalf("expected compose to mount the full migrations directory %q", expectedMount)
	}
	if strings.Contains(compose, "/migrations/000001_init.sql:") {
		t.Fatalf("compose must not mount only 000001_init.sql; later migrations would be skipped")
	}

	migrationDir := filepath.Join(root, "rims-goProgect", "migrations")
	entries, err := os.ReadDir(migrationDir)
	if err != nil {
		t.Fatalf("read migrations directory: %v", err)
	}
	var sqlCount int
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			sqlCount++
		}
	}
	if sqlCount < 2 {
		t.Fatalf("expected multiple SQL migrations to be initialized, got %d", sqlCount)
	}
}
