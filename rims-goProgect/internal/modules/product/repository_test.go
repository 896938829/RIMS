// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package product

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"rims-go/internal/types"
)

type productSQLLogWriter struct {
	lines []string
}

func (w *productSQLLogWriter) Printf(format string, args ...interface{}) {
	w.lines = append(w.lines, fmt.Sprintf(format, args...))
}

func (w *productSQLLogWriter) String() string {
	return strings.Join(w.lines, "\n")
}

func TestInventoryRepositoryListByWarehouseAppliesKeywordToRowsAndTotal(t *testing.T) {
	logWriter := &productSQLLogWriter{}
	gormDB, err := gorm.Open(postgres.New(postgres.Config{
		DSN: "host=127.0.0.1 user=rims password=rims dbname=rims sslmode=disable",
	}), &gorm.Config{
		DryRun:               true,
		DisableAutomaticPing: true,
		Logger:               logger.New(logWriter, logger.Config{LogLevel: logger.Info}),
	})
	if err != nil {
		t.Fatalf("open dry-run db: %v", err)
	}
	repo := &inventoryRepo{gormDB: gormDB}

	if _, _, err := repo.ListByWarehouse(context.Background(), 1, types.PageRequest{
		Page:     1,
		PageSize: 20,
		Keyword:  "sale_1776407432915",
	}); err != nil {
		t.Fatalf("ListByWarehouse dry run: %v", err)
	}

	sql := strings.ToLower(logWriter.String())
	if strings.Count(sql, "join products") < 2 {
		t.Fatalf("SQL = %s, want keyword product join in both count and list queries", sql)
	}
	if strings.Count(sql, "products.code like") < 2 {
		t.Fatalf("SQL = %s, want keyword product filters in both count and list queries", sql)
	}
}
