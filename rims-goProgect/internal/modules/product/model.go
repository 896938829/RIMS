// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package product

import "rims-go/internal/types"

// Product represents a product in the catalog (global, not warehouse-scoped).
type Product struct {
	types.AuditableModel
	Code        string  `gorm:"uniqueIndex;size:32;not null"`
	Name        string  `gorm:"size:128;not null"`
	Category    string  `gorm:"size:64;index"`
	Spec        string  `gorm:"size:128"`
	Unit        string  `gorm:"size:16;not null"`
	Barcode     string  `gorm:"size:64"`
	RetailPrice float64 `gorm:"type:decimal(12,2);default:0;not null"`
	CostPrice   float64 `gorm:"type:decimal(12,2);default:0;not null"`
	ImageURL    string  `gorm:"size:512"`
	Status      int8    `gorm:"default:1;not null;index"` // 1=active, 0=disabled
}

// TableName overrides the default table name.
func (Product) TableName() string { return "products" }

// Inventory represents standard inventory per warehouse per product.
type Inventory struct {
	types.AuditableModel
	WarehouseID    uint     `gorm:"not null;uniqueIndex:idx_inv_wh_product;index"`
	ProductID      uint     `gorm:"not null;uniqueIndex:idx_inv_wh_product;index"`
	Quantity       int      `gorm:"default:0;not null"`
	LockedQty      int      `gorm:"default:0;not null"`
	AlertThreshold int      `gorm:"default:0;not null"`
	Status         int8     `gorm:"default:1;not null;index"` // 1=normal, 2=low stock, 0=disabled
	Product        *Product `gorm:"foreignKey:ProductID" json:"product,omitempty"`
}

// TableName overrides the default table name.
func (Inventory) TableName() string { return "inventories" }

// NonStdInventory represents non-standard inventory (permission-protected, per warehouse).
type NonStdInventory struct {
	types.AuditableModel
	WarehouseID    uint   `gorm:"not null;index"`
	TempLabel      string `gorm:"size:64;not null"`
	Description    string `gorm:"size:255;not null"`
	Unit           string `gorm:"size:16;not null"`
	Quantity       int    `gorm:"not null"`
	ConvertedQty   int    `gorm:"default:0;not null"`
	SourceMethod   string `gorm:"size:32"`
	SourceDocument string `gorm:"size:64"`
	Status         int8   `gorm:"default:1;not null;index"` // 1=active, 2=partial converted, 3=fully converted, 0=cancelled
}

// TableName overrides the default table name.
func (NonStdInventory) TableName() string { return "non_std_inventories" }
