// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package audit

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"

	"rims-go/internal/types"
)

// maxRangeDays bounds the [start, end) window a list query is allowed to
// cover. Same rule as the report module; keeps scans predictable.
const maxRangeDays = 366

// AuditService is the composition-root-wired write & query surface for the
// audit module. All business writes that need atomic audit guarantees must
// call Log from within an active db.RunInTx callback; Log itself does not
// create or manage transactions.
type AuditService struct {
	repo AuditRepository
}

// NewAuditService returns a ready-to-use AuditService.
func NewAuditService(repo AuditRepository) *AuditService {
	return &AuditService{repo: repo}
}

// Log persists one audit entry. The caller is responsible for running Log
// inside any active business transaction via db.RunInTx so that audit row
// commit/rollback stays atomic with the source write (requirements §5.4 /
// §8.2). For non-transactional flows (e.g. login) best-effort is acceptable
// and callers may ignore the error.
func (s *AuditService) Log(ctx context.Context, e Entry) error {
	if e.Action == "" || e.Resource == "" {
		return types.ErrValidation("audit: action and resource are required")
	}
	result := e.Result
	if result == "" {
		result = ResultSuccess
	}

	detailsJSON, err := marshalDetails(e.Before, e.After)
	if err != nil {
		return types.ErrSystem(err)
	}

	var whID *uint
	if e.Actor.WarehouseID != 0 {
		v := e.Actor.WarehouseID
		whID = &v
	}

	row := &AuditLog{
		TraceID:     e.Actor.TraceID,
		UserID:      e.Actor.UserID,
		Username:    e.Actor.Username,
		RoleCode:    e.Actor.RoleCode,
		WarehouseID: whID,
		Action:      e.Action,
		Resource:    e.Resource,
		ResourceID:  e.ResourceID,
		DocNo:       e.DocNo,
		Description: e.Description,
		Details:     detailsJSON,
		IPAddress:   e.Actor.IPAddress,
		UserAgent:   e.Actor.UserAgent,
		Result:      result,
		ErrorCode:   e.ErrorCode,
		ErrorMsg:    e.ErrorMsg,
	}

	if err := s.repo.Create(ctx, row); err != nil {
		return types.ErrSystem(err)
	}
	return nil
}

// List returns paginated audit logs matching the filter. Time range is
// validated before touching the repo: empty range is allowed (no bound),
// but an explicit range wider than maxRangeDays is rejected.
func (s *AuditService) List(ctx context.Context, req ListRequest) (types.PageResult, error) {
	filter, err := buildListFilter(req)
	if err != nil {
		return types.PageResult{}, err
	}

	page := types.PageRequest{Page: req.Page, PageSize: req.PageSize}
	rows, total, err := s.repo.List(ctx, filter, page)
	if err != nil {
		return types.PageResult{}, types.ErrSystem(err)
	}

	items := make([]AuditLogResponse, len(rows))
	for i := range rows {
		items[i] = ToAuditLogResponse(&rows[i])
	}
	return types.NewPageResult(page, items, total), nil
}

// Get returns a single audit log by ID.
func (s *AuditService) Get(ctx context.Context, id uint) (*AuditLogResponse, error) {
	row, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, types.ErrNotFound("审计日志")
		}
		return nil, types.ErrSystem(err)
	}
	resp := ToAuditLogResponse(row)
	return &resp, nil
}

// buildListFilter validates ListRequest fields and produces a repo ListFilter.
func buildListFilter(req ListRequest) (ListFilter, error) {
	var f ListFilter

	if req.UserID > 0 {
		v := req.UserID
		f.UserID = &v
	}
	if req.WarehouseID > 0 {
		v := req.WarehouseID
		f.WarehouseID = &v
	}
	if req.Resource != "" {
		if !IsValidResource(req.Resource) {
			return f, types.ErrValidation("invalid resource")
		}
		f.Resource = req.Resource
	}
	if req.ResourceID > 0 {
		v := req.ResourceID
		f.ResourceID = &v
	}
	if req.Action != "" {
		if !IsValidAction(req.Action) {
			return f, types.ErrValidation("invalid action")
		}
		f.Action = req.Action
	}
	if req.Result != "" {
		if !IsValidResult(req.Result) {
			return f, types.ErrValidation("invalid result")
		}
		f.Result = req.Result
	}
	f.DocNo = req.DocNo
	f.Keyword = req.Keyword

	start, end, err := parseTimeRange(req.StartTime, req.EndTime)
	if err != nil {
		return f, err
	}
	f.StartTime = start
	f.EndTime = end

	return f, nil
}

// parseTimeRange accepts either RFC3339 timestamps or plain YYYY-MM-DD dates.
// Returns nil pointers when the corresponding input is empty. Enforces the
// maxRangeDays window when both ends are supplied.
func parseTimeRange(startStr, endStr string) (*time.Time, *time.Time, error) {
	var start, end *time.Time

	if startStr != "" {
		t, err := parseFlexibleTime(startStr)
		if err != nil {
			return nil, nil, types.ErrValidation("invalid startTime: " + err.Error())
		}
		start = &t
	}
	if endStr != "" {
		t, err := parseFlexibleTime(endStr)
		if err != nil {
			return nil, nil, types.ErrValidation("invalid endTime: " + err.Error())
		}
		end = &t
	}

	if start != nil && end != nil {
		if end.Before(*start) {
			return nil, nil, types.ErrValidation("endTime must be after startTime")
		}
		if end.Sub(*start) > maxRangeDays*24*time.Hour {
			return nil, nil, types.ErrValidation("time range too large (max 366 days)")
		}
	}
	return start, end, nil
}

func parseFlexibleTime(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02", s)
}

// marshalDetails encodes before/after snapshots into the JSONB payload
// shape used in the details column: '{"before":{...},"after":{...}}'.
// Nil maps are omitted rather than stored as literal nulls.
func marshalDetails(before, after map[string]any) (string, error) {
	if before == nil && after == nil {
		return "{}", nil
	}
	payload := make(map[string]any, 2)
	if before != nil {
		payload["before"] = before
	}
	if after != nil {
		payload["after"] = after
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
