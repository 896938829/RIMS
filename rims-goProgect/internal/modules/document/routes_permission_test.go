// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package document

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestIdempotentDocumentRoutesUseRegistryPermission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		path string
		want string
	}{
		{path: "/api/v1/documents", want: "document:create"},
		{path: "/api/v1/documents/9/complete", want: "document:complete"},
		{path: "/api/v1/documents/9/confirm", want: "stocktake:confirm"},
		{path: "/api/v1/documents/9/settle", want: "stocktake:settle"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			var got string
			idemCalls := 0
			router := gin.New()
			api := router.Group("/api/v1")
			RegisterRoutes(
				api,
				nil,
				func(c *gin.Context) { c.Next() },
				func(c *gin.Context) { c.Next() },
				func(c *gin.Context) { idemCalls++; c.Next() },
				func(code string) gin.HandlerFunc {
					return func(c *gin.Context) {
						got = code
						c.AbortWithStatus(http.StatusNoContent)
					}
				},
			)

			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, tt.path, nil))
			if recorder.Code != http.StatusNoContent || got != tt.want || idemCalls != 0 {
				t.Fatalf("status/permission/idempotency = %d/%q/%d, want 204/%q/0", recorder.Code, got, idemCalls, tt.want)
			}
		})
	}
}
