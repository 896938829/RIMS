// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package user

import (
	"context"

	"gorm.io/gorm"

	"rims-go/internal/db"
	"rims-go/internal/types"
)

// UserRepository defines data access operations for users.
type UserRepository interface {
	Create(ctx context.Context, user *User) error
	GetByID(ctx context.Context, id uint) (*User, error)
	GetByUsername(ctx context.Context, username string) (*User, error)
	List(ctx context.Context, page types.PageRequest) ([]User, int64, error)
	Update(ctx context.Context, user *User) error
	Delete(ctx context.Context, id uint) error
}

type userRepo struct {
	gormDB *gorm.DB
}

// NewUserRepository creates a new UserRepository backed by GORM.
func NewUserRepository(gormDB *gorm.DB) UserRepository {
	return &userRepo{gormDB: gormDB}
}

func (r *userRepo) getDB(ctx context.Context) *gorm.DB {
	return db.FromCtx(ctx, r.gormDB)
}

func (r *userRepo) Create(ctx context.Context, u *User) error {
	return r.getDB(ctx).Create(u).Error
}

func (r *userRepo) GetByID(ctx context.Context, id uint) (*User, error) {
	var u User
	err := r.getDB(ctx).Preload("Role.Permissions").First(&u, id).Error
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *userRepo) GetByUsername(ctx context.Context, username string) (*User, error) {
	var u User
	err := r.getDB(ctx).Preload("Role.Permissions").Where("username = ?", username).First(&u).Error
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *userRepo) List(ctx context.Context, page types.PageRequest) ([]User, int64, error) {
	page.Defaults()
	d := r.getDB(ctx).Model(&User{})

	if page.Keyword != "" {
		kw := "%" + page.Keyword + "%"
		d = d.Where("username LIKE ? OR real_name LIKE ? OR phone LIKE ?", kw, kw, kw)
	}

	var total int64
	if err := d.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var users []User
	if err := d.Preload("Role").
		Offset(page.Offset()).Limit(page.PageSize).
		Order("id DESC").
		Find(&users).Error; err != nil {
		return nil, 0, err
	}

	return users, total, nil
}

func (r *userRepo) Update(ctx context.Context, u *User) error {
	return r.getDB(ctx).Save(u).Error
}

func (r *userRepo) Delete(ctx context.Context, id uint) error {
	return r.getDB(ctx).Delete(&User{}, id).Error
}

// RoleRepository defines data access operations for roles.
type RoleRepository interface {
	Create(ctx context.Context, role *Role) error
	GetByID(ctx context.Context, id uint) (*Role, error)
	GetByCode(ctx context.Context, code string) (*Role, error)
	List(ctx context.Context) ([]Role, error)
	Update(ctx context.Context, role *Role) error
	Delete(ctx context.Context, id uint) error
	AssignPermissions(ctx context.Context, roleID uint, permIDs []uint) error
	ListPermissions(ctx context.Context) ([]Permission, error)
}

type roleRepo struct {
	gormDB *gorm.DB
}

// NewRoleRepository creates a new RoleRepository backed by GORM.
func NewRoleRepository(gormDB *gorm.DB) RoleRepository {
	return &roleRepo{gormDB: gormDB}
}

func (r *roleRepo) getDB(ctx context.Context) *gorm.DB {
	return db.FromCtx(ctx, r.gormDB)
}

func (r *roleRepo) Create(ctx context.Context, role *Role) error {
	return r.getDB(ctx).Create(role).Error
}

func (r *roleRepo) GetByID(ctx context.Context, id uint) (*Role, error) {
	var role Role
	err := r.getDB(ctx).Preload("Permissions").First(&role, id).Error
	if err != nil {
		return nil, err
	}
	return &role, nil
}

func (r *roleRepo) GetByCode(ctx context.Context, code string) (*Role, error) {
	var role Role
	err := r.getDB(ctx).Preload("Permissions").Where("code = ?", code).First(&role).Error
	if err != nil {
		return nil, err
	}
	return &role, nil
}

func (r *roleRepo) List(ctx context.Context) ([]Role, error) {
	var roles []Role
	err := r.getDB(ctx).Preload("Permissions").Order("id ASC").Find(&roles).Error
	return roles, err
}

func (r *roleRepo) Update(ctx context.Context, role *Role) error {
	return r.getDB(ctx).Save(role).Error
}

func (r *roleRepo) Delete(ctx context.Context, id uint) error {
	return r.getDB(ctx).Delete(&Role{}, id).Error
}

func (r *roleRepo) AssignPermissions(ctx context.Context, roleID uint, permIDs []uint) error {
	d := r.getDB(ctx)

	var role Role
	if err := d.First(&role, roleID).Error; err != nil {
		return err
	}

	var perms []Permission
	if err := d.Where("id IN ?", permIDs).Find(&perms).Error; err != nil {
		return err
	}

	return d.Model(&role).Association("Permissions").Replace(perms)
}

func (r *roleRepo) ListPermissions(ctx context.Context) ([]Permission, error) {
	var perms []Permission
	err := r.getDB(ctx).Order("resource ASC, action ASC").Find(&perms).Error
	return perms, err
}
