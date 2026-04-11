// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package audit

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"rims-go/internal/types"
)

// Handler handles HTTP requests for audit log query endpoints. All endpoints
// are admin-only; non-admin callers receive 10002 权限不足. No write endpoint
// is exposed — audit records only originate from inside the service layer.
type Handler struct {
	svc *AuditService
}

// NewHandler creates a new audit Handler.
func NewHandler(svc *AuditService) *Handler {
	return &Handler{svc: svc}
}

// List godoc
// @Summary 审计日志列表
// @Description 分页查询审计日志，支持按用户/仓库/资源/动作/单据号/时间范围过滤。仅管理员可访问。
// @Tags 审计
// @Security BearerAuth
// @Produce json
// @Param userId query int false "操作人ID"
// @Param warehouseId query int false "仓库ID"
// @Param resource query string false "资源类型 (user/role/warehouse/product/inventory/non_std_inventory/document/file)"
// @Param resourceId query int false "资源ID"
// @Param action query string false "动作 (login/create/update/delete/complete/confirm/settle/convert/bind/unbind/assign)"
// @Param docNo query string false "单据号"
// @Param result query string false "结果 (success/failure)"
// @Param startTime query string false "起始时间 (RFC3339 或 YYYY-MM-DD)"
// @Param endTime query string false "结束时间 (RFC3339 或 YYYY-MM-DD)"
// @Param keyword query string false "描述/用户名/单据号关键字"
// @Param page query int false "页码" default(1)
// @Param pageSize query int false "每页数量" default(20)
// @Success 200 {object} types.Response{data=types.PageResult}
// @Failure 400 {object} types.Response
// @Failure 403 {object} types.Response
// @Router /api/v1/audit/logs [get]
func (h *Handler) List(c *gin.Context) {
	if !types.IsAdmin(c) {
		types.FailFromError(c, types.ErrForbidden())
		return
	}
	var req ListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		types.Fail(c, http.StatusBadRequest, types.ErrValidation(err.Error()))
		return
	}
	result, err := h.svc.List(c.Request.Context(), req)
	if err != nil {
		types.FailFromError(c, err)
		return
	}
	types.OKWithPage(c, result)
}

// Get godoc
// @Summary 审计日志详情
// @Description 获取单条审计日志的完整内容（含 details JSON）。仅管理员可访问。
// @Tags 审计
// @Security BearerAuth
// @Produce json
// @Param id path int true "审计日志ID"
// @Success 200 {object} types.Response{data=AuditLogResponse}
// @Failure 403 {object} types.Response
// @Failure 404 {object} types.Response
// @Router /api/v1/audit/logs/{id} [get]
func (h *Handler) Get(c *gin.Context) {
	if !types.IsAdmin(c) {
		types.FailFromError(c, types.ErrForbidden())
		return
	}
	id, err := parseID(c, "id")
	if err != nil {
		return
	}
	resp, err := h.svc.Get(c.Request.Context(), id)
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
