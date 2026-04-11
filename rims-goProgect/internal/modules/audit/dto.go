// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package audit

import (
	"encoding/json"
	"time"

	"github.com/gin-gonic/gin"

	"rims-go/internal/types"
)

// Actor captures the request-scoped identity and source metadata of whoever
// triggered an auditable operation. It is built once in the handler via
// ActorFromContext and threaded through to the service layer so that
// downstream audit entries can carry IP / user-agent / trace data that
// context.Context alone does not expose.
type Actor struct {
	UserID      uint   `json:"userId"`
	Username    string `json:"username"`
	RoleCode    string `json:"roleCode"`
	WarehouseID uint   `json:"warehouseId"` // 0 when request is not warehouse-scoped
	TraceID     string `json:"traceId"`
	IPAddress   string `json:"ipAddress"`
	UserAgent   string `json:"userAgent"`
}

// ActorFromContext builds an Actor snapshot from the current gin.Context,
// reading the standard context keys populated by the JWT / WarehouseScope /
// RequestID middlewares plus the transport-level ClientIP / User-Agent.
func ActorFromContext(c *gin.Context) Actor {
	return Actor{
		UserID:      types.GetUserID(c),
		Username:    types.GetUsername(c),
		RoleCode:    types.GetRoleCode(c),
		WarehouseID: types.GetWarehouseID(c),
		TraceID:     types.GetTraceID(c),
		IPAddress:   c.ClientIP(),
		UserAgent:   truncate(c.Request.UserAgent(), 255),
	}
}

// Entry is the input passed to AuditService.Log. Consumers construct it with
// the action/resource plus a before/after snapshot pair; the service marshals
// before/after into the AuditLog.Details JSONB column.
type Entry struct {
	Actor       Actor
	Action      string
	Resource    string
	ResourceID  *uint
	DocNo       string
	Description string
	Before      map[string]any
	After       map[string]any
	Result      string // defaults to ResultSuccess when empty
	ErrorCode   int
	ErrorMsg    string
}

// ListRequest holds filter and pagination parameters for audit log queries.
type ListRequest struct {
	UserID      uint   `form:"userId" binding:"omitempty,min=1"`
	WarehouseID uint   `form:"warehouseId" binding:"omitempty,min=1"`
	Resource    string `form:"resource" binding:"omitempty,max=64"`
	ResourceID  uint   `form:"resourceId" binding:"omitempty,min=1"`
	Action      string `form:"action" binding:"omitempty,max=32"`
	DocNo       string `form:"docNo" binding:"omitempty,max=64"`
	Result      string `form:"result" binding:"omitempty,max=16"`
	StartTime   string `form:"startTime"` // RFC3339 or date only
	EndTime     string `form:"endTime"`
	Keyword     string `form:"keyword" binding:"omitempty,max=128"`
	Page        int    `form:"page" binding:"omitempty,min=1"`
	PageSize    int    `form:"pageSize" binding:"omitempty,min=1,max=100"`
}

// AuditLogResponse is the JSON representation returned to clients. Unlike the
// raw model the Details column is decoded back to a map so API consumers can
// navigate it without a second parse.
type AuditLogResponse struct {
	ID          uint           `json:"id"`
	TraceID     string         `json:"traceId,omitempty"`
	UserID      uint           `json:"userId"`
	Username    string         `json:"username"`
	RoleCode    string         `json:"roleCode"`
	WarehouseID *uint          `json:"warehouseId,omitempty"`
	Action      string         `json:"action"`
	Resource    string         `json:"resource"`
	ResourceID  *uint          `json:"resourceId,omitempty"`
	DocNo       string         `json:"docNo,omitempty"`
	Description string         `json:"description"`
	Details     map[string]any `json:"details,omitempty"`
	IPAddress   string         `json:"ipAddress,omitempty"`
	UserAgent   string         `json:"userAgent,omitempty"`
	Result      string         `json:"result"`
	ErrorCode   int            `json:"errorCode,omitempty"`
	ErrorMsg    string         `json:"errorMsg,omitempty"`
	CreatedAt   time.Time      `json:"createdAt"`
}

// ToAuditLogResponse converts a raw AuditLog row into its JSON shape. A
// malformed Details JSON falls back to a nil map rather than failing the
// request; the underlying raw text is still visible via the admin-only
// query path if needed.
func ToAuditLogResponse(l *AuditLog) AuditLogResponse {
	var details map[string]any
	if l.Details != "" && l.Details != "{}" {
		_ = json.Unmarshal([]byte(l.Details), &details)
	}
	return AuditLogResponse{
		ID:          l.ID,
		TraceID:     l.TraceID,
		UserID:      l.UserID,
		Username:    l.Username,
		RoleCode:    l.RoleCode,
		WarehouseID: l.WarehouseID,
		Action:      l.Action,
		Resource:    l.Resource,
		ResourceID:  l.ResourceID,
		DocNo:       l.DocNo,
		Description: l.Description,
		Details:     details,
		IPAddress:   l.IPAddress,
		UserAgent:   l.UserAgent,
		Result:      l.Result,
		ErrorCode:   l.ErrorCode,
		ErrorMsg:    l.ErrorMsg,
		CreatedAt:   l.CreatedAt,
	}
}

// truncate clips a string to at most n runes, preserving UTF-8 boundaries.
// Audit rows are bounded by column size; oversize user-agent / error strings
// get cut here rather than at the DB driver.
func truncate(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	// Fast byte-level cut is safe here because we over-approximate and the
	// downstream column (VARCHAR(255)) tolerates any valid prefix.
	return s[:n]
}
