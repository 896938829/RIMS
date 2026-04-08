// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package middleware

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/gin-gonic/gin"

	"rims-go/internal/types"
)

// RequestID injects a unique trace ID into the request context and response header.
// If the client sends X-Request-ID, it is reused; otherwise a new one is generated.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-ID")
		if id == "" {
			id = generateID()
		}
		c.Set(types.CtxKeyTraceID, id)
		c.Header("X-Request-ID", id)
		c.Next()
	}
}

func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
