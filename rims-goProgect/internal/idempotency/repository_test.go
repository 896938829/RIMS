// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package idempotency

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type zeroRowsConnPool struct{}

func (zeroRowsConnPool) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	return nil, errors.New("unexpected prepare")
}

type captureExecPool struct {
	query string
	args  []interface{}
}

func (p *captureExecPool) PrepareContext(context.Context, string) (*sql.Stmt, error) {
	return nil, errors.New("unexpected prepare")
}

func (p *captureExecPool) ExecContext(_ context.Context, query string, args ...interface{}) (sql.Result, error) {
	p.query = query
	p.args = append([]interface{}(nil), args...)
	return zeroRowsResult{}, nil
}

func (*captureExecPool) QueryContext(context.Context, string, ...interface{}) (*sql.Rows, error) {
	return nil, errors.New("unexpected query")
}

func (*captureExecPool) QueryRowContext(context.Context, string, ...interface{}) *sql.Row {
	return nil
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

func TestRepositoryReplayLeaseUsesAtomicCompletedAndUnexpiredCAS(t *testing.T) {
	pool := &captureExecPool{}
	gormDB, err := gorm.Open(postgres.New(postgres.Config{Conn: pool}), &gorm.Config{})
	if err != nil {
		t.Fatalf("open fake gorm db: %v", err)
	}
	repo := NewRepository(gormDB)
	now := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)
	leaseUntil := now.Add(2 * time.Minute)

	err = repo.ExtendCompletedReplayLease(
		context.Background(), 7, "POST /api/v1/documents", "key-1", now, leaseUntil,
	)
	if !errors.Is(err, ErrRecordNotFound) {
		t.Fatalf("error = %v, want ErrRecordNotFound for lost CAS", err)
	}
	sqlText := strings.ToLower(pool.query)
	for _, required := range []string{"state", "expires_at >", "user_id", "scope", "idempotency_key"} {
		if !strings.Contains(sqlText, required) {
			t.Fatalf("lease SQL = %q, want condition %q", pool.query, required)
		}
	}
}
