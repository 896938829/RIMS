// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"rims-go/internal/config"
	"rims-go/internal/db"
	"rims-go/internal/maintenance"
	"rims-go/internal/modules/file"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		log.Fatalf("cleanup: %v", err)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("usage: cleanup")
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	gormDB, err := db.New(cfg)
	if err != nil {
		return fmt.Errorf("init db: %w", err)
	}
	sqlDB, err := gormDB.DB()
	if err != nil {
		return fmt.Errorf("unwrap sql db: %w", err)
	}
	defer sqlDB.Close()

	uploadDir := cfg.UploadDir
	if uploadDir == "" {
		uploadDir = "./uploads"
	}
	storage, err := file.NewLocalStorage(uploadDir, "/uploads")
	if err != nil {
		return fmt.Errorf("init file storage: %w", err)
	}

	result, err := maintenance.NewCleaner(sqlDB, storage, maintenance.Options{
		IdempotencyKeyTTL:    time.Duration(cfg.IdempotencyKeyTTLHours) * time.Hour,
		FileDeletedRetention: time.Duration(cfg.FileDeletedRetentionDays) * 24 * time.Hour,
		AuditLogRetention:    time.Duration(cfg.AuditLogRetentionDays) * 24 * time.Hour,
		BatchSize:            cfg.CleanupBatchSize,
	}).Run(ctx)
	if err != nil {
		return err
	}
	log.Printf(
		"cleanup completed: idempotency_keys=%d file_objects=%d file_metadata=%d audit_logs=%d",
		result.IdempotencyKeysDeleted,
		result.FileObjectsDeleted,
		result.FileMetadataDeleted,
		result.AuditLogsDeleted,
	)
	return nil
}
