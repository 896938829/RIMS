// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package migration

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeMigrationDB struct {
	applied            map[string]string
	execs              []migrationExec
	events             []string
	failExecContaining string
	closed             bool
}

type migrationExec struct {
	query string
	args  []any
}

func (db *fakeMigrationDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	db.execs = append(db.execs, migrationExec{query: query, args: append([]any(nil), args...)})
	trimmed := strings.TrimSpace(query)
	switch {
	case strings.Contains(trimmed, "pg_advisory_lock"):
		db.events = append(db.events, "LOCK")
	case strings.Contains(trimmed, "pg_advisory_unlock"):
		db.events = append(db.events, "UNLOCK")
	default:
		db.events = append(db.events, "EXEC:"+trimmed)
	}
	if db.failExecContaining != "" && strings.Contains(query, db.failExecContaining) {
		return nil, errors.New("injected exec failure")
	}
	if strings.HasPrefix(strings.TrimSpace(query), "INSERT INTO schema_migrations") {
		if db.applied == nil {
			db.applied = map[string]string{}
		}
		db.applied[args[0].(string)] = args[1].(string)
	}
	return fakeSQLResult{}, nil
}

func (db *fakeMigrationDB) QueryRowContext(ctx context.Context, query string, args ...any) rowScanner {
	version := args[0].(string)
	checksum, ok := db.applied[version]
	if !ok {
		return fakeMigrationRow{err: sql.ErrNoRows}
	}
	return fakeMigrationRow{checksum: checksum}
}

func (db *fakeMigrationDB) Conn(context.Context) (migrationConnection, error) {
	return db, nil
}

func (db *fakeMigrationDB) BeginTx(context.Context, *sql.TxOptions) (migrationTx, error) {
	db.events = append(db.events, "BEGIN")
	return &fakeMigrationTx{db: db}, nil
}

func (db *fakeMigrationDB) Close() error {
	db.closed = true
	return nil
}

type fakeMigrationTx struct {
	db *fakeMigrationDB
}

func (tx *fakeMigrationTx) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return tx.db.ExecContext(ctx, query, args...)
}

func (tx *fakeMigrationTx) QueryRowContext(ctx context.Context, query string, args ...any) rowScanner {
	return tx.db.QueryRowContext(ctx, query, args...)
}

func (tx *fakeMigrationTx) Commit() error {
	tx.db.events = append(tx.db.events, "COMMIT")
	return nil
}

func (tx *fakeMigrationTx) Rollback() error {
	tx.db.events = append(tx.db.events, "ROLLBACK")
	return nil
}

type fakeMigrationRow struct {
	checksum string
	err      error
}

func (r fakeMigrationRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	*(dest[0].(*string)) = r.checksum
	return nil
}

type fakeSQLResult struct{}

func (fakeSQLResult) LastInsertId() (int64, error) { return 0, nil }
func (fakeSQLResult) RowsAffected() (int64, error) { return 0, nil }

var _ driver.Result = fakeSQLResult{}

func TestRunnerUpCreatesSchemaMigrationsAndAppliesSQLFilesInFilenameOrder(t *testing.T) {
	dir := t.TempDir()
	writeMigrationFile(t, dir, "000002_second.sql", "SELECT 2;")
	writeMigrationFile(t, dir, "000001_first.sql", "SELECT 1;")
	writeMigrationFile(t, dir, "notes.txt", "not a migration")
	db := &fakeMigrationDB{}

	err := NewRunner(db, dir).Up(context.Background())
	if err != nil {
		t.Fatalf("Up() error = %v", err)
	}

	if len(db.execs) != 7 {
		t.Fatalf("exec count = %d, want lock + create table + 2 migrations + 2 inserts + unlock", len(db.execs))
	}
	if !strings.Contains(db.execs[0].query, "pg_advisory_lock") {
		t.Fatalf("first exec = %q, want PostgreSQL advisory lock", db.execs[0].query)
	}
	if !strings.Contains(db.execs[1].query, "CREATE TABLE IF NOT EXISTS schema_migrations") {
		t.Fatalf("second exec = %q, want schema_migrations creation", db.execs[1].query)
	}
	if !strings.Contains(db.execs[1].query, "applied_at timestamptz NOT NULL DEFAULT now()") {
		t.Fatalf("schema_migrations DDL = %q, want applied_at default", db.execs[1].query)
	}
	if strings.TrimSpace(db.execs[2].query) != "SELECT 1;" {
		t.Fatalf("first migration exec = %q, want 000001_first.sql", db.execs[2].query)
	}
	if strings.TrimSpace(db.execs[4].query) != "SELECT 2;" {
		t.Fatalf("second migration exec = %q, want 000002_second.sql", db.execs[4].query)
	}
	if db.execs[3].args[0] != "000001_first.sql" || db.execs[5].args[0] != "000002_second.sql" {
		t.Fatalf("inserted versions = %v/%v, want sorted filenames", db.execs[3].args, db.execs[5].args)
	}
	if db.applied["000001_first.sql"] != checksumOf("SELECT 1;") {
		t.Fatalf("recorded checksum for first migration = %q, want SHA-256", db.applied["000001_first.sql"])
	}
	if !db.closed {
		t.Fatal("migration connection was not closed")
	}
}

func TestRunnerUpLocksWholeRunAndUsesOneTransactionPerPendingMigration(t *testing.T) {
	dir := t.TempDir()
	writeMigrationFile(t, dir, "000001_first.sql", "SELECT 1;")
	writeMigrationFile(t, dir, "000002_second.sql", "SELECT 2;")
	db := &fakeMigrationDB{}

	err := NewRunner(db, dir).Up(context.Background())
	if err != nil {
		t.Fatalf("Up() error = %v", err)
	}

	if countMigrationEvents(db.events, "LOCK") != 1 {
		t.Fatalf("events = %v, want one advisory lock", db.events)
	}
	if countMigrationEvents(db.events, "UNLOCK") != 1 {
		t.Fatalf("events = %v, want one advisory unlock", db.events)
	}
	if countMigrationEvents(db.events, "BEGIN") != 2 || countMigrationEvents(db.events, "COMMIT") != 2 {
		t.Fatalf("events = %v, want one begin/commit pair per pending migration", db.events)
	}
	if countMigrationEvents(db.events, "ROLLBACK") != 0 {
		t.Fatalf("events = %v, want no rollback on successful migrations", db.events)
	}
	assertMigrationEventOrder(t, db.events, "LOCK", "BEGIN")
	assertMigrationEventOrder(t, db.events, "BEGIN", "EXEC:SELECT 1;")
	assertMigrationEventOrder(t, db.events, "EXEC:SELECT 1;", "EXEC:INSERT INTO schema_migrations (version, checksum) VALUES ($1, $2)")
	assertMigrationEventOrder(t, db.events, "EXEC:INSERT INTO schema_migrations (version, checksum) VALUES ($1, $2)", "COMMIT")
	assertMigrationEventOrder(t, db.events, "COMMIT", "UNLOCK")
}

func TestRunnerUpSkipsAlreadyAppliedMatchingChecksum(t *testing.T) {
	dir := t.TempDir()
	writeMigrationFile(t, dir, "000001_init.sql", "SELECT 1;")
	db := &fakeMigrationDB{applied: map[string]string{
		"000001_init.sql": checksumOf("SELECT 1;"),
	}}

	err := NewRunner(db, dir).Up(context.Background())
	if err != nil {
		t.Fatalf("Up() error = %v", err)
	}

	if len(db.execs) != 3 {
		t.Fatalf("exec count = %d, want lock + schema_migrations creation + unlock when checksum matches", len(db.execs))
	}
	if countMigrationEvents(db.events, "BEGIN") != 0 {
		t.Fatalf("events = %v, want no transaction for already applied matching checksum", db.events)
	}
}

func TestRunnerUpRejectsAlreadyAppliedChangedChecksum(t *testing.T) {
	dir := t.TempDir()
	writeMigrationFile(t, dir, "000001_init.sql", "SELECT 1;")
	db := &fakeMigrationDB{applied: map[string]string{
		"000001_init.sql": checksumOf("SELECT changed;"),
	}}

	err := NewRunner(db, dir).Up(context.Background())
	if err == nil {
		t.Fatal("Up() error = nil, want checksum mismatch")
	}
	if !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("Up() error = %v, want ErrChecksumMismatch", err)
	}
	if len(db.execs) != 3 {
		t.Fatalf("exec count = %d, want lock + schema_migrations creation + unlock after checksum mismatch", len(db.execs))
	}
	if countMigrationEvents(db.events, "BEGIN") != 0 {
		t.Fatalf("events = %v, want no transaction after checksum mismatch", db.events)
	}
}

func TestRunnerUpRollsBackMigrationTransactionWhenSQLExecutionFails(t *testing.T) {
	dir := t.TempDir()
	writeMigrationFile(t, dir, "000001_broken.sql", "SELECT broken;")
	db := &fakeMigrationDB{failExecContaining: "SELECT broken;"}

	err := NewRunner(db, dir).Up(context.Background())
	if err == nil {
		t.Fatal("Up() error = nil, want migration execution error")
	}

	if countMigrationEvents(db.events, "BEGIN") != 1 {
		t.Fatalf("events = %v, want one transaction begin", db.events)
	}
	if countMigrationEvents(db.events, "ROLLBACK") != 1 {
		t.Fatalf("events = %v, want rollback for failed migration", db.events)
	}
	if countMigrationEvents(db.events, "COMMIT") != 0 {
		t.Fatalf("events = %v, want no commit for failed migration", db.events)
	}
	if _, ok := db.applied["000001_broken.sql"]; ok {
		t.Fatalf("failed migration was recorded in schema_migrations: %#v", db.applied)
	}
	assertMigrationEventOrder(t, db.events, "ROLLBACK", "UNLOCK")
}

func countMigrationEvents(events []string, want string) int {
	count := 0
	for _, event := range events {
		if event == want {
			count++
		}
	}
	return count
}

func assertMigrationEventOrder(t *testing.T, events []string, before, after string) {
	t.Helper()
	beforeIndex := -1
	afterIndex := -1
	for i, event := range events {
		if event == before && beforeIndex == -1 {
			beforeIndex = i
		}
		if event == after && afterIndex == -1 {
			afterIndex = i
		}
	}
	if beforeIndex == -1 || afterIndex == -1 || beforeIndex >= afterIndex {
		t.Fatalf("events = %v, want %q before %q", events, before, after)
	}
}

func writeMigrationFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write migration file: %v", err)
	}
}

func checksumOf(content string) string {
	sum := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", sum[:])
}
