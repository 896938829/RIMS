// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package user

import "github.com/gin-gonic/gin"

// RegisterRoutes registers all user, auth, and role routes.
func RegisterRoutes(
	rg *gin.RouterGroup,
	handler *Handler,
	authMw gin.HandlerFunc,
	perm func(string) gin.HandlerFunc,
) {
	// Public auth routes
	auth := rg.Group("/auth")
	auth.POST("/login", handler.Login)

	// Protected user routes
	users := rg.Group("/users")
	users.Use(authMw)
	users.POST("", perm("user:create"), handler.CreateUser)
	users.GET("", perm("user:list"), handler.ListUsers)
	users.GET("/me", handler.GetCurrentUser)
	users.PUT("/me/password", handler.ChangePassword)
	users.GET("/:id", perm("user:read"), handler.GetUser)
	users.PUT("/:id", perm("user:update"), handler.UpdateUser)
	users.DELETE("/:id", perm("user:delete"), handler.DeleteUser)
	users.PUT("/:id/password", perm("user:reset_password"), handler.ResetPassword)

	// Protected role routes
	roles := rg.Group("/roles")
	roles.Use(authMw)
	roles.POST("", perm("role:create"), handler.CreateRole)
	roles.GET("", handler.ListRoles)
	roles.GET("/:id", handler.GetRole)
	roles.PUT("/:id", perm("role:update"), handler.UpdateRole)
	roles.DELETE("/:id", perm("role:delete"), handler.DeleteRole)
	roles.PUT("/:id/permissions", perm("role:assign_permissions"), handler.AssignPermissions)

	// Protected permission routes
	permissions := rg.Group("/permissions")
	permissions.Use(authMw)
	permissions.GET("", handler.ListPermissions)
}
