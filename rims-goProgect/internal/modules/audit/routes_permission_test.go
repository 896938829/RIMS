// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package audit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"rims-go/internal/types"
)

func TestAuditRoutesRequireReadPermission(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name string
		path string
	}{
		{name: "list audit logs", path: "/api/v1/audit/logs"},
		{name: "get audit log", path: "/api/v1/audit/logs/5"},
	}

	for _, tt := range tests {
		t.Run(tt.name+" rejects missing permission", func(t *testing.T) {
			router := newAuditPermissionRouter("__none__")
			rec := performAuditPermissionRequest(router, tt.path)

			if rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403 when audit:read is missing; body=%s", rec.Code, rec.Body.String())
			}
		})

		t.Run(tt.name+" enters handler with audit read permission", func(t *testing.T) {
			router := newAuditPermissionRouter("audit:read")
			rec := performAuditPermissionRequest(router, tt.path)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 when audit:read is granted; body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func newAuditPermissionRouter(allowedPermission string) *gin.Engine {
	router := gin.New()
	handler := NewHandler(NewAuditService(routeAuditRepo{}))
	api := router.Group("/api/v1")
	RegisterRoutes(api, handler, routeAuditAuth(), routeAuditPermissionGate(allowedPermission))
	return router
}

func performAuditPermissionRequest(router *gin.Engine, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func routeAuditAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(types.CtxKeyUserID, uint(42))
		c.Set(types.CtxKeyRoleID, uint(9))
		c.Set(types.CtxKeyRoleCode, "staff")
		c.Next()
	}
}

func routeAuditPermissionGate(allowedPermission string) func(string) gin.HandlerFunc {
	return func(requiredPermission string) gin.HandlerFunc {
		return func(c *gin.Context) {
			if requiredPermission != allowedPermission {
				types.FailFromError(c, types.ErrForbidden())
				c.Abort()
				return
			}
			c.Next()
		}
	}
}

type routeAuditRepo struct{}

func (routeAuditRepo) Create(context.Context, *AuditLog) error { return nil }
func (routeAuditRepo) GetByID(context.Context, uint) (*AuditLog, error) {
	log := &AuditLog{
		Action:      ActionLogin,
		Resource:    ResourceUser,
		Description: "login",
		Details:     "{}",
		Result:      ResultSuccess,
	}
	log.ID = 5
	return log, nil
}
func (routeAuditRepo) List(context.Context, ListFilter, types.PageRequest) ([]AuditLog, int64, error) {
	return []AuditLog{}, 0, nil
}
