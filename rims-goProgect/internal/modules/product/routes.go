// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package product

import (
	"github.com/gin-gonic/gin"

	"rims-go/internal/idempotency"
)

// RegisterRoutes registers all product, inventory, and non-standard inventory routes.
func RegisterRoutes(
	rg *gin.RouterGroup,
	handler *Handler,
	authMw, whScope, idemMw gin.HandlerFunc,
	perm func(string) gin.HandlerFunc,
) {
	// Product catalog (global, no warehouse scope)
	products := rg.Group("/products")
	products.Use(authMw)
	products.POST("", perm("product:create"), handler.CreateProduct)
	products.GET("", handler.ListProducts)
	products.GET("/barcode/:barcode", handler.GetProductByBarcode)
	products.GET("/:id", handler.GetProduct)
	products.PUT("/:id", perm("product:update"), handler.UpdateProduct)
	products.DELETE("/:id", perm("product:delete"), handler.DeleteProduct)

	// Standard inventory (warehouse-scoped)
	inventory := rg.Group("/inventory")
	inventory.Use(authMw, whScope)
	inventory.GET("", handler.ListInventory)
	inventory.GET("/alerts", handler.ListAlerts)
	inventory.GET("/barcode/:barcode", handler.GetInventoryByBarcode)
	inventory.GET("/:id", handler.GetInventory)
	inventory.PUT("/:id", perm("inventory:update"), handler.UpdateInventory)

	// Non-standard inventory (warehouse-scoped, permission-protected)
	convertRoute := idempotency.RegisteredMutationRoute(idempotency.ConvertNonStandardInventoryMutation)
	nonStd := rg.Group(convertRoute.GroupPath)
	nonStd.Use(authMw, whScope)
	nonStd.POST("", perm("non_std:create"), handler.CreateNonStd)
	nonStd.GET("", perm("non_std:read"), handler.ListNonStd)
	nonStd.GET("/:id", perm("non_std:read"), handler.GetNonStd)
	nonStd.PUT("/:id", perm("non_std:update"), handler.UpdateNonStd)
	nonStd.DELETE("/:id", perm("non_std:delete"), handler.DeleteNonStd)
	nonStd.Handle(convertRoute.Method, convertRoute.Path, perm("non_std:convert"), idemMw, handler.ConvertNonStd)
}
