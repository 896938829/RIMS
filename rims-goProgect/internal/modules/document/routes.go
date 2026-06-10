// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package document

import "github.com/gin-gonic/gin"

// RegisterRoutes registers all document and inventory transaction routes.
func RegisterRoutes(
	rg *gin.RouterGroup,
	handler *Handler,
	authMw, whScope, idemMw gin.HandlerFunc,
) {
	// Documents (warehouse-scoped)
	docs := rg.Group("/documents")
	docs.Use(authMw, whScope)
	docs.POST("", idemMw, handler.CreateDocument)
	docs.GET("", handler.ListDocuments)
	docs.GET("/:id", handler.GetDocument)
	docs.POST("/:id/complete", idemMw, handler.CompleteDocument)
	docs.POST("/:id/confirm", handler.ConfirmStocktake)
	docs.POST("/:id/settle", handler.SettleStocktake)

	// Inventory transactions (warehouse-scoped)
	txns := rg.Group("/transactions")
	txns.Use(authMw, whScope)
	txns.GET("", handler.ListTransactions)
}
