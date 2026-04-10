// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package report

import (
	"fmt"
	"time"

	"rims-go/internal/types"
)

const dateLayout = "2006-01-02"

// TimeRangeRequest is embedded by every report request that filters by date.
type TimeRangeRequest struct {
	StartDate string `form:"startDate" binding:"required,datetime=2006-01-02"`
	EndDate   string `form:"endDate"   binding:"required,datetime=2006-01-02,gtefield=StartDate"`
}

// Parse returns the start time (inclusive) and the exclusive end time
// (end date + 24h) in UTC. It also validates end >= start.
func (r TimeRangeRequest) Parse() (time.Time, time.Time, error) {
	start, err := time.ParseInLocation(dateLayout, r.StartDate, time.UTC)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid startDate: %w", err)
	}
	end, err := time.ParseInLocation(dateLayout, r.EndDate, time.UTC)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid endDate: %w", err)
	}
	if end.Before(start) {
		return time.Time{}, time.Time{}, fmt.Errorf("endDate must be on or after startDate")
	}
	return start, end.Add(24 * time.Hour), nil
}

// --- Sales stats ---

type SalesStatRequest struct {
	TimeRangeRequest
}

type SalesStatResponse struct {
	Revenue     float64  `json:"revenue"`
	OrderCount  int64    `json:"orderCount"`
	SKUCount    int64    `json:"skuCount"`
	Quantity    int64    `json:"quantity"`
	CostAmount  *float64 `json:"costAmount,omitempty"`  // admin-only
	GrossProfit *float64 `json:"grossProfit,omitempty"` // admin-only
}

// SalesStatRaw is the package-private flat struct that the repository scans into.
type SalesStatRaw struct {
	Revenue    float64 `gorm:"column:revenue"`
	CostAmount float64 `gorm:"column:cost_amount"`
	OrderCount int64   `gorm:"column:order_count"`
	SKUCount   int64   `gorm:"column:sku_count"`
	Quantity   int64   `gorm:"column:quantity"`
}

// --- Sales trend ---

type SalesTrendRequest struct {
	TimeRangeRequest
	Bucket string `form:"bucket" binding:"required,oneof=day week month"`
}

type SalesTrendPoint struct {
	Period     string  `json:"period"`
	Revenue    float64 `json:"revenue"`
	OrderCount int64   `json:"orderCount"`
	Quantity   int64   `json:"quantity"`
}

type SalesTrendResponse struct {
	List []SalesTrendPoint `json:"list"`
}

// SalesTrendRaw is the repository-scan result for a trend bucket.
type SalesTrendRaw struct {
	BucketTS   time.Time `gorm:"column:bucket_ts"`
	Revenue    float64   `gorm:"column:revenue"`
	OrderCount int64     `gorm:"column:order_count"`
	Quantity   int64     `gorm:"column:quantity"`
}

// --- Product ranking ---

type ProductRankRequest struct {
	TimeRangeRequest
	Metric string `form:"metric" binding:"required,oneof=qty amount"`
	Limit  int    `form:"limit"  binding:"omitempty,min=1,max=100"`
}

type ProductRankItem struct {
	ProductID   uint     `json:"productId"    gorm:"column:product_id"`
	ProductCode string   `json:"productCode"  gorm:"column:product_code"`
	ProductName string   `json:"productName"  gorm:"column:product_name"`
	Quantity    int64    `json:"quantity"     gorm:"column:quantity"`
	Amount      float64  `json:"amount"       gorm:"column:amount"`
	CostAmount  float64  `json:"-"            gorm:"column:cost_amount"`
	GrossProfit *float64 `json:"grossProfit,omitempty"`
}

type ProductRankResponse struct {
	List []ProductRankItem `json:"list"`
}

// --- Inventory overview ---

type InventoryOverviewResponse struct {
	SKUCount      int64    `json:"skuCount"`
	TotalQty      int64    `json:"totalQty"`
	LowStockCount int64    `json:"lowStockCount"`
	TotalValue    *float64 `json:"totalValue,omitempty"` // admin-only
}

type InventoryOverviewRaw struct {
	SKUCount      int64   `gorm:"column:sku_count"`
	TotalQty      int64   `gorm:"column:total_qty"`
	TotalValue    float64 `gorm:"column:total_value"`
	LowStockCount int64   `gorm:"column:low_stock_count"`
}

// --- Inventory turnover ---

type TurnoverRequest struct {
	TimeRangeRequest
	Limit int `form:"limit" binding:"omitempty,min=1,max=100"`
}

type TurnoverItem struct {
	ProductID    uint    `json:"productId"    gorm:"column:product_id"`
	ProductCode  string  `json:"productCode"  gorm:"column:product_code"`
	ProductName  string  `json:"productName"  gorm:"column:product_name"`
	OutboundQty  int64   `json:"outboundQty"  gorm:"column:outbound_qty"`
	AvgStock     float64 `json:"avgStock"     gorm:"column:avg_stock"`
	TurnoverRate float64 `json:"turnoverRate"`
}

type TurnoverResponse struct {
	List []TurnoverItem `json:"list"`
}

// --- Slow-moving alert ---

type SlowMovingRequest struct {
	TimeRangeRequest
	types.PageRequest
	MaxSales int `form:"maxSales" binding:"omitempty,min=0"`
}

type SlowMovingItem struct {
	ProductID    uint       `json:"productId"    gorm:"column:product_id"`
	ProductCode  string     `json:"productCode"  gorm:"column:product_code"`
	ProductName  string     `json:"productName"  gorm:"column:product_name"`
	CurrentStock int64      `json:"currentStock" gorm:"column:current_stock"`
	SalesQty     int64      `json:"salesQty"     gorm:"column:sales_qty"`
	LastSoldAt   *time.Time `json:"lastSoldAt"   gorm:"column:last_sold_at"`
}
