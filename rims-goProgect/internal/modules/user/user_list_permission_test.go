// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package user

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"rims-go/internal/types"
)

func TestUserRepositoryListPreloadsRolePermissionsForDeclaredDTO(t *testing.T) {
	gormDB, err := gorm.Open(postgres.New(postgres.Config{
		DSN: "host=127.0.0.1 user=rims password=rims dbname=rims sslmode=disable",
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("open dry-run db: %v", err)
	}
	repo := &userRepo{gormDB: gormDB}

	query := repo.listQuery(context.Background(), types.PageRequest{})

	if _, ok := query.Statement.Preloads["Role.Permissions"]; !ok {
		t.Fatalf("preloads = %#v, want Role.Permissions", query.Statement.Preloads)
	}
}

func TestListUsersHandlerReturnsActualPermissionCodes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &userRepoForServiceTest{listUsers: []User{{
		BaseModel: types.BaseModel{ID: 7},
		Username:  "alice",
		RoleID:    2,
		Status:    1,
		Role: roleWithPermissions(
			2, "operator", "file:upload", "document:create", "document:create",
		),
	}}}
	handler := NewHandler(NewUserService(repo, &roleRepoForServiceTest{}, nil), nil, nil)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/users", nil)

	handler.ListUsers(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status/body = %d/%s", recorder.Code, recorder.Body.String())
	}
	var envelope struct {
		Data struct {
			Items []UserResponse `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(envelope.Data.Items) != 1 {
		t.Fatalf("items = %#v, want one; body=%s", envelope.Data.Items, recorder.Body.String())
	}
	want := []string{"document:create", "file:upload"}
	got := envelope.Data.Items[0].PermissionCodes
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("permissionCodes = %#v, want %#v", got, want)
	}
}
