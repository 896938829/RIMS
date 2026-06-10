// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package warehouse

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"rims-go/internal/types"
)

func TestWarehouseUserBindingRoutesRequireDistinctPermissions(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name          string
		method        string
		path          string
		body          string
		permission    string
		successStatus int
	}{
		{
			name:          "bind users",
			method:        http.MethodPost,
			path:          "/api/v1/warehouses/7/users",
			body:          `{"userIds":[101]}`,
			permission:    "warehouse:bind_user",
			successStatus: http.StatusOK,
		},
		{
			name:          "unbind user",
			method:        http.MethodDelete,
			path:          "/api/v1/warehouses/7/users/202",
			permission:    "warehouse:unbind_user",
			successStatus: http.StatusNoContent,
		},
		{
			name:          "list warehouse users",
			method:        http.MethodGet,
			path:          "/api/v1/warehouses/7/users",
			permission:    "warehouse:list_users",
			successStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name+" rejects missing permission", func(t *testing.T) {
			router := newWarehousePermissionRouter("__none__")
			rec := performWarehousePermissionRequest(router, tt.method, tt.path, tt.body)

			if rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403 when %s is missing; body=%s", rec.Code, tt.permission, rec.Body.String())
			}
		})

		t.Run(tt.name+" enters handler with required permission", func(t *testing.T) {
			router := newWarehousePermissionRouter(tt.permission)
			rec := performWarehousePermissionRequest(router, tt.method, tt.path, tt.body)

			if rec.Code != tt.successStatus {
				t.Fatalf("status = %d, want %d when %s is granted; body=%s", rec.Code, tt.successStatus, tt.permission, rec.Body.String())
			}
		})
	}
}

func newWarehousePermissionRouter(allowedPermission string) *gin.Engine {
	router := gin.New()
	handler := NewHandler(NewWarehouseService(
		routeWarehouseRepo{},
		routeUserWarehouseRepo{},
		func(ctx context.Context, fn func(context.Context) error) error {
			return fn(ctx)
		},
	))
	api := router.Group("/api/v1")
	RegisterRoutes(api, handler, routeWarehouseAuth(), routePermissionGate(allowedPermission))
	return router
}

func performWarehousePermissionRequest(router *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func routeWarehouseAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(types.CtxKeyUserID, uint(42))
		c.Set(types.CtxKeyRoleID, uint(9))
		c.Set(types.CtxKeyRoleCode, "staff")
		c.Next()
	}
}

func routePermissionGate(allowedPermission string) func(string) gin.HandlerFunc {
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

type routeWarehouseRepo struct{}

func (routeWarehouseRepo) Create(context.Context, *Warehouse) error { return nil }
func (routeWarehouseRepo) GetByID(context.Context, uint) (*Warehouse, error) {
	return &Warehouse{Status: 1}, nil
}
func (routeWarehouseRepo) GetByCode(context.Context, string) (*Warehouse, error) {
	return nil, gorm.ErrRecordNotFound
}
func (routeWarehouseRepo) List(context.Context, types.PageRequest) ([]Warehouse, int64, error) {
	return nil, 0, nil
}
func (routeWarehouseRepo) ListByIDs(context.Context, []uint) ([]Warehouse, int64, error) {
	return nil, 0, nil
}
func (routeWarehouseRepo) Update(context.Context, *Warehouse) error { return nil }
func (routeWarehouseRepo) Delete(context.Context, uint) error       { return nil }

type routeUserWarehouseRepo struct{}

func (routeUserWarehouseRepo) Create(context.Context, *UserWarehouse) error { return nil }
func (routeUserWarehouseRepo) Delete(context.Context, uint, uint) error     { return nil }
func (routeUserWarehouseRepo) DeleteByWarehouseID(context.Context, uint) error {
	return nil
}
func (routeUserWarehouseRepo) GetByUserAndWarehouse(_ context.Context, userID, warehouseID uint) (*UserWarehouse, error) {
	if userID == 101 {
		return nil, gorm.ErrRecordNotFound
	}
	return &UserWarehouse{UserID: userID, WarehouseID: warehouseID}, nil
}
func (routeUserWarehouseRepo) ListByUserID(context.Context, uint) ([]UserWarehouse, error) {
	return nil, nil
}
func (routeUserWarehouseRepo) ListByWarehouseID(context.Context, uint, types.PageRequest) ([]WarehouseUserInfo, int64, error) {
	return []WarehouseUserInfo{}, 0, nil
}
func (routeUserWarehouseRepo) GetDefaultByUserID(context.Context, uint) (*UserWarehouse, error) {
	return nil, gorm.ErrRecordNotFound
}
func (routeUserWarehouseRepo) ClearDefault(context.Context, uint) error     { return nil }
func (routeUserWarehouseRepo) SetDefault(context.Context, uint, uint) error { return nil }
func (routeUserWarehouseRepo) CountByUserID(context.Context, uint) (int64, error) {
	return 0, nil
}
func (routeUserWarehouseRepo) GetUserRoleCode(context.Context, uint) (string, error) {
	return "admin", nil
}
func (routeUserWarehouseRepo) GetDefaultWarehouseID(context.Context, uint) (uint, error) {
	return 7, nil
}
func (routeUserWarehouseRepo) HasAccess(context.Context, uint, uint) (bool, error) {
	return true, nil
}
