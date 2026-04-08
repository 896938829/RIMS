// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package types

import (
	"fmt"
	"net/http"
)

// Business error codes.
const (
	ErrCodeOK               = 0
	ErrCodeAuthFailed       = 10001
	ErrCodePermissionDenied = 10002
	ErrCodeValidation       = 10003
	ErrCodeNotFound         = 10004
	ErrCodeDuplicate        = 10005
	ErrCodeInsufficientStock = 20001
	ErrCodeInvalidState     = 20002
	ErrCodeDuplicateSubmit  = 20003
	ErrCodeSystemError      = 50000
)

// AppError represents a business-level error with code and message.
type AppError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Err     error  `json:"-"`
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%d] %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("[%d] %s", e.Code, e.Message)
}

func (e *AppError) Unwrap() error {
	return e.Err
}

// HTTPStatus maps business error codes to HTTP status codes.
func (e *AppError) HTTPStatus() int {
	switch e.Code {
	case ErrCodeAuthFailed:
		return http.StatusUnauthorized
	case ErrCodePermissionDenied:
		return http.StatusForbidden
	case ErrCodeValidation:
		return http.StatusBadRequest
	case ErrCodeNotFound:
		return http.StatusNotFound
	case ErrCodeDuplicate, ErrCodeDuplicateSubmit:
		return http.StatusConflict
	case ErrCodeInsufficientStock, ErrCodeInvalidState:
		return http.StatusUnprocessableEntity
	default:
		return http.StatusInternalServerError
	}
}

// Error constructors.

func NewAppError(code int, message string, err error) *AppError {
	return &AppError{Code: code, Message: message, Err: err}
}

func ErrAuth(msg string) *AppError {
	return &AppError{Code: ErrCodeAuthFailed, Message: msg}
}

func ErrForbidden() *AppError {
	return &AppError{Code: ErrCodePermissionDenied, Message: "权限不足"}
}

func ErrValidation(msg string) *AppError {
	return &AppError{Code: ErrCodeValidation, Message: msg}
}

func ErrNotFound(entity string) *AppError {
	return &AppError{Code: ErrCodeNotFound, Message: fmt.Sprintf("%s不存在", entity)}
}

func ErrDuplicate(msg string) *AppError {
	return &AppError{Code: ErrCodeDuplicate, Message: msg}
}

func ErrInsufficientStock() *AppError {
	return &AppError{Code: ErrCodeInsufficientStock, Message: "库存不足"}
}

func ErrInvalidState(msg string) *AppError {
	return &AppError{Code: ErrCodeInvalidState, Message: msg}
}

func ErrSystem(err error) *AppError {
	return &AppError{Code: ErrCodeSystemError, Message: "系统异常", Err: err}
}
