// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package user

import "rims-go/internal/types"

// User represents a system user account.
type User struct {
	types.BaseModel
	Username     string `gorm:"uniqueIndex:idx_users_username_active,where:deleted_at IS NULL;size:64;not null"`
	PasswordHash string `gorm:"size:255;not null" json:"-"`
	RealName     string `gorm:"size:64"`
	Phone        string `gorm:"size:20"`
	Email        string `gorm:"size:128"`
	RoleID       uint   `gorm:"not null;index"`
	Status       int8   `gorm:"default:1;not null"` // 1=active, 0=disabled
	Role         *Role  `gorm:"foreignKey:RoleID" json:"role,omitempty"`
}

// TableName overrides the default table name.
func (User) TableName() string { return "users" }

// Role defines a set of permissions assigned to users.
type Role struct {
	types.BaseModel
	Code        string       `gorm:"uniqueIndex:idx_roles_code_active,where:deleted_at IS NULL;size:32;not null"`
	Name        string       `gorm:"size:64;not null"`
	Description string       `gorm:"size:255"`
	Permissions []Permission `gorm:"many2many:role_permissions" json:"permissions,omitempty"`
}

// TableName overrides the default table name.
func (Role) TableName() string { return "roles" }

// Permission represents a single action on a resource.
type Permission struct {
	types.BaseModel
	Code     string `gorm:"uniqueIndex:idx_permissions_code_active,where:deleted_at IS NULL;size:64;not null"` // e.g. "product:create"
	Name     string `gorm:"size:64;not null"`
	Resource string `gorm:"size:32;not null;index"` // e.g. "product"
	Action   string `gorm:"size:32;not null"`       // e.g. "create"
}

// TableName overrides the default table name.
func (Permission) TableName() string { return "permissions" }
