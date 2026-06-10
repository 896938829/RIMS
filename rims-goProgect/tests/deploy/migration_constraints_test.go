// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package deploy_test

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func constraintsRepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to locate test file")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

func TestInventoryConstraintMigrationIncludesRequiredChecks(t *testing.T) {
	root := constraintsRepoRoot(t)
	migrationPath := filepath.Join(root, "rims-goProgect", "migrations", "000008_inventory_constraints.sql")
	migrationBytes, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatalf("read inventory constraints migration: %v", err)
	}

	migration := normalizeSQL(string(migrationBytes))
	constraints := []struct {
		name       string
		table      string
		expression string
	}{
		{
			name:       "chk_inventories_quantity_non_negative",
			table:      "inventories",
			expression: "quantity >= 0",
		},
		{
			name:       "chk_inventories_locked_qty_non_negative",
			table:      "inventories",
			expression: "locked_qty >= 0",
		},
		{
			name:       "chk_inventories_alert_threshold_non_negative",
			table:      "inventories",
			expression: "alert_threshold >= 0",
		},
		{
			name:       "chk_non_std_inventories_quantity_non_negative",
			table:      "non_std_inventories",
			expression: "quantity >= 0",
		},
		{
			name:       "chk_non_std_inventories_converted_qty_non_negative",
			table:      "non_std_inventories",
			expression: "converted_qty >= 0",
		},
		{
			name:       "chk_non_std_inventories_converted_qty_lte_quantity",
			table:      "non_std_inventories",
			expression: "converted_qty <= quantity",
		},
	}

	for _, constraint := range constraints {
		assertConstraintGuarded(t, migration, constraint.table, constraint.name, constraint.expression)
	}
}

func TestInventoryConstraintMigrationAllowsNegativeDocumentLineQuantity(t *testing.T) {
	root := constraintsRepoRoot(t)
	migrationPath := filepath.Join(root, "rims-goProgect", "migrations", "000008_inventory_constraints.sql")
	migrationBytes, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatalf("read inventory constraints migration: %v", err)
	}

	migration := normalizeSQL(string(migrationBytes))
	forbiddenTokens := []string{
		"chk_document_lines_quantity_non_negative",
		"alter table document_lines validate constraint",
		"alter table document_lines add constraint",
	}
	for _, token := range forbiddenTokens {
		if strings.Contains(migration, token) {
			t.Fatalf("expected inventory constraint migration not to include document_lines quantity constraint token %q", token)
		}
	}
}

func TestIdempotencyMigrationIncludesRequiredTableAndIndexes(t *testing.T) {
	root := constraintsRepoRoot(t)
	migrationPath := filepath.Join(root, "rims-goProgect", "migrations", "000009_idempotency_keys.sql")
	migrationBytes, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatalf("read idempotency migration: %v", err)
	}

	migration := normalizeSQL(string(migrationBytes))
	requiredTokens := []string{
		"spdx-license-identifier: agpl-3.0-or-later",
		"create table if not exists idempotency_keys",
		"id bigserial primary key",
		"created_at timestamptz not null default now()",
		"updated_at timestamptz not null default now()",
		"deleted_at timestamptz",
		"user_id bigint not null",
		"scope varchar(255) not null",
		"idempotency_key varchar(255) not null",
		"request_hash varchar(64) not null",
		"state varchar(32) not null",
		"status_code integer",
		"response_body jsonb",
		"expires_at timestamptz not null",
		"create unique index if not exists idx_idempotency_user_scope_key",
		"on idempotency_keys (user_id, scope, idempotency_key)",
		"where deleted_at is null",
		"create index if not exists idx_idempotency_expires_at",
		"on idempotency_keys (expires_at)",
	}

	for _, token := range requiredTokens {
		if !strings.Contains(migration, token) {
			t.Fatalf("expected idempotency migration to include %q", token)
		}
	}
}

func normalizeSQL(sql string) string {
	return strings.Join(strings.Fields(strings.ToLower(sql)), " ")
}

func assertConstraintGuarded(t *testing.T, migration, table, name, expression string) {
	t.Helper()

	guardPattern := regexp.MustCompile(`if not exists \([^)]*from pg_constraint[^)]*conname = '` + regexp.QuoteMeta(name) + `'[^)]*\) then`)
	if !guardPattern.MatchString(migration) {
		t.Fatalf("expected %s to be guarded by a pg_constraint IF NOT EXISTS check", name)
	}

	addConstraint := "alter table " + table +
		" add constraint " + name +
		" check (" + expression + ") not valid"
	if !strings.Contains(migration, addConstraint) {
		t.Fatalf("expected %s to add exact check constraint %q", name, addConstraint)
	}

	validateConstraint := "alter table " + table + " validate constraint " + name
	if !strings.Contains(migration, validateConstraint) {
		t.Fatalf("expected %s to be validated with %q", name, validateConstraint)
	}
}
