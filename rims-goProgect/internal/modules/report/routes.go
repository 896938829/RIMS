// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package report

import "github.com/gin-gonic/gin"

// RegisterRoutes registers all report & analytics routes on the given router
// group. All endpoints require JWT authentication and warehouse scoping.
func RegisterRoutes(rg *gin.RouterGroup, handler *Handler, authMw, whScope gin.HandlerFunc) {
	g := rg.Group("/reports")
	g.Use(authMw, whScope)

	g.GET("/sales/stats", handler.GetSalesStats)
	g.GET("/sales/trend", handler.GetSalesTrend)
	g.GET("/sales/ranking", handler.GetProductRanking)
	g.GET("/inventory/overview", handler.GetInventoryOverview)
	g.GET("/inventory/turnover", handler.GetInventoryTurnover)
	g.GET("/inventory/slow-moving", handler.GetSlowMoving)
}
