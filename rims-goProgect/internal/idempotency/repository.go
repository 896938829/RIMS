// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package idempotency

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"rims-go/internal/db"
)

var ErrRecordNotFound = errors.New("idempotency record not found")
var ErrNoProcessingRecord = errors.New("idempotency processing record not found")

// Repository defines idempotency key persistence operations.
type Repository interface {
	Get(ctx context.Context, userID uint, scope, key string) (*Record, error)
	Create(ctx context.Context, record *Record) error
	Complete(ctx context.Context, userID uint, scope, key string, statusCode int, responseBody []byte) error
	DeleteProcessing(ctx context.Context, userID uint, scope, key string) error
	Delete(ctx context.Context, userID uint, scope, key string) error
}

type repository struct {
	gormDB *gorm.DB
}

// NewRepository creates a GORM-backed idempotency repository.
func NewRepository(gormDB *gorm.DB) Repository {
	return &repository{gormDB: gormDB}
}

func (r *repository) getDB(ctx context.Context) *gorm.DB {
	return db.FromCtx(ctx, r.gormDB)
}

func (r *repository) Get(ctx context.Context, userID uint, scope, key string) (*Record, error) {
	var record Record
	err := r.getDB(ctx).
		Where("user_id = ? AND scope = ? AND idempotency_key = ?", userID, scope, key).
		First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrRecordNotFound
	}
	if err != nil {
		return nil, err
	}
	return &record, nil
}

func (r *repository) Create(ctx context.Context, record *Record) error {
	return r.getDB(ctx).Create(record).Error
}

func (r *repository) Complete(ctx context.Context, userID uint, scope, key string, statusCode int, responseBody []byte) error {
	result := r.getDB(ctx).
		Model(&Record{}).
		Where("user_id = ? AND scope = ? AND idempotency_key = ? AND state = ?", userID, scope, key, StateProcessing).
		Updates(map[string]interface{}{
			"state":         StateCompleted,
			"status_code":   statusCode,
			"response_body": JSONB(responseBody),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNoProcessingRecord
	}
	return nil
}

func (r *repository) DeleteProcessing(ctx context.Context, userID uint, scope, key string) error {
	result := r.getDB(ctx).
		Where("user_id = ? AND scope = ? AND idempotency_key = ? AND state = ?", userID, scope, key, StateProcessing).
		Delete(&Record{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNoProcessingRecord
	}
	return nil
}

func (r *repository) Delete(ctx context.Context, userID uint, scope, key string) error {
	result := r.getDB(ctx).
		Where("user_id = ? AND scope = ? AND idempotency_key = ?", userID, scope, key).
		Delete(&Record{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrRecordNotFound
	}
	return nil
}
