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
	createErr error
}

func (r userRepoForServiceTest) Create(ctx context.Context, user *User) error {
	return r.createErr
}

func (r userRepoForServiceTest) GetByID(ctx context.Context, id uint) (*User, error) {
	return nil, gorm.ErrRecordNotFound
}

func (r userRepoForServiceTest) GetByUsername(ctx context.Context, username string) (*User, error) {
	return nil, gorm.ErrRecordNotFound
}

func (r userRepoForServiceTest) GetAuthUser(ctx context.Context, userID uint) (uint, string, uint, string, int8, error) {
	return 0, "", 0, "", 0, gorm.ErrRecordNotFound
}

func (r userRepoForServiceTest) List(ctx context.Context, page types.PageRequest) ([]User, int64, error) {
	return nil, 0, nil
}

func (r userRepoForServiceTest) Update(ctx context.Context, user *User) error {
	return nil
}

func (r userRepoForServiceTest) Delete(ctx context.Context, id uint) error {
	return nil
}

type roleRepoForServiceTest struct{}

func (roleRepoForServiceTest) Create(ctx context.Context, role *Role) error {
	return nil
}

func (roleRepoForServiceTest) GetByID(ctx context.Context, id uint) (*Role, error) {
	return &Role{
		BaseModel: types.BaseModel{ID: id},
		Code:      "operator",
		Name:      "Operator",
	}, nil
}

func (roleRepoForServiceTest) GetByCode(ctx context.Context, code string) (*Role, error) {
	return nil, gorm.ErrRecordNotFound
}

func (roleRepoForServiceTest) List(ctx context.Context) ([]Role, error) {
	return nil, nil
}

func (roleRepoForServiceTest) Update(ctx context.Context, role *Role) error {
	return nil
}

func (roleRepoForServiceTest) Delete(ctx context.Context, id uint) error {
	return nil
}

func (roleRepoForServiceTest) AssignPermissions(ctx context.Context, roleID uint, permIDs []uint) error {
	return nil
}

func (roleRepoForServiceTest) ListPermissions(ctx context.Context) ([]Permission, error) {
	return nil, nil
}

func (roleRepoForServiceTest) HasPermission(ctx context.Context, roleID uint, code string) (bool, error) {
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
			svc := NewUserService(userRepoForServiceTest{createErr: tt.err}, roleRepoForServiceTest{}, nil)

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
		userRepoForServiceTest{createErr: errors.New("CHECK constraint failed: users_status_check")},
		roleRepoForServiceTest{},
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
