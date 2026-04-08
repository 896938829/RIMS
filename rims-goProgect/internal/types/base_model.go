// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package types

import (
	"time"

	"gorm.io/gorm"
)

// BaseModel provides common fields for all GORM entities.
type BaseModel struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// AuditableModel extends BaseModel with operator tracking fields.
type AuditableModel struct {
	BaseModel
	CreatedBy uint `gorm:"not null;default:0" json:"createdBy"`
	UpdatedBy uint `gorm:"not null;default:0" json:"updatedBy"`
}
