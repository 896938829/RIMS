// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package user

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type sqlLogWriter struct {
	lines []string
}

func (w *sqlLogWriter) Printf(format string, args ...interface{}) {
	w.lines = append(w.lines, fmt.Sprintf(format, args...))
}

func (w *sqlLogWriter) String() string {
	return strings.Join(w.lines, "\n")
}

func TestRoleRepositoryHasPermissionFiltersSoftDeletedRole(t *testing.T) {
	logWriter := &sqlLogWriter{}
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
	repo := &roleRepo{gormDB: gormDB}

	if _, err := repo.HasPermission(context.Background(), 7, "product:create"); err != nil {
		t.Fatalf("has permission dry run: %v", err)
	}

	sql := strings.ToLower(logWriter.String())
	if !strings.Contains(sql, "join roles as r") {
		t.Fatalf("SQL = %s, want join roles", sql)
	}
	if !strings.Contains(sql, "r.deleted_at is null") {
		t.Fatalf("SQL = %s, want filter for non-deleted roles", sql)
	}
}
