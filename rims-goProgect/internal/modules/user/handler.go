// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package user

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"rims-go/internal/modules/audit"
	"rims-go/internal/types"
)

// AuditLogger is the narrow audit contract consumed by the user handler.
// It is satisfied structurally by *audit.AuditService. The login flow uses
// it best-effort: an audit write failure never fails the login response
// since login is not inside a business transaction.
type AuditLogger interface {
	Log(ctx context.Context, e audit.Entry) error
}

// Handler handles HTTP requests for user and auth endpoints.
type Handler struct {
	userSvc  *UserService
	roleSvc  *RoleService
	auditSvc AuditLogger
}

// NewHandler creates a new user Handler.
func NewHandler(userSvc *UserService, roleSvc *RoleService, auditSvc AuditLogger) *Handler {
	return &Handler{userSvc: userSvc, roleSvc: roleSvc, auditSvc: auditSvc}
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

	// Best-effort audit: always record login attempts (success or failure).
	// An audit insert failure must not break the login response.
	h.auditLogin(c, req.Username, resp, err)

	if err != nil {
		types.FailFromError(c, err)
		return
	}
	types.OK(c, resp)
}

// auditLogin records a login attempt to the audit log. On success it captures
// the resolved user identity; on failure it captures the supplied username and
// the AppError code/message. Errors from the audit write itself are swallowed
// since login is not inside a business transaction (no rollback to do).
func (h *Handler) auditLogin(c *gin.Context, username string, resp *LoginResponse, loginErr error) {
	if h.auditSvc == nil {
		return
	}
	entry := audit.Entry{
		Actor: audit.Actor{
			Username:  username,
			TraceID:   types.GetTraceID(c),
			IPAddress: c.ClientIP(),
			UserAgent: c.Request.UserAgent(),
		},
		Action:      audit.ActionLogin,
		Resource:    audit.ResourceUser,
		Description: "用户登录",
	}
	if loginErr != nil {
		entry.Result = audit.ResultFailure
		var appErr *types.AppError
		if errors.As(loginErr, &appErr) {
			entry.ErrorCode = appErr.Code
			entry.ErrorMsg = appErr.Message
		} else {
			entry.ErrorMsg = loginErr.Error()
		}
	} else if resp != nil {
		entry.Result = audit.ResultSuccess
		entry.Actor.UserID = resp.User.ID
		entry.Actor.RoleCode = resp.User.RoleCode
		uid := resp.User.ID
		entry.ResourceID = &uid
	}
	_ = h.auditSvc.Log(c.Request.Context(), entry)
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
		h.auditAppError(c, audit.ActionCreate, audit.ResourceUser, nil, "创建用户失败", nil, err)
		types.FailFromError(c, err)
		return
	}
	h.auditSuccess(c, audit.ActionCreate, audit.ResourceUser, resp.ID, "创建用户", map[string]any{
		"username": resp.Username,
		"roleID":   resp.RoleID,
		"status":   resp.Status,
	})
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
		h.auditAppError(c, audit.ActionUpdate, audit.ResourceUser, &id, "更新用户失败", nil, err)
		types.FailFromError(c, err)
		return
	}
	h.auditSuccess(c, audit.ActionUpdate, audit.ResourceUser, id, "更新用户", map[string]any{
		"realName": resp.RealName,
		"roleID":   resp.RoleID,
		"status":   resp.Status,
	})
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
		h.auditAppError(c, audit.ActionDelete, audit.ResourceUser, &id, "删除用户失败", nil, err)
		types.FailFromError(c, err)
		return
	}
	h.auditSuccess(c, audit.ActionDelete, audit.ResourceUser, id, "删除用户", nil)
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
	h.auditSuccess(c, audit.ActionUpdate, audit.ResourceUser, userID, "修改密码", map[string]any{
		"userID":          userID,
		"passwordChanged": true,
	})
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
	h.auditSuccess(c, audit.ActionUpdate, audit.ResourceUser, id, "重置密码", map[string]any{
		"targetUserID":  id,
		"passwordReset": true,
	})
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
		h.auditAppError(c, audit.ActionCreate, audit.ResourceRole, nil, "创建角色失败", map[string]any{
			"code": req.Code,
			"name": req.Name,
		}, err)
		types.FailFromError(c, err)
		return
	}
	h.auditSuccess(c, audit.ActionCreate, audit.ResourceRole, resp.ID, "创建角色", map[string]any{
		"code":        resp.Code,
		"name":        resp.Name,
		"description": resp.Description,
	})
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
		h.auditAppError(c, audit.ActionUpdate, audit.ResourceRole, &id, "更新角色失败", nil, err)
		types.FailFromError(c, err)
		return
	}
	h.auditSuccess(c, audit.ActionUpdate, audit.ResourceRole, id, "更新角色", map[string]any{
		"name":        resp.Name,
		"description": resp.Description,
	})
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
		h.auditAppError(c, audit.ActionDelete, audit.ResourceRole, &id, "删除角色失败", nil, err)
		types.FailFromError(c, err)
		return
	}
	h.auditSuccess(c, audit.ActionDelete, audit.ResourceRole, id, "删除角色", nil)
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
		h.auditAppError(c, audit.ActionAssign, audit.ResourcePermission, &id, "分配角色权限失败", map[string]any{
			"roleID":        id,
			"permissionIDs": req.PermissionIDs,
		}, err)
		types.FailFromError(c, err)
		return
	}
	h.auditSuccess(c, audit.ActionAssign, audit.ResourcePermission, id, "分配角色权限", map[string]any{
		"roleID":        id,
		"permissionIDs": req.PermissionIDs,
	})
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
		appErr := types.ErrValidation("无效的ID")
		types.Fail(c, http.StatusBadRequest, appErr)
		return 0, appErr
	}
	return uint(id), nil
}

func (h *Handler) auditSuccess(c *gin.Context, action, resource string, resourceID uint, description string, after map[string]any) {
	if h.auditSvc == nil {
		return
	}
	id := resourceID
	_ = h.auditSvc.Log(c.Request.Context(), audit.Entry{
		Actor:       audit.ActorFromContext(c),
		Action:      action,
		Resource:    resource,
		ResourceID:  &id,
		Description: description,
		After:       after,
		Result:      audit.ResultSuccess,
	})
}

func (h *Handler) auditAppError(c *gin.Context, action, resource string, resourceID *uint, description string, after map[string]any, err error) {
	if h.auditSvc == nil {
		return
	}
	var appErr *types.AppError
	if !errors.As(err, &appErr) {
		return
	}
	_ = h.auditSvc.Log(c.Request.Context(), audit.Entry{
		Actor:       audit.ActorFromContext(c),
		Action:      action,
		Resource:    resource,
		ResourceID:  resourceID,
		Description: description,
		After:       after,
		Result:      audit.ResultFailure,
		ErrorCode:   appErr.Code,
		ErrorMsg:    appErr.Message,
	})
}
