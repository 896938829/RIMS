// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package product

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

const routePermissionWarehouseID uint = 88

func TestProductInventoryAndNonStdRoutesRequireExpectedPermissions(t *testing.T) {
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
			name:          "create product",
			method:        http.MethodPost,
			path:          "/api/v1/products",
			body:          `{"code":"P001","name":"测试商品","unit":"件","retailPrice":10,"costPrice":5}`,
			permission:    "product:create",
			successStatus: http.StatusCreated,
		},
		{
			name:          "update product",
			method:        http.MethodPut,
			path:          "/api/v1/products/1",
			body:          `{"name":"更新商品"}`,
			permission:    "product:update",
			successStatus: http.StatusOK,
		},
		{
			name:          "delete product",
			method:        http.MethodDelete,
			path:          "/api/v1/products/1",
			permission:    "product:delete",
			successStatus: http.StatusNoContent,
		},
		{
			name:          "update inventory",
			method:        http.MethodPut,
			path:          "/api/v1/inventory/5",
			body:          `{"alertThreshold":3}`,
			permission:    "inventory:update",
			successStatus: http.StatusOK,
		},
		{
			name:          "create non-standard inventory",
			method:        http.MethodPost,
			path:          "/api/v1/non-std-inventory",
			body:          `{"tempLabel":"TMP-1","description":"散货","unit":"件","quantity":3}`,
			permission:    "non_std:create",
			successStatus: http.StatusCreated,
		},
		{
			name:          "list non-standard inventory",
			method:        http.MethodGet,
			path:          "/api/v1/non-std-inventory",
			permission:    "non_std:read",
			successStatus: http.StatusOK,
		},
		{
			name:          "get non-standard inventory",
			method:        http.MethodGet,
			path:          "/api/v1/non-std-inventory/9",
			permission:    "non_std:read",
			successStatus: http.StatusOK,
		},
		{
			name:          "update non-standard inventory",
			method:        http.MethodPut,
			path:          "/api/v1/non-std-inventory/9",
			body:          `{"description":"已更新"}`,
			permission:    "non_std:update",
			successStatus: http.StatusOK,
		},
		{
			name:          "delete non-standard inventory",
			method:        http.MethodDelete,
			path:          "/api/v1/non-std-inventory/9",
			permission:    "non_std:delete",
			successStatus: http.StatusNoContent,
		},
		{
			name:          "convert non-standard inventory",
			method:        http.MethodPost,
			path:          "/api/v1/non-std-inventory/9/convert",
			body:          `{"productId":1,"quantity":1}`,
			permission:    "non_std:convert",
			successStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name+" rejects missing permission", func(t *testing.T) {
			router := newProductPermissionRouter("__none__")
			rec := performProductPermissionRequest(router, tt.method, tt.path, tt.body)

			if rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403 when %s is missing; body=%s", rec.Code, tt.permission, rec.Body.String())
			}
		})

		t.Run(tt.name+" enters handler with required permission", func(t *testing.T) {
			router := newProductPermissionRouter(tt.permission)
			rec := performProductPermissionRequest(router, tt.method, tt.path, tt.body)

			if rec.Code != tt.successStatus {
				t.Fatalf("status = %d, want %d when %s is granted; body=%s", rec.Code, tt.successStatus, tt.permission, rec.Body.String())
			}
		})
	}
}

func newProductPermissionRouter(allowedPermission string) *gin.Engine {
	router := gin.New()
	handler := NewHandler(NewProductService(
		routeProductRepo{},
		routeInventoryRepo{},
		routeNonStdRepo{},
		func(ctx context.Context, fn func(context.Context) error) error {
			return fn(ctx)
		},
	))
	api := router.Group("/api/v1")
	RegisterRoutes(api, handler, routeProductAuth(), routeWarehouseScope(), routeNoopIdempotency(), routeProductPermissionGate(allowedPermission))
	return router
}

func performProductPermissionRequest(router *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func routeProductAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(types.CtxKeyUserID, uint(42))
		c.Set(types.CtxKeyRoleID, uint(9))
		c.Set(types.CtxKeyRoleCode, "staff")
		c.Next()
	}
}

func routeWarehouseScope() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(types.CtxKeyWarehouseID, routePermissionWarehouseID)
		c.Next()
	}
}

func routeNoopIdempotency() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
	}
}

func routeProductPermissionGate(allowedPermission string) func(string) gin.HandlerFunc {
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

type routeProductRepo struct{}

func (routeProductRepo) Create(_ context.Context, p *Product) error {
	p.ID = 1
	p.Status = 1
	return nil
}
func (routeProductRepo) GetByID(_ context.Context, id uint) (*Product, error) {
	return &Product{Code: "P001", Name: "测试商品", Unit: "件", Status: 1}, nil
}
func (routeProductRepo) GetByCode(context.Context, string) (*Product, error) {
	return nil, gorm.ErrRecordNotFound
}
func (routeProductRepo) GetByBarcode(context.Context, string) (*Product, error) {
	return nil, gorm.ErrRecordNotFound
}
func (routeProductRepo) List(context.Context, types.PageRequest) ([]Product, int64, error) {
	return []Product{}, 0, nil
}
func (routeProductRepo) Update(context.Context, *Product) error { return nil }
func (routeProductRepo) Delete(context.Context, uint) error     { return nil }
func (routeProductRepo) CountDocumentLinesByProductID(context.Context, uint) (int64, error) {
	return 0, nil
}

type routeInventoryRepo struct{}

func (routeInventoryRepo) Create(_ context.Context, inv *Inventory) error {
	inv.ID = 50
	return nil
}
func (routeInventoryRepo) GetByID(_ context.Context, id uint) (*Inventory, error) {
	return &Inventory{
		WarehouseID: routePermissionWarehouseID,
		ProductID:   1,
		Quantity:    10,
		Status:      1,
		Product:     &Product{Code: "P001", Name: "测试商品", Unit: "件", Status: 1},
	}, nil
}
func (routeInventoryRepo) LockItem(context.Context, uint, uint) error { return nil }
func (routeInventoryRepo) GetByWarehouseAndProductForUpdate(context.Context, uint, uint) (*Inventory, error) {
	return nil, gorm.ErrRecordNotFound
}
func (routeInventoryRepo) GetByWarehouseAndProduct(context.Context, uint, uint) (*Inventory, error) {
	return nil, gorm.ErrRecordNotFound
}
func (routeInventoryRepo) ExistsByProductID(context.Context, uint) (bool, error) {
	return false, nil
}
func (routeInventoryRepo) ListByWarehouse(context.Context, uint, types.PageRequest) ([]Inventory, int64, error) {
	return []Inventory{}, 0, nil
}
func (routeInventoryRepo) ListAlerts(context.Context, uint, types.PageRequest) ([]Inventory, int64, error) {
	return []Inventory{}, 0, nil
}
func (routeInventoryRepo) Update(context.Context, *Inventory) error { return nil }
func (routeInventoryRepo) UpdateSettings(context.Context, uint, *int, *int8, uint) error {
	return nil
}
func (routeInventoryRepo) Delete(context.Context, uint) error { return nil }

type routeNonStdRepo struct{}

func (routeNonStdRepo) Create(_ context.Context, ns *NonStdInventory) error {
	ns.ID = 9
	ns.Status = 1
	return nil
}
func (routeNonStdRepo) GetByID(_ context.Context, id uint) (*NonStdInventory, error) {
	return routePermissionNonStd(id), nil
}
func (routeNonStdRepo) GetByIDForUpdate(_ context.Context, id uint) (*NonStdInventory, error) {
	return routePermissionNonStd(id), nil
}
func (routeNonStdRepo) GetByTempLabel(context.Context, string) (*NonStdInventory, error) {
	return nil, gorm.ErrRecordNotFound
}
func (routeNonStdRepo) ListByWarehouse(context.Context, uint, types.PageRequest) ([]NonStdInventory, int64, error) {
	return []NonStdInventory{}, 0, nil
}
func (routeNonStdRepo) Update(context.Context, *NonStdInventory) error { return nil }
func (routeNonStdRepo) Delete(context.Context, uint) error             { return nil }

func routePermissionNonStd(id uint) *NonStdInventory {
	ns := &NonStdInventory{
		WarehouseID:  routePermissionWarehouseID,
		TempLabel:    "TMP-1",
		Description:  "散货",
		Unit:         "件",
		Quantity:     10,
		ConvertedQty: 0,
		Status:       1,
	}
	ns.ID = id
	return ns
}
