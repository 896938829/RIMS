// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package types

import "github.com/gin-gonic/gin"

// Context keys used by middleware to store request-scoped values.
const (
	CtxKeyUserID      = "userID"
	CtxKeyUsername     = "username"
	CtxKeyRoleID      = "roleID"
	CtxKeyRoleCode    = "roleCode"
	CtxKeyWarehouseID = "warehouseID"
	CtxKeyTraceID     = "traceID"
)

// GetUserID returns the authenticated user's ID from context.
func GetUserID(c *gin.Context) uint {
	v, _ := c.Get(CtxKeyUserID)
	id, _ := v.(uint)
	return id
}

// GetUsername returns the authenticated user's username from context.
func GetUsername(c *gin.Context) string {
	v, _ := c.Get(CtxKeyUsername)
	s, _ := v.(string)
	return s
}

// GetRoleID returns the authenticated user's role ID from context.
func GetRoleID(c *gin.Context) uint {
	v, _ := c.Get(CtxKeyRoleID)
	id, _ := v.(uint)
	return id
}

// GetRoleCode returns the authenticated user's role code from context.
func GetRoleCode(c *gin.Context) string {
	v, _ := c.Get(CtxKeyRoleCode)
	s, _ := v.(string)
	return s
}

// GetWarehouseID returns the current warehouse ID from context.
func GetWarehouseID(c *gin.Context) uint {
	v, _ := c.Get(CtxKeyWarehouseID)
	id, _ := v.(uint)
	return id
}

// GetTraceID returns the request trace ID from context.
func GetTraceID(c *gin.Context) string {
	v, _ := c.Get(CtxKeyTraceID)
	s, _ := v.(string)
	return s
}

// IsAdmin checks if the current user has admin role.
func IsAdmin(c *gin.Context) bool {
	return GetRoleCode(c) == "admin"
}
