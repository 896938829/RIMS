// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package warehouse

import (
	"context"

	"gorm.io/gorm"

	"rims-go/internal/db"
	"rims-go/internal/types"
)

// WarehouseRepository defines data access operations for warehouses.
type WarehouseRepository interface {
	Create(ctx context.Context, w *Warehouse) error
	GetByID(ctx context.Context, id uint) (*Warehouse, error)
	GetByCode(ctx context.Context, code string) (*Warehouse, error)
	List(ctx context.Context, page types.PageRequest) ([]Warehouse, int64, error)
	ListByIDs(ctx context.Context, ids []uint) ([]Warehouse, int64, error)
	Update(ctx context.Context, w *Warehouse) error
	Delete(ctx context.Context, id uint) error
}

type warehouseRepo struct {
	gormDB *gorm.DB
}

// NewWarehouseRepository creates a new WarehouseRepository backed by GORM.
func NewWarehouseRepository(gormDB *gorm.DB) WarehouseRepository {
	return &warehouseRepo{gormDB: gormDB}
}

func (r *warehouseRepo) getDB(ctx context.Context) *gorm.DB {
	return db.FromCtx(ctx, r.gormDB)
}

func (r *warehouseRepo) Create(ctx context.Context, w *Warehouse) error {
	return r.getDB(ctx).Create(w).Error
}

func (r *warehouseRepo) GetByID(ctx context.Context, id uint) (*Warehouse, error) {
	var w Warehouse
	err := r.getDB(ctx).First(&w, id).Error
	if err != nil {
		return nil, err
	}
	return &w, nil
}

func (r *warehouseRepo) GetByCode(ctx context.Context, code string) (*Warehouse, error) {
	var w Warehouse
	err := r.getDB(ctx).Where("code = ?", code).First(&w).Error
	if err != nil {
		return nil, err
	}
	return &w, nil
}

func (r *warehouseRepo) List(ctx context.Context, page types.PageRequest) ([]Warehouse, int64, error) {
	page.Defaults()
	d := r.getDB(ctx).Model(&Warehouse{})

	if page.Keyword != "" {
		kw := "%" + page.Keyword + "%"
		d = d.Where("code LIKE ? OR name LIKE ?", kw, kw)
	}

	var total int64
	if err := d.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var warehouses []Warehouse
	if err := d.Offset(page.Offset()).Limit(page.PageSize).
		Order("id DESC").
		Find(&warehouses).Error; err != nil {
		return nil, 0, err
	}

	return warehouses, total, nil
}

func (r *warehouseRepo) ListByIDs(ctx context.Context, ids []uint) ([]Warehouse, int64, error) {
	var warehouses []Warehouse
	d := r.getDB(ctx).Where("id IN ?", ids)

	var total int64
	if err := d.Model(&Warehouse{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := d.Order("id DESC").Find(&warehouses).Error; err != nil {
		return nil, 0, err
	}

	return warehouses, total, nil
}

func (r *warehouseRepo) Update(ctx context.Context, w *Warehouse) error {
	return r.getDB(ctx).Save(w).Error
}

func (r *warehouseRepo) Delete(ctx context.Context, id uint) error {
	return r.getDB(ctx).Delete(&Warehouse{}, id).Error
}

// UserWarehouseRepository defines data access operations for user-warehouse bindings.
type UserWarehouseRepository interface {
	Create(ctx context.Context, uw *UserWarehouse) error
	Delete(ctx context.Context, userID, warehouseID uint) error
	DeleteByWarehouseID(ctx context.Context, warehouseID uint) error
	GetByUserAndWarehouse(ctx context.Context, userID, warehouseID uint) (*UserWarehouse, error)
	ListByUserID(ctx context.Context, userID uint) ([]UserWarehouse, error)
	ListByWarehouseID(ctx context.Context, warehouseID uint, page types.PageRequest) ([]WarehouseUserInfo, int64, error)
	CountActiveBindingsByWarehouseID(ctx context.Context, warehouseID uint) (int64, error)
	GetDefaultByUserID(ctx context.Context, userID uint) (*UserWarehouse, error)
	ClearDefault(ctx context.Context, userID uint) error
	SetDefault(ctx context.Context, userID, warehouseID uint) error
	CountByUserID(ctx context.Context, userID uint) (int64, error)
	GetUserRoleCode(ctx context.Context, userID uint) (string, error)

	// Middleware interface methods (primitive return types to avoid circular imports)
	GetDefaultWarehouseID(ctx context.Context, userID uint) (uint, error)
	HasAccess(ctx context.Context, userID, warehouseID uint) (bool, error)
}

type userWarehouseRepo struct {
	gormDB *gorm.DB
}

// NewUserWarehouseRepository creates a new UserWarehouseRepository backed by GORM.
func NewUserWarehouseRepository(gormDB *gorm.DB) UserWarehouseRepository {
	return &userWarehouseRepo{gormDB: gormDB}
}

func (r *userWarehouseRepo) getDB(ctx context.Context) *gorm.DB {
	return db.FromCtx(ctx, r.gormDB)
}

func (r *userWarehouseRepo) Create(ctx context.Context, uw *UserWarehouse) error {
	return r.getDB(ctx).Create(uw).Error
}

func (r *userWarehouseRepo) Delete(ctx context.Context, userID, warehouseID uint) error {
	return r.getDB(ctx).
		Where("user_id = ? AND warehouse_id = ?", userID, warehouseID).
		Delete(&UserWarehouse{}).Error
}

func (r *userWarehouseRepo) DeleteByWarehouseID(ctx context.Context, warehouseID uint) error {
	return r.getDB(ctx).
		Where("warehouse_id = ?", warehouseID).
		Delete(&UserWarehouse{}).Error
}

func (r *userWarehouseRepo) GetByUserAndWarehouse(ctx context.Context, userID, warehouseID uint) (*UserWarehouse, error) {
	var uw UserWarehouse
	err := r.getDB(ctx).
		Preload("Warehouse").
		Where("user_id = ? AND warehouse_id = ?", userID, warehouseID).
		First(&uw).Error
	if err != nil {
		return nil, err
	}
	return &uw, nil
}

func (r *userWarehouseRepo) ListByUserID(ctx context.Context, userID uint) ([]UserWarehouse, error) {
	var list []UserWarehouse
	err := r.getDB(ctx).
		Preload("Warehouse").
		Where("user_id = ?", userID).
		Order("is_default DESC, id ASC").
		Find(&list).Error
	return list, err
}

// WarehouseUserInfo is a flat struct returned by join queries to avoid cross-package model imports.
type WarehouseUserInfo struct {
	UserWarehouseID uint   `json:"userWarehouseId"`
	UserID          uint   `json:"userId"`
	Username        string `json:"username"`
	RealName        string `json:"realName"`
	IsDefault       bool   `json:"isDefault"`
}

func (r *userWarehouseRepo) ListByWarehouseID(ctx context.Context, warehouseID uint, page types.PageRequest) ([]WarehouseUserInfo, int64, error) {
	page.Defaults()
	d := r.getDB(ctx).
		Table("user_warehouses uw").
		Joins("JOIN users u ON u.id = uw.user_id AND u.deleted_at IS NULL").
		Where("uw.warehouse_id = ? AND uw.deleted_at IS NULL", warehouseID)

	if page.Keyword != "" {
		kw := "%" + page.Keyword + "%"
		d = d.Where("u.username LIKE ? OR u.real_name LIKE ?", kw, kw)
	}

	var total int64
	if err := d.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var list []WarehouseUserInfo
	if err := d.
		Select("uw.id AS user_warehouse_id, uw.user_id, u.username, u.real_name, uw.is_default").
		Offset(page.Offset()).Limit(page.PageSize).
		Order("uw.id ASC").
		Scan(&list).Error; err != nil {
		return nil, 0, err
	}

	return list, total, nil
}

func (r *userWarehouseRepo) CountActiveBindingsByWarehouseID(ctx context.Context, warehouseID uint) (int64, error) {
	var count int64
	err := r.getDB(ctx).
		Model(&UserWarehouse{}).
		Where("warehouse_id = ?", warehouseID).
		Count(&count).Error
	return count, err
}

func (r *userWarehouseRepo) GetDefaultByUserID(ctx context.Context, userID uint) (*UserWarehouse, error) {
	var uw UserWarehouse
	err := r.getDB(ctx).
		Preload("Warehouse").
		Where("user_id = ? AND is_default = ?", userID, true).
		First(&uw).Error
	if err != nil {
		return nil, err
	}
	return &uw, nil
}

func (r *userWarehouseRepo) ClearDefault(ctx context.Context, userID uint) error {
	return r.getDB(ctx).
		Model(&UserWarehouse{}).
		Where("user_id = ? AND is_default = ?", userID, true).
		Update("is_default", false).Error
}

func (r *userWarehouseRepo) SetDefault(ctx context.Context, userID, warehouseID uint) error {
	return r.getDB(ctx).
		Model(&UserWarehouse{}).
		Where("user_id = ? AND warehouse_id = ?", userID, warehouseID).
		Update("is_default", true).Error
}

func (r *userWarehouseRepo) CountByUserID(ctx context.Context, userID uint) (int64, error) {
	var count int64
	err := r.getDB(ctx).Model(&UserWarehouse{}).Where("user_id = ?", userID).Count(&count).Error
	return count, err
}

// GetUserRoleCode fetches a user's role code via join, avoiding cross-package model imports.
func (r *userWarehouseRepo) GetUserRoleCode(ctx context.Context, userID uint) (string, error) {
	var roleCode string
	err := r.getDB(ctx).
		Table("users u").
		Joins("JOIN roles r ON r.id = u.role_id").
		Where("u.id = ? AND u.deleted_at IS NULL", userID).
		Select("r.code").
		Scan(&roleCode).Error
	return roleCode, err
}

// GetDefaultWarehouseID returns the default warehouse ID for a user (middleware interface).
func (r *userWarehouseRepo) GetDefaultWarehouseID(ctx context.Context, userID uint) (uint, error) {
	var warehouseID uint
	err := r.getDB(ctx).
		Model(&UserWarehouse{}).
		Where("user_id = ? AND is_default = ?", userID, true).
		Select("warehouse_id").
		Scan(&warehouseID).Error
	return warehouseID, err
}

// HasAccess checks whether a user has access to a given warehouse (middleware interface).
func (r *userWarehouseRepo) HasAccess(ctx context.Context, userID, warehouseID uint) (bool, error) {
	var count int64
	err := r.getDB(ctx).
		Model(&UserWarehouse{}).
		Where("user_id = ? AND warehouse_id = ?", userID, warehouseID).
		Count(&count).Error
	return count > 0, err
}
