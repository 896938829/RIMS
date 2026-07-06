// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package user

import "time"

// --- Auth DTOs ---

// LoginRequest holds login credentials.
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// LoginResponse is returned on successful authentication.
type LoginResponse struct {
	Token     string    `json:"token"`
	ExpiresAt int64     `json:"expiresAt"`
	User      UserBrief `json:"user"`
}

// RegisterRequest holds public self-service registration data.
type RegisterRequest struct {
	Username string `json:"username" binding:"required,min=3,max=64"`
	Password string `json:"password" binding:"required,min=6,max=72"`
	RealName string `json:"realName" binding:"max=64"`
	Phone    string `json:"phone" binding:"max=20"`
	Email    string `json:"email" binding:"omitempty,email,max=128"`
}

// UserBrief is a compact user representation for token responses.
type UserBrief struct {
	ID       uint   `json:"id"`
	Username string `json:"username"`
	RealName string `json:"realName"`
	RoleCode string `json:"roleCode"`
	RoleName string `json:"roleName"`
}

// --- User CRUD DTOs ---

// CreateUserRequest holds data for creating a new user.
type CreateUserRequest struct {
	Username string `json:"username" binding:"required,min=3,max=64"`
	Password string `json:"password" binding:"required,min=6,max=72"`
	RealName string `json:"realName" binding:"max=64"`
	Phone    string `json:"phone" binding:"max=20"`
	Email    string `json:"email" binding:"omitempty,email,max=128"`
	RoleID   uint   `json:"roleId" binding:"required"`
}

// UpdateUserRequest holds data for updating an existing user.
type UpdateUserRequest struct {
	RealName *string `json:"realName" binding:"omitempty,max=64"`
	Phone    *string `json:"phone" binding:"omitempty,max=20"`
	Email    *string `json:"email" binding:"omitempty,email,max=128"`
	RoleID   *uint   `json:"roleId"`
	Status   *int8   `json:"status" binding:"omitempty,oneof=0 1"`
}

// ChangePasswordRequest holds data for changing a user's password.
type ChangePasswordRequest struct {
	OldPassword string `json:"oldPassword" binding:"required"`
	NewPassword string `json:"newPassword" binding:"required,min=6,max=72"`
}

// ResetPasswordRequest holds data for admin resetting a user's password.
type ResetPasswordRequest struct {
	NewPassword string `json:"newPassword" binding:"required,min=6,max=72"`
}

// UserResponse is the full user representation in API responses.
type UserResponse struct {
	ID        uint      `json:"id"`
	Username  string    `json:"username"`
	RealName  string    `json:"realName"`
	Phone     string    `json:"phone"`
	Email     string    `json:"email"`
	RoleID    uint      `json:"roleId"`
	RoleCode  string    `json:"roleCode,omitempty"`
	RoleName  string    `json:"roleName,omitempty"`
	Status    int8      `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// ToResponse converts a User model to a UserResponse DTO.
func ToResponse(u *User) UserResponse {
	resp := UserResponse{
		ID:        u.ID,
		Username:  u.Username,
		RealName:  u.RealName,
		Phone:     u.Phone,
		Email:     u.Email,
		RoleID:    u.RoleID,
		Status:    u.Status,
		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
	}
	if u.Role != nil {
		resp.RoleCode = u.Role.Code
		resp.RoleName = u.Role.Name
	}
	return resp
}

// --- Role DTOs ---

// CreateRoleRequest holds data for creating a role.
type CreateRoleRequest struct {
	Code        string `json:"code" binding:"required,min=2,max=32"`
	Name        string `json:"name" binding:"required,max=64"`
	Description string `json:"description" binding:"max=255"`
}

// UpdateRoleRequest holds data for updating a role.
type UpdateRoleRequest struct {
	Name        *string `json:"name" binding:"omitempty,max=64"`
	Description *string `json:"description" binding:"omitempty,max=255"`
}

// AssignPermissionsRequest holds a list of permission IDs to assign to a role.
type AssignPermissionsRequest struct {
	PermissionIDs []uint `json:"permissionIds" binding:"required,min=1"`
}

// RoleResponse is the role representation in API responses.
type RoleResponse struct {
	ID          uint                 `json:"id"`
	Code        string               `json:"code"`
	Name        string               `json:"name"`
	Description string               `json:"description"`
	Permissions []PermissionResponse `json:"permissions,omitempty"`
	CreatedAt   time.Time            `json:"createdAt"`
}

// PermissionResponse is the permission representation in API responses.
type PermissionResponse struct {
	ID       uint   `json:"id"`
	Code     string `json:"code"`
	Name     string `json:"name"`
	Resource string `json:"resource"`
	Action   string `json:"action"`
}

// ToRoleResponse converts a Role model to a RoleResponse DTO.
func ToRoleResponse(r *Role) RoleResponse {
	resp := RoleResponse{
		ID:          r.ID,
		Code:        r.Code,
		Name:        r.Name,
		Description: r.Description,
		CreatedAt:   r.CreatedAt,
	}
	for _, p := range r.Permissions {
		resp.Permissions = append(resp.Permissions, PermissionResponse{
			ID:       p.ID,
			Code:     p.Code,
			Name:     p.Name,
			Resource: p.Resource,
			Action:   p.Action,
		})
	}
	return resp
}
