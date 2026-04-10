// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package report

import (
	"context"
	"time"

	"gorm.io/gorm"

	"rims-go/internal/db"
	"rims-go/internal/types"
)

// reportRepository implements ReportRepository using raw GORM .Table(...) joins
// so this module does not import other modules' GORM entities.
type reportRepository struct {
	gormDB *gorm.DB
}

// NewReportRepository creates a ReportRepository backed by GORM.
func NewReportRepository(gormDB *gorm.DB) ReportRepository {
	return &reportRepository{gormDB: gormDB}
}

func (r *reportRepository) getDB(ctx context.Context) *gorm.DB {
	return db.FromCtx(ctx, r.gormDB)
}

// SalesStats aggregates completed sales documents within [start, end).
func (r *reportRepository) SalesStats(ctx context.Context, whID uint, start, end time.Time) (SalesStatRaw, error) {
	var raw SalesStatRaw
	err := r.getDB(ctx).
		Table("documents AS d").
		Joins("JOIN document_lines AS l ON l.document_id = d.id AND l.deleted_at IS NULL").
		Where("d.deleted_at IS NULL").
		Where("d.warehouse_id = ? AND d.doc_type = ? AND d.status = ?", whID, 2, 2).
		Where("d.operated_at >= ? AND d.operated_at < ?", start, end).
		Select(`
			COALESCE(SUM(l.quantity * l.retail_price), 0) AS revenue,
			COALESCE(SUM(l.quantity * l.cost_price),   0) AS cost_amount,
			COUNT(DISTINCT d.id)                           AS order_count,
			COUNT(DISTINCT l.product_id)                   AS sku_count,
			COALESCE(SUM(l.quantity),                  0) AS quantity
		`).
		Scan(&raw).Error
	return raw, err
}

// SalesTrend buckets completed sales by day/week/month.
func (r *reportRepository) SalesTrend(ctx context.Context, whID uint, start, end time.Time, bucket string) ([]SalesTrendRaw, error) {
	// Bucket was already whitelisted in the service, but guard again here.
	if _, ok := allowedBuckets[bucket]; !ok {
		return nil, types.ErrValidation("invalid bucket")
	}
	var rows []SalesTrendRaw
	err := r.getDB(ctx).
		Table("documents AS d").
		Joins("JOIN document_lines AS l ON l.document_id = d.id AND l.deleted_at IS NULL").
		Where("d.deleted_at IS NULL").
		Where("d.warehouse_id = ? AND d.doc_type = ? AND d.status = ?", whID, 2, 2).
		Where("d.operated_at >= ? AND d.operated_at < ?", start, end).
		Select(`
			DATE_TRUNC(?, d.operated_at) AS bucket_ts,
			COALESCE(SUM(l.quantity * l.retail_price), 0) AS revenue,
			COUNT(DISTINCT d.id)                          AS order_count,
			COALESCE(SUM(l.quantity),                 0) AS quantity
		`, bucket).
		Group("bucket_ts").
		Order("bucket_ts ASC").
		Scan(&rows).Error
	return rows, err
}

// ProductRanking returns top-N products by qty or amount within the window.
func (r *reportRepository) ProductRanking(ctx context.Context, whID uint, start, end time.Time, metric string, limit int) ([]ProductRankItem, error) {
	orderBy := "quantity DESC"
	if metric == "amount" {
		orderBy = "amount DESC"
	}
	var items []ProductRankItem
	err := r.getDB(ctx).
		Table("documents AS d").
		Joins("JOIN document_lines AS l ON l.document_id = d.id AND l.deleted_at IS NULL").
		Where("d.deleted_at IS NULL").
		Where("d.warehouse_id = ? AND d.doc_type = ? AND d.status = ?", whID, 2, 2).
		Where("d.operated_at >= ? AND d.operated_at < ?", start, end).
		Select(`
			l.product_id                                   AS product_id,
			MAX(l.product_code)                            AS product_code,
			MAX(l.product_name)                            AS product_name,
			COALESCE(SUM(l.quantity),                  0) AS quantity,
			COALESCE(SUM(l.quantity * l.retail_price), 0) AS amount,
			COALESCE(SUM(l.quantity * l.cost_price),   0) AS cost_amount
		`).
		Group("l.product_id").
		Order(orderBy).
		Limit(limit).
		Scan(&items).Error
	return items, err
}

// InventoryOverview summarizes the current inventory snapshot for a warehouse.
func (r *reportRepository) InventoryOverview(ctx context.Context, whID uint) (InventoryOverviewRaw, error) {
	var raw InventoryOverviewRaw
	err := r.getDB(ctx).
		Table("inventories AS i").
		Joins("JOIN products AS p ON p.id = i.product_id AND p.deleted_at IS NULL").
		Where("i.deleted_at IS NULL").
		Where("i.warehouse_id = ? AND p.status = 1", whID).
		Select(`
			COUNT(*)                                              AS sku_count,
			COALESCE(SUM(i.quantity),                         0) AS total_qty,
			COALESCE(SUM(i.quantity * p.cost_price),          0) AS total_value,
			COALESCE(SUM(CASE WHEN i.alert_threshold > 0 AND i.quantity <= i.alert_threshold THEN 1 ELSE 0 END), 0) AS low_stock_count
		`).
		Scan(&raw).Error
	return raw, err
}

// InventoryTurnover computes per-product outbound qty and average stock within the window.
func (r *reportRepository) InventoryTurnover(ctx context.Context, whID uint, start, end time.Time, limit int) ([]TurnoverItem, error) {
	var items []TurnoverItem
	err := r.getDB(ctx).
		Table("inventory_transactions AS t").
		Joins("JOIN products AS p ON p.id = t.product_id AND p.deleted_at IS NULL").
		Where("t.deleted_at IS NULL").
		Where("t.warehouse_id = ? AND t.doc_type = ? AND t.direction = ?", whID, 2, -1).
		Where("t.operated_at >= ? AND t.operated_at < ?", start, end).
		Select(`
			t.product_id                                   AS product_id,
			MAX(p.code)                                    AS product_code,
			MAX(p.name)                                    AS product_name,
			COALESCE(SUM(t.quantity),                  0) AS outbound_qty,
			COALESCE(AVG((t.before_qty + t.after_qty) / 2.0), 0) AS avg_stock
		`).
		Group("t.product_id").
		Order("outbound_qty DESC").
		Limit(limit).
		Scan(&items).Error
	return items, err
}

// SlowMoving returns products whose outbound qty in [start, end) is <= maxSales,
// paginated.
func (r *reportRepository) SlowMoving(ctx context.Context, whID uint, start, end time.Time, maxSales int, page types.PageRequest) ([]SlowMovingItem, int64, error) {
	page.Defaults()
	subquery := r.getDB(ctx).
		Table("inventory_transactions").
		Select("product_id, SUM(quantity) AS sold, MAX(operated_at) AS last_sold").
		Where("deleted_at IS NULL").
		Where("warehouse_id = ? AND doc_type = ? AND direction = ?", whID, 2, -1).
		Where("operated_at >= ? AND operated_at < ?", start, end).
		Group("product_id")

	base := r.getDB(ctx).
		Table("inventories AS i").
		Joins("JOIN products AS p ON p.id = i.product_id AND p.deleted_at IS NULL").
		Joins("LEFT JOIN (?) AS s ON s.product_id = i.product_id", subquery).
		Where("i.deleted_at IS NULL").
		Where("i.warehouse_id = ? AND p.status = 1 AND i.quantity > 0", whID).
		Where("COALESCE(s.sold, 0) <= ?", maxSales)

	var total int64
	if err := base.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var items []SlowMovingItem
	err := base.
		Select(`
			i.product_id       AS product_id,
			p.code             AS product_code,
			p.name             AS product_name,
			i.quantity         AS current_stock,
			COALESCE(s.sold, 0) AS sales_qty,
			s.last_sold        AS last_sold_at
		`).
		Order("sales_qty ASC, i.quantity DESC").
		Offset(page.Offset()).
		Limit(page.PageSize).
		Scan(&items).Error
	return items, total, err
}
