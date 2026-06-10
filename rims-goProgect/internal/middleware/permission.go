// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package middleware

import (
	"context"
	"errors"

	"github.com/gin-gonic/gin"

	"rims-go/internal/types"
)

// PermissionChecker defines the permission lookup required by route-level RBAC.
type PermissionChecker interface {
	HasPermission(ctx context.Context, roleID uint, code string) (bool, error)
}

// Permission requires the current role to have the given permission code.
// Admin roles bypass the lookup so admin access does not depend on seeded rows.
func Permission(checker PermissionChecker, code string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if types.IsAdmin(c) {
			c.Next()
			return
		}

		roleID := types.GetRoleID(c)
		if roleID == 0 {
			types.FailFromError(c, types.ErrForbidden())
			c.Abort()
			return
		}
		if checker == nil {
			types.FailFromError(c, types.ErrSystem(errors.New("permission checker is nil")))
			c.Abort()
			return
		}

		ok, err := checker.HasPermission(c.Request.Context(), roleID, code)
		if err != nil {
			types.FailFromError(c, types.ErrSystem(err))
			c.Abort()
			return
		}
		if !ok {
			types.FailFromError(c, types.ErrForbidden())
			c.Abort()
			return
		}

		c.Next()
	}
}
