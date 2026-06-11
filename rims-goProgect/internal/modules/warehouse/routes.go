// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package warehouse

import "github.com/gin-gonic/gin"

// RegisterRoutes registers all warehouse-related routes.
func RegisterRoutes(
	rg *gin.RouterGroup,
	handler *Handler,
	authMw gin.HandlerFunc,
	perm func(string) gin.HandlerFunc,
) {
	// Warehouse CRUD
	warehouses := rg.Group("/warehouses")
	warehouses.Use(authMw)
	warehouses.POST("", perm("warehouse:create"), handler.CreateWarehouse)
	warehouses.GET("", handler.ListWarehouses)
	warehouses.GET("/:id", perm("warehouse:read"), handler.GetWarehouse)
	warehouses.PUT("/:id", perm("warehouse:update"), handler.UpdateWarehouse)
	warehouses.DELETE("/:id", perm("warehouse:delete"), handler.DeleteWarehouse)

	// Warehouse-User bindings
	warehouses.POST("/:id/users", perm("warehouse:bind_user"), handler.BindUsers)
	warehouses.DELETE("/:id/users/:userId", perm("warehouse:unbind_user"), handler.UnbindUser)
	warehouses.GET("/:id/users", perm("warehouse:list_users"), handler.ListWarehouseUsers)

	// Current user's warehouse operations (registered under /users)
	users := rg.Group("/users")
	users.Use(authMw)
	users.GET("/me/warehouses", handler.GetMyWarehouses)
	users.PUT("/me/warehouses/default", handler.SetDefaultWarehouse)
	users.PUT("/me/warehouses/current", handler.SwitchCurrentWarehouse)
}
