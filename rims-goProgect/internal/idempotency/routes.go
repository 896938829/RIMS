// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package idempotency

import "github.com/gin-gonic/gin"

var allowedMutationScopes = map[string]struct{}{
	"POST /api/v1/documents":                     {},
	"POST /api/v1/documents/:id/complete":        {},
	"POST /api/v1/files/upload":                  {},
	"POST /api/v1/files/:id/replace":             {},
	"POST /api/v1/non-std-inventory/:id/convert": {},
}

func isAllowedMutationScope(scope string) bool {
	_, ok := allowedMutationScopes[scope]
	return ok
}

// RegisterRoutes registers authenticated idempotency status routes.
func RegisterRoutes(rg *gin.RouterGroup, handler *Handler, authMw gin.HandlerFunc) {
	operations := rg.Group("/operations")
	operations.Use(authMw)
	operations.GET("/idempotency/:key", handler.GetStatus)
}
