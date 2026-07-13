// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package idempotency

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// MutationRouteID is the stable identity of an idempotent mutation route.
type MutationRouteID string

const (
	CreateDocumentMutation              MutationRouteID = "create_document"
	CompleteDocumentMutation            MutationRouteID = "complete_document"
	UploadFileMutation                  MutationRouteID = "upload_file"
	ReplaceFileMutation                 MutationRouteID = "replace_file"
	ConvertNonStandardInventoryMutation MutationRouteID = "convert_non_standard_inventory"
)

// MutationRoute is the single source for route registration and status scope.
type MutationRoute struct {
	ID        MutationRouteID
	Method    string
	GroupPath string
	Path      string
}

// Scope returns the method plus full Gin route template stored with a key.
func (r MutationRoute) Scope() string {
	return r.Method + " /api/v1" + r.GroupPath + r.Path
}

var registeredMutationRoutes = []MutationRoute{
	{ID: CreateDocumentMutation, Method: http.MethodPost, GroupPath: "/documents", Path: ""},
	{ID: CompleteDocumentMutation, Method: http.MethodPost, GroupPath: "/documents", Path: "/:id/complete"},
	{ID: UploadFileMutation, Method: http.MethodPost, GroupPath: "/files", Path: "/upload"},
	{ID: ReplaceFileMutation, Method: http.MethodPost, GroupPath: "/files", Path: "/:id/replace"},
	{ID: ConvertNonStandardInventoryMutation, Method: http.MethodPost, GroupPath: "/non-std-inventory", Path: "/:id/convert"},
}

// RegisteredMutationRoutes returns a copy of the public mutation registry.
func RegisteredMutationRoutes() []MutationRoute {
	return append([]MutationRoute(nil), registeredMutationRoutes...)
}

// RegisteredMutationRoute returns one required mutation route by stable ID.
func RegisteredMutationRoute(id MutationRouteID) MutationRoute {
	for _, route := range registeredMutationRoutes {
		if route.ID == id {
			return route
		}
	}
	panic("unknown idempotent mutation route: " + string(id))
}

func isAllowedMutationScope(scope string) bool {
	for _, route := range registeredMutationRoutes {
		if route.Scope() == scope {
			return true
		}
	}
	return false
}

// RegisterRoutes registers authenticated idempotency status routes.
func RegisterRoutes(rg *gin.RouterGroup, handler *Handler, authMw gin.HandlerFunc) {
	operations := rg.Group("/operations")
	operations.Use(authMw)
	operations.GET("/idempotency", handler.GetStatus)
	operations.GET("/idempotency/:key", handler.GetStatus)
}
