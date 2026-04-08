// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"

	"rims-go/internal/auth"
	"rims-go/internal/types"
)

// JWTAuth validates the Bearer token and sets user identity in context.
func JWTAuth(tokenSvc *auth.TokenService) gin.HandlerFunc {
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

		c.Set(types.CtxKeyUserID, claims.UserID)
		c.Set(types.CtxKeyUsername, claims.Username)
		c.Set(types.CtxKeyRoleID, claims.RoleID)
		c.Set(types.CtxKeyRoleCode, claims.RoleCode)
		c.Next()
	}
}
