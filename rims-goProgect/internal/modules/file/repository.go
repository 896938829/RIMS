// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package file

import (
	"context"

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
	if err := d.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var list []FileAttachment
	if err := d.Offset(page.Offset()).Limit(page.PageSize).
		Order("id DESC").
		Find(&list).Error; err != nil {
		return nil, 0, err
	}

	return list, total, nil
}

func (r *fileRepo) SoftDelete(ctx context.Context, id uint) error {
	return r.getDB(ctx).Delete(&FileAttachment{}, id).Error
}
