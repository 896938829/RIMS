// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package middleware

import (
	"context"
	"strings"

	"github.com/gin-gonic/gin"

	"rims-go/internal/auth"
	"rims-go/internal/types"
)

// AuthUserProvider loads the current user identity for an already-valid token.
type AuthUserProvider interface {
	GetAuthUser(ctx context.Context, userID uint) (uint, string, uint, string, int8, error)
}

// JWTAuth validates the Bearer token and sets user identity in context.
func JWTAuth(tokenSvc *auth.TokenService, users AuthUserProvider) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			types.Fail(c, 401, types.ErrAuth("缺少认证头"))
			c.Abort()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" || parts[1] == "" {
			types.Fail(c, 401, types.ErrAuth("认证头格式错误"))
			c.Abort()
			return
		}

		claims, err := tokenSvc.ParseToken(parts[1])
		if err != nil {
			types.Fail(c, 401, types.ErrAuth("无效的令牌"))
			c.Abort()
			return
		}

		userID, username, roleID, roleCode, status, err := users.GetAuthUser(c.Request.Context(), claims.UserID)
		if err != nil || status != 1 || roleID == 0 || roleCode == "" {
			types.Fail(c, 401, types.ErrAuth("无效的令牌"))
			c.Abort()
			return
		}

		c.Set(types.CtxKeyUserID, userID)
		c.Set(types.CtxKeyUsername, username)
		c.Set(types.CtxKeyRoleID, roleID)
		c.Set(types.CtxKeyRoleCode, roleCode)
		c.Next()
	}
}
