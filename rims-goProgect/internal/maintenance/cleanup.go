// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package maintenance

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const (
	defaultIdempotencyKeyTTL    = 24 * time.Hour
	defaultFileDeletedRetention = 30 * 24 * time.Hour
	defaultAuditLogRetention    = 0
	defaultCleanupBatchSize     = 1000
	defaultStoragePrepareLease  = time.Hour
)

type rowsScanner interface {
	Next() bool
	Scan(dest ...any) error
	Close() error
	Err() error
}

type sqlExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (rowsScanner, error)
}

type sqlDBAdapter struct {
	db *sql.DB
}

func (a sqlDBAdapter) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return a.db.ExecContext(ctx, query, args...)
}

func (a sqlDBAdapter) QueryContext(ctx context.Context, query string, args ...any) (rowsScanner, error) {
	rows, err := a.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

type objectStorage interface {
	Delete(ctx context.Context, objectKey string) error
}

// Options controls one-shot maintenance cleanup behavior.
type Options struct {
	IdempotencyKeyTTL    time.Duration
	FileDeletedRetention time.Duration
	AuditLogRetention    time.Duration
	StoragePrepareLease  time.Duration
	BatchSize            int
	Now                  func() time.Time
}

// Result reports how many records and storage objects were cleaned.
type Result struct {
	IdempotencyKeysDeleted int64
	StorageObjectsDeleted  int64
	StorageTasksCleared    int64
	FileObjectsDeleted     int64
	FileMetadataDeleted    int64
	AuditLogsDeleted       int64
}

// DefaultOptions returns the operational defaults for the cleanup command.
func DefaultOptions() Options {
	return Options{
		IdempotencyKeyTTL:    defaultIdempotencyKeyTTL,
		FileDeletedRetention: defaultFileDeletedRetention,
		AuditLogRetention:    defaultAuditLogRetention,
		StoragePrepareLease:  defaultStoragePrepareLease,
		BatchSize:            defaultCleanupBatchSize,
		Now:                  time.Now,
	}
}

// Cleaner performs explicit one-shot maintenance tasks.
type Cleaner struct {
	db      sqlExecutor
	storage objectStorage
	options Options
	initErr error
}

// NewCleaner creates a cleanup runner.
func NewCleaner(db any, storage objectStorage, options Options) *Cleaner {
	executor, err := cleanupExecutor(db)
	return &Cleaner{db: executor, storage: storage, options: options, initErr: err}
}

// Run executes all enabled cleanup tasks once.
func (c *Cleaner) Run(ctx context.Context) (Result, error) {
	if c.initErr != nil {
		return Result{}, c.initErr
	}
	if c.db == nil {
		return Result{}, fmt.Errorf("cleanup: db is required")
	}
	options := normalizeOptions(c.options)
	now := options.Now()

	var result Result
	var err error
	result.IdempotencyKeysDeleted, err = c.deleteExpiredIdempotencyKeys(ctx, now)
	if err != nil {
		return result, err
	}
	storageResult, err := c.cleanupPendingStorageObjects(
		ctx, now.Add(-options.StoragePrepareLease), options.BatchSize,
	)
	result.StorageObjectsDeleted = storageResult.StorageObjectsDeleted
	result.StorageTasksCleared = storageResult.StorageTasksCleared
	if err != nil {
		return result, err
	}

	fileResult, err := c.cleanupDeletedFiles(ctx, now.Add(-options.FileDeletedRetention), options.BatchSize)
	result.FileObjectsDeleted = fileResult.FileObjectsDeleted
	result.FileMetadataDeleted = fileResult.FileMetadataDeleted
	if err != nil {
		return result, err
	}

	if options.AuditLogRetention > 0 {
		result.AuditLogsDeleted, err = c.deleteOldAuditLogs(ctx, now.Add(-options.AuditLogRetention), options.BatchSize)
		if err != nil {
			return result, err
		}
	}

	return result, nil
}

func (c *Cleaner) cleanupPendingStorageObjects(ctx context.Context, preparationCutoff time.Time, batchSize int) (Result, error) {
	if c.storage == nil {
		return Result{}, fmt.Errorf("cleanup pending storage objects: storage is required")
	}
	const query = `SELECT object_key FROM file_storage_cleanup_queue
WHERE ready_at IS NOT NULL OR queued_at < $1
ORDER BY updated_at, object_key
LIMIT $2`
	rows, err := c.db.QueryContext(ctx, query, preparationCutoff, batchSize)
	if err != nil {
		return Result{}, fmt.Errorf("select pending storage cleanup: %w", err)
	}
	defer rows.Close()

	var result Result
	for rows.Next() {
		var objectKey string
		if err := rows.Scan(&objectKey); err != nil {
			return result, fmt.Errorf("scan pending storage cleanup: %w", err)
		}
		active, err := c.hasActiveObjectReference(ctx, objectKey)
		if err != nil {
			return result, err
		}
		if active {
			cleared, err := c.clearStorageCleanupTask(ctx, objectKey)
			if err != nil {
				return result, err
			}
			result.StorageTasksCleared += cleared
			continue
		}
		if err := c.storage.Delete(ctx, objectKey); err != nil {
			if recordErr := c.recordStorageCleanupRetryFailure(ctx, objectKey, err); recordErr != nil {
				return result, errors.Join(
					fmt.Errorf("delete pending storage object %s: %w", objectKey, err),
					recordErr,
				)
			}
			return result, fmt.Errorf("delete pending storage object %s: %w", objectKey, err)
		}
		result.StorageObjectsDeleted++
		cleared, err := c.clearStorageCleanupTask(ctx, objectKey)
		if err != nil {
			return result, err
		}
		result.StorageTasksCleared += cleared
	}
	if err := rows.Err(); err != nil {
		return result, fmt.Errorf("iterate pending storage cleanup: %w", err)
	}
	return result, nil
}

func (c *Cleaner) recordStorageCleanupRetryFailure(ctx context.Context, objectKey string, cleanupErr error) error {
	const query = `UPDATE file_storage_cleanup_queue
SET cleanup_error = $2,
    attempt_count = attempt_count + 1,
    ready_at = CURRENT_TIMESTAMP,
    updated_at = CURRENT_TIMESTAMP
WHERE object_key = $1`
	if _, err := c.db.ExecContext(ctx, query, objectKey, cleanupErr.Error()); err != nil {
		return fmt.Errorf("record pending storage cleanup retry %s: %w", objectKey, err)
	}
	return nil
}

func (c *Cleaner) clearStorageCleanupTask(ctx context.Context, objectKey string) (int64, error) {
	const query = `DELETE FROM file_storage_cleanup_queue WHERE object_key = $1`
	result, err := c.db.ExecContext(ctx, query, objectKey)
	if err != nil {
		return 0, fmt.Errorf("clear storage cleanup responsibility %s: %w", objectKey, err)
	}
	return rowsAffected(result)
}

func cleanupExecutor(db any) (sqlExecutor, error) {
	switch d := db.(type) {
	case nil:
		return nil, nil
	case sqlExecutor:
		return d, nil
	case *sql.DB:
		return sqlDBAdapter{db: d}, nil
	default:
		return nil, fmt.Errorf("cleanup: unsupported db type %T", db)
	}
}

func normalizeOptions(options Options) Options {
	defaults := DefaultOptions()
	if options.IdempotencyKeyTTL <= 0 {
		options.IdempotencyKeyTTL = defaults.IdempotencyKeyTTL
	}
	if options.FileDeletedRetention <= 0 {
		options.FileDeletedRetention = defaults.FileDeletedRetention
	}
	if options.AuditLogRetention < 0 {
		options.AuditLogRetention = defaults.AuditLogRetention
	}
	if options.StoragePrepareLease <= 0 {
		options.StoragePrepareLease = defaults.StoragePrepareLease
	}
	if options.BatchSize <= 0 {
		options.BatchSize = defaults.BatchSize
	}
	if options.Now == nil {
		options.Now = defaults.Now
	}
	return options
}

func (c *Cleaner) deleteExpiredIdempotencyKeys(ctx context.Context, now time.Time) (int64, error) {
	const query = `DELETE FROM idempotency_keys WHERE expires_at < $1`
	result, err := c.db.ExecContext(ctx, query, now)
	if err != nil {
		return 0, fmt.Errorf("cleanup expired idempotency keys: %w", err)
	}
	return rowsAffected(result)
}

func (c *Cleaner) cleanupDeletedFiles(ctx context.Context, cutoff time.Time, batchSize int) (Result, error) {
	if c.storage == nil {
		return Result{}, fmt.Errorf("cleanup files: storage is required")
	}

	const query = `SELECT id, object_key FROM file_attachments
WHERE deleted_at IS NOT NULL AND deleted_at < $1
ORDER BY id
LIMIT $2`
	rows, err := c.db.QueryContext(ctx, query, cutoff, batchSize)
	if err != nil {
		return Result{}, fmt.Errorf("select deleted files for cleanup: %w", err)
	}
	defer rows.Close()

	var result Result
	for rows.Next() {
		var id uint
		var objectKey string
		if err := rows.Scan(&id, &objectKey); err != nil {
			return result, fmt.Errorf("scan deleted file: %w", err)
		}
		active, err := c.hasActiveObjectReference(ctx, objectKey)
		if err != nil {
			return result, err
		}
		if active {
			continue
		}
		if err := c.storage.Delete(ctx, objectKey); err != nil {
			return result, fmt.Errorf("delete storage object %s: %w", objectKey, err)
		}
		result.FileObjectsDeleted++
		deleted, err := c.hardDeleteFileMetadata(ctx, id)
		if err != nil {
			return result, err
		}
		result.FileMetadataDeleted += deleted
	}
	if err := rows.Err(); err != nil {
		return result, fmt.Errorf("iterate deleted files: %w", err)
	}
	return result, nil
}

func (c *Cleaner) hasActiveObjectReference(ctx context.Context, objectKey string) (bool, error) {
	const query = `SELECT id FROM file_attachments
WHERE object_key = $1 AND deleted_at IS NULL
LIMIT 1`
	rows, err := c.db.QueryContext(ctx, query, objectKey)
	if err != nil {
		return false, fmt.Errorf("check active file object reference %s: %w", objectKey, err)
	}
	defer rows.Close()
	active := rows.Next()
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("iterate active file object reference %s: %w", objectKey, err)
	}
	return active, nil
}

func (c *Cleaner) hardDeleteFileMetadata(ctx context.Context, id uint) (int64, error) {
	const query = `DELETE FROM file_attachments WHERE id = $1 AND deleted_at IS NOT NULL`
	result, err := c.db.ExecContext(ctx, query, id)
	if err != nil {
		return 0, fmt.Errorf("delete file metadata %d: %w", id, err)
	}
	return rowsAffected(result)
}

func (c *Cleaner) deleteOldAuditLogs(ctx context.Context, cutoff time.Time, batchSize int) (int64, error) {
	const query = `DELETE FROM audit_logs
WHERE id IN (
	SELECT id FROM audit_logs
	WHERE created_at < $1
	ORDER BY id
	LIMIT $2
)`
	result, err := c.db.ExecContext(ctx, query, cutoff, batchSize)
	if err != nil {
		return 0, fmt.Errorf("cleanup old audit logs: %w", err)
	}
	return rowsAffected(result)
}

func rowsAffected(result sql.Result) (int64, error) {
	rows, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("read rows affected: %w", err)
	}
	return rows, nil
}
