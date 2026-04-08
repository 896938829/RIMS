// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package middleware

import (
	"log"
	"time"

	"github.com/gin-gonic/gin"

	"rims-go/internal/types"
)

// Logger logs each request with method, path, status, latency and trace ID.
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		latency := time.Since(start)
		traceID := types.GetTraceID(c)
		log.Printf("[%s] %s %s | %d | %v | user=%d",
			traceID,
			c.Request.Method,
			c.Request.URL.Path,
			c.Writer.Status(),
			latency,
			types.GetUserID(c),
		)
	}
}
