// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package audit

import (
	"context"
	"time"

	"gorm.io/gorm"

	"rims-go/internal/db"
	"rims-go/internal/types"
)

// ListFilter holds resolved query conditions for AuditRepository.List.
// Pointer fields distinguish "not filtered" from "filter by zero".
type ListFilter struct {
	UserID      *uint
	WarehouseID *uint
	Resource    string
	ResourceID  *uint
	Action      string
	DocNo       string
	Result      string
	StartTime   *time.Time
	EndTime     *time.Time
	Keyword     string
}

// AuditRepository defines data-access operations for audit log rows.
// The repository intentionally exposes no Update or hard-Delete: audit
// records are append-only. Retention cleanup is a scheduled job concern.
type AuditRepository interface {
	Create(ctx context.Context, l *AuditLog) error
	GetByID(ctx context.Context, id uint) (*AuditLog, error)
	List(ctx context.Context, filter ListFilter, page types.PageRequest) ([]AuditLog, int64, error)
}

type auditRepo struct {
	gormDB *gorm.DB
}

// NewAuditRepository returns a GORM-backed AuditRepository.
func NewAuditRepository(gormDB *gorm.DB) AuditRepository {
	return &auditRepo{gormDB: gormDB}
}

// getDB resolves to the active transactional *gorm.DB from ctx when a caller
// is running inside db.RunInTx; otherwise it returns the base handle. This is
// the hook that lets audit writes commit/rollback atomically with the outer
// business transaction.
func (r *auditRepo) getDB(ctx context.Context) *gorm.DB {
	return db.FromCtx(ctx, r.gormDB)
}

func (r *auditRepo) Create(ctx context.Context, l *AuditLog) error {
	return r.getDB(ctx).Create(l).Error
}

func (r *auditRepo) GetByID(ctx context.Context, id uint) (*AuditLog, error) {
	var l AuditLog
	if err := r.getDB(ctx).First(&l, id).Error; err != nil {
		return nil, err
	}
	return &l, nil
}

func (r *auditRepo) List(ctx context.Context, filter ListFilter, page types.PageRequest) ([]AuditLog, int64, error) {
	page.Defaults()
	d := r.getDB(ctx).Model(&AuditLog{})

	if filter.UserID != nil {
		d = d.Where("user_id = ?", *filter.UserID)
	}
	if filter.WarehouseID != nil {
		d = d.Where("warehouse_id = ?", *filter.WarehouseID)
	}
	if filter.Resource != "" {
		d = d.Where("resource = ?", filter.Resource)
	}
	if filter.ResourceID != nil {
		d = d.Where("resource_id = ?", *filter.ResourceID)
	}
	if filter.Action != "" {
		d = d.Where("action = ?", filter.Action)
	}
	if filter.DocNo != "" {
		d = d.Where("doc_no = ?", filter.DocNo)
	}
	if filter.Result != "" {
		d = d.Where("result = ?", filter.Result)
	}
	if filter.StartTime != nil {
		d = d.Where("created_at >= ?", *filter.StartTime)
	}
	if filter.EndTime != nil {
		d = d.Where("created_at < ?", *filter.EndTime)
	}
	if filter.Keyword != "" {
		like := "%" + filter.Keyword + "%"
		d = d.Where("description ILIKE ? OR username ILIKE ? OR doc_no ILIKE ?", like, like, like)
	}

	var total int64
	if err := d.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var list []AuditLog
	if err := d.Offset(page.Offset()).Limit(page.PageSize).
		Order("id DESC").
		Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}
