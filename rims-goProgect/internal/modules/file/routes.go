// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package file

import "github.com/gin-gonic/gin"

// RegisterRoutes registers all file attachment routes. Files are not
// warehouse-scoped; the business_id reference already carries scope via the
// associated business object.
func RegisterRoutes(
	rg *gin.RouterGroup,
	handler *Handler,
	authMw, idemMw gin.HandlerFunc,
) {
	files := rg.Group("/files")
	files.Use(authMw)
	files.POST("/upload", idemMw, handler.Upload)
	files.GET("", handler.List)
	files.GET("/:id", handler.Get)
	files.GET("/:id/download", handler.Download)
	files.DELETE("/:id", handler.Delete)
}
