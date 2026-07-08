// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package product

import (
	"context"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"rims-go/internal/db"
	"rims-go/internal/types"
)

// --- Product Repository ---

// ProductRepository defines data access operations for products.
type ProductRepository interface {
	Create(ctx context.Context, p *Product) error
	GetByID(ctx context.Context, id uint) (*Product, error)
	GetByCode(ctx context.Context, code string) (*Product, error)
	GetByBarcode(ctx context.Context, barcode string) (*Product, error)
	List(ctx context.Context, page types.PageRequest) ([]Product, int64, error)
	Update(ctx context.Context, p *Product) error
	Delete(ctx context.Context, id uint) error
	CountDocumentLinesByProductID(ctx context.Context, productID uint) (int64, error)
}

type productRepo struct {
	gormDB *gorm.DB
}

// NewProductRepository creates a new ProductRepository backed by GORM.
func NewProductRepository(gormDB *gorm.DB) ProductRepository {
	return &productRepo{gormDB: gormDB}
}

func (r *productRepo) getDB(ctx context.Context) *gorm.DB {
	return db.FromCtx(ctx, r.gormDB)
}

func (r *productRepo) Create(ctx context.Context, p *Product) error {
	return r.getDB(ctx).Create(p).Error
}

func (r *productRepo) GetByID(ctx context.Context, id uint) (*Product, error) {
	var p Product
	err := r.getDB(ctx).First(&p, id).Error
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *productRepo) GetByCode(ctx context.Context, code string) (*Product, error) {
	var p Product
	err := r.getDB(ctx).Where("code = ?", code).First(&p).Error
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *productRepo) GetByBarcode(ctx context.Context, barcode string) (*Product, error) {
	var p Product
	err := r.getDB(ctx).Where("barcode = ?", barcode).First(&p).Error
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *productRepo) List(ctx context.Context, page types.PageRequest) ([]Product, int64, error) {
	page.Defaults()
	d := r.getDB(ctx).Model(&Product{})

	if page.Keyword != "" {
		kw := "%" + page.Keyword + "%"
		d = d.Where("code LIKE ? OR name LIKE ? OR barcode LIKE ? OR category LIKE ?", kw, kw, kw, kw)
	}

	var total int64
	if err := d.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var products []Product
	if err := d.Offset(page.Offset()).Limit(page.PageSize).
		Order("id DESC").
		Find(&products).Error; err != nil {
		return nil, 0, err
	}

	return products, total, nil
}

func (r *productRepo) Update(ctx context.Context, p *Product) error {
	return r.getDB(ctx).Save(p).Error
}

func (r *productRepo) Delete(ctx context.Context, id uint) error {
	return r.getDB(ctx).Delete(&Product{}, id).Error
}

func (r *productRepo) CountDocumentLinesByProductID(ctx context.Context, productID uint) (int64, error) {
	var count int64
	err := r.getDB(ctx).
		Table("document_lines").
		Where("product_id = ? AND deleted_at IS NULL", productID).
		Count(&count).Error
	return count, err
}

// --- Inventory Repository ---

// InventoryRepository defines data access operations for standard inventory.
type InventoryRepository interface {
	Create(ctx context.Context, inv *Inventory) error
	GetByID(ctx context.Context, id uint) (*Inventory, error)
	LockItem(ctx context.Context, warehouseID, productID uint) error
	GetByWarehouseAndProductForUpdate(ctx context.Context, warehouseID, productID uint) (*Inventory, error)
	GetByWarehouseAndProduct(ctx context.Context, warehouseID, productID uint) (*Inventory, error)
	ExistsByProductID(ctx context.Context, productID uint) (bool, error)
	ListByWarehouse(ctx context.Context, warehouseID uint, page types.PageRequest) ([]Inventory, int64, error)
	ListAlerts(ctx context.Context, warehouseID uint, page types.PageRequest) ([]Inventory, int64, error)
	Update(ctx context.Context, inv *Inventory) error
	UpdateSettings(ctx context.Context, id uint, alertThreshold *int, status *int8, updatedBy uint) error
	Delete(ctx context.Context, id uint) error
}

type inventoryRepo struct {
	gormDB *gorm.DB
}

// NewInventoryRepository creates a new InventoryRepository backed by GORM.
func NewInventoryRepository(gormDB *gorm.DB) InventoryRepository {
	return &inventoryRepo{gormDB: gormDB}
}

func (r *inventoryRepo) getDB(ctx context.Context) *gorm.DB {
	return db.FromCtx(ctx, r.gormDB)
}

func (r *inventoryRepo) Create(ctx context.Context, inv *Inventory) error {
	return r.getDB(ctx).Create(inv).Error
}

func (r *inventoryRepo) GetByID(ctx context.Context, id uint) (*Inventory, error) {
	var inv Inventory
	err := r.getDB(ctx).Preload("Product").First(&inv, id).Error
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

func (r *inventoryRepo) LockItem(ctx context.Context, warehouseID, productID uint) error {
	lockKey := fmt.Sprintf("%d:%d", warehouseID, productID)
	return r.getDB(ctx).
		Exec("SELECT pg_advisory_xact_lock(hashtext(?), hashtext(?))", "rims_inventory", lockKey).
		Error
}

func (r *inventoryRepo) GetByWarehouseAndProductForUpdate(ctx context.Context, warehouseID, productID uint) (*Inventory, error) {
	var inv Inventory
	err := r.getDB(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("warehouse_id = ? AND product_id = ?", warehouseID, productID).
		First(&inv).Error
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

func (r *inventoryRepo) GetByWarehouseAndProduct(ctx context.Context, warehouseID, productID uint) (*Inventory, error) {
	var inv Inventory
	err := r.getDB(ctx).
		Where("warehouse_id = ? AND product_id = ?", warehouseID, productID).
		First(&inv).Error
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

func (r *inventoryRepo) ExistsByProductID(ctx context.Context, productID uint) (bool, error) {
	var count int64
	err := r.getDB(ctx).Model(&Inventory{}).Where("product_id = ?", productID).Count(&count).Error
	return count > 0, err
}

func (r *inventoryRepo) ListByWarehouse(ctx context.Context, warehouseID uint, page types.PageRequest) ([]Inventory, int64, error) {
	page.Defaults()
	d := r.getDB(ctx).Model(&Inventory{}).Where("warehouse_id = ?", warehouseID)

	if page.Keyword != "" {
		kw := "%" + page.Keyword + "%"
		d = d.Joins("JOIN products ON products.id = inventories.product_id AND products.deleted_at IS NULL").
			Where("products.code LIKE ? OR products.name LIKE ? OR products.barcode LIKE ?", kw, kw, kw)
	}

	var total int64
	if err := d.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var inventories []Inventory
	if err := d.Preload("Product").
		Offset(page.Offset()).Limit(page.PageSize).
		Order("inventories.id DESC").
		Find(&inventories).Error; err != nil {
		return nil, 0, err
	}

	return inventories, total, nil
}

func (r *inventoryRepo) ListAlerts(ctx context.Context, warehouseID uint, page types.PageRequest) ([]Inventory, int64, error) {
	page.Defaults()
	d := r.getDB(ctx).Model(&Inventory{}).
		Where("warehouse_id = ? AND alert_threshold > 0 AND quantity <= alert_threshold", warehouseID)

	var total int64
	if err := d.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var inventories []Inventory
	if err := r.getDB(ctx).Preload("Product").
		Where("warehouse_id = ? AND alert_threshold > 0 AND quantity <= alert_threshold", warehouseID).
		Offset(page.Offset()).Limit(page.PageSize).
		Order("inventories.quantity ASC").
		Find(&inventories).Error; err != nil {
		return nil, 0, err
	}

	return inventories, total, nil
}

func (r *inventoryRepo) Update(ctx context.Context, inv *Inventory) error {
	return r.getDB(ctx).Save(inv).Error
}

func (r *inventoryRepo) UpdateSettings(ctx context.Context, id uint, alertThreshold *int, status *int8, updatedBy uint) error {
	updates := map[string]any{
		"updated_by": updatedBy,
	}
	if alertThreshold != nil {
		updates["alert_threshold"] = *alertThreshold
	}
	if status != nil {
		updates["status"] = *status
	} else if alertThreshold != nil {
		threshold := *alertThreshold
		updates["status"] = gorm.Expr(
			"CASE WHEN ? > 0 AND quantity <= ? THEN ? WHEN status = ? THEN ? ELSE status END",
			threshold, threshold, int8(2), int8(2), int8(1),
		)
	}
	return r.getDB(ctx).Model(&Inventory{}).Where("id = ?", id).Updates(updates).Error
}

func (r *inventoryRepo) Delete(ctx context.Context, id uint) error {
	return r.getDB(ctx).Delete(&Inventory{}, id).Error
}

// --- Non-Standard Inventory Repository ---

// NonStdInventoryRepository defines data access operations for non-standard inventory.
type NonStdInventoryRepository interface {
	Create(ctx context.Context, ns *NonStdInventory) error
	GetByID(ctx context.Context, id uint) (*NonStdInventory, error)
	GetByIDForUpdate(ctx context.Context, id uint) (*NonStdInventory, error)
	GetByTempLabel(ctx context.Context, tempLabel string) (*NonStdInventory, error)
	ListByWarehouse(ctx context.Context, warehouseID uint, page types.PageRequest) ([]NonStdInventory, int64, error)
	Update(ctx context.Context, ns *NonStdInventory) error
	Delete(ctx context.Context, id uint) error
}

type nonStdRepo struct {
	gormDB *gorm.DB
}

// NewNonStdInventoryRepository creates a new NonStdInventoryRepository backed by GORM.
func NewNonStdInventoryRepository(gormDB *gorm.DB) NonStdInventoryRepository {
	return &nonStdRepo{gormDB: gormDB}
}

func (r *nonStdRepo) getDB(ctx context.Context) *gorm.DB {
	return db.FromCtx(ctx, r.gormDB)
}

func (r *nonStdRepo) Create(ctx context.Context, ns *NonStdInventory) error {
	return r.getDB(ctx).Create(ns).Error
}

func (r *nonStdRepo) GetByID(ctx context.Context, id uint) (*NonStdInventory, error) {
	var ns NonStdInventory
	err := r.getDB(ctx).First(&ns, id).Error
	if err != nil {
		return nil, err
	}
	return &ns, nil
}

func (r *nonStdRepo) GetByIDForUpdate(ctx context.Context, id uint) (*NonStdInventory, error) {
	var ns NonStdInventory
	err := r.getDB(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&ns, id).Error
	if err != nil {
		return nil, err
	}
	return &ns, nil
}

func (r *nonStdRepo) GetByTempLabel(ctx context.Context, tempLabel string) (*NonStdInventory, error) {
	var ns NonStdInventory
	err := r.getDB(ctx).Where("temp_label = ?", tempLabel).First(&ns).Error
	if err != nil {
		return nil, err
	}
	return &ns, nil
}

func (r *nonStdRepo) ListByWarehouse(ctx context.Context, warehouseID uint, page types.PageRequest) ([]NonStdInventory, int64, error) {
	page.Defaults()
	d := r.getDB(ctx).Model(&NonStdInventory{}).Where("warehouse_id = ?", warehouseID)

	if page.Keyword != "" {
		kw := "%" + page.Keyword + "%"
		d = d.Where("temp_label LIKE ? OR description LIKE ?", kw, kw)
	}

	var total int64
	if err := d.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var list []NonStdInventory
	if err := d.Offset(page.Offset()).Limit(page.PageSize).
		Order("id DESC").
		Find(&list).Error; err != nil {
		return nil, 0, err
	}

	return list, total, nil
}

func (r *nonStdRepo) Update(ctx context.Context, ns *NonStdInventory) error {
	return r.getDB(ctx).Save(ns).Error
}

func (r *nonStdRepo) Delete(ctx context.Context, id uint) error {
	return r.getDB(ctx).Delete(&NonStdInventory{}, id).Error
}
