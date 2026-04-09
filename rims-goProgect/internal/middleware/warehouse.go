// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package middleware

import (
	"context"
	"strconv"

	"github.com/gin-gonic/gin"

	"rims-go/internal/types"
)

// WarehouseAccess defines the interface the warehouse middleware needs.
// Satisfied by warehouse.UserWarehouseRepository.
type WarehouseAccess interface {
	GetDefaultWarehouseID(ctx context.Context, userID uint) (uint, error)
	HasAccess(ctx context.Context, userID, warehouseID uint) (bool, error)
}

// WarehouseScope validates and sets the current warehouse context.
// It reads X-Warehouse-ID from the request header; if absent, falls back
// to the user's default warehouse. It then verifies the user has access.
func WarehouseScope(checker WarehouseAccess) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := types.GetUserID(c)
		if userID == 0 {
			types.Fail(c, 401, types.ErrAuth("未认证"))
			c.Abort()
			return
		}

		// 1. Try X-Warehouse-ID header
		warehouseID := parseWarehouseHeader(c)

		// 2. Fall back to user's default warehouse
		if warehouseID == 0 {
			defaultID, err := checker.GetDefaultWarehouseID(c.Request.Context(), userID)
			if err != nil || defaultID == 0 {
				types.Fail(c, 403, types.ErrValidation("请先选择仓库"))
				c.Abort()
				return
			}
			warehouseID = defaultID
		}

		// 3. Validate user has access
		ok, err := checker.HasAccess(c.Request.Context(), userID, warehouseID)
		if err != nil || !ok {
			types.Fail(c, 403, types.ErrForbidden())
			c.Abort()
			return
		}

		// 4. Set warehouse ID in context
		c.Set(types.CtxKeyWarehouseID, warehouseID)
		c.Next()
	}
}

func parseWarehouseHeader(c *gin.Context) uint {
	header := c.GetHeader("X-Warehouse-ID")
	if header == "" {
		return 0
	}
	id, err := strconv.ParseUint(header, 10, 64)
	if err != nil {
		return 0
	}
	return uint(id)
}
