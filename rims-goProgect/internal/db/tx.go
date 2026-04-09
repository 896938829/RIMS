// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package db

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

type txKey struct{}

// RunInTx executes fn within a database transaction. The transaction handle
// is stored in the context so that repositories can pick it up via FromCtx.
func RunInTx(ctx context.Context, db *gorm.DB, fn func(ctx context.Context) error) error {
	tx := db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return fmt.Errorf("begin tx: %w", tx.Error)
	}

	txCtx := context.WithValue(ctx, txKey{}, tx)
	if err := fn(txCtx); err != nil {
		if rbErr := tx.Rollback().Error; rbErr != nil {
			return fmt.Errorf("rollback failed: %v (original: %w)", rbErr, err)
		}
		return err
	}

	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// TxRunner executes a function within a database transaction.
// In production, use NewTxRunner to create one backed by a real *gorm.DB.
// In tests, supply a pass-through implementation that simply calls fn(ctx).
type TxRunner func(ctx context.Context, fn func(ctx context.Context) error) error

// NewTxRunner creates a TxRunner backed by the given *gorm.DB.
func NewTxRunner(db *gorm.DB) TxRunner {
	return func(ctx context.Context, fn func(ctx context.Context) error) error {
		return RunInTx(ctx, db, fn)
	}
}

// FromCtx returns the transaction from context if present, otherwise the
// fallback *gorm.DB with the given context applied.
func FromCtx(ctx context.Context, fallback *gorm.DB) *gorm.DB {
	if tx, ok := ctx.Value(txKey{}).(*gorm.DB); ok {
		return tx
	}
	return fallback.WithContext(ctx)
}
