// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package user

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

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

func TestRoleWriteHandlersRequireAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewHandler(nil, NewRoleService(roleRepoStub{}), nil)

	tests := []struct {
		name       string
		method     string
		target     string
		body       string
		params     gin.Params
		handleFunc func(*gin.Context)
	}{
		{
			name:       "create role",
			method:     http.MethodPost,
			target:     "/api/v1/roles",
			body:       `{"code":"operator","name":"Operator"}`,
			handleFunc: handler.CreateRole,
		},
		{
			name:       "update role",
			method:     http.MethodPut,
			target:     "/api/v1/roles/1",
			body:       `{"name":"Operator"}`,
			params:     gin.Params{{Key: "id", Value: "1"}},
			handleFunc: handler.UpdateRole,
		},
		{
			name:       "delete role",
			method:     http.MethodDelete,
			target:     "/api/v1/roles/1",
			params:     gin.Params{{Key: "id", Value: "1"}},
			handleFunc: handler.DeleteRole,
		},
		{
			name:       "assign permissions",
			method:     http.MethodPut,
			target:     "/api/v1/roles/1/permissions",
			body:       `{"permissionIds":[1]}`,
			params:     gin.Params{{Key: "id", Value: "1"}},
			handleFunc: handler.AssignPermissions,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Params = tt.params
			c.Set(types.CtxKeyRoleCode, "operator")
			c.Request = httptest.NewRequest(tt.method, tt.target, strings.NewReader(tt.body))
			c.Request.Header.Set("Content-Type", "application/json")

			tt.handleFunc(c)

			if w.Code != http.StatusForbidden {
				t.Fatalf("expected status %d, got %d with body %s", http.StatusForbidden, w.Code, w.Body.String())
			}
		})
	}
}
