// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package user

import (
	"context"
	"errors"
	"testing"

	"gorm.io/gorm"

	"rims-go/internal/types"
)

type userRepoForServiceTest struct {
	createErr            error
	usersByID            map[uint]*User
	countActiveAdmins    int64
	countActiveAdminsErr error
	lockActiveAdminErr   error
	lockActiveAdminCalls int
	updateCalled         bool
	deleteCalled         bool
}

func (r *userRepoForServiceTest) Create(ctx context.Context, user *User) error {
	return r.createErr
}

func (r *userRepoForServiceTest) GetByID(ctx context.Context, id uint) (*User, error) {
	if r.usersByID != nil {
		if u, ok := r.usersByID[id]; ok {
			return u, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (r *userRepoForServiceTest) GetByUsername(ctx context.Context, username string) (*User, error) {
	return nil, gorm.ErrRecordNotFound
}

func (r *userRepoForServiceTest) GetAuthUser(ctx context.Context, userID uint) (uint, string, uint, string, int8, error) {
	return 0, "", 0, "", 0, gorm.ErrRecordNotFound
}

func (r *userRepoForServiceTest) List(ctx context.Context, page types.PageRequest) ([]User, int64, error) {
	return nil, 0, nil
}

func (r *userRepoForServiceTest) Update(ctx context.Context, user *User) error {
	r.updateCalled = true
	return nil
}

func (r *userRepoForServiceTest) Delete(ctx context.Context, id uint) error {
	r.deleteCalled = true
	return nil
}

func (r *userRepoForServiceTest) LockActiveAdminGuard(ctx context.Context) error {
	r.lockActiveAdminCalls++
	return r.lockActiveAdminErr
}

func (r *userRepoForServiceTest) CountActiveAdmins(ctx context.Context) (int64, error) {
	return r.countActiveAdmins, r.countActiveAdminsErr
}

type roleRepoForServiceTest struct {
	rolesByID map[uint]*Role
}

func (*roleRepoForServiceTest) Create(ctx context.Context, role *Role) error {
	return nil
}

func (r *roleRepoForServiceTest) GetByID(ctx context.Context, id uint) (*Role, error) {
	if r.rolesByID != nil {
		if role, ok := r.rolesByID[id]; ok {
			return role, nil
		}
	}
	return &Role{
		BaseModel: types.BaseModel{ID: id},
		Code:      "operator",
		Name:      "Operator",
	}, nil
}

func (*roleRepoForServiceTest) GetByCode(ctx context.Context, code string) (*Role, error) {
	return nil, gorm.ErrRecordNotFound
}

func (*roleRepoForServiceTest) List(ctx context.Context) ([]Role, error) {
	return nil, nil
}

func (*roleRepoForServiceTest) Update(ctx context.Context, role *Role) error {
	return nil
}

func (*roleRepoForServiceTest) Delete(ctx context.Context, id uint) error {
	return nil
}

func (*roleRepoForServiceTest) AssignPermissions(ctx context.Context, roleID uint, permIDs []uint) error {
	return nil
}

func (*roleRepoForServiceTest) ListPermissions(ctx context.Context) ([]Permission, error) {
	return nil, nil
}

func (*roleRepoForServiceTest) HasPermission(ctx context.Context, roleID uint, code string) (bool, error) {
	return false, nil
}

func TestUserServiceCreateMapsUniqueConstraintToDuplicate(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "postgres 23505", err: errors.New(`ERROR: duplicate key value violates unique constraint "idx_users_username" (SQLSTATE 23505)`)},
		{name: "sqlite unique", err: errors.New("UNIQUE constraint failed: users.username")},
		{name: "mysql duplicate", err: errors.New("Error 1062 (23000): Duplicate entry 'alice' for key 'users.username'")},
		{name: "gorm duplicate", err: gorm.ErrDuplicatedKey},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewUserService(&userRepoForServiceTest{createErr: tt.err}, &roleRepoForServiceTest{}, nil)

			_, err := svc.Create(context.Background(), CreateUserRequest{
				Username: "alice",
				Password: "secret1",
				RoleID:   2,
			})

			var appErr *types.AppError
			if !errors.As(err, &appErr) {
				t.Fatalf("err = %v, want AppError", err)
			}
			if appErr.Code != types.ErrCodeDuplicate || appErr.Message != "用户名已存在" {
				t.Fatalf("app error code=%d message=%q, want duplicate username", appErr.Code, appErr.Message)
			}
		})
	}
}

func TestUserServiceCreateDoesNotMapGenericConstraintFailureToDuplicate(t *testing.T) {
	svc := NewUserService(
		&userRepoForServiceTest{createErr: errors.New("CHECK constraint failed: users_status_check")},
		&roleRepoForServiceTest{},
		nil,
	)

	_, err := svc.Create(context.Background(), CreateUserRequest{
		Username: "alice",
		Password: "secret1",
		RoleID:   2,
	})

	var appErr *types.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("err = %v, want AppError", err)
	}
	if appErr.Code != types.ErrCodeSystemError {
		t.Fatalf("app error code=%d, want system error for non-unique constraint", appErr.Code)
	}
}

func TestUserServiceUpdateRejectsNonAdminRoleChange(t *testing.T) {
	target := &User{
		BaseModel: types.BaseModel{ID: 10},
		Username:  "clerk",
		RoleID:    2,
		Status:    1,
		Role:      &Role{BaseModel: types.BaseModel{ID: 2}, Code: "operator"},
	}
	repo := &userRepoForServiceTest{usersByID: map[uint]*User{10: target}}
	svc := NewUserService(repo, &roleRepoForServiceTest{}, nil)
	newRoleID := uint(3)

	_, err := svc.Update(actorContext(99, "operator"), 10, UpdateUserRequest{RoleID: &newRoleID})

	assertAppErrorCode(t, err, types.ErrCodePermissionDenied)
	if repo.updateCalled {
		t.Fatal("Update was called, want role change rejected before persistence")
	}
}

func TestUserServiceUpdateRejectsSelfDisable(t *testing.T) {
	target := &User{
		BaseModel: types.BaseModel{ID: 10},
		Username:  "admin",
		RoleID:    1,
		Status:    1,
		Role:      &Role{BaseModel: types.BaseModel{ID: 1}, Code: "admin"},
	}
	repo := &userRepoForServiceTest{usersByID: map[uint]*User{10: target}, countActiveAdmins: 2}
	svc := NewUserService(repo, &roleRepoForServiceTest{}, nil)
	disabled := int8(0)

	_, err := svc.Update(actorContext(10, "admin"), 10, UpdateUserRequest{Status: &disabled})

	assertAppErrorCode(t, err, types.ErrCodeInvalidState)
	if repo.updateCalled {
		t.Fatal("Update was called, want self-disable rejected before persistence")
	}
}

func TestUserServiceUpdateRejectsDisablingLastActiveAdmin(t *testing.T) {
	target := &User{
		BaseModel: types.BaseModel{ID: 10},
		Username:  "admin",
		RoleID:    1,
		Status:    1,
		Role:      &Role{BaseModel: types.BaseModel{ID: 1}, Code: "admin"},
	}
	repo := &userRepoForServiceTest{usersByID: map[uint]*User{10: target}, countActiveAdmins: 1}
	svc := NewUserService(repo, &roleRepoForServiceTest{}, nil)
	disabled := int8(0)

	_, err := svc.Update(actorContext(99, "admin"), 10, UpdateUserRequest{Status: &disabled})

	assertAppErrorCode(t, err, types.ErrCodeInvalidState)
	if repo.lockActiveAdminCalls != 1 {
		t.Fatalf("LockActiveAdminGuard calls = %d, want 1", repo.lockActiveAdminCalls)
	}
	if repo.updateCalled {
		t.Fatal("Update was called, want last active admin disable rejected before persistence")
	}
}

func TestUserServiceUpdateRejectsRemovingLastActiveAdminRole(t *testing.T) {
	adminRole := &Role{BaseModel: types.BaseModel{ID: 1}, Code: "admin"}
	operatorRole := &Role{BaseModel: types.BaseModel{ID: 2}, Code: "operator"}
	target := &User{
		BaseModel: types.BaseModel{ID: 10},
		Username:  "admin",
		RoleID:    1,
		Status:    1,
		Role:      adminRole,
	}
	repo := &userRepoForServiceTest{usersByID: map[uint]*User{10: target}, countActiveAdmins: 1}
	roles := &roleRepoForServiceTest{rolesByID: map[uint]*Role{1: adminRole, 2: operatorRole}}
	svc := NewUserService(repo, roles, nil)
	newRoleID := uint(2)

	_, err := svc.Update(actorContext(99, "admin"), 10, UpdateUserRequest{RoleID: &newRoleID})

	assertAppErrorCode(t, err, types.ErrCodeInvalidState)
	if repo.lockActiveAdminCalls != 1 {
		t.Fatalf("LockActiveAdminGuard calls = %d, want 1", repo.lockActiveAdminCalls)
	}
	if repo.updateCalled {
		t.Fatal("Update was called, want last active admin role removal rejected before persistence")
	}
}

func TestUserServiceDeleteRejectsDeletingLastActiveAdmin(t *testing.T) {
	target := &User{
		BaseModel: types.BaseModel{ID: 10},
		Username:  "admin",
		RoleID:    1,
		Status:    1,
		Role:      &Role{BaseModel: types.BaseModel{ID: 1}, Code: "admin"},
	}
	repo := &userRepoForServiceTest{usersByID: map[uint]*User{10: target}, countActiveAdmins: 1}
	svc := NewUserService(repo, &roleRepoForServiceTest{}, nil)

	err := svc.Delete(context.Background(), 10)

	assertAppErrorCode(t, err, types.ErrCodeInvalidState)
	if repo.lockActiveAdminCalls != 1 {
		t.Fatalf("LockActiveAdminGuard calls = %d, want 1", repo.lockActiveAdminCalls)
	}
	if repo.deleteCalled {
		t.Fatal("Delete was called, want last active admin delete rejected before persistence")
	}
}

func actorContext(userID uint, roleCode string) context.Context {
	ctx := context.WithValue(context.Background(), types.CtxKeyUserID, userID)
	return context.WithValue(ctx, types.CtxKeyRoleCode, roleCode)
}

func assertAppErrorCode(t *testing.T, err error, want int) {
	t.Helper()
	var appErr *types.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("err = %v, want AppError code %d", err, want)
	}
	if appErr.Code != want {
		t.Fatalf("app error code=%d message=%q, want %d", appErr.Code, appErr.Message, want)
	}
}
