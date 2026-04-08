// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package user

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"rims-go/internal/types"
)

// Handler handles HTTP requests for user and auth endpoints.
type Handler struct {
	userSvc *UserService
	roleSvc *RoleService
}

// NewHandler creates a new user Handler.
func NewHandler(userSvc *UserService, roleSvc *RoleService) *Handler {
	return &Handler{userSvc: userSvc, roleSvc: roleSvc}
}

// --- Auth ---

// Login godoc
// @Summary 用户登录
// @Tags 认证
// @Accept json
// @Produce json
// @Param payload body LoginRequest true "登录凭证"
// @Success 200 {object} types.Response{data=LoginResponse}
// @Failure 401 {object} types.Response
// @Router /api/v1/auth/login [post]
func (h *Handler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		types.Fail(c, http.StatusBadRequest, types.ErrValidation(err.Error()))
		return
	}
	resp, err := h.userSvc.Login(c.Request.Context(), req)
	if err != nil {
		types.FailFromError(c, err)
		return
	}
	types.OK(c, resp)
}

// --- Users ---

// CreateUser godoc
// @Summary 创建用户
// @Tags 用户
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param payload body CreateUserRequest true "用户信息"
// @Success 201 {object} types.Response{data=UserResponse}
// @Failure 400 {object} types.Response
// @Router /api/v1/users [post]
func (h *Handler) CreateUser(c *gin.Context) {
	var req CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		types.Fail(c, http.StatusBadRequest, types.ErrValidation(err.Error()))
		return
	}
	resp, err := h.userSvc.Create(c.Request.Context(), req)
	if err != nil {
		types.FailFromError(c, err)
		return
	}
	types.OKCreated(c, resp)
}

// ListUsers godoc
// @Summary 用户列表
// @Tags 用户
// @Security BearerAuth
// @Produce json
// @Param page query int false "页码" default(1)
// @Param pageSize query int false "每页数量" default(20)
// @Param keyword query string false "搜索关键字"
// @Success 200 {object} types.Response{data=types.PageResult}
// @Router /api/v1/users [get]
func (h *Handler) ListUsers(c *gin.Context) {
	var page types.PageRequest
	if err := c.ShouldBindQuery(&page); err != nil {
		types.Fail(c, http.StatusBadRequest, types.ErrValidation(err.Error()))
		return
	}
	result, err := h.userSvc.List(c.Request.Context(), page)
	if err != nil {
		types.FailFromError(c, err)
		return
	}
	types.OKWithPage(c, result)
}

// GetUser godoc
// @Summary 获取用户详情
// @Tags 用户
// @Security BearerAuth
// @Produce json
// @Param id path int true "用户ID"
// @Success 200 {object} types.Response{data=UserResponse}
// @Failure 404 {object} types.Response
// @Router /api/v1/users/{id} [get]
func (h *Handler) GetUser(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		return
	}
	resp, err := h.userSvc.GetByID(c.Request.Context(), id)
	if err != nil {
		types.FailFromError(c, err)
		return
	}
	types.OK(c, resp)
}

// UpdateUser godoc
// @Summary 更新用户
// @Tags 用户
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path int true "用户ID"
// @Param payload body UpdateUserRequest true "更新内容"
// @Success 200 {object} types.Response{data=UserResponse}
// @Router /api/v1/users/{id} [put]
func (h *Handler) UpdateUser(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		return
	}
	var req UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		types.Fail(c, http.StatusBadRequest, types.ErrValidation(err.Error()))
		return
	}
	resp, err := h.userSvc.Update(c.Request.Context(), id, req)
	if err != nil {
		types.FailFromError(c, err)
		return
	}
	types.OK(c, resp)
}

// DeleteUser godoc
// @Summary 删除用户
// @Tags 用户
// @Security BearerAuth
// @Param id path int true "用户ID"
// @Success 204
// @Router /api/v1/users/{id} [delete]
func (h *Handler) DeleteUser(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		return
	}
	if err := h.userSvc.Delete(c.Request.Context(), id); err != nil {
		types.FailFromError(c, err)
		return
	}
	types.OKNoContent(c)
}

// ChangePassword godoc
// @Summary 修改密码
// @Tags 用户
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param payload body ChangePasswordRequest true "密码"
// @Success 200 {object} types.Response
// @Router /api/v1/users/me/password [put]
func (h *Handler) ChangePassword(c *gin.Context) {
	var req ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		types.Fail(c, http.StatusBadRequest, types.ErrValidation(err.Error()))
		return
	}
	userID := types.GetUserID(c)
	if err := h.userSvc.ChangePassword(c.Request.Context(), userID, req); err != nil {
		types.FailFromError(c, err)
		return
	}
	types.OK(c, nil)
}

// ResetPassword godoc
// @Summary 重置用户密码（管理员）
// @Tags 用户
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path int true "用户ID"
// @Param payload body ResetPasswordRequest true "新密码"
// @Success 200 {object} types.Response
// @Router /api/v1/users/{id}/password [put]
func (h *Handler) ResetPassword(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		return
	}
	var req ResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		types.Fail(c, http.StatusBadRequest, types.ErrValidation(err.Error()))
		return
	}
	if err := h.userSvc.ResetPassword(c.Request.Context(), id, req); err != nil {
		types.FailFromError(c, err)
		return
	}
	types.OK(c, nil)
}

// GetCurrentUser godoc
// @Summary 获取当前用户信息
// @Tags 用户
// @Security BearerAuth
// @Produce json
// @Success 200 {object} types.Response{data=UserResponse}
// @Router /api/v1/users/me [get]
func (h *Handler) GetCurrentUser(c *gin.Context) {
	userID := types.GetUserID(c)
	resp, err := h.userSvc.GetByID(c.Request.Context(), userID)
	if err != nil {
		types.FailFromError(c, err)
		return
	}
	types.OK(c, resp)
}

// --- Roles ---

// CreateRole godoc
// @Summary 创建角色
// @Tags 角色
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param payload body CreateRoleRequest true "角色信息"
// @Success 201 {object} types.Response{data=RoleResponse}
// @Router /api/v1/roles [post]
func (h *Handler) CreateRole(c *gin.Context) {
	var req CreateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		types.Fail(c, http.StatusBadRequest, types.ErrValidation(err.Error()))
		return
	}
	resp, err := h.roleSvc.Create(c.Request.Context(), req)
	if err != nil {
		types.FailFromError(c, err)
		return
	}
	types.OKCreated(c, resp)
}

// ListRoles godoc
// @Summary 角色列表
// @Tags 角色
// @Security BearerAuth
// @Produce json
// @Success 200 {object} types.Response{data=[]RoleResponse}
// @Router /api/v1/roles [get]
func (h *Handler) ListRoles(c *gin.Context) {
	roles, err := h.roleSvc.List(c.Request.Context())
	if err != nil {
		types.FailFromError(c, err)
		return
	}
	types.OK(c, roles)
}

// GetRole godoc
// @Summary 获取角色详情
// @Tags 角色
// @Security BearerAuth
// @Produce json
// @Param id path int true "角色ID"
// @Success 200 {object} types.Response{data=RoleResponse}
// @Router /api/v1/roles/{id} [get]
func (h *Handler) GetRole(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		return
	}
	resp, err := h.roleSvc.GetByID(c.Request.Context(), id)
	if err != nil {
		types.FailFromError(c, err)
		return
	}
	types.OK(c, resp)
}

// UpdateRole godoc
// @Summary 更新角色
// @Tags 角色
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path int true "角色ID"
// @Param payload body UpdateRoleRequest true "更新内容"
// @Success 200 {object} types.Response{data=RoleResponse}
// @Router /api/v1/roles/{id} [put]
func (h *Handler) UpdateRole(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		return
	}
	var req UpdateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		types.Fail(c, http.StatusBadRequest, types.ErrValidation(err.Error()))
		return
	}
	resp, err := h.roleSvc.Update(c.Request.Context(), id, req)
	if err != nil {
		types.FailFromError(c, err)
		return
	}
	types.OK(c, resp)
}

// DeleteRole godoc
// @Summary 删除角色
// @Tags 角色
// @Security BearerAuth
// @Param id path int true "角色ID"
// @Success 204
// @Router /api/v1/roles/{id} [delete]
func (h *Handler) DeleteRole(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		return
	}
	if err := h.roleSvc.Delete(c.Request.Context(), id); err != nil {
		types.FailFromError(c, err)
		return
	}
	types.OKNoContent(c)
}

// AssignPermissions godoc
// @Summary 分配角色权限
// @Tags 角色
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path int true "角色ID"
// @Param payload body AssignPermissionsRequest true "权限ID列表"
// @Success 200 {object} types.Response
// @Router /api/v1/roles/{id}/permissions [put]
func (h *Handler) AssignPermissions(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		return
	}
	var req AssignPermissionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		types.Fail(c, http.StatusBadRequest, types.ErrValidation(err.Error()))
		return
	}
	if err := h.roleSvc.AssignPermissions(c.Request.Context(), id, req); err != nil {
		types.FailFromError(c, err)
		return
	}
	types.OK(c, nil)
}

// ListPermissions godoc
// @Summary 权限列表
// @Tags 角色
// @Security BearerAuth
// @Produce json
// @Success 200 {object} types.Response{data=[]PermissionResponse}
// @Router /api/v1/permissions [get]
func (h *Handler) ListPermissions(c *gin.Context) {
	perms, err := h.roleSvc.ListPermissions(c.Request.Context())
	if err != nil {
		types.FailFromError(c, err)
		return
	}
	types.OK(c, perms)
}

// parseID extracts and validates an unsigned integer path parameter.
func parseID(c *gin.Context, param string) (uint, error) {
	idStr := c.Param(param)
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil || id == 0 {
		types.Fail(c, http.StatusBadRequest, types.ErrValidation("无效的ID"))
		return 0, err
	}
	return uint(id), nil
}
