// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package warehouse

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"rims-go/internal/types"
)

func TestWarehouseReadDetailRouteRequiresReadPermission(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := newWarehousePermissionRouter("__none__")
	rec := performWarehousePermissionRequest(router, http.MethodGet, "/api/v1/warehouses/7", "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 when warehouse:read is missing; body=%s", rec.Code, rec.Body.String())
	}

	router = newWarehousePermissionRouter("warehouse:read")
	rec = performWarehousePermissionRequest(router, http.MethodGet, "/api/v1/warehouses/7", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 when warehouse:read is granted; body=%s", rec.Code, rec.Body.String())
	}
}

func TestWarehouseReadDetailHidesInaccessibleWarehouseForNonAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)

	uwRepo := &routeUserWarehouseRepo{hasAccessSet: true, hasAccessAllowed: false}
	router := newWarehousePermissionRouterWithUserWarehouseRepo("warehouse:read", uwRepo)
	rec := performWarehousePermissionRequest(router, http.MethodGet, "/api/v1/warehouses/7", "")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for inaccessible warehouse detail; body=%s", rec.Code, rec.Body.String())
	}
	if uwRepo.hasAccessCalls != 1 || uwRepo.hasAccessUserID != 42 || uwRepo.hasAccessWarehouseID != 7 {
		t.Fatalf("HasAccess calls/user/warehouse = %d/%d/%d, want 1/42/7",
			uwRepo.hasAccessCalls, uwRepo.hasAccessUserID, uwRepo.hasAccessWarehouseID)
	}
}

func TestWarehouseServiceReadScope(t *testing.T) {
	t.Run("admin can read any warehouse without binding check", func(t *testing.T) {
		uwRepo := &routeUserWarehouseRepo{hasAccessSet: true, hasAccessAllowed: false}
		svc := NewWarehouseService(routeWarehouseRepo{}, uwRepo, passThroughRouteWarehouseTx)

		resp, err := svc.GetByID(context.Background(), 42, "admin", 7)

		if err != nil {
			t.Fatalf("GetByID() error = %v, want nil for admin", err)
		}
		if resp == nil {
			t.Fatal("GetByID() response = nil, want warehouse response")
		}
		if uwRepo.hasAccessCalls != 0 {
			t.Fatalf("HasAccess calls = %d, want 0 for admin", uwRepo.hasAccessCalls)
		}
	})

	t.Run("non-admin inaccessible warehouse is hidden", func(t *testing.T) {
		uwRepo := &routeUserWarehouseRepo{hasAccessSet: true, hasAccessAllowed: false}
		svc := NewWarehouseService(routeWarehouseRepo{}, uwRepo, passThroughRouteWarehouseTx)

		resp, err := svc.GetByID(context.Background(), 42, "staff", 7)

		if resp != nil {
			t.Fatalf("GetByID() response = %#v, want nil for inaccessible warehouse", resp)
		}
		assertWarehouseAppErrorCode(t, err, types.ErrCodeNotFound)
		if uwRepo.hasAccessCalls != 1 || uwRepo.hasAccessUserID != 42 || uwRepo.hasAccessWarehouseID != 7 {
			t.Fatalf("HasAccess calls/user/warehouse = %d/%d/%d, want 1/42/7",
				uwRepo.hasAccessCalls, uwRepo.hasAccessUserID, uwRepo.hasAccessWarehouseID)
		}
	})

	t.Run("non-admin access check errors become system errors", func(t *testing.T) {
		uwRepo := &routeUserWarehouseRepo{hasAccessErr: errors.New("access lookup failed")}
		svc := NewWarehouseService(routeWarehouseRepo{}, uwRepo, passThroughRouteWarehouseTx)

		resp, err := svc.GetByID(context.Background(), 42, "staff", 7)

		if resp != nil {
			t.Fatalf("GetByID() response = %#v, want nil on access error", resp)
		}
		assertWarehouseAppErrorCode(t, err, types.ErrCodeSystemError)
	})
}

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

func TestWarehouseUserBindingReusesCountByUserIDForNonAdmin(t *testing.T) {
	uwRepo := &routeUserWarehouseRepo{userRoleCode: "staff"}
	svc := NewWarehouseService(routeWarehouseRepo{}, uwRepo, passThroughRouteWarehouseTx)

	err := svc.BindUsers(context.Background(), 7, BindUsersRequest{UserIDs: []uint{101}})

	if err != nil {
		t.Fatalf("BindUsers() error = %v, want nil", err)
	}
	if uwRepo.countByUserIDCalls != 1 || uwRepo.countByUserIDUserID != 101 {
		t.Fatalf("CountByUserID calls/user = %d/%d, want 1/101", uwRepo.countByUserIDCalls, uwRepo.countByUserIDUserID)
	}
}

func newWarehousePermissionRouter(allowedPermission string) *gin.Engine {
	return newWarehousePermissionRouterWithUserWarehouseRepo(allowedPermission, &routeUserWarehouseRepo{})
}

func newWarehousePermissionRouterWithUserWarehouseRepo(allowedPermission string, uwRepo *routeUserWarehouseRepo) *gin.Engine {
	router := gin.New()
	handler := NewHandler(NewWarehouseService(
		routeWarehouseRepo{},
		uwRepo,
		passThroughRouteWarehouseTx,
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

func passThroughRouteWarehouseTx(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
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

type routeUserWarehouseRepo struct {
	hasAccessSet         bool
	hasAccessAllowed     bool
	hasAccessErr         error
	hasAccessCalls       int
	hasAccessUserID      uint
	hasAccessWarehouseID uint
	userRoleCode         string
	countByUserIDCalls   int
	countByUserIDUserID  uint
	countByUserIDResult  int64
	countByUserIDErr     error
}

func (r *routeUserWarehouseRepo) Create(context.Context, *UserWarehouse) error { return nil }
func (r *routeUserWarehouseRepo) Delete(context.Context, uint, uint) error     { return nil }
func (r *routeUserWarehouseRepo) DeleteByWarehouseID(context.Context, uint) error {
	return nil
}
func (r *routeUserWarehouseRepo) GetByUserAndWarehouse(_ context.Context, userID, warehouseID uint) (*UserWarehouse, error) {
	if userID == 101 {
		return nil, gorm.ErrRecordNotFound
	}
	return &UserWarehouse{UserID: userID, WarehouseID: warehouseID}, nil
}
func (r *routeUserWarehouseRepo) ListByUserID(context.Context, uint) ([]UserWarehouse, error) {
	return nil, nil
}
func (r *routeUserWarehouseRepo) ListByWarehouseID(context.Context, uint, types.PageRequest) ([]WarehouseUserInfo, int64, error) {
	return []WarehouseUserInfo{}, 0, nil
}
func (r *routeUserWarehouseRepo) GetDefaultByUserID(context.Context, uint) (*UserWarehouse, error) {
	return nil, gorm.ErrRecordNotFound
}
func (r *routeUserWarehouseRepo) ClearDefault(context.Context, uint) error     { return nil }
func (r *routeUserWarehouseRepo) SetDefault(context.Context, uint, uint) error { return nil }
func (r *routeUserWarehouseRepo) CountByUserID(ctx context.Context, userID uint) (int64, error) {
	r.countByUserIDCalls++
	r.countByUserIDUserID = userID
	if r.countByUserIDErr != nil {
		return 0, r.countByUserIDErr
	}
	return r.countByUserIDResult, nil
}
func (r *routeUserWarehouseRepo) GetUserRoleCode(context.Context, uint) (string, error) {
	if r.userRoleCode != "" {
		return r.userRoleCode, nil
	}
	return "admin", nil
}
func (r *routeUserWarehouseRepo) GetDefaultWarehouseID(context.Context, uint) (uint, error) {
	return 7, nil
}
func (r *routeUserWarehouseRepo) HasAccess(ctx context.Context, userID, warehouseID uint) (bool, error) {
	r.hasAccessCalls++
	r.hasAccessUserID = userID
	r.hasAccessWarehouseID = warehouseID
	if r.hasAccessErr != nil {
		return false, r.hasAccessErr
	}
	if r.hasAccessSet {
		return r.hasAccessAllowed, nil
	}
	return true, nil
}

func assertWarehouseAppErrorCode(t *testing.T, err error, wantCode int) *types.AppError {
	t.Helper()
	var appErr *types.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("error = %v, want AppError code %d", err, wantCode)
	}
	if appErr.Code != wantCode {
		t.Fatalf("error code = %d, want %d (error %v)", appErr.Code, wantCode, err)
	}
	return appErr
}
