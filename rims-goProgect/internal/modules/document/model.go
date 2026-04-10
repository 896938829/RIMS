// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package document

import (
	"time"

	"rims-go/internal/types"
)

// Document type constants.
const (
	DocTypeInbound    int8 = 1 // 入库单
	DocTypeSales      int8 = 2 // 销售单
	DocTypeReturn     int8 = 3 // 退货单
	DocTypeTransfer   int8 = 4 // 调拨单
	DocTypeStocktake  int8 = 5 // 盘点单
	DocTypeConversion int8 = 6 // 非标转换单
)

// Document status constants.
const (
	StatusDraft     int8 = 1 // 草稿
	StatusCompleted int8 = 2 // 已完成

	// Stocktake-specific statuses.
	StatusStRecording int8 = 1 // 盘点中
	StatusStConfirmed int8 = 2 // 差异已确认
	StatusStSettled   int8 = 3 // 已结转
)

// Inventory transaction direction constants.
const (
	DirectionIn  int8 = 1  // 入库
	DirectionOut int8 = -1 // 出库
)

// Document number prefixes by doc type.
var docNoPrefixes = map[int8]string{
	DocTypeInbound:    "RK",
	DocTypeSales:      "XS",
	DocTypeReturn:     "TH",
	DocTypeTransfer:   "DB",
	DocTypeStocktake:  "PD",
	DocTypeConversion: "ZH",
}

// Document represents a business document (inbound, sales, return, transfer, stocktake, conversion).
type Document struct {
	types.AuditableModel
	DocNo         string     `gorm:"uniqueIndex;size:32;not null"`
	DocType       int8       `gorm:"not null;index"`
	Status        int8       `gorm:"not null;index;default:1"`
	WarehouseID   uint       `gorm:"not null;index"`
	ToWarehouseID uint       `gorm:"default:0"`
	RefDocID      uint       `gorm:"default:0;index"`
	RefDocNo      string     `gorm:"size:32;default:''"`
	Remark        string     `gorm:"size:512;default:''"`
	OperatedAt    *time.Time `gorm:"default:null"`
}

func (Document) TableName() string { return "documents" }

// DocumentLine represents a line item within a document.
type DocumentLine struct {
	types.BaseModel
	DocumentID  uint    `gorm:"not null;index"`
	ProductID   uint    `gorm:"default:0"`
	NonStdInvID uint    `gorm:"default:0"`
	ProductCode string  `gorm:"size:32;default:''"`
	ProductName string  `gorm:"size:128;default:''"`
	Quantity    int     `gorm:"not null;default:0"`
	Unit        string  `gorm:"size:16;default:''"`
	CostPrice   float64 `gorm:"type:decimal(12,2);default:0;not null"`
	RetailPrice float64 `gorm:"type:decimal(12,2);default:0;not null"`
	SystemQty   int     `gorm:"default:0;not null"`
	ActualQty   int     `gorm:"default:0;not null"`
	DiffQty     int     `gorm:"default:0;not null"`
	Remark      string  `gorm:"size:255;default:''"`
}

func (DocumentLine) TableName() string { return "document_lines" }

// InventoryTransaction records every inventory change for audit and traceability.
type InventoryTransaction struct {
	types.BaseModel
	WarehouseID uint      `gorm:"not null;index"`
	ProductID   uint      `gorm:"not null;index"`
	DocID       uint      `gorm:"not null;index"`
	DocNo       string    `gorm:"size:32;not null"`
	DocType     int8      `gorm:"not null;index"`
	Direction   int8      `gorm:"not null"`
	Quantity    int       `gorm:"not null"`
	BeforeQty   int       `gorm:"not null"`
	AfterQty    int       `gorm:"not null"`
	OperatorID  uint      `gorm:"not null"`
	OperatedAt  time.Time `gorm:"not null;index"`
}

func (InventoryTransaction) TableName() string { return "inventory_transactions" }
