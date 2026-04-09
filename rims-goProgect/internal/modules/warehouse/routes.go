// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package warehouse

import "github.com/gin-gonic/gin"

// RegisterRoutes registers all warehouse-related routes.
func RegisterRoutes(
	rg *gin.RouterGroup,
	handler *Handler,
	authMw gin.HandlerFunc,
) {
	// Warehouse CRUD
	warehouses := rg.Group("/warehouses")
	warehouses.Use(authMw)
	warehouses.POST("", handler.CreateWarehouse)
	warehouses.GET("", handler.ListWarehouses)
	warehouses.GET("/:id", handler.GetWarehouse)
	warehouses.PUT("/:id", handler.UpdateWarehouse)
	warehouses.DELETE("/:id", handler.DeleteWarehouse)

	// Warehouse-User bindings
	warehouses.POST("/:id/users", handler.BindUsers)
	warehouses.DELETE("/:id/users/:userId", handler.UnbindUser)
	warehouses.GET("/:id/users", handler.ListWarehouseUsers)

	// Current user's warehouse operations (registered under /users)
	users := rg.Group("/users")
	users.Use(authMw)
	users.GET("/me/warehouses", handler.GetMyWarehouses)
	users.PUT("/me/warehouses/default", handler.SetDefaultWarehouse)
	users.PUT("/me/warehouses/current", handler.SwitchCurrentWarehouse)
}
