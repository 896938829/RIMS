// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package idempotency

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"rims-go/internal/types"
)

// StatusService is the service surface required by the status handler.
type StatusService interface {
	Status(ctx context.Context, userID uint, scope, key string) (*OperationStatus, error)
}

// Handler exposes safe idempotency operation metadata.
type Handler struct {
	service StatusService
}

// NewHandler creates an idempotency status handler.
func NewHandler(service StatusService) *Handler {
	return &Handler{service: service}
}

// GetStatus handles GET /operations/idempotency/:key.
func (h *Handler) GetStatus(c *gin.Context) {
	key := c.Param("key")
	if err := ValidateKey(key); err != nil {
		types.Fail(c, http.StatusBadRequest, types.ErrValidation("无效的幂等键"))
		return
	}

	scope := c.Query("scope")
	if !isAllowedMutationScope(scope) {
		types.Fail(c, http.StatusBadRequest, types.ErrValidation("无效的幂等操作范围"))
		return
	}

	status, err := h.service.Status(
		c.Request.Context(),
		types.GetUserID(c),
		scope,
		key,
	)
	if errors.Is(err, ErrRecordNotFound) {
		types.Fail(c, http.StatusNotFound, types.ErrNotFound("幂等操作"))
		return
	}
	if err != nil {
		types.FailFromError(c, err)
		return
	}

	types.OK(c, status)
}
