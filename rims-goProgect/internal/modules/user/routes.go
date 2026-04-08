// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package user

import "github.com/gin-gonic/gin"

// RegisterRoutes registers all user, auth, and role routes.
func RegisterRoutes(
	rg *gin.RouterGroup,
	handler *Handler,
	authMw gin.HandlerFunc,
) {
	// Public auth routes
	auth := rg.Group("/auth")
	auth.POST("/login", handler.Login)

	// Protected user routes
	users := rg.Group("/users")
	users.Use(authMw)
	users.POST("", handler.CreateUser)
	users.GET("", handler.ListUsers)
	users.GET("/me", handler.GetCurrentUser)
	users.PUT("/me/password", handler.ChangePassword)
	users.GET("/:id", handler.GetUser)
	users.PUT("/:id", handler.UpdateUser)
	users.DELETE("/:id", handler.DeleteUser)
	users.PUT("/:id/password", handler.ResetPassword)

	// Protected role routes
	roles := rg.Group("/roles")
	roles.Use(authMw)
	roles.POST("", handler.CreateRole)
	roles.GET("", handler.ListRoles)
	roles.GET("/:id", handler.GetRole)
	roles.PUT("/:id", handler.UpdateRole)
	roles.DELETE("/:id", handler.DeleteRole)
	roles.PUT("/:id/permissions", handler.AssignPermissions)

	// Protected permission routes
	permissions := rg.Group("/permissions")
	permissions.Use(authMw)
	permissions.GET("", handler.ListPermissions)
}
