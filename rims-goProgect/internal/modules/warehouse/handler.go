// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package warehouse

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"rims-go/internal/modules/audit"
	"rims-go/internal/types"
)

// AuditLogger is the narrow audit contract consumed by the warehouse handler.
type AuditLogger interface {
	Log(ctx context.Context, e audit.Entry) error
}

// Handler handles HTTP requests for warehouse endpoints.
type Handler struct {
	warehouseSvc *WarehouseService
	auditSvc     AuditLogger
}

// NewHandler creates a new warehouse Handler.
func NewHandler(warehouseSvc *WarehouseService, auditSvc ...AuditLogger) *Handler {
	h := &Handler{warehouseSvc: warehouseSvc}
	if len(auditSvc) > 0 {
		h.auditSvc = auditSvc[0]
	}
	return h
}

// --- Warehouse CRUD ---

// CreateWarehouse godoc
// @Summary 创建仓库
// @Tags 仓库
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param payload body CreateWarehouseRequest true "仓库信息"
// @Success 201 {object} types.Response{data=WarehouseResponse}
// @Failure 400 {object} types.Response
// @Failure 403 {object} types.Response
// @Router /api/v1/warehouses [post]
func (h *Handler) CreateWarehouse(c *gin.Context) {
	var req CreateWarehouseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		types.Fail(c, http.StatusBadRequest, types.ErrValidation(err.Error()))
		return
	}
	resp, err := h.warehouseSvc.Create(c.Request.Context(), types.GetUserID(c), req)
	if err != nil {
		types.FailFromError(c, err)
		return
	}
	id := resp.ID
	h.auditSuccess(c, audit.ActionCreate, audit.ResourceWarehouse, &id, "创建仓库", map[string]any{
		"warehouseID": resp.ID,
		"type":        audit.ResourceWarehouse,
		"code":        resp.Code,
		"name":        resp.Name,
		"status":      resp.Status,
	})
	types.OKCreated(c, resp)
}

// ListWarehouses godoc
// @Summary 仓库列表
// @Tags 仓库
// @Security BearerAuth
// @Produce json
// @Param page query int false "页码" default(1)
// @Param pageSize query int false "每页数量" default(20)
// @Param keyword query string false "搜索关键字"
// @Success 200 {object} types.Response{data=types.PageResult}
// @Router /api/v1/warehouses [get]
func (h *Handler) ListWarehouses(c *gin.Context) {
	var page types.PageRequest
	if err := c.ShouldBindQuery(&page); err != nil {
		types.Fail(c, http.StatusBadRequest, types.ErrValidation(err.Error()))
		return
	}
	result, err := h.warehouseSvc.List(
		c.Request.Context(),
		types.GetUserID(c),
		types.GetRoleCode(c),
		page,
	)
	if err != nil {
		types.FailFromError(c, err)
		return
	}
	types.OKWithPage(c, result)
}

// GetWarehouse godoc
// @Summary 获取仓库详情
// @Tags 仓库
// @Security BearerAuth
// @Produce json
// @Param id path int true "仓库ID"
// @Success 200 {object} types.Response{data=WarehouseResponse}
// @Failure 404 {object} types.Response
// @Router /api/v1/warehouses/{id} [get]
func (h *Handler) GetWarehouse(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		return
	}
	resp, err := h.warehouseSvc.GetByID(c.Request.Context(), id)
	if err != nil {
		types.FailFromError(c, err)
		return
	}
	types.OK(c, resp)
}

// UpdateWarehouse godoc
// @Summary 更新仓库
// @Tags 仓库
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path int true "仓库ID"
// @Param payload body UpdateWarehouseRequest true "更新内容"
// @Success 200 {object} types.Response{data=WarehouseResponse}
// @Failure 403 {object} types.Response
// @Router /api/v1/warehouses/{id} [put]
func (h *Handler) UpdateWarehouse(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		return
	}
	var req UpdateWarehouseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		types.Fail(c, http.StatusBadRequest, types.ErrValidation(err.Error()))
		return
	}
	resp, err := h.warehouseSvc.Update(c.Request.Context(), types.GetUserID(c), id, req)
	if err != nil {
		types.FailFromError(c, err)
		return
	}
	h.auditSuccess(c, audit.ActionUpdate, audit.ResourceWarehouse, &id, "更新仓库", map[string]any{
		"warehouseID": resp.ID,
		"type":        audit.ResourceWarehouse,
		"name":        resp.Name,
		"status":      resp.Status,
	})
	types.OK(c, resp)
}

// DeleteWarehouse godoc
// @Summary 删除仓库
// @Tags 仓库
// @Security BearerAuth
// @Param id path int true "仓库ID"
// @Success 204
// @Failure 403 {object} types.Response
// @Router /api/v1/warehouses/{id} [delete]
func (h *Handler) DeleteWarehouse(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		return
	}
	if err := h.warehouseSvc.Delete(c.Request.Context(), id); err != nil {
		types.FailFromError(c, err)
		return
	}
	h.auditSuccess(c, audit.ActionDelete, audit.ResourceWarehouse, &id, "删除仓库", map[string]any{
		"warehouseID": id,
		"type":        audit.ResourceWarehouse,
	})
	types.OKNoContent(c)
}

// --- User-Warehouse Binding ---

// BindUsers godoc
// @Summary 绑定用户到仓库
// @Tags 仓库
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path int true "仓库ID"
// @Param payload body BindUsersRequest true "用户ID列表"
// @Success 200 {object} types.Response
// @Failure 403 {object} types.Response
// @Router /api/v1/warehouses/{id}/users [post]
func (h *Handler) BindUsers(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		return
	}
	var req BindUsersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		types.Fail(c, http.StatusBadRequest, types.ErrValidation(err.Error()))
		return
	}
	if err := h.warehouseSvc.BindUsers(c.Request.Context(), id, req); err != nil {
		types.FailFromError(c, err)
		return
	}
	h.auditSuccess(c, audit.ActionBind, audit.ResourceUserWarehouse, nil, "绑定用户到仓库", map[string]any{
		"warehouseID": id,
		"userIDs":     req.UserIDs,
	})
	types.OK(c, nil)
}

// UnbindUser godoc
// @Summary 解绑用户与仓库
// @Tags 仓库
// @Security BearerAuth
// @Param id path int true "仓库ID"
// @Param userId path int true "用户ID"
// @Success 204
// @Failure 403 {object} types.Response
// @Router /api/v1/warehouses/{id}/users/{userId} [delete]
func (h *Handler) UnbindUser(c *gin.Context) {
	warehouseID, err := parseID(c, "id")
	if err != nil {
		return
	}
	userID, err := parseID(c, "userId")
	if err != nil {
		return
	}
	if err := h.warehouseSvc.UnbindUser(c.Request.Context(), warehouseID, userID); err != nil {
		types.FailFromError(c, err)
		return
	}
	h.auditSuccess(c, audit.ActionUnbind, audit.ResourceUserWarehouse, nil, "解绑用户与仓库", map[string]any{
		"warehouseID": warehouseID,
		"userID":      userID,
	})
	types.OKNoContent(c)
}

// ListWarehouseUsers godoc
// @Summary 仓库用户列表
// @Tags 仓库
// @Security BearerAuth
// @Produce json
// @Param id path int true "仓库ID"
// @Param page query int false "页码" default(1)
// @Param pageSize query int false "每页数量" default(20)
// @Param keyword query string false "搜索关键字"
// @Success 200 {object} types.Response{data=types.PageResult}
// @Failure 404 {object} types.Response
// @Router /api/v1/warehouses/{id}/users [get]
func (h *Handler) ListWarehouseUsers(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		return
	}
	var page types.PageRequest
	if err := c.ShouldBindQuery(&page); err != nil {
		types.Fail(c, http.StatusBadRequest, types.ErrValidation(err.Error()))
		return
	}
	result, err := h.warehouseSvc.ListWarehouseUsers(c.Request.Context(), id, page)
	if err != nil {
		types.FailFromError(c, err)
		return
	}
	types.OKWithPage(c, result)
}

// --- Current User's Warehouses ---

// GetMyWarehouses godoc
// @Summary 获取当前用户的仓库列表
// @Tags 仓库
// @Security BearerAuth
// @Produce json
// @Success 200 {object} types.Response{data=[]UserWarehouseResponse}
// @Router /api/v1/users/me/warehouses [get]
func (h *Handler) GetMyWarehouses(c *gin.Context) {
	userID := types.GetUserID(c)
	result, err := h.warehouseSvc.GetMyWarehouses(c.Request.Context(), userID)
	if err != nil {
		types.FailFromError(c, err)
		return
	}
	types.OK(c, result)
}

// SetDefaultWarehouse godoc
// @Summary 设置默认仓库
// @Tags 仓库
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param payload body SetDefaultWarehouseRequest true "仓库ID"
// @Success 200 {object} types.Response
// @Failure 403 {object} types.Response
// @Router /api/v1/users/me/warehouses/default [put]
func (h *Handler) SetDefaultWarehouse(c *gin.Context) {
	var req SetDefaultWarehouseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		types.Fail(c, http.StatusBadRequest, types.ErrValidation(err.Error()))
		return
	}
	userID := types.GetUserID(c)
	if err := h.warehouseSvc.SetDefaultWarehouse(c.Request.Context(), userID, req); err != nil {
		types.FailFromError(c, err)
		return
	}
	h.auditSuccess(c, audit.ActionUpdate, audit.ResourceUserWarehouse, nil, "设置默认仓库", map[string]any{
		"warehouseID": req.WarehouseID,
		"userID":      userID,
	})
	types.OK(c, nil)
}

// SwitchCurrentWarehouse godoc
// @Summary 切换当前仓库
// @Tags 仓库
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param payload body SwitchWarehouseRequest true "仓库ID"
// @Success 200 {object} types.Response{data=WarehouseResponse}
// @Failure 403 {object} types.Response
// @Router /api/v1/users/me/warehouses/current [put]
func (h *Handler) SwitchCurrentWarehouse(c *gin.Context) {
	var req SwitchWarehouseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		types.Fail(c, http.StatusBadRequest, types.ErrValidation(err.Error()))
		return
	}
	userID := types.GetUserID(c)
	resp, err := h.warehouseSvc.SwitchCurrentWarehouse(c.Request.Context(), userID, req)
	if err != nil {
		types.FailFromError(c, err)
		return
	}
	types.OK(c, resp)
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

func (h *Handler) auditSuccess(c *gin.Context, action, resource string, resourceID *uint, description string, after map[string]any) {
	if h.auditSvc == nil {
		return
	}
	_ = h.auditSvc.Log(c.Request.Context(), audit.Entry{
		Actor:       audit.ActorFromContext(c),
		Action:      action,
		Resource:    resource,
		ResourceID:  resourceID,
		Description: description,
		After:       after,
		Result:      audit.ResultSuccess,
	})
}
