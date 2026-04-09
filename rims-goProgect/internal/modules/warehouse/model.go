// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package warehouse

import "rims-go/internal/types"

// Warehouse represents a physical warehouse or storage location.
type Warehouse struct {
	types.AuditableModel
	Code          string `gorm:"uniqueIndex;size:32;not null"`
	Name          string `gorm:"size:128;not null"`
	Status        int8   `gorm:"default:1;not null"` // 1=active, 0=disabled
	Address       string `gorm:"size:255"`
	ContactPerson string `gorm:"size:64"`
	ContactPhone  string `gorm:"size:20"`
}

// TableName overrides the default table name.
func (Warehouse) TableName() string { return "warehouses" }

// UserWarehouse represents the binding between a user and a warehouse.
type UserWarehouse struct {
	types.BaseModel
	UserID      uint       `gorm:"not null;uniqueIndex:idx_user_warehouse"`
	WarehouseID uint       `gorm:"not null;uniqueIndex:idx_user_warehouse;index"`
	IsDefault   bool       `gorm:"default:false;not null"`
	Warehouse   *Warehouse `gorm:"foreignKey:WarehouseID" json:"warehouse,omitempty"`
}

// TableName overrides the default table name.
func (UserWarehouse) TableName() string { return "user_warehouses" }
