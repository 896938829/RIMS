// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// CORS adds Cross-Origin Resource Sharing headers.
func CORS(origins string) gin.HandlerFunc {
	allowedOrigins := strings.Split(origins, ",")
	allowWildcard := false
	for i := range allowedOrigins {
		allowedOrigins[i] = strings.TrimSpace(allowedOrigins[i])
		if allowedOrigins[i] == "*" {
			allowWildcard = true
		}
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		allowOrigin := ""
		if allowWildcard {
			allowOrigin = "*"
		} else {
			for _, o := range allowedOrigins {
				if o != "" && o == origin {
					allowOrigin = origin
					break
				}
			}
		}

		if allowOrigin != "" {
			c.Header("Access-Control-Allow-Origin", allowOrigin)
			c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Authorization,Content-Type,X-Request-ID,X-Warehouse-ID,Idempotency-Key")
			c.Header("Access-Control-Expose-Headers", "X-Request-ID")
			c.Header("Access-Control-Max-Age", "86400")
		}

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
