// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package audit

import "rims-go/internal/types"

// Action constants are the well-known values stored in AuditLog.Action.
// Consumers should prefer these constants over string literals; unknown
// values are still allowed so new actions don't need an immediate code change.
const (
	ActionLogin    = "login"
	ActionLogout   = "logout"
	ActionCreate   = "create"
	ActionUpdate   = "update"
	ActionDelete   = "delete"
	ActionComplete = "complete"
	ActionConfirm  = "confirm"
	ActionSettle   = "settle"
	ActionConvert  = "convert"
	ActionBind     = "bind"
	ActionUnbind   = "unbind"
	ActionAssign   = "assign"
)

// Resource constants name the business entity an audit record targets.
const (
	ResourceUser            = "user"
	ResourceRole            = "role"
	ResourcePermission      = "permission"
	ResourceWarehouse       = "warehouse"
	ResourceUserWarehouse   = "user_warehouse"
	ResourceProduct         = "product"
	ResourceInventory       = "inventory"
	ResourceNonStdInventory = "non_std_inventory"
	ResourceDocument        = "document"
	ResourceFile            = "file"
)

// Result values for AuditLog.Result.
const (
	ResultSuccess = "success"
	ResultFailure = "failure"
)

// AuditLog is an append-only record of a single auditable operation.
// Rows are written from inside the service layer (either from the active
// business transaction for atomic writes, or best-effort for non-tx flows
// like login).
type AuditLog struct {
	types.BaseModel
	TraceID     string `gorm:"size:64;not null;default:'';index:idx_audit_trace" json:"traceId"`
	UserID      uint   `gorm:"not null;default:0;index:idx_audit_user_time,priority:1" json:"userId"`
	Username    string `gorm:"size:64;not null;default:''" json:"username"`
	RoleCode    string `gorm:"size:32;not null;default:''" json:"roleCode"`
	WarehouseID *uint  `gorm:"index:idx_audit_wh_time,priority:1" json:"warehouseId,omitempty"`
	Action      string `gorm:"size:32;not null;index:idx_audit_action" json:"action"`
	Resource    string `gorm:"size:64;not null;index:idx_audit_res,priority:1" json:"resource"`
	ResourceID  *uint  `gorm:"index:idx_audit_res,priority:2" json:"resourceId,omitempty"`
	DocNo       string `gorm:"size:64;not null;default:'';index:idx_audit_doc_no" json:"docNo,omitempty"`
	Description string `gorm:"size:255;not null;default:''" json:"description"`
	Details     string `gorm:"type:jsonb;not null;default:'{}'" json:"details"`
	IPAddress   string `gorm:"size:45;not null;default:''" json:"ipAddress,omitempty"`
	UserAgent   string `gorm:"size:255;not null;default:''" json:"userAgent,omitempty"`
	Result      string `gorm:"size:16;not null;default:'success';index:idx_audit_user_time,priority:2" json:"result"`
	ErrorCode   int    `gorm:"not null;default:0" json:"errorCode,omitempty"`
	ErrorMsg    string `gorm:"size:255;not null;default:''" json:"errorMsg,omitempty"`
}

// TableName overrides the default GORM table name.
func (AuditLog) TableName() string { return "audit_logs" }

// IsValidAction reports whether the given string matches a known Action const.
// Used by the service layer to reject unknown values in list filters.
func IsValidAction(a string) bool {
	switch a {
	case ActionLogin, ActionLogout, ActionCreate, ActionUpdate, ActionDelete,
		ActionComplete, ActionConfirm, ActionSettle, ActionConvert,
		ActionBind, ActionUnbind, ActionAssign:
		return true
	}
	return false
}

// IsValidResource reports whether the given string matches a known Resource const.
func IsValidResource(r string) bool {
	switch r {
	case ResourceUser, ResourceRole, ResourcePermission,
		ResourceWarehouse, ResourceUserWarehouse,
		ResourceProduct, ResourceInventory, ResourceNonStdInventory,
		ResourceDocument, ResourceFile:
		return true
	}
	return false
}

// IsValidResult reports whether the given string matches a known Result const.
func IsValidResult(r string) bool {
	return r == ResultSuccess || r == ResultFailure
}
