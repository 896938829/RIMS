// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package file

import (
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/gin-gonic/gin"

	"rims-go/internal/types"
)

// Handler handles HTTP requests for file attachment endpoints.
type Handler struct {
	svc *FileService
}

// NewHandler creates a new file Handler.
func NewHandler(svc *FileService) *Handler {
	return &Handler{svc: svc}
}

// Upload godoc
// @Summary 上传文件
// @Tags 文件
// @Security BearerAuth
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "文件内容"
// @Param businessType formData string false "业务类型 (product_image/doc_attachment/import_template/export_result/other)"
// @Param businessId formData int false "关联业务对象ID"
// @Success 201 {object} types.Response{data=FileResponse}
// @Failure 400 {object} types.Response
// @Router /api/v1/files/upload [post]
func (h *Handler) Upload(c *gin.Context) {
	fh, err := c.FormFile("file")
	if err != nil {
		types.Fail(c, http.StatusBadRequest, types.ErrValidation("未提供文件"))
		return
	}

	f, err := fh.Open()
	if err != nil {
		types.Fail(c, http.StatusBadRequest, types.ErrValidation("文件读取失败"))
		return
	}
	defer f.Close()

	businessType := c.PostForm("businessType")

	var businessID *uint
	if bidStr := c.PostForm("businessId"); bidStr != "" {
		bid, err := strconv.ParseUint(bidStr, 10, 64)
		if err != nil || bid == 0 {
			types.Fail(c, http.StatusBadRequest, types.ErrValidation("无效的businessId"))
			return
		}
		v := uint(bid)
		businessID = &v
	}

	req := UploadRequest{
		BusinessType: businessType,
		BusinessID:   businessID,
		OriginalName: fh.Filename,
		Reader:       f,
		DeclaredSize: fh.Size,
	}
	record, err := h.svc.Upload(c.Request.Context(), types.GetUserID(c), req)
	if err != nil {
		types.FailFromError(c, err)
		return
	}
	resp := ToFileResponse(record, types.IsAdmin(c))
	types.OKCreated(c, &resp)
}

// List godoc
// @Summary 文件列表
// @Tags 文件
// @Security BearerAuth
// @Produce json
// @Param businessType query string false "业务类型"
// @Param businessId query int false "业务对象ID"
// @Param page query int false "页码" default(1)
// @Param pageSize query int false "每页数量" default(20)
// @Success 200 {object} types.Response{data=types.PageResult}
// @Router /api/v1/files [get]
func (h *Handler) List(c *gin.Context) {
	var req ListFilesRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		types.Fail(c, http.StatusBadRequest, types.ErrValidation(err.Error()))
		return
	}

	filter := ListFilter{BusinessType: req.BusinessType}
	if req.BusinessID > 0 {
		bid := req.BusinessID
		filter.BusinessID = &bid
	}
	page := types.PageRequest{Page: req.Page, PageSize: req.PageSize}
	result, err := h.svc.List(c.Request.Context(), filter, page, types.IsAdmin(c))
	if err != nil {
		types.FailFromError(c, err)
		return
	}
	types.OKWithPage(c, result)
}

// Get godoc
// @Summary 文件详情
// @Tags 文件
// @Security BearerAuth
// @Produce json
// @Param id path int true "文件ID"
// @Success 200 {object} types.Response{data=FileResponse}
// @Failure 404 {object} types.Response
// @Router /api/v1/files/{id} [get]
func (h *Handler) Get(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		return
	}
	f, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		types.FailFromError(c, err)
		return
	}
	resp := ToFileResponse(f, types.IsAdmin(c))
	types.OK(c, &resp)
}

// Download godoc
// @Summary 下载文件
// @Tags 文件
// @Security BearerAuth
// @Produce octet-stream
// @Param id path int true "文件ID"
// @Success 200 {file} binary
// @Failure 404 {object} types.Response
// @Router /api/v1/files/{id}/download [get]
func (h *Handler) Download(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		return
	}
	rc, f, err := h.svc.OpenForDownload(c.Request.Context(), id)
	if err != nil {
		types.FailFromError(c, err)
		return
	}
	defer rc.Close()

	c.Header("Content-Type", f.MimeType)
	c.Header("Content-Length", strconv.FormatInt(f.FileSize, 10))
	// RFC 5987 filename* parameter handles non-ASCII original names.
	c.Header("Content-Disposition",
		`attachment; filename="`+sanitizeFilename(f.OriginalName)+`"; filename*=UTF-8''`+url.PathEscape(f.OriginalName))
	c.Status(http.StatusOK)
	if _, err := io.Copy(c.Writer, rc); err != nil {
		// Response is already committed; nothing useful to return.
		return
	}
}

// Delete godoc
// @Summary 删除文件
// @Tags 文件
// @Security BearerAuth
// @Param id path int true "文件ID"
// @Success 204
// @Failure 403 {object} types.Response
// @Failure 404 {object} types.Response
// @Router /api/v1/files/{id} [delete]
func (h *Handler) Delete(c *gin.Context) {
	id, err := parseID(c, "id")
	if err != nil {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), id, types.GetUserID(c), types.IsAdmin(c)); err != nil {
		types.FailFromError(c, err)
		return
	}
	types.OKNoContent(c)
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

// sanitizeFilename strips characters that would break a Content-Disposition
// filename= value. Non-ASCII characters are dropped here; filename*= carries
// the full UTF-8 version.
func sanitizeFilename(name string) string {
	b := make([]byte, 0, len(name))
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c < 0x20 || c == 0x7f || c == '"' || c == '\\' || c > 0x7e {
			b = append(b, '_')
			continue
		}
		b = append(b, c)
	}
	return string(b)
}
