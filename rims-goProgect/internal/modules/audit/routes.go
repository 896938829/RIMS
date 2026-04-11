// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package audit

import "github.com/gin-gonic/gin"

// RegisterRoutes registers all audit log query routes on the given router
// group. Routes require JWT authentication; admin-only enforcement lives in
// the handler per project convention (see CLAUDE.md module pattern).
//
// The group is NOT warehouse-scoped: audit records span non-warehouse areas
// like user / role management. Admins can still filter by warehouseId via
// the query string when they want a single-warehouse slice.
func RegisterRoutes(rg *gin.RouterGroup, handler *Handler, authMw gin.HandlerFunc) {
	g := rg.Group("/audit")
	g.Use(authMw)
	g.GET("/logs", handler.List)
	g.GET("/logs/:id", handler.Get)
}
