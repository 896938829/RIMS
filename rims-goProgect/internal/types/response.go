// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package types

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Response is the unified API response envelope.
type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
	TraceID string      `json:"traceId,omitempty"`
}

// OK sends a successful response with data.
func OK(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Code:    ErrCodeOK,
		Message: "success",
		Data:    data,
		TraceID: GetTraceID(c),
	})
}

// OKCreated sends a 201 response with data.
func OKCreated(c *gin.Context, data interface{}) {
	c.JSON(http.StatusCreated, Response{
		Code:    ErrCodeOK,
		Message: "success",
		Data:    data,
		TraceID: GetTraceID(c),
	})
}

// OKWithPage sends a successful paginated response.
func OKWithPage(c *gin.Context, page PageResult) {
	c.JSON(http.StatusOK, Response{
		Code:    ErrCodeOK,
		Message: "success",
		Data:    page,
		TraceID: GetTraceID(c),
	})
}

// OKNoContent sends a 204 response.
func OKNoContent(c *gin.Context) {
	c.Status(http.StatusNoContent)
}

// Fail sends an error response with a specific HTTP status and AppError.
func Fail(c *gin.Context, httpStatus int, appErr *AppError) {
	c.JSON(httpStatus, Response{
		Code:    appErr.Code,
		Message: appErr.Message,
		TraceID: GetTraceID(c),
	})
}

// FailFromError inspects err for *AppError and sends the appropriate response.
// For non-AppError errors, it sends a generic 500 response.
func FailFromError(c *gin.Context, err error) {
	var appErr *AppError
	if errors.As(err, &appErr) {
		Fail(c, appErr.HTTPStatus(), appErr)
		return
	}
	Fail(c, http.StatusInternalServerError, ErrSystem(err))
}
