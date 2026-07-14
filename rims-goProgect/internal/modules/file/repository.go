// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package file

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"rims-go/internal/db"
	"rims-go/internal/types"
)

// ListFilter holds conditions for listing file attachments.
type ListFilter struct {
	BusinessType string
	BusinessID   *uint
}

// FileRepository defines data access operations for file attachments.
type FileRepository interface {
	Create(ctx context.Context, f *FileAttachment) error
	Update(ctx context.Context, f *FileAttachment) error
	GetByID(ctx context.Context, id uint) (*FileAttachment, error)
	GetByHash(ctx context.Context, hash string) (*FileAttachment, error)
	List(ctx context.Context, filter ListFilter, page types.PageRequest) ([]FileAttachment, int64, error)
	SoftDelete(ctx context.Context, id uint) error
}

type fileRepo struct {
	gormDB *gorm.DB
}

// NewFileRepository creates a new FileRepository backed by GORM.
func NewFileRepository(gormDB *gorm.DB) FileRepository {
	return &fileRepo{gormDB: gormDB}
}

func (r *fileRepo) getDB(ctx context.Context) *gorm.DB {
	return db.FromCtx(ctx, r.gormDB)
}

func (r *fileRepo) Create(ctx context.Context, f *FileAttachment) error {
	return r.getDB(ctx).Create(f).Error
}

// IsFixtureAttachmentBinding reports whether a binding belongs to disposable
// local M9 test data. Only those bindings may enter the reserved object-key namespace.
func (r *fileRepo) IsFixtureAttachmentBinding(ctx context.Context, businessType string, businessID uint) (bool, error) {
	if businessType != BusinessTypeDocAttachment || businessID == 0 {
		return false, nil
	}
	var fixture bool
	err := r.getDB(ctx).Raw(`
SELECT EXISTS (
  SELECT 1
  FROM documents
  WHERE id = ?
    AND (doc_no LIKE 'M9DOC%' OR remark LIKE 'M9-E2E:%')
)`, businessID).Scan(&fixture).Error
	return fixture, err
}

func (r *fileRepo) Update(ctx context.Context, f *FileAttachment) error {
	return r.getDB(ctx).Save(f).Error
}

func (r *fileRepo) GetByID(ctx context.Context, id uint) (*FileAttachment, error) {
	var f FileAttachment
	if err := r.getDB(ctx).First(&f, id).Error; err != nil {
		return nil, err
	}
	return &f, nil
}

func (r *fileRepo) GetByHash(ctx context.Context, hash string) (*FileAttachment, error) {
	var f FileAttachment
	if err := r.getDB(ctx).Where("file_hash = ?", hash).First(&f).Error; err != nil {
		return nil, err
	}
	return &f, nil
}

func (r *fileRepo) List(ctx context.Context, filter ListFilter, page types.PageRequest) ([]FileAttachment, int64, error) {
	page.Defaults()
	d := r.getDB(ctx).Model(&FileAttachment{})

	if filter.BusinessType != "" {
		d = d.Where("business_type = ?", filter.BusinessType)
	}
	if filter.BusinessID != nil {
		d = d.Where("business_id = ?", *filter.BusinessID)
	}

	var total int64
	if err := d.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var list []FileAttachment
	order := "id DESC"
	if filter.BusinessType != "" && filter.BusinessID != nil {
		order = "position ASC, id ASC"
	}
	if err := d.Session(&gorm.Session{}).Offset(page.Offset()).Limit(page.PageSize).
		Order(order).
		Find(&list).Error; err != nil {
		return nil, 0, err
	}

	return list, total, nil
}

func (r *fileRepo) SoftDelete(ctx context.Context, id uint) error {
	return r.getDB(ctx).Delete(&FileAttachment{}, id).Error
}

func (r *fileRepo) CountByBinding(ctx context.Context, businessType string, businessID uint) (int64, error) {
	var count int64
	err := r.getDB(ctx).Model(&FileAttachment{}).
		Where("business_type = ? AND business_id = ?", businessType, businessID).
		Count(&count).Error
	return count, err
}

// ListAllByBinding returns the complete visible attachment set for one binding.
func (r *fileRepo) ListAllByBinding(ctx context.Context, businessType string, businessID uint) ([]FileAttachment, error) {
	var files []FileAttachment
	err := r.getDB(ctx).
		Where("business_type = ? AND business_id = ?", businessType, businessID).
		Order("position ASC, id ASC").
		Find(&files).Error
	return files, err
}

// MaxPositionByBinding returns -1 when a binding has no visible attachments.
func (r *fileRepo) MaxPositionByBinding(ctx context.Context, businessType string, businessID uint) (int, error) {
	var row struct {
		Position int
	}
	err := r.getDB(ctx).Model(&FileAttachment{}).
		Select("COALESCE(MAX(position), -1) AS position").
		Where("business_type = ? AND business_id = ?", businessType, businessID).
		Find(&row).Error
	return row.Position, err
}

// UpdatePositions atomically assigns positions according to fileIDs order.
func (r *fileRepo) UpdatePositions(ctx context.Context, businessType string, businessID uint, fileIDs []uint) error {
	if len(fileIDs) == 0 {
		return nil
	}

	db := r.getDB(ctx)
	update := func(tx *gorm.DB) error {
		return updatePositions(tx, businessType, businessID, fileIDs)
	}
	if db.DryRun {
		return update(db)
	}
	return db.Transaction(update)
}

func updatePositions(tx *gorm.DB, businessType string, businessID uint, fileIDs []uint) error {
	caseParts := make([]string, 0, len(fileIDs))
	caseArgs := make([]interface{}, 0, len(fileIDs)*2)
	for position, id := range fileIDs {
		caseParts = append(caseParts, "WHEN ? THEN ?")
		caseArgs = append(caseArgs, id, position)
	}

	result := tx.Model(&FileAttachment{}).
		Where("business_type = ? AND business_id = ?", businessType, businessID).
		Where("id IN ?", fileIDs).
		Update("position", gorm.Expr("CASE id "+strings.Join(caseParts, " ")+" ELSE position END", caseArgs...))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != int64(len(fileIDs)) {
		return fmt.Errorf("update attachment positions: affected %d rows, want %d", result.RowsAffected, len(fileIDs))
	}
	return nil
}
