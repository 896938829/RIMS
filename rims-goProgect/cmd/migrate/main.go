// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"rims-go/internal/config"
	"rims-go/internal/db"
	"rims-go/internal/migration"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		log.Fatalf("migrate: %v", err)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) != 1 || args[0] != "up" {
		return fmt.Errorf("usage: migrate up")
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

	return migration.NewRunner(sqlDB, cfg.MigrationsDir).Up(ctx)
}
