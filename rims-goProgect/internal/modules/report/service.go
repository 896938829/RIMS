// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package report

import (
	"context"
	"fmt"
	"time"

	"rims-go/internal/types"
)

const (
	maxRangeDays       = 366
	defaultRankLimit   = 10
	defaultTurnoverLim = 20
)

// ReportRepository is the data-access contract used by ReportService.
// It is the only surface a service test needs to mock.
type ReportRepository interface {
	SalesStats(ctx context.Context, whID uint, start, end time.Time) (SalesStatRaw, error)
	SalesTrend(ctx context.Context, whID uint, start, end time.Time, bucket string) ([]SalesTrendRaw, error)
	ProductRanking(ctx context.Context, whID uint, start, end time.Time, metric string, limit int) ([]ProductRankItem, error)
	InventoryOverview(ctx context.Context, whID uint) (InventoryOverviewRaw, error)
	InventoryTurnover(ctx context.Context, whID uint, start, end time.Time, limit int) ([]TurnoverItem, error)
	SlowMoving(ctx context.Context, whID uint, start, end time.Time, maxSales int, page types.PageRequest) ([]SlowMovingItem, int64, error)
}

// ReportService holds business rules for the report module:
// time-range validation, bucket/metric whitelisting, admin field gating,
// turnover safe-rate computation.
type ReportService struct {
	repo ReportRepository
}

func NewReportService(repo ReportRepository) *ReportService {
	return &ReportService{repo: repo}
}

// parseAndValidateRange parses the request's date range and enforces the
// maximum-window rule. Returns an AppError with ErrCodeValidation on failure.
func parseAndValidateRange(tr TimeRangeRequest) (time.Time, time.Time, error) {
	start, end, err := tr.Parse()
	if err != nil {
		return time.Time{}, time.Time{}, types.ErrValidation(err.Error())
	}
	if end.Sub(start) > maxRangeDays*24*time.Hour {
		return time.Time{}, time.Time{}, types.ErrValidation("time range too large (max 366 days)")
	}
	return start, end, nil
}

// --- SalesStats ---

func (s *ReportService) GetSalesStats(ctx context.Context, whID uint, req SalesStatRequest, isAdmin bool) (SalesStatResponse, error) {
	start, end, err := parseAndValidateRange(req.TimeRangeRequest)
	if err != nil {
		return SalesStatResponse{}, err
	}
	raw, err := s.repo.SalesStats(ctx, whID, start, end)
	if err != nil {
		return SalesStatResponse{}, err
	}
	resp := SalesStatResponse{
		Revenue:    raw.Revenue,
		OrderCount: raw.OrderCount,
		SKUCount:   raw.SKUCount,
		Quantity:   raw.Quantity,
	}
	if isAdmin {
		cost := raw.CostAmount
		profit := raw.Revenue - raw.CostAmount
		resp.CostAmount = &cost
		resp.GrossProfit = &profit
	}
	return resp, nil
}

// --- SalesTrend ---

var allowedBuckets = map[string]bool{"day": true, "week": true, "month": true}

func (s *ReportService) GetSalesTrend(ctx context.Context, whID uint, req SalesTrendRequest) (SalesTrendResponse, error) {
	if !allowedBuckets[req.Bucket] {
		return SalesTrendResponse{}, types.ErrValidation("invalid bucket")
	}
	start, end, err := parseAndValidateRange(req.TimeRangeRequest)
	if err != nil {
		return SalesTrendResponse{}, err
	}
	rows, err := s.repo.SalesTrend(ctx, whID, start, end, req.Bucket)
	if err != nil {
		return SalesTrendResponse{}, err
	}
	points := make([]SalesTrendPoint, 0, len(rows))
	for _, r := range rows {
		points = append(points, SalesTrendPoint{
			Period:     formatBucket(r.BucketTS, req.Bucket),
			Revenue:    r.Revenue,
			OrderCount: r.OrderCount,
			Quantity:   r.Quantity,
		})
	}
	return SalesTrendResponse{List: points}, nil
}

func formatBucket(ts time.Time, bucket string) string {
	switch bucket {
	case "month":
		return ts.Format("2006-01")
	case "week":
		y, w := ts.ISOWeek()
		// ISO week format: YYYY-Www
		return formatISOWeek(y, w)
	default:
		return ts.Format("2006-01-02")
	}
}

func formatISOWeek(year, week int) string {
	return fmt.Sprintf("%d-W%02d", year, week)
}

// --- ProductRanking ---

var allowedMetrics = map[string]bool{"qty": true, "amount": true}

func (s *ReportService) GetProductRanking(ctx context.Context, whID uint, req ProductRankRequest, isAdmin bool) (ProductRankResponse, error) {
	if !allowedMetrics[req.Metric] {
		return ProductRankResponse{}, types.ErrValidation("invalid metric")
	}
	start, end, err := parseAndValidateRange(req.TimeRangeRequest)
	if err != nil {
		return ProductRankResponse{}, err
	}
	limit := req.Limit
	if limit <= 0 {
		limit = defaultRankLimit
	}
	items, err := s.repo.ProductRanking(ctx, whID, start, end, req.Metric, limit)
	if err != nil {
		return ProductRankResponse{}, err
	}
	// Field gating: populate GrossProfit only for admin, always drop CostAmount from JSON.
	for i := range items {
		if isAdmin {
			gp := items[i].Amount - items[i].CostAmount
			items[i].GrossProfit = &gp
		} else {
			items[i].GrossProfit = nil
		}
		items[i].CostAmount = 0 // not serialized anyway, but zero out for hygiene
	}
	return ProductRankResponse{List: items}, nil
}

// --- InventoryOverview ---

func (s *ReportService) GetInventoryOverview(ctx context.Context, whID uint, isAdmin bool) (InventoryOverviewResponse, error) {
	raw, err := s.repo.InventoryOverview(ctx, whID)
	if err != nil {
		return InventoryOverviewResponse{}, err
	}
	resp := InventoryOverviewResponse{
		SKUCount:      raw.SKUCount,
		TotalQty:      raw.TotalQty,
		LowStockCount: raw.LowStockCount,
	}
	if isAdmin {
		v := raw.TotalValue
		resp.TotalValue = &v
	}
	return resp, nil
}

// --- InventoryTurnover ---

func (s *ReportService) GetInventoryTurnover(ctx context.Context, whID uint, req TurnoverRequest) (TurnoverResponse, error) {
	start, end, err := parseAndValidateRange(req.TimeRangeRequest)
	if err != nil {
		return TurnoverResponse{}, err
	}
	limit := req.Limit
	if limit <= 0 {
		limit = defaultTurnoverLim
	}
	items, err := s.repo.InventoryTurnover(ctx, whID, start, end, limit)
	if err != nil {
		return TurnoverResponse{}, err
	}
	out := make([]TurnoverItem, 0, len(items))
	for _, it := range items {
		// Drop rows with no movement and no stock.
		if it.OutboundQty == 0 && it.AvgStock == 0 {
			continue
		}
		if it.AvgStock > 0 {
			it.TurnoverRate = float64(it.OutboundQty) / it.AvgStock
		} else {
			it.TurnoverRate = 0
		}
		out = append(out, it)
	}
	return TurnoverResponse{List: out}, nil
}

// --- SlowMoving ---

func (s *ReportService) GetSlowMoving(ctx context.Context, whID uint, req SlowMovingRequest) (types.PageResult, error) {
	start, end, err := parseAndValidateRange(req.TimeRangeRequest)
	if err != nil {
		return types.PageResult{}, err
	}
	req.PageRequest.Defaults()
	list, total, err := s.repo.SlowMoving(ctx, whID, start, end, req.MaxSales, req.PageRequest)
	if err != nil {
		return types.PageResult{}, err
	}
	return types.NewPageResult(req.PageRequest, list, total), nil
}
