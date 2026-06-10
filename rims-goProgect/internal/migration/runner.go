// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package migration

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

var ErrChecksumMismatch = errors.New("migration checksum mismatch")

type rowScanner interface {
	Scan(dest ...any) error
}

type sqlExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) rowScanner
}

type migrationConnector interface {
	Conn(ctx context.Context) (migrationConnection, error)
}

type migrationConnection interface {
	sqlExecutor
	BeginTx(ctx context.Context, opts *sql.TxOptions) (migrationTx, error)
	Close() error
}

type migrationTx interface {
	sqlExecutor
	Commit() error
	Rollback() error
}

type sqlDBAdapter struct {
	db *sql.DB
}

func (a sqlDBAdapter) Conn(ctx context.Context) (migrationConnection, error) {
	conn, err := a.db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	return sqlConnAdapter{conn: conn}, nil
}

type sqlConnAdapter struct {
	conn *sql.Conn
}

func (a sqlConnAdapter) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return a.conn.ExecContext(ctx, query, args...)
}

func (a sqlConnAdapter) QueryRowContext(ctx context.Context, query string, args ...any) rowScanner {
	return a.conn.QueryRowContext(ctx, query, args...)
}

func (a sqlConnAdapter) BeginTx(ctx context.Context, opts *sql.TxOptions) (migrationTx, error) {
	tx, err := a.conn.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return sqlTxAdapter{tx: tx}, nil
}

func (a sqlConnAdapter) Close() error {
	return a.conn.Close()
}

type sqlTxAdapter struct {
	tx *sql.Tx
}

func (a sqlTxAdapter) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return a.tx.ExecContext(ctx, query, args...)
}

func (a sqlTxAdapter) QueryRowContext(ctx context.Context, query string, args ...any) rowScanner {
	return a.tx.QueryRowContext(ctx, query, args...)
}

func (a sqlTxAdapter) Commit() error {
	return a.tx.Commit()
}

func (a sqlTxAdapter) Rollback() error {
	return a.tx.Rollback()
}

// Runner applies SQL files from a directory and records their checksums in
// schema_migrations.
type Runner struct {
	db      migrationConnector
	dir     string
	initErr error
}

// NewRunner creates a migration runner using the supplied SQL executor.
func NewRunner(db any, dir string) *Runner {
	executor, err := migrationExecutor(db)
	return &Runner{db: executor, dir: dir, initErr: err}
}

// Up applies pending .sql files in filename order.
func (r *Runner) Up(ctx context.Context) (err error) {
	if r.initErr != nil {
		return r.initErr
	}
	if r.db == nil {
		return fmt.Errorf("migration runner: db is required")
	}
	if r.dir == "" {
		return fmt.Errorf("migration runner: migrations dir is required")
	}
	conn, err := r.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("open migration connection: %w", err)
	}
	defer func() {
		if closeErr := conn.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close migration connection: %w", closeErr)
		}
	}()

	if err := r.lock(ctx, conn); err != nil {
		return err
	}
	defer func() {
		if unlockErr := r.unlock(ctx, conn); err == nil && unlockErr != nil {
			err = unlockErr
		}
	}()

	if err := r.ensureSchemaTable(ctx, conn); err != nil {
		return err
	}

	files, err := migrationFiles(r.dir)
	if err != nil {
		return err
	}
	for _, file := range files {
		if err := r.applyFile(ctx, conn, file); err != nil {
			return err
		}
	}
	return nil
}

func migrationExecutor(db any) (migrationConnector, error) {
	switch d := db.(type) {
	case nil:
		return nil, nil
	case migrationConnector:
		return d, nil
	case migrationConnection:
		return directMigrationConnector{conn: d}, nil
	case *sql.DB:
		return sqlDBAdapter{db: d}, nil
	default:
		return nil, fmt.Errorf("migration runner: unsupported db type %T", db)
	}
}

type directMigrationConnector struct {
	conn migrationConnection
}

func (d directMigrationConnector) Conn(context.Context) (migrationConnection, error) {
	return d.conn, nil
}

const migrationAdvisoryLockKey int64 = 202606100010

func (r *Runner) lock(ctx context.Context, exec sqlExecutor) error {
	const query = `SELECT pg_advisory_lock($1)`
	if _, err := exec.ExecContext(ctx, query, migrationAdvisoryLockKey); err != nil {
		return fmt.Errorf("acquire migration advisory lock: %w", err)
	}
	return nil
}

func (r *Runner) unlock(ctx context.Context, exec sqlExecutor) error {
	const query = `SELECT pg_advisory_unlock($1)`
	if _, err := exec.ExecContext(ctx, query, migrationAdvisoryLockKey); err != nil {
		return fmt.Errorf("release migration advisory lock: %w", err)
	}
	return nil
}

func (r *Runner) ensureSchemaTable(ctx context.Context, exec sqlExecutor) error {
	const ddl = `CREATE TABLE IF NOT EXISTS schema_migrations (
version text PRIMARY KEY,
checksum text NOT NULL,
applied_at timestamptz NOT NULL DEFAULT now()
)`
	if _, err := exec.ExecContext(ctx, ddl); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	return nil
}

func (r *Runner) applyFile(ctx context.Context, conn migrationConnection, path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read migration %s: %w", path, err)
	}
	version := filepath.Base(path)
	checksum := checksumSQL(content)

	appliedChecksum, ok, err := r.appliedChecksum(ctx, conn, version)
	if err != nil {
		return err
	}
	if ok {
		if appliedChecksum != checksum {
			return fmt.Errorf("%w: %s", ErrChecksumMismatch, version)
		}
		return nil
	}

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration %s: %w", version, err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if _, err := tx.ExecContext(ctx, string(content)); err != nil {
		return fmt.Errorf("apply migration %s: %w", version, err)
	}
	const insertSQL = `INSERT INTO schema_migrations (version, checksum) VALUES ($1, $2)`
	if _, err := tx.ExecContext(ctx, insertSQL, version, checksum); err != nil {
		return fmt.Errorf("record migration %s: %w", version, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %s: %w", version, err)
	}
	committed = true
	return nil
}

func (r *Runner) appliedChecksum(ctx context.Context, exec sqlExecutor, version string) (string, bool, error) {
	var checksum string
	const query = `SELECT checksum FROM schema_migrations WHERE version = $1`
	err := exec.QueryRowContext(ctx, query, version).Scan(&checksum)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("query schema_migrations %s: %w", version, err)
	}
	return checksum, true, nil
}

func migrationFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
			continue
		}
		files = append(files, filepath.Join(dir, entry.Name()))
	}
	sort.Slice(files, func(i, j int) bool {
		return filepath.Base(files[i]) < filepath.Base(files[j])
	})
	return files, nil
}

func checksumSQL(content []byte) string {
	sum := sha256.Sum256(content)
	return fmt.Sprintf("%x", sum[:])
}
