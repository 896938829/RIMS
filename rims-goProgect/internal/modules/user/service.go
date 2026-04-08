// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package user

import (
	"context"
	"errors"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"rims-go/internal/auth"
	"rims-go/internal/types"
)

// UserService handles user-related business logic.
type UserService struct {
	userRepo UserRepository
	roleRepo RoleRepository
	tokenSvc *auth.TokenService
}

// NewUserService creates a new UserService.
func NewUserService(userRepo UserRepository, roleRepo RoleRepository, tokenSvc *auth.TokenService) *UserService {
	return &UserService{
		userRepo: userRepo,
		roleRepo: roleRepo,
		tokenSvc: tokenSvc,
	}
}

// Login authenticates a user and returns a JWT token.
func (s *UserService) Login(ctx context.Context, req LoginRequest) (*LoginResponse, error) {
	u, err := s.userRepo.GetByUsername(ctx, req.Username)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, types.ErrAuth("用户名或密码错误")
		}
		return nil, types.ErrSystem(err)
	}

	if u.Status != 1 {
		return nil, types.ErrAuth("账号已禁用")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.Password)); err != nil {
		return nil, types.ErrAuth("用户名或密码错误")
	}

	roleCode := ""
	roleName := ""
	if u.Role != nil {
		roleCode = u.Role.Code
		roleName = u.Role.Name
	}

	token, expiresAt, err := s.tokenSvc.GenerateToken(u.ID, u.Username, u.RoleID, roleCode)
	if err != nil {
		return nil, types.ErrSystem(err)
	}

	return &LoginResponse{
		Token:     token,
		ExpiresAt: expiresAt,
		User: UserBrief{
			ID:       u.ID,
			Username: u.Username,
			RealName: u.RealName,
			RoleCode: roleCode,
			RoleName: roleName,
		},
	}, nil
}

// Create creates a new user account.
func (s *UserService) Create(ctx context.Context, req CreateUserRequest) (*UserResponse, error) {
	// Check username uniqueness
	existing, err := s.userRepo.GetByUsername(ctx, strings.TrimSpace(req.Username))
	if err == nil && existing != nil {
		return nil, types.ErrDuplicate("用户名已存在")
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, types.ErrSystem(err)
	}

	// Validate role exists
	role, err := s.roleRepo.GetByID(ctx, req.RoleID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, types.ErrValidation("角色不存在")
		}
		return nil, types.ErrSystem(err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, types.ErrSystem(err)
	}

	u := &User{
		Username:     strings.TrimSpace(req.Username),
		PasswordHash: string(hash),
		RealName:     strings.TrimSpace(req.RealName),
		Phone:        strings.TrimSpace(req.Phone),
		Email:        strings.TrimSpace(req.Email),
		RoleID:       req.RoleID,
		Status:       1,
	}

	if err := s.userRepo.Create(ctx, u); err != nil {
		return nil, types.ErrSystem(err)
	}

	u.Role = role
	resp := ToResponse(u)
	return &resp, nil
}

// GetByID retrieves a user by ID.
func (s *UserService) GetByID(ctx context.Context, id uint) (*UserResponse, error) {
	u, err := s.userRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, types.ErrNotFound("用户")
		}
		return nil, types.ErrSystem(err)
	}
	resp := ToResponse(u)
	return &resp, nil
}

// List returns a paginated list of users.
func (s *UserService) List(ctx context.Context, page types.PageRequest) (types.PageResult, error) {
	users, total, err := s.userRepo.List(ctx, page)
	if err != nil {
		return types.PageResult{}, types.ErrSystem(err)
	}

	items := make([]UserResponse, len(users))
	for i := range users {
		items[i] = ToResponse(&users[i])
	}

	return types.NewPageResult(page, items, total), nil
}

// Update modifies an existing user's profile fields.
func (s *UserService) Update(ctx context.Context, id uint, req UpdateUserRequest) (*UserResponse, error) {
	u, err := s.userRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, types.ErrNotFound("用户")
		}
		return nil, types.ErrSystem(err)
	}

	if req.RealName != nil {
		u.RealName = strings.TrimSpace(*req.RealName)
	}
	if req.Phone != nil {
		u.Phone = strings.TrimSpace(*req.Phone)
	}
	if req.Email != nil {
		u.Email = strings.TrimSpace(*req.Email)
	}
	if req.RoleID != nil {
		if _, err := s.roleRepo.GetByID(ctx, *req.RoleID); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, types.ErrValidation("角色不存在")
			}
			return nil, types.ErrSystem(err)
		}
		u.RoleID = *req.RoleID
	}
	if req.Status != nil {
		u.Status = *req.Status
	}

	if err := s.userRepo.Update(ctx, u); err != nil {
		return nil, types.ErrSystem(err)
	}

	// Re-fetch to include updated role
	u, _ = s.userRepo.GetByID(ctx, id)
	resp := ToResponse(u)
	return &resp, nil
}

// Delete soft-deletes a user by ID.
func (s *UserService) Delete(ctx context.Context, id uint) error {
	if _, err := s.userRepo.GetByID(ctx, id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return types.ErrNotFound("用户")
		}
		return types.ErrSystem(err)
	}
	if err := s.userRepo.Delete(ctx, id); err != nil {
		return types.ErrSystem(err)
	}
	return nil
}

// ChangePassword allows a user to change their own password.
func (s *UserService) ChangePassword(ctx context.Context, userID uint, req ChangePasswordRequest) error {
	u, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return types.ErrNotFound("用户")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.OldPassword)); err != nil {
		return types.ErrAuth("原密码错误")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return types.ErrSystem(err)
	}

	u.PasswordHash = string(hash)
	if err := s.userRepo.Update(ctx, u); err != nil {
		return types.ErrSystem(err)
	}
	return nil
}

// ResetPassword allows an admin to reset another user's password.
func (s *UserService) ResetPassword(ctx context.Context, userID uint, req ResetPasswordRequest) error {
	u, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return types.ErrNotFound("用户")
		}
		return types.ErrSystem(err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return types.ErrSystem(err)
	}

	u.PasswordHash = string(hash)
	if err := s.userRepo.Update(ctx, u); err != nil {
		return types.ErrSystem(err)
	}
	return nil
}

// RoleService handles role and permission business logic.
type RoleService struct {
	roleRepo RoleRepository
}

// NewRoleService creates a new RoleService.
func NewRoleService(roleRepo RoleRepository) *RoleService {
	return &RoleService{roleRepo: roleRepo}
}

// Create creates a new role.
func (s *RoleService) Create(ctx context.Context, req CreateRoleRequest) (*RoleResponse, error) {
	existing, err := s.roleRepo.GetByCode(ctx, req.Code)
	if err == nil && existing != nil {
		return nil, types.ErrDuplicate("角色编码已存在")
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, types.ErrSystem(err)
	}

	role := &Role{
		Code:        req.Code,
		Name:        req.Name,
		Description: req.Description,
	}
	if err := s.roleRepo.Create(ctx, role); err != nil {
		return nil, types.ErrSystem(err)
	}

	resp := ToRoleResponse(role)
	return &resp, nil
}

// GetByID retrieves a role by ID.
func (s *RoleService) GetByID(ctx context.Context, id uint) (*RoleResponse, error) {
	role, err := s.roleRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, types.ErrNotFound("角色")
		}
		return nil, types.ErrSystem(err)
	}
	resp := ToRoleResponse(role)
	return &resp, nil
}

// List returns all roles.
func (s *RoleService) List(ctx context.Context) ([]RoleResponse, error) {
	roles, err := s.roleRepo.List(ctx)
	if err != nil {
		return nil, types.ErrSystem(err)
	}
	result := make([]RoleResponse, len(roles))
	for i := range roles {
		result[i] = ToRoleResponse(&roles[i])
	}
	return result, nil
}

// Update modifies an existing role.
func (s *RoleService) Update(ctx context.Context, id uint, req UpdateRoleRequest) (*RoleResponse, error) {
	role, err := s.roleRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, types.ErrNotFound("角色")
		}
		return nil, types.ErrSystem(err)
	}

	if req.Name != nil {
		role.Name = *req.Name
	}
	if req.Description != nil {
		role.Description = *req.Description
	}

	if err := s.roleRepo.Update(ctx, role); err != nil {
		return nil, types.ErrSystem(err)
	}

	resp := ToRoleResponse(role)
	return &resp, nil
}

// Delete removes a role by ID.
func (s *RoleService) Delete(ctx context.Context, id uint) error {
	if _, err := s.roleRepo.GetByID(ctx, id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return types.ErrNotFound("角色")
		}
		return types.ErrSystem(err)
	}
	return s.roleRepo.Delete(ctx, id)
}

// AssignPermissions replaces a role's permission set.
func (s *RoleService) AssignPermissions(ctx context.Context, roleID uint, req AssignPermissionsRequest) error {
	if _, err := s.roleRepo.GetByID(ctx, roleID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return types.ErrNotFound("角色")
		}
		return types.ErrSystem(err)
	}
	return s.roleRepo.AssignPermissions(ctx, roleID, req.PermissionIDs)
}

// ListPermissions returns all available permissions.
func (s *RoleService) ListPermissions(ctx context.Context) ([]PermissionResponse, error) {
	perms, err := s.roleRepo.ListPermissions(ctx)
	if err != nil {
		return nil, types.ErrSystem(err)
	}
	result := make([]PermissionResponse, len(perms))
	for i, p := range perms {
		result[i] = PermissionResponse{
			ID:       p.ID,
			Code:     p.Code,
			Name:     p.Name,
			Resource: p.Resource,
			Action:   p.Action,
		}
	}
	return result, nil
}
