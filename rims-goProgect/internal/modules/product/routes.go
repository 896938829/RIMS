// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package product

import "github.com/gin-gonic/gin"

// RegisterRoutes registers all product, inventory, and non-standard inventory routes.
func RegisterRoutes(
	rg *gin.RouterGroup,
	handler *Handler,
	authMw, whScope, idemMw gin.HandlerFunc,
) {
	// Product catalog (global, no warehouse scope)
	products := rg.Group("/products")
	products.Use(authMw)
	products.POST("", handler.CreateProduct)
	products.GET("", handler.ListProducts)
	products.GET("/barcode/:barcode", handler.GetProductByBarcode)
	products.GET("/:id", handler.GetProduct)
	products.PUT("/:id", handler.UpdateProduct)
	products.DELETE("/:id", handler.DeleteProduct)

	// Standard inventory (warehouse-scoped)
	inventory := rg.Group("/inventory")
	inventory.Use(authMw, whScope)
	inventory.GET("", handler.ListInventory)
	inventory.GET("/alerts", handler.ListAlerts)
	inventory.GET("/:id", handler.GetInventory)
	inventory.PUT("/:id", handler.UpdateInventory)

	// Non-standard inventory (warehouse-scoped, all admin-only in handlers)
	nonStd := rg.Group("/non-std-inventory")
	nonStd.Use(authMw, whScope)
	nonStd.POST("", handler.CreateNonStd)
	nonStd.GET("", handler.ListNonStd)
	nonStd.GET("/:id", handler.GetNonStd)
	nonStd.PUT("/:id", handler.UpdateNonStd)
	nonStd.DELETE("/:id", handler.DeleteNonStd)
	nonStd.POST("/:id/convert", idemMw, handler.ConvertNonStd)
}
