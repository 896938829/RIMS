// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package idempotency

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type zeroRowsConnPool struct{}

func (zeroRowsConnPool) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	return nil, errors.New("unexpected prepare")
}

func (zeroRowsConnPool) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return zeroRowsResult{}, nil
}

func (zeroRowsConnPool) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return nil, errors.New("unexpected query")
}

func (zeroRowsConnPool) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return nil
}

type zeroRowsResult struct{}

func (zeroRowsResult) LastInsertId() (int64, error) {
	return 0, nil
}

func (zeroRowsResult) RowsAffected() (int64, error) {
	return 0, nil
}

var _ driver.Result = zeroRowsResult{}

func newZeroRowsRepository(t *testing.T) Repository {
	t.Helper()
	gormDB, err := gorm.Open(postgres.New(postgres.Config{Conn: zeroRowsConnPool{}}), &gorm.Config{})
	if err != nil {
		t.Fatalf("open fake gorm db: %v", err)
	}
	return NewRepository(gormDB)
}

func TestRepositoryCompleteReturnsErrorWhenNoProcessingRecordUpdated(t *testing.T) {
	repo := newZeroRowsRepository(t)

	err := repo.Complete(context.Background(), 7, "POST /documents", "key-1", 201, []byte(`{"ok":true}`))
	if err == nil {
		t.Fatal("expected error when no processing record is completed")
	}
}

func TestRepositoryDeleteProcessingReturnsErrorWhenNoProcessingRecordDeleted(t *testing.T) {
	repo := newZeroRowsRepository(t)

	err := repo.DeleteProcessing(context.Background(), 7, "POST /documents", "key-1")
	if err == nil {
		t.Fatal("expected error when no processing record is deleted")
	}
}
