// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package product

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"rims-go/internal/types"
)

// Handler handles HTTP requests for product, inventory, and non-standard inventory endpoints.
type Handler struct {
	productSvc *ProductService
}

// NewHandler creates a new product Handler.
func NewHandler(productSvc *ProductService) *Handler {
	return &Handler{productSvc: productSvc}
}

// --- Product CRUD ---

// CreateProduct godoc
// @Summary 创建商品
// @Tags 商品
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param payload body CreateProductRequest true "商品信息"
// @Success 201 {object} types.Response{data=AdminProductResponse}
// @Failure 400 {object} types.Response
// @Failure 403 {object} types.Response
// @Router /api/v1/products [post]
func (h *Handler) CreateProduct(c *gin.Context) {
	if !types.IsAdmin(c) {
		types.FailFromError(c, types.ErrForbidden())
		return
	}
	var req CreateProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		types.Fail(c, http.StatusBadRequest, types.ErrValidation(err.Error()))
		return
	}
	resp, err := h.productSvc.Create(c.Request.Context(), types.GetUserID(c), req)
	if err != nil {
		types.FailFromError(c, err)
		return
	}
	types.OKCreated(c, resp)
}

// ListProducts godoc
// @Summary 商品列表
// @Tags 商品
// @Security BearerAuth
// @Produce json
// @Param page query int false "页码" default(1)
// @Param pageSize query int false "每页数量" default(20)
// @Param keyword query string false "搜索关键字"
// @Success 200 {object} types.Response{data=types.PageResult}
// @Router /api/v1/products [get]
func (h *Handler) ListProducts(c *gin.Context) {
	var page types.PageRequest
	if err := c.ShouldBindQuery(&page); err != nil {
		types.Fail(c, http.StatusBadRequest, types.ErrValidation(err.Error()))
		return
	}
	result, err := h.productSvc.List(c.Request.Context(), page, types.IsAdmin(c))
	if err != nil {
		types.FailFromError(c, err)
		return
	}
	types.OKWithPage(c, result)
}

// GetProduct godoc
// @Summary 获取商品详情
// @Tags 商品
// @Security BearerAuth
// @Produce json
// @Param id path int true "商品ID"
// @Success 200 {object} types.Response{data=ProductResponse}
// @Failure 404 {object} types.Response
// @Router /api/v1/products/{id} [get]
func (h *Handler) GetProduct(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		return
	}
	p, err := h.productSvc.GetByID(c.Request.Context(), id)
	if err != nil {
		types.FailFromError(c, err)
		return
	}
	if types.IsAdmin(c) {
		resp := ToAdminProductResponse(p)
		types.OK(c, &resp)
	} else {
		resp := ToProductResponse(p)
		types.OK(c, &resp)
	}
}

// GetProductByBarcode godoc
// @Summary 条码查询商品
// @Tags 商品
// @Security BearerAuth
// @Produce json
// @Param barcode path string true "条码"
// @Success 200 {object} types.Response{data=ProductResponse}
// @Failure 404 {object} types.Response
// @Router /api/v1/products/barcode/{barcode} [get]
func (h *Handler) GetProductByBarcode(c *gin.Context) {
	barcode := c.Param("barcode")
	if barcode == "" {
		types.Fail(c, http.StatusBadRequest, types.ErrValidation("条码不能为空"))
		return
	}
	p, err := h.productSvc.GetByBarcode(c.Request.Context(), barcode)
	if err != nil {
		types.FailFromError(c, err)
		return
	}
	if types.IsAdmin(c) {
		resp := ToAdminProductResponse(p)
		types.OK(c, &resp)
	} else {
		resp := ToProductResponse(p)
		types.OK(c, &resp)
	}
}

// UpdateProduct godoc
// @Summary 更新商品
// @Tags 商品
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path int true "商品ID"
// @Param payload body UpdateProductRequest true "更新内容"
// @Success 200 {object} types.Response{data=AdminProductResponse}
// @Failure 403 {object} types.Response
// @Router /api/v1/products/{id} [put]
func (h *Handler) UpdateProduct(c *gin.Context) {
	if !types.IsAdmin(c) {
		types.FailFromError(c, types.ErrForbidden())
		return
	}
	id, err := parseID(c, "id")
	if err != nil {
		return
	}
	var req UpdateProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		types.Fail(c, http.StatusBadRequest, types.ErrValidation(err.Error()))
		return
	}
	resp, err := h.productSvc.Update(c.Request.Context(), types.GetUserID(c), id, req)
	if err != nil {
		types.FailFromError(c, err)
		return
	}
	types.OK(c, resp)
}

// DeleteProduct godoc
// @Summary 删除商品
// @Tags 商品
// @Security BearerAuth
// @Param id path int true "商品ID"
// @Success 204
// @Failure 403 {object} types.Response
// @Router /api/v1/products/{id} [delete]
func (h *Handler) DeleteProduct(c *gin.Context) {
	if !types.IsAdmin(c) {
		types.FailFromError(c, types.ErrForbidden())
		return
	}
	id, err := parseID(c, "id")
	if err != nil {
		return
	}
	if err := h.productSvc.Delete(c.Request.Context(), id); err != nil {
		types.FailFromError(c, err)
		return
	}
	types.OKNoContent(c)
}

// --- Standard Inventory ---

// ListInventory godoc
// @Summary 库存列表
// @Tags 库存
// @Security BearerAuth
// @Produce json
// @Param X-Warehouse-ID header int false "仓库ID"
// @Param page query int false "页码" default(1)
// @Param pageSize query int false "每页数量" default(20)
// @Param keyword query string false "搜索关键字"
// @Success 200 {object} types.Response{data=types.PageResult}
// @Router /api/v1/inventory [get]
func (h *Handler) ListInventory(c *gin.Context) {
	var page types.PageRequest
	if err := c.ShouldBindQuery(&page); err != nil {
		types.Fail(c, http.StatusBadRequest, types.ErrValidation(err.Error()))
		return
	}
	warehouseID := types.GetWarehouseID(c)
	result, err := h.productSvc.ListInventory(c.Request.Context(), warehouseID, page)
	if err != nil {
		types.FailFromError(c, err)
		return
	}
	types.OKWithPage(c, result)
}

// GetInventory godoc
// @Summary 获取库存详情
// @Tags 库存
// @Security BearerAuth
// @Produce json
// @Param X-Warehouse-ID header int false "仓库ID"
// @Param id path int true "库存ID"
// @Success 200 {object} types.Response{data=InventoryResponse}
// @Failure 404 {object} types.Response
// @Router /api/v1/inventory/{id} [get]
func (h *Handler) GetInventory(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		return
	}
	warehouseID := types.GetWarehouseID(c)
	resp, err := h.productSvc.GetInventory(c.Request.Context(), warehouseID, id)
	if err != nil {
		types.FailFromError(c, err)
		return
	}
	types.OK(c, resp)
}

// UpdateInventory godoc
// @Summary 更新库存设置
// @Tags 库存
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param X-Warehouse-ID header int false "仓库ID"
// @Param id path int true "库存ID"
// @Param payload body UpdateInventoryRequest true "更新内容"
// @Success 200 {object} types.Response{data=InventoryResponse}
// @Failure 403 {object} types.Response
// @Router /api/v1/inventory/{id} [put]
func (h *Handler) UpdateInventory(c *gin.Context) {
	if !types.IsAdmin(c) {
		types.FailFromError(c, types.ErrForbidden())
		return
	}
	id, err := parseID(c, "id")
	if err != nil {
		return
	}
	var req UpdateInventoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		types.Fail(c, http.StatusBadRequest, types.ErrValidation(err.Error()))
		return
	}
	warehouseID := types.GetWarehouseID(c)
	resp, err := h.productSvc.UpdateInventory(c.Request.Context(), types.GetUserID(c), warehouseID, id, req)
	if err != nil {
		types.FailFromError(c, err)
		return
	}
	types.OK(c, resp)
}

// ListAlerts godoc
// @Summary 库存预警列表
// @Tags 库存
// @Security BearerAuth
// @Produce json
// @Param X-Warehouse-ID header int false "仓库ID"
// @Param page query int false "页码" default(1)
// @Param pageSize query int false "每页数量" default(20)
// @Success 200 {object} types.Response{data=types.PageResult}
// @Router /api/v1/inventory/alerts [get]
func (h *Handler) ListAlerts(c *gin.Context) {
	var page types.PageRequest
	if err := c.ShouldBindQuery(&page); err != nil {
		types.Fail(c, http.StatusBadRequest, types.ErrValidation(err.Error()))
		return
	}
	warehouseID := types.GetWarehouseID(c)
	result, err := h.productSvc.ListAlerts(c.Request.Context(), warehouseID, page)
	if err != nil {
		types.FailFromError(c, err)
		return
	}
	types.OKWithPage(c, result)
}

// --- Non-Standard Inventory ---

// CreateNonStd godoc
// @Summary 创建非标库存
// @Tags 非标库存
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param X-Warehouse-ID header int false "仓库ID"
// @Param payload body CreateNonStdInventoryRequest true "非标库存信息"
// @Success 201 {object} types.Response{data=NonStdInventoryResponse}
// @Failure 400 {object} types.Response
// @Failure 403 {object} types.Response
// @Router /api/v1/non-std-inventory [post]
func (h *Handler) CreateNonStd(c *gin.Context) {
	if !types.IsAdmin(c) {
		types.FailFromError(c, types.ErrForbidden())
		return
	}
	var req CreateNonStdInventoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		types.Fail(c, http.StatusBadRequest, types.ErrValidation(err.Error()))
		return
	}
	warehouseID := types.GetWarehouseID(c)
	resp, err := h.productSvc.CreateNonStd(c.Request.Context(), types.GetUserID(c), warehouseID, req)
	if err != nil {
		types.FailFromError(c, err)
		return
	}
	types.OKCreated(c, resp)
}

// ListNonStd godoc
// @Summary 非标库存列表
// @Tags 非标库存
// @Security BearerAuth
// @Produce json
// @Param X-Warehouse-ID header int false "仓库ID"
// @Param page query int false "页码" default(1)
// @Param pageSize query int false "每页数量" default(20)
// @Param keyword query string false "搜索关键字"
// @Success 200 {object} types.Response{data=types.PageResult}
// @Failure 403 {object} types.Response
// @Router /api/v1/non-std-inventory [get]
func (h *Handler) ListNonStd(c *gin.Context) {
	if !types.IsAdmin(c) {
		types.FailFromError(c, types.ErrForbidden())
		return
	}
	var page types.PageRequest
	if err := c.ShouldBindQuery(&page); err != nil {
		types.Fail(c, http.StatusBadRequest, types.ErrValidation(err.Error()))
		return
	}
	warehouseID := types.GetWarehouseID(c)
	result, err := h.productSvc.ListNonStd(c.Request.Context(), warehouseID, page)
	if err != nil {
		types.FailFromError(c, err)
		return
	}
	types.OKWithPage(c, result)
}

// GetNonStd godoc
// @Summary 获取非标库存详情
// @Tags 非标库存
// @Security BearerAuth
// @Produce json
// @Param X-Warehouse-ID header int false "仓库ID"
// @Param id path int true "非标库存ID"
// @Success 200 {object} types.Response{data=NonStdInventoryResponse}
// @Failure 403 {object} types.Response
// @Failure 404 {object} types.Response
// @Router /api/v1/non-std-inventory/{id} [get]
func (h *Handler) GetNonStd(c *gin.Context) {
	if !types.IsAdmin(c) {
		types.FailFromError(c, types.ErrForbidden())
		return
	}
	id, err := parseID(c, "id")
	if err != nil {
		return
	}
	warehouseID := types.GetWarehouseID(c)
	resp, err := h.productSvc.GetNonStd(c.Request.Context(), warehouseID, id)
	if err != nil {
		types.FailFromError(c, err)
		return
	}
	types.OK(c, resp)
}

// UpdateNonStd godoc
// @Summary 更新非标库存
// @Tags 非标库存
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param X-Warehouse-ID header int false "仓库ID"
// @Param id path int true "非标库存ID"
// @Param payload body UpdateNonStdInventoryRequest true "更新内容"
// @Success 200 {object} types.Response{data=NonStdInventoryResponse}
// @Failure 403 {object} types.Response
// @Router /api/v1/non-std-inventory/{id} [put]
func (h *Handler) UpdateNonStd(c *gin.Context) {
	if !types.IsAdmin(c) {
		types.FailFromError(c, types.ErrForbidden())
		return
	}
	id, err := parseID(c, "id")
	if err != nil {
		return
	}
	var req UpdateNonStdInventoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		types.Fail(c, http.StatusBadRequest, types.ErrValidation(err.Error()))
		return
	}
	warehouseID := types.GetWarehouseID(c)
	resp, err := h.productSvc.UpdateNonStd(c.Request.Context(), types.GetUserID(c), warehouseID, id, req)
	if err != nil {
		types.FailFromError(c, err)
		return
	}
	types.OK(c, resp)
}

// DeleteNonStd godoc
// @Summary 删除非标库存
// @Tags 非标库存
// @Security BearerAuth
// @Param X-Warehouse-ID header int false "仓库ID"
// @Param id path int true "非标库存ID"
// @Success 204
// @Failure 403 {object} types.Response
// @Router /api/v1/non-std-inventory/{id} [delete]
func (h *Handler) DeleteNonStd(c *gin.Context) {
	if !types.IsAdmin(c) {
		types.FailFromError(c, types.ErrForbidden())
		return
	}
	id, err := parseID(c, "id")
	if err != nil {
		return
	}
	warehouseID := types.GetWarehouseID(c)
	if err := h.productSvc.DeleteNonStd(c.Request.Context(), warehouseID, id); err != nil {
		types.FailFromError(c, err)
		return
	}
	types.OKNoContent(c)
}

// ConvertNonStd godoc
// @Summary 非标转标准库存
// @Tags 非标库存
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param X-Warehouse-ID header int false "仓库ID"
// @Param id path int true "非标库存ID"
// @Param payload body ConvertNonStdRequest true "转换信息"
// @Success 200 {object} types.Response
// @Failure 403 {object} types.Response
// @Router /api/v1/non-std-inventory/{id}/convert [post]
func (h *Handler) ConvertNonStd(c *gin.Context) {
	if !types.IsAdmin(c) {
		types.FailFromError(c, types.ErrForbidden())
		return
	}
	id, err := parseID(c, "id")
	if err != nil {
		return
	}
	var req ConvertNonStdRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		types.Fail(c, http.StatusBadRequest, types.ErrValidation(err.Error()))
		return
	}
	warehouseID := types.GetWarehouseID(c)
	if err := h.productSvc.ConvertNonStd(c.Request.Context(), types.GetUserID(c), warehouseID, id, req); err != nil {
		types.FailFromError(c, err)
		return
	}
	types.OK(c, nil)
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
