// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package file

import (
	"github.com/gin-gonic/gin"

	"rims-go/internal/idempotency"
)

// RegisterRoutes registers all file attachment routes. Files are not
// warehouse-scoped; the business_id reference already carries scope via the
// associated business object.
func RegisterRoutes(
	rg *gin.RouterGroup,
	handler *Handler,
	authMw, idemMw gin.HandlerFunc,
	perm func(string) gin.HandlerFunc,
) {
	uploadRoute := idempotency.RegisteredMutationRoute(idempotency.UploadFileMutation)
	replaceRoute := idempotency.RegisteredMutationRoute(idempotency.ReplaceFileMutation)
	files := rg.Group(uploadRoute.GroupPath)
	files.Use(authMw)
	files.Handle(uploadRoute.Method, uploadRoute.Path, perm(uploadRoute.PermissionCode), idemMw, handler.Upload)
	files.GET("", handler.List)
	files.PUT("/reorder", handler.Reorder)
	files.Handle(replaceRoute.Method, replaceRoute.Path, perm(replaceRoute.PermissionCode), idemMw, handler.Replace)
	files.GET("/:id", handler.Get)
	files.GET("/:id/download", handler.Download)
	files.DELETE("/:id", handler.Delete)
}
