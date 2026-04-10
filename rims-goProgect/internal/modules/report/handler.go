// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package report

import (
	"github.com/gin-gonic/gin"

	"rims-go/internal/types"
)

// Handler exposes the report module's HTTP endpoints.
type Handler struct {
	svc *ReportService
}

// NewHandler returns a new report Handler.
func NewHandler(svc *ReportService) *Handler {
	return &Handler{svc: svc}
}

// GetSalesStats godoc
// @Summary 销售统计
// @Description 指定时间范围内的销售汇总（营收、订单数、SKU 数、数量）。成本与毛利仅管理员可见。
// @Tags 报表分析
// @Security BearerAuth
// @Produce json
// @Param X-Warehouse-ID header int false "仓库ID"
// @Param startDate query string true "开始日期 YYYY-MM-DD"
// @Param endDate   query string true "结束日期 YYYY-MM-DD"
// @Success 200 {object} types.Response{data=SalesStatResponse}
// @Failure 400 {object} types.Response
// @Router /api/v1/reports/sales/stats [get]
func (h *Handler) GetSalesStats(c *gin.Context) {
	var req SalesStatRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		types.FailFromError(c, types.ErrValidation(err.Error()))
		return
	}
	resp, err := h.svc.GetSalesStats(
		c.Request.Context(),
		types.GetWarehouseID(c),
		req,
		types.IsAdmin(c),
	)
	if err != nil {
		types.FailFromError(c, err)
		return
	}
	types.OK(c, resp)
}

// GetSalesTrend godoc
// @Summary 销售趋势
// @Description 按日/周/月聚合的销售趋势曲线
// @Tags 报表分析
// @Security BearerAuth
// @Produce json
// @Param X-Warehouse-ID header int false "仓库ID"
// @Param startDate query string true "开始日期 YYYY-MM-DD"
// @Param endDate   query string true "结束日期 YYYY-MM-DD"
// @Param bucket    query string true "时间粒度 day/week/month"
// @Success 200 {object} types.Response{data=SalesTrendResponse}
// @Failure 400 {object} types.Response
// @Router /api/v1/reports/sales/trend [get]
func (h *Handler) GetSalesTrend(c *gin.Context) {
	var req SalesTrendRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		types.FailFromError(c, types.ErrValidation(err.Error()))
		return
	}
	resp, err := h.svc.GetSalesTrend(
		c.Request.Context(),
		types.GetWarehouseID(c),
		req,
	)
	if err != nil {
		types.FailFromError(c, err)
		return
	}
	types.OK(c, resp)
}

// GetProductRanking godoc
// @Summary 商品销售排行
// @Description 按数量或金额排序的销售排行榜
// @Tags 报表分析
// @Security BearerAuth
// @Produce json
// @Param X-Warehouse-ID header int false "仓库ID"
// @Param startDate query string true "开始日期 YYYY-MM-DD"
// @Param endDate   query string true "结束日期 YYYY-MM-DD"
// @Param metric    query string true "排序指标 qty/amount"
// @Param limit     query int false "返回条数 默认10"
// @Success 200 {object} types.Response{data=ProductRankResponse}
// @Failure 400 {object} types.Response
// @Router /api/v1/reports/sales/ranking [get]
func (h *Handler) GetProductRanking(c *gin.Context) {
	var req ProductRankRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		types.FailFromError(c, types.ErrValidation(err.Error()))
		return
	}
	resp, err := h.svc.GetProductRanking(
		c.Request.Context(),
		types.GetWarehouseID(c),
		req,
		types.IsAdmin(c),
	)
	if err != nil {
		types.FailFromError(c, err)
		return
	}
	types.OK(c, resp)
}

// GetInventoryOverview godoc
// @Summary 库存概况
// @Description 当前仓库的 SKU 数、总库存、库存金额（管理员）、低库存数量
// @Tags 报表分析
// @Security BearerAuth
// @Produce json
// @Param X-Warehouse-ID header int false "仓库ID"
// @Success 200 {object} types.Response{data=InventoryOverviewResponse}
// @Router /api/v1/reports/inventory/overview [get]
func (h *Handler) GetInventoryOverview(c *gin.Context) {
	resp, err := h.svc.GetInventoryOverview(
		c.Request.Context(),
		types.GetWarehouseID(c),
		types.IsAdmin(c),
	)
	if err != nil {
		types.FailFromError(c, err)
		return
	}
	types.OK(c, resp)
}

// GetInventoryTurnover godoc
// @Summary 库存周转率
// @Description 指定窗口内的商品周转率（出库数量 / 平均库存）
// @Tags 报表分析
// @Security BearerAuth
// @Produce json
// @Param X-Warehouse-ID header int false "仓库ID"
// @Param startDate query string true "开始日期 YYYY-MM-DD"
// @Param endDate   query string true "结束日期 YYYY-MM-DD"
// @Param limit     query int false "返回条数 默认20"
// @Success 200 {object} types.Response{data=TurnoverResponse}
// @Failure 400 {object} types.Response
// @Router /api/v1/reports/inventory/turnover [get]
func (h *Handler) GetInventoryTurnover(c *gin.Context) {
	var req TurnoverRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		types.FailFromError(c, types.ErrValidation(err.Error()))
		return
	}
	resp, err := h.svc.GetInventoryTurnover(
		c.Request.Context(),
		types.GetWarehouseID(c),
		req,
	)
	if err != nil {
		types.FailFromError(c, err)
		return
	}
	types.OK(c, resp)
}

// GetSlowMoving godoc
// @Summary 滞销商品预警
// @Description 指定窗口内销量低于阈值的商品分页列表
// @Tags 报表分析
// @Security BearerAuth
// @Produce json
// @Param X-Warehouse-ID header int false "仓库ID"
// @Param startDate query string true "开始日期 YYYY-MM-DD"
// @Param endDate   query string true "结束日期 YYYY-MM-DD"
// @Param maxSales  query int false "最大销量阈值 默认0"
// @Param page      query int false "页码 默认1"
// @Param pageSize  query int false "每页数量 默认20"
// @Success 200 {object} types.Response{data=types.PageResult}
// @Failure 400 {object} types.Response
// @Router /api/v1/reports/inventory/slow-moving [get]
func (h *Handler) GetSlowMoving(c *gin.Context) {
	var req SlowMovingRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		types.FailFromError(c, types.ErrValidation(err.Error()))
		return
	}
	resp, err := h.svc.GetSlowMoving(
		c.Request.Context(),
		types.GetWarehouseID(c),
		req,
	)
	if err != nil {
		types.FailFromError(c, err)
		return
	}
	types.OKWithPage(c, resp)
}
