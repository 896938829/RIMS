// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"rims-go/internal/config"
	"rims-go/internal/db"
	"rims-go/internal/idempotency"
	"rims-go/internal/modules/audit"
	"rims-go/internal/modules/document"
	"rims-go/internal/modules/file"
	"rims-go/internal/modules/product"
	"rims-go/internal/modules/user"
	"rims-go/internal/modules/warehouse"
)

// @title RIMS API
// @version 1.0
// @description 零售端库存管理系统 API
// @BasePath /
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization

// Run boots the HTTP server with environment-based configuration.
func Run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	gormDB, err := db.New(cfg)
	if err != nil {
		return fmt.Errorf("init db: %w", err)
	}

	if cfg.DBAutoMigrate {
		if err := gormDB.AutoMigrate(
			&user.Role{},
			&user.Permission{},
			&user.User{},
			&warehouse.Warehouse{},
			&warehouse.UserWarehouse{},
			&product.Product{},
			&product.Inventory{},
			&product.NonStdInventory{},
			&document.Document{},
			&document.DocumentLine{},
			&document.InventoryTransaction{},
			&file.FileAttachment{},
			&audit.AuditLog{},
			&idempotency.Record{},
		); err != nil {
			return fmt.Errorf("auto migrate: %w", err)
		}
	}

	addr := fmt.Sprintf(":%s", cfg.AppPort)
	server := &http.Server{
		Addr:         addr,
		Handler:      buildRouter(cfg, gormDB),
		ReadTimeout:  time.Duration(cfg.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(cfg.WriteTimeout) * time.Second,
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)

	return serveWithGracefulShutdown(server, server.ListenAndServe, signals, 10*time.Second)
}

func serveWithGracefulShutdown(server *http.Server, serve func() error, signals <-chan os.Signal, timeout time.Duration) error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- serve()
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-signals:
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		return err
	}

	err := <-errCh
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
