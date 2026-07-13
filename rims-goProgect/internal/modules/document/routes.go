// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package document

import (
	"github.com/gin-gonic/gin"

	"rims-go/internal/idempotency"
)

// RegisterRoutes registers all document and inventory transaction routes.
func RegisterRoutes(
	rg *gin.RouterGroup,
	handler *Handler,
	authMw, whScope, idemMw gin.HandlerFunc,
	perm func(string) gin.HandlerFunc,
) {
	// Documents (warehouse-scoped)
	createRoute := idempotency.RegisteredMutationRoute(idempotency.CreateDocumentMutation)
	completeRoute := idempotency.RegisteredMutationRoute(idempotency.CompleteDocumentMutation)
	confirmRoute := idempotency.RegisteredMutationRoute(idempotency.ConfirmStocktakeMutation)
	settleRoute := idempotency.RegisteredMutationRoute(idempotency.SettleStocktakeMutation)
	docs := rg.Group(createRoute.GroupPath)
	docs.Use(authMw, whScope)
	docs.Handle(createRoute.Method, createRoute.Path, perm(createRoute.PermissionCode), idemMw, handler.CreateDocument)
	docs.GET("", handler.ListDocuments)
	docs.GET("/:id", handler.GetDocument)
	docs.Handle(completeRoute.Method, completeRoute.Path, perm(completeRoute.PermissionCode), idemMw, handler.CompleteDocument)
	docs.Handle(confirmRoute.Method, confirmRoute.Path, perm(confirmRoute.PermissionCode), idemMw, handler.ConfirmStocktake)
	docs.Handle(settleRoute.Method, settleRoute.Path, perm(settleRoute.PermissionCode), idemMw, handler.SettleStocktake)

	// Inventory transactions (warehouse-scoped)
	txns := rg.Group("/transactions")
	txns.Use(authMw, whScope)
	txns.GET("", handler.ListTransactions)
}
