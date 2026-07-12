// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package file

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"rims-go/internal/types"
)

func TestPositionMigrationBackfillsStableBindingOrderAndAddsVisibleIndex(t *testing.T) {
	contents, err := os.ReadFile("../../../migrations/000014_file_attachment_position.sql")
	if err != nil {
		t.Fatalf("read position migration: %v", err)
	}

	sql := strings.ToLower(string(contents))
	for _, fragment := range []string{
		"position integer not null default 0",
		"partition by business_type, business_id",
		"order by created_at asc, id asc",
		"row_number()",
		"business_type, business_id, position, id",
		"where deleted_at is null",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migration missing %q", fragment)
		}
	}
}

type fileSQLLogWriter struct {
	lines []string
}

func (w *fileSQLLogWriter) Printf(format string, args ...interface{}) {
	w.lines = append(w.lines, fmt.Sprintf(format, args...))
}

func (w *fileSQLLogWriter) String() string {
	return strings.Join(w.lines, "\n")
}

func newFileDryRunDB(t *testing.T, logWriter *fileSQLLogWriter) *gorm.DB {
	t.Helper()

	gormDB, err := gorm.Open(postgres.New(postgres.Config{
		DSN: "host=127.0.0.1 user=rims password=rims dbname=rims sslmode=disable",
	}), &gorm.Config{
		DryRun:                 true,
		DisableAutomaticPing:   true,
		SkipDefaultTransaction: true,
		Logger:                 logger.New(logWriter, logger.Config{LogLevel: logger.Info}),
	})
	if err != nil {
		t.Fatalf("open dry-run db: %v", err)
	}
	return gormDB
}

func TestListAllByBindingUsesExactVisibleBindingAndStablePositionOrder(t *testing.T) {
	logWriter := &fileSQLLogWriter{}
	repo := &fileRepo{gormDB: newFileDryRunDB(t, logWriter)}

	if _, err := repo.ListAllByBinding(context.Background(), BusinessTypeProductImage, 42); err != nil {
		t.Fatalf("ListAllByBinding dry run: %v", err)
	}

	sql := strings.ToLower(logWriter.String())
	for _, fragment := range []string{
		"business_type = 'product_image'",
		"business_id = 42",
		"deleted_at\" is null",
		"order by position asc, id asc",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("SQL = %s, want %q", sql, fragment)
		}
	}
}

func TestListUsesStablePositionOrderForExactBinding(t *testing.T) {
	logWriter := &fileSQLLogWriter{}
	repo := &fileRepo{gormDB: newFileDryRunDB(t, logWriter)}
	businessID := uint(42)

	if _, _, err := repo.List(context.Background(), ListFilter{
		BusinessType: BusinessTypeProductImage,
		BusinessID:   &businessID,
	}, types.PageRequest{Page: 1, PageSize: 20}); err != nil {
		t.Fatalf("List dry run: %v", err)
	}

	sql := strings.ToLower(logWriter.String())
	if !strings.Contains(sql, "order by position asc, id asc") {
		t.Fatalf("SQL = %s, want stable position order", sql)
	}
}

func TestMaxPositionByBindingUsesVisibleExactBinding(t *testing.T) {
	logWriter := &fileSQLLogWriter{}
	repo := &fileRepo{gormDB: newFileDryRunDB(t, logWriter)}

	if _, err := repo.MaxPositionByBinding(context.Background(), BusinessTypeDocAttachment, 7); err != nil {
		t.Fatalf("MaxPositionByBinding dry run: %v", err)
	}

	sql := strings.ToLower(logWriter.String())
	for _, fragment := range []string{
		"coalesce(max(position), -1)",
		"business_type = 'doc_attachment'",
		"business_id = 7",
		"deleted_at\" is null",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("SQL = %s, want %q", sql, fragment)
		}
	}
}

func TestUpdatePositionsAssignsSliceOrderAndRejectsUnexpectedAffectedSet(t *testing.T) {
	logWriter := &fileSQLLogWriter{}
	repo := &fileRepo{gormDB: newFileDryRunDB(t, logWriter)}

	if err := updatePositions(repo.gormDB, BusinessTypeProductImage, 42, []uint{9, 4, 12}); err == nil {
		t.Fatal("UpdatePositions dry run returned nil with zero affected rows")
	}

	sql := strings.ToLower(logWriter.String())
	for _, fragment := range []string{
		"update \"file_attachments\" set \"position\"=case id",
		"when 9 then 0",
		"when 4 then 1",
		"when 12 then 2",
		"business_type = 'product_image'",
		"business_id = 42",
		"id in (9,4,12)",
		"deleted_at\" is null",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("SQL = %s, want %q", sql, fragment)
		}
	}
}
