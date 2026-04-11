// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package document

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"rims-go/internal/modules/audit"
	"rims-go/internal/types"
)

// Handler handles HTTP requests for documents and inventory transactions.
type Handler struct {
	docSvc *DocumentService
}

// NewHandler creates a new document Handler.
func NewHandler(docSvc *DocumentService) *Handler {
	return &Handler{docSvc: docSvc}
}

// CreateDocument godoc
// @Summary 创建单据
// @Description 创建草稿单据（入库/销售/退货/调拨/盘点/转换）
// @Tags 单据
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param X-Warehouse-ID header int false "仓库ID"
// @Param payload body CreateDocumentRequest true "单据信息"
// @Success 201 {object} types.Response{data=DocumentResponse}
// @Failure 400 {object} types.Response
// @Failure 403 {object} types.Response
// @Router /api/v1/documents [post]
func (h *Handler) CreateDocument(c *gin.Context) {
	var req CreateDocumentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		types.Fail(c, http.StatusBadRequest, types.ErrValidation(err.Error()))
		return
	}

	resp, err := h.docSvc.Create(
		c.Request.Context(),
		types.GetUserID(c),
		types.GetWarehouseID(c),
		req,
	)
	if err != nil {
		types.FailFromError(c, err)
		return
	}
	types.OKCreated(c, resp)
}

// ListDocuments godoc
// @Summary 单据列表
// @Description 分页查询当前仓库的单据，可按类型筛选
// @Tags 单据
// @Security BearerAuth
// @Produce json
// @Param X-Warehouse-ID header int false "仓库ID"
// @Param docType query int false "单据类型 1=入库 2=销售 3=退货 4=调拨 5=盘点 6=转换"
// @Param page query int false "页码" default(1)
// @Param pageSize query int false "每页数量" default(20)
// @Param keyword query string false "搜索单据号"
// @Success 200 {object} types.Response{data=types.PageResult}
// @Router /api/v1/documents [get]
func (h *Handler) ListDocuments(c *gin.Context) {
	var page types.PageRequest
	if err := c.ShouldBindQuery(&page); err != nil {
		types.Fail(c, http.StatusBadRequest, types.ErrValidation(err.Error()))
		return
	}

	var docType int8
	if dt := c.Query("docType"); dt != "" {
		n, err := strconv.Atoi(dt)
		if err != nil || n < 0 || n > 6 {
			types.Fail(c, http.StatusBadRequest, types.ErrValidation("无效的单据类型"))
			return
		}
		docType = int8(n)
	}

	result, err := h.docSvc.List(
		c.Request.Context(),
		types.GetWarehouseID(c),
		docType,
		page,
	)
	if err != nil {
		types.FailFromError(c, err)
		return
	}
	types.OKWithPage(c, result)
}

// GetDocument godoc
// @Summary 单据详情
// @Description 获取单据及其明细行
// @Tags 单据
// @Security BearerAuth
// @Produce json
// @Param X-Warehouse-ID header int false "仓库ID"
// @Param id path int true "单据ID"
// @Success 200 {object} types.Response{data=DocumentDetailResponse}
// @Failure 404 {object} types.Response
// @Router /api/v1/documents/{id} [get]
func (h *Handler) GetDocument(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		return
	}

	resp, err := h.docSvc.Get(
		c.Request.Context(),
		types.GetWarehouseID(c),
		id,
	)
	if err != nil {
		types.FailFromError(c, err)
		return
	}
	types.OK(c, resp)
}

// CompleteDocument godoc
// @Summary 完成单据
// @Description 执行单据并更新库存（入库/销售/退货/调拨/转换）
// @Tags 单据
// @Security BearerAuth
// @Produce json
// @Param X-Warehouse-ID header int false "仓库ID"
// @Param id path int true "单据ID"
// @Success 204
// @Failure 400 {object} types.Response
// @Failure 403 {object} types.Response
// @Failure 404 {object} types.Response
// @Failure 422 {object} types.Response
// @Router /api/v1/documents/{id}/complete [post]
func (h *Handler) CompleteDocument(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		return
	}

	if err := h.docSvc.Complete(
		c.Request.Context(),
		audit.ActorFromContext(c),
		types.GetWarehouseID(c),
		id,
		types.IsAdmin(c),
	); err != nil {
		types.FailFromError(c, err)
		return
	}
	types.OKNoContent(c)
}

// ConfirmStocktake godoc
// @Summary 确认盘点差异
// @Description 将盘点单从"盘点中"转为"差异已确认"
// @Tags 单据
// @Security BearerAuth
// @Produce json
// @Param X-Warehouse-ID header int false "仓库ID"
// @Param id path int true "单据ID"
// @Success 204
// @Failure 400 {object} types.Response
// @Failure 403 {object} types.Response
// @Failure 404 {object} types.Response
// @Failure 422 {object} types.Response
// @Router /api/v1/documents/{id}/confirm [post]
func (h *Handler) ConfirmStocktake(c *gin.Context) {
	if !types.IsAdmin(c) {
		types.FailFromError(c, types.ErrForbidden())
		return
	}

	id, err := parseID(c, "id")
	if err != nil {
		return
	}

	if err := h.docSvc.ConfirmStocktake(
		c.Request.Context(),
		types.GetUserID(c),
		types.GetWarehouseID(c),
		id,
	); err != nil {
		types.FailFromError(c, err)
		return
	}
	types.OKNoContent(c)
}

// SettleStocktake godoc
// @Summary 盘点结转
// @Description 将已确认的盘点单结转，应用库存差异
// @Tags 单据
// @Security BearerAuth
// @Produce json
// @Param X-Warehouse-ID header int false "仓库ID"
// @Param id path int true "单据ID"
// @Success 204
// @Failure 400 {object} types.Response
// @Failure 403 {object} types.Response
// @Failure 404 {object} types.Response
// @Failure 422 {object} types.Response
// @Router /api/v1/documents/{id}/settle [post]
func (h *Handler) SettleStocktake(c *gin.Context) {
	if !types.IsAdmin(c) {
		types.FailFromError(c, types.ErrForbidden())
		return
	}

	id, err := parseID(c, "id")
	if err != nil {
		return
	}

	if err := h.docSvc.SettleStocktake(
		c.Request.Context(),
		types.GetUserID(c),
		types.GetWarehouseID(c),
		id,
	); err != nil {
		types.FailFromError(c, err)
		return
	}
	types.OKNoContent(c)
}

// ListTransactions godoc
// @Summary 库存流水列表
// @Description 分页查询当前仓库的库存变动记录
// @Tags 单据
// @Security BearerAuth
// @Produce json
// @Param X-Warehouse-ID header int false "仓库ID"
// @Param page query int false "页码" default(1)
// @Param pageSize query int false "每页数量" default(20)
// @Param keyword query string false "搜索单据号"
// @Success 200 {object} types.Response{data=types.PageResult}
// @Router /api/v1/transactions [get]
func (h *Handler) ListTransactions(c *gin.Context) {
	var page types.PageRequest
	if err := c.ShouldBindQuery(&page); err != nil {
		types.Fail(c, http.StatusBadRequest, types.ErrValidation(err.Error()))
		return
	}

	result, err := h.docSvc.ListTransactions(
		c.Request.Context(),
		types.GetWarehouseID(c),
		page,
	)
	if err != nil {
		types.FailFromError(c, err)
		return
	}
	types.OKWithPage(c, result)
}

// parseID extracts and validates a uint path parameter.
func parseID(c *gin.Context, name string) (uint, error) {
	idStr := c.Param(name)
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil || id == 0 {
		types.Fail(c, http.StatusBadRequest, types.ErrValidation("无效的ID"))
		return 0, err
	}
	return uint(id), nil
}
