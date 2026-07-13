// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package user

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"rims-go/internal/auth"
	"rims-go/internal/types"
)

func TestUserResponsesExposeStableActualPermissionCodes(t *testing.T) {
	tests := []struct {
		name string
		role *Role
		want []string
	}{
		{
			name: "ordinary user permissions are sorted and deduplicated",
			role: roleWithPermissions(2, "operator", "document:complete", "document:create", "document:create"),
			want: []string{"document:complete", "document:create"},
		},
		{
			name: "admin reports relation permissions rather than an invented wildcard",
			role: roleWithPermissions(1, "admin", "file:upload", "document:create"),
			want: []string{"document:create", "file:upload"},
		},
		{
			name: "role with no permissions returns an empty list",
			role: roleWithPermissions(3, "viewer"),
			want: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := &User{BaseModel: types.BaseModel{ID: 7}, Username: "alice", RoleID: tt.role.ID, Role: tt.role}
			response := ToResponse(u)
			brief := ToUserBrief(u)
			if !reflect.DeepEqual(response.PermissionCodes, tt.want) {
				t.Fatalf("UserResponse permissionCodes = %#v, want %#v", response.PermissionCodes, tt.want)
			}
			if !reflect.DeepEqual(brief.PermissionCodes, tt.want) {
				t.Fatalf("UserBrief permissionCodes = %#v, want %#v", brief.PermissionCodes, tt.want)
			}
		})
	}
}

func TestUserServiceLoginReturnsCurrentRolePermissionRelation(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("secret1"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	u := &User{
		BaseModel:    types.BaseModel{ID: 7},
		Username:     "alice",
		PasswordHash: string(hash),
		RoleID:       2,
		Status:       1,
		Role:         roleWithPermissions(2, "operator", "file:upload", "document:create"),
	}
	svc := NewUserService(
		&userRepoForServiceTest{usersByUsername: map[string]*User{"alice": u}},
		&roleRepoForServiceTest{},
		auth.NewTokenService("permission-test-secret", 1),
	)

	response, err := svc.Login(context.Background(), LoginRequest{Username: "alice", Password: "secret1"})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	want := []string{"document:create", "file:upload"}
	if !reflect.DeepEqual(response.User.PermissionCodes, want) {
		t.Fatalf("login permissionCodes = %#v, want %#v", response.User.PermissionCodes, want)
	}
}

func TestUserServiceCurrentUserReflectsSameRolePermissionUpdate(t *testing.T) {
	role := roleWithPermissions(2, "operator", "document:create", "document:complete")
	u := &User{BaseModel: types.BaseModel{ID: 7}, Username: "alice", RoleID: role.ID, Role: role}
	svc := NewUserService(&userRepoForServiceTest{usersByID: map[uint]*User{7: u}}, &roleRepoForServiceTest{}, nil)

	before, err := svc.GetByID(context.Background(), 7)
	if err != nil {
		t.Fatalf("first GetByID() error = %v", err)
	}
	role.Permissions = []Permission{{Code: "document:create"}}
	after, err := svc.GetByID(context.Background(), 7)
	if err != nil {
		t.Fatalf("second GetByID() error = %v", err)
	}
	if reflect.DeepEqual(before.PermissionCodes, after.PermissionCodes) || !reflect.DeepEqual(after.PermissionCodes, []string{"document:create"}) {
		t.Fatalf("permission refresh before=%#v after=%#v", before.PermissionCodes, after.PermissionCodes)
	}
}

func TestUserServiceMapsPermissionProjectionQueryFailures(t *testing.T) {
	queryErr := errors.New("permission relation query failed")
	tests := []struct {
		name string
		run  func(*UserService) error
		repo *userRepoForServiceTest
	}{
		{
			name: "login",
			repo: &userRepoForServiceTest{getByUsernameErr: queryErr},
			run: func(svc *UserService) error {
				_, err := svc.Login(context.Background(), LoginRequest{Username: "alice", Password: "secret1"})
				return err
			},
		},
		{
			name: "current user",
			repo: &userRepoForServiceTest{getByIDErr: queryErr},
			run: func(svc *UserService) error {
				_, err := svc.GetByID(context.Background(), 7)
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run(NewUserService(tt.repo, &roleRepoForServiceTest{}, auth.NewTokenService("permission-test-secret", 1)))
			var appErr *types.AppError
			if !errors.As(err, &appErr) || appErr.Code != types.ErrCodeSystemError {
				t.Fatalf("error = %#v, want system AppError", err)
			}
		})
	}
}

func roleWithPermissions(id uint, code string, permissionCodes ...string) *Role {
	permissions := make([]Permission, len(permissionCodes))
	for i, permissionCode := range permissionCodes {
		permissions[i] = Permission{Code: permissionCode}
	}
	return &Role{
		BaseModel:   types.BaseModel{ID: id},
		Code:        code,
		Name:        code,
		Permissions: permissions,
	}
}
