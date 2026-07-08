// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package user

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"rims-go/internal/middleware"
	"rims-go/internal/types"
)

type roleRepoStub struct{}

func (roleRepoStub) Create(ctx context.Context, role *Role) error {
	role.ID = 1
	return nil
}

func (roleRepoStub) GetByID(ctx context.Context, id uint) (*Role, error) {
	return &Role{Code: "operator", Name: "Operator"}, nil
}

func (roleRepoStub) GetByCode(ctx context.Context, code string) (*Role, error) {
	return nil, gorm.ErrRecordNotFound
}

func (roleRepoStub) List(ctx context.Context) ([]Role, error) {
	return nil, nil
}

func (roleRepoStub) Update(ctx context.Context, role *Role) error {
	return nil
}

func (roleRepoStub) Delete(ctx context.Context, id uint) error {
	return nil
}

func (roleRepoStub) AssignPermissions(ctx context.Context, roleID uint, permIDs []uint) error {
	return nil
}

func (roleRepoStub) ListPermissions(ctx context.Context) ([]Permission, error) {
	return nil, nil
}

func (roleRepoStub) HasPermission(ctx context.Context, roleID uint, code string) (bool, error) {
	return false, nil
}

func (roleRepoStub) CountActiveUsersByRoleID(ctx context.Context, roleID uint) (int64, error) {
	return 0, nil
}

type routePermissionChecker struct {
	allowed map[string]bool
	codes   []string
}

func (c *routePermissionChecker) HasPermission(ctx context.Context, roleID uint, code string) (bool, error) {
	c.codes = append(c.codes, code)
	return c.allowed[code], nil
}

func TestRoleWriteRoutesRequirePermission(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewHandler(nil, NewRoleService(roleRepoStub{}), nil)
	checker := &routePermissionChecker{allowed: map[string]bool{}}
	router := newRoleRouteAuthRouter(handler, checker)

	tests := []struct {
		name   string
		method string
		target string
		body   string
		code   string
	}{
		{
			name:   "create role",
			method: http.MethodPost,
			target: "/api/v1/roles",
			body:   `{"code":"operator","name":"Operator"}`,
			code:   "role:create",
		},
		{
			name:   "update role",
			method: http.MethodPut,
			target: "/api/v1/roles/1",
			body:   `{"name":"Operator"}`,
			code:   "role:update",
		},
		{
			name:   "delete role",
			method: http.MethodDelete,
			target: "/api/v1/roles/1",
			code:   "role:delete",
		},
		{
			name:   "assign permissions",
			method: http.MethodPut,
			target: "/api/v1/roles/1/permissions",
			body:   `{"permissionIds":[1]}`,
			code:   "role:assign_permissions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker.codes = nil
			w := httptest.NewRecorder()
			req := httptest.NewRequest(tt.method, tt.target, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")

			router.ServeHTTP(w, req)
			if w.Code != http.StatusForbidden {
				t.Fatalf("expected status %d, got %d with body %s", http.StatusForbidden, w.Code, w.Body.String())
			}
			assertRouteAuthCode(t, w, types.ErrCodePermissionDenied)
			if len(checker.codes) != 1 || checker.codes[0] != tt.code {
				t.Fatalf("permission codes = %#v, want [%q]", checker.codes, tt.code)
			}
		})
	}
}

func TestUserReadRoutesRequirePermission(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewHandler(NewUserService(&userRepoForServiceTest{}, &roleRepoForServiceTest{}, nil), nil, nil)
	checker := &routePermissionChecker{allowed: map[string]bool{}}
	router := newRoleRouteAuthRouter(handler, checker)

	tests := []struct {
		name   string
		target string
		code   string
	}{
		{
			name:   "list users",
			target: "/api/v1/users",
			code:   "user:list",
		},
		{
			name:   "read user",
			target: "/api/v1/users/1",
			code:   "user:read",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker.codes = nil
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tt.target, nil)

			router.ServeHTTP(w, req)
			if w.Code != http.StatusForbidden {
				t.Fatalf("expected status %d, got %d with body %s", http.StatusForbidden, w.Code, w.Body.String())
			}
			assertRouteAuthCode(t, w, types.ErrCodePermissionDenied)
			if len(checker.codes) != 1 || checker.codes[0] != tt.code {
				t.Fatalf("permission codes = %#v, want [%q]", checker.codes, tt.code)
			}
		})
	}
}

func TestRoleAndPermissionReadRoutesRequirePermission(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewHandler(nil, NewRoleService(roleRepoStub{}), nil)
	checker := &routePermissionChecker{allowed: map[string]bool{}}
	router := newRoleRouteAuthRouter(handler, checker)

	tests := []struct {
		name   string
		target string
		code   string
	}{
		{
			name:   "list roles",
			target: "/api/v1/roles",
			code:   "role:list",
		},
		{
			name:   "read role",
			target: "/api/v1/roles/1",
			code:   "role:read",
		},
		{
			name:   "list permissions",
			target: "/api/v1/permissions",
			code:   "permission:list",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker.codes = nil
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tt.target, nil)

			router.ServeHTTP(w, req)
			if w.Code != http.StatusForbidden {
				t.Fatalf("expected status %d, got %d with body %s", http.StatusForbidden, w.Code, w.Body.String())
			}
			assertRouteAuthCode(t, w, types.ErrCodePermissionDenied)
			if len(checker.codes) != 1 || checker.codes[0] != tt.code {
				t.Fatalf("permission codes = %#v, want [%q]", checker.codes, tt.code)
			}
		})
	}
}

func TestRoleWriteRoutesAllowRoleWithPermission(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewHandler(nil, NewRoleService(roleRepoStub{}), nil)
	tests := []struct {
		name       string
		method     string
		target     string
		body       string
		code       string
		wantStatus int
	}{
		{
			name:       "create role",
			method:     http.MethodPost,
			target:     "/api/v1/roles",
			body:       `{"code":"operator","name":"Operator"}`,
			code:       "role:create",
			wantStatus: http.StatusCreated,
		},
		{
			name:       "update role",
			method:     http.MethodPut,
			target:     "/api/v1/roles/1",
			body:       `{"name":"Operator"}`,
			code:       "role:update",
			wantStatus: http.StatusOK,
		},
		{
			name:       "delete role",
			method:     http.MethodDelete,
			target:     "/api/v1/roles/1",
			code:       "role:delete",
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "assign permissions",
			method:     http.MethodPut,
			target:     "/api/v1/roles/1/permissions",
			body:       `{"permissionIds":[1]}`,
			code:       "role:assign_permissions",
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := &routePermissionChecker{allowed: map[string]bool{tt.code: true}}
			router := newRoleRouteAuthRouter(handler, checker)
			w := httptest.NewRecorder()
			req := httptest.NewRequest(tt.method, tt.target, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")

			router.ServeHTTP(w, req)
			if w.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d with body %s", tt.wantStatus, w.Code, w.Body.String())
			}
		})
	}
}

func newRoleRouteAuthRouter(handler *Handler, checker *routePermissionChecker) *gin.Engine {
	r := gin.New()
	api := r.Group("/api/v1")
	authMw := func(c *gin.Context) {
		c.Set(types.CtxKeyRoleID, uint(7))
		c.Set(types.CtxKeyRoleCode, "operator")
		c.Next()
	}
	RegisterRoutes(api, handler, authMw, func(code string) gin.HandlerFunc {
		return middleware.Permission(checker, code)
	})
	return r
}

func assertRouteAuthCode(t *testing.T, w *httptest.ResponseRecorder, want int) {
	t.Helper()
	var resp types.Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response JSON = %q, unmarshal error: %v", w.Body.String(), err)
	}
	if resp.Code != want {
		t.Fatalf("response code = %d, want %d; body=%s", resp.Code, want, w.Body.String())
	}
}
