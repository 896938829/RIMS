// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package maintenance

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeCleanupDB struct {
	execs            []cleanupExec
	fileRows         []cleanupFileRow
	activeObjectKeys map[string]bool
	rowsClosed       bool
}

type cleanupExec struct {
	query string
	args  []any
}

type cleanupFileRow struct {
	id        uint
	objectKey string
}

func (db *fakeCleanupDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	db.execs = append(db.execs, cleanupExec{query: query, args: append([]any(nil), args...)})
	return fakeCleanupResult{rows: 1}, nil
}

func (db *fakeCleanupDB) QueryContext(ctx context.Context, query string, args ...any) (rowsScanner, error) {
	db.execs = append(db.execs, cleanupExec{query: query, args: append([]any(nil), args...)})
	if strings.Contains(query, "FROM file_storage_cleanup_queue") {
		return &retryStorageCleanupRows{}, nil
	}
	if strings.Contains(query, "object_key = $1") && strings.Contains(query, "deleted_at IS NULL") {
		objectKey := args[0].(string)
		rows := []cleanupFileRow{}
		if db.activeObjectKeys[objectKey] {
			rows = []cleanupFileRow{{id: 99, objectKey: objectKey}}
		}
		return &fakeCleanupRows{rows: rows}, nil
	}
	return &fakeCleanupRows{rows: db.fileRows, onClose: func() { db.rowsClosed = true }}, nil
}

type fakeCleanupRows struct {
	rows    []cleanupFileRow
	idx     int
	onClose func()
}

func (r *fakeCleanupRows) Next() bool {
	return r.idx < len(r.rows)
}

func (r *fakeCleanupRows) Scan(dest ...any) error {
	row := r.rows[r.idx]
	r.idx++
	*(dest[0].(*uint)) = row.id
	*(dest[1].(*string)) = row.objectKey
	return nil
}

func (r *fakeCleanupRows) Close() error {
	if r.onClose != nil {
		r.onClose()
	}
	return nil
}

func (r *fakeCleanupRows) Err() error { return nil }

type fakeCleanupResult struct {
	rows int64
}

func (r fakeCleanupResult) LastInsertId() (int64, error) { return 0, nil }
func (r fakeCleanupResult) RowsAffected() (int64, error) { return r.rows, nil }

var _ driver.Result = fakeCleanupResult{}

type fakeCleanupStorage struct {
	deleted []string
	errFor  map[string]error
}

func (s *fakeCleanupStorage) Delete(ctx context.Context, objectKey string) error {
	s.deleted = append(s.deleted, objectKey)
	if s.errFor != nil && s.errFor[objectKey] != nil {
		return s.errFor[objectKey]
	}
	return nil
}

func TestDefaultOptionsMatchOperationalRetentionDefaults(t *testing.T) {
	opts := DefaultOptions()

	if opts.IdempotencyKeyTTL != 24*time.Hour {
		t.Fatalf("IdempotencyKeyTTL = %s, want 24h", opts.IdempotencyKeyTTL)
	}
	if opts.FileDeletedRetention != 30*24*time.Hour {
		t.Fatalf("FileDeletedRetention = %s, want 30d", opts.FileDeletedRetention)
	}
	if opts.AuditLogRetention != 0 {
		t.Fatalf("AuditLogRetention = %s, want disabled", opts.AuditLogRetention)
	}
	if opts.BatchSize != 1000 {
		t.Fatalf("BatchSize = %d, want 1000", opts.BatchSize)
	}
}

func TestCleanerHardDeletesExpiredIdempotencyKeys(t *testing.T) {
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	db := &fakeCleanupDB{}
	storage := &fakeCleanupStorage{}

	result, err := NewCleaner(db, storage, Options{
		Now:                  func() time.Time { return now },
		FileDeletedRetention: 30 * 24 * time.Hour,
		AuditLogRetention:    0,
		BatchSize:            1000,
	}).Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	idempotencyDelete := findCleanupExec(t, db.execs, "DELETE FROM idempotency_keys")
	if !strings.Contains(idempotencyDelete.query, "expires_at < $1") {
		t.Fatalf("idempotency cleanup SQL = %q, want hard delete by expires_at", idempotencyDelete.query)
	}
	if got := idempotencyDelete.args[0].(time.Time); !got.Equal(now) {
		t.Fatalf("idempotency cutoff = %s, want now %s", got, now)
	}
	if result.IdempotencyKeysDeleted != 1 {
		t.Fatalf("IdempotencyKeysDeleted = %d, want RowsAffected", result.IdempotencyKeysDeleted)
	}
}

func TestCleanerSelectsSoftDeletedFilesOlderThanRetentionForObjectCleanup(t *testing.T) {
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	db := &fakeCleanupDB{fileRows: []cleanupFileRow{{id: 12, objectKey: "2026/04/a.png"}}}
	storage := &fakeCleanupStorage{}

	result, err := NewCleaner(db, storage, Options{
		Now:                  func() time.Time { return now },
		FileDeletedRetention: 30 * 24 * time.Hour,
		AuditLogRetention:    0,
		BatchSize:            50,
	}).Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	fileSelect := findCleanupExec(t, db.execs, "SELECT id, object_key FROM file_attachments")
	if !strings.Contains(fileSelect.query, "deleted_at IS NOT NULL") || !strings.Contains(fileSelect.query, "deleted_at < $1") {
		t.Fatalf("file cleanup SQL = %q, want old soft-deleted rows", fileSelect.query)
	}
	wantCutoff := now.Add(-30 * 24 * time.Hour)
	if got := fileSelect.args[0].(time.Time); !got.Equal(wantCutoff) {
		t.Fatalf("file cutoff = %s, want %s", got, wantCutoff)
	}
	if fileSelect.args[1] != 50 {
		t.Fatalf("file batch size = %v, want 50", fileSelect.args[1])
	}
	if len(storage.deleted) != 1 || storage.deleted[0] != "2026/04/a.png" {
		t.Fatalf("storage deletes = %v, want object key before metadata delete", storage.deleted)
	}
	metadataDelete := findCleanupExec(t, db.execs, "DELETE FROM file_attachments")
	if metadataDelete.args[0] != uint(12) {
		t.Fatalf("metadata delete args = %v, want file id 12", metadataDelete.args)
	}
	if result.FileObjectsDeleted != 1 || result.FileMetadataDeleted != 1 {
		t.Fatalf("file result = %#v, want one object and one metadata row", result)
	}
	if !db.rowsClosed {
		t.Fatal("file rows were not closed")
	}
}

func TestCleanerRetainsFileMetadataWhenObjectDeletionFails(t *testing.T) {
	db := &fakeCleanupDB{fileRows: []cleanupFileRow{{id: 12, objectKey: "2026/04/a.png"}}}
	storage := &fakeCleanupStorage{errFor: map[string]error{
		"2026/04/a.png": errors.New("disk unavailable"),
	}}

	_, err := NewCleaner(db, storage, Options{
		Now:                  func() time.Time { return time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC) },
		FileDeletedRetention: 30 * 24 * time.Hour,
		AuditLogRetention:    0,
		BatchSize:            1000,
	}).Run(context.Background())
	if err == nil {
		t.Fatal("Run() error = nil, want storage deletion error")
	}
	if containsCleanupExec(db.execs, "DELETE FROM file_attachments") {
		t.Fatalf("metadata delete was executed despite storage failure: %#v", db.execs)
	}
}

func TestCleanerSkipsObjectDeletionWhenObjectKeyStillHasActiveAttachment(t *testing.T) {
	db := &fakeCleanupDB{
		fileRows:         []cleanupFileRow{{id: 12, objectKey: "2026/04/shared.png"}},
		activeObjectKeys: map[string]bool{"2026/04/shared.png": true},
	}
	storage := &fakeCleanupStorage{}

	result, err := NewCleaner(db, storage, Options{
		Now:                  func() time.Time { return time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC) },
		FileDeletedRetention: 30 * 24 * time.Hour,
		AuditLogRetention:    0,
		BatchSize:            1000,
	}).Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	activeSelect := findCleanupExec(t, db.execs, "object_key = $1")
	if activeSelect.args[0] != "2026/04/shared.png" {
		t.Fatalf("active object lookup args = %v, want shared object key", activeSelect.args)
	}
	if len(storage.deleted) != 0 {
		t.Fatalf("storage deletes = %v, want no delete while an active row still references object_key", storage.deleted)
	}
	if containsCleanupExec(db.execs, "DELETE FROM file_attachments") {
		t.Fatalf("metadata delete was executed despite active object reference: %#v", db.execs)
	}
	if result.FileObjectsDeleted != 0 || result.FileMetadataDeleted != 0 {
		t.Fatalf("file result = %#v, want no object or metadata deletion", result)
	}
}

func TestCleanerSkipsAuditCleanupWhenRetentionDisabled(t *testing.T) {
	db := &fakeCleanupDB{}
	storage := &fakeCleanupStorage{}

	_, err := NewCleaner(db, storage, Options{
		Now:                  func() time.Time { return time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC) },
		FileDeletedRetention: 30 * 24 * time.Hour,
		AuditLogRetention:    0,
		BatchSize:            1000,
	}).Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if containsCleanupExec(db.execs, "audit_logs") {
		t.Fatalf("audit cleanup SQL was executed while retention is disabled: %#v", db.execs)
	}
}

type retryStorageCleanupDB struct {
	pending []string
	execs   []cleanupExec
}

func (db *retryStorageCleanupDB) ExecContext(_ context.Context, query string, args ...any) (sql.Result, error) {
	db.execs = append(db.execs, cleanupExec{query: query, args: append([]any(nil), args...)})
	if strings.Contains(query, "DELETE FROM file_storage_cleanup_queue") && len(args) == 1 {
		key := args[0].(string)
		for index, pending := range db.pending {
			if pending == key {
				db.pending = append(db.pending[:index], db.pending[index+1:]...)
				break
			}
		}
	}
	return fakeCleanupResult{rows: 1}, nil
}

func (db *retryStorageCleanupDB) QueryContext(_ context.Context, query string, args ...any) (rowsScanner, error) {
	db.execs = append(db.execs, cleanupExec{query: query, args: append([]any(nil), args...)})
	if strings.Contains(query, "FROM file_storage_cleanup_queue") {
		return &retryStorageCleanupRows{keys: append([]string(nil), db.pending...)}, nil
	}
	return &retryStorageCleanupRows{}, nil
}

type retryStorageCleanupRows struct {
	keys []string
	idx  int
}

func (r *retryStorageCleanupRows) Next() bool { return r.idx < len(r.keys) }
func (r *retryStorageCleanupRows) Scan(dest ...any) error {
	*(dest[0].(*string)) = r.keys[r.idx]
	r.idx++
	return nil
}
func (r *retryStorageCleanupRows) Close() error { return nil }
func (r *retryStorageCleanupRows) Err() error   { return nil }

func TestCleanerRetriesDurableStorageCleanupResponsibilityUntilDeleteSucceeds(t *testing.T) {
	const objectKey = "2026/07/orphaned-upload.txt"
	db := &retryStorageCleanupDB{pending: []string{objectKey}}
	storage := &fakeCleanupStorage{errFor: map[string]error{objectKey: errors.New("disk unavailable")}}
	cleaner := NewCleaner(db, storage, Options{BatchSize: 10})

	if _, err := cleaner.Run(context.Background()); err == nil || !strings.Contains(err.Error(), "disk unavailable") {
		t.Fatalf("first Run() error = %v, want retryable storage deletion failure", err)
	}
	if len(db.pending) != 1 || db.pending[0] != objectKey {
		t.Fatalf("pending after failed retry = %v, want retained responsibility", db.pending)
	}

	delete(storage.errFor, objectKey)
	if _, err := cleaner.Run(context.Background()); err != nil {
		t.Fatalf("second Run() error = %v", err)
	}
	if len(db.pending) != 0 {
		t.Fatalf("pending after successful retry = %v, want responsibility cleared", db.pending)
	}
	if len(storage.deleted) != 2 || storage.deleted[0] != objectKey || storage.deleted[1] != objectKey {
		t.Fatalf("storage retry deletes = %v, want two attempts for %q", storage.deleted, objectKey)
	}
	queueSelect := findCleanupExec(t, db.execs, "SELECT object_key FROM file_storage_cleanup_queue")
	if !strings.Contains(queueSelect.query, "ready_at IS NOT NULL") ||
		!strings.Contains(queueSelect.query, "queued_at < $1") {
		t.Fatalf("storage cleanup queue SQL = %q, want failed-ready or expired-preparation fencing", queueSelect.query)
	}
}

func findCleanupExec(t *testing.T, execs []cleanupExec, fragment string) cleanupExec {
	t.Helper()
	for _, exec := range execs {
		if strings.Contains(exec.query, fragment) {
			return exec
		}
	}
	t.Fatalf("did not find cleanup SQL containing %q in %#v", fragment, execs)
	return cleanupExec{}
}

func containsCleanupExec(execs []cleanupExec, fragment string) bool {
	for _, exec := range execs {
		if strings.Contains(exec.query, fragment) {
			return true
		}
	}
	return false
}
