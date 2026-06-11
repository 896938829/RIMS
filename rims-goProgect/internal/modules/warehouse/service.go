// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package warehouse

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"

	"rims-go/internal/db"
	"rims-go/internal/types"
)

// WarehouseService handles warehouse-related business logic.
type WarehouseService struct {
	warehouseRepo     WarehouseRepository
	userWarehouseRepo UserWarehouseRepository
	txRunner          db.TxRunner
}

// NewWarehouseService creates a new WarehouseService.
func NewWarehouseService(
	warehouseRepo WarehouseRepository,
	userWarehouseRepo UserWarehouseRepository,
	txRunner db.TxRunner,
) *WarehouseService {
	return &WarehouseService{
		warehouseRepo:     warehouseRepo,
		userWarehouseRepo: userWarehouseRepo,
		txRunner:          txRunner,
	}
}

// Create creates a new warehouse.
func (s *WarehouseService) Create(ctx context.Context, userID uint, req CreateWarehouseRequest) (*WarehouseResponse, error) {
	code := strings.TrimSpace(req.Code)
	existing, err := s.warehouseRepo.GetByCode(ctx, code)
	if err == nil && existing != nil {
		return nil, types.ErrDuplicate("仓库编码已存在")
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, types.ErrSystem(err)
	}

	status := int8(1)
	if req.Status != nil {
		status = *req.Status
	}

	w := &Warehouse{
		Code:          code,
		Name:          strings.TrimSpace(req.Name),
		Status:        status,
		Address:       strings.TrimSpace(req.Address),
		ContactPerson: strings.TrimSpace(req.ContactPerson),
		ContactPhone:  strings.TrimSpace(req.ContactPhone),
	}
	w.CreatedBy = userID
	w.UpdatedBy = userID

	if err := s.warehouseRepo.Create(ctx, w); err != nil {
		return nil, types.ErrSystem(err)
	}

	resp := ToWarehouseResponse(w)
	return &resp, nil
}

// GetByID retrieves a warehouse by ID within the actor's access scope.
func (s *WarehouseService) GetByID(ctx context.Context, userID uint, roleCode string, id uint) (*WarehouseResponse, error) {
	if roleCode != "admin" {
		ok, err := s.userWarehouseRepo.HasAccess(ctx, userID, id)
		if err != nil {
			return nil, types.ErrSystem(err)
		}
		if !ok {
			return nil, types.ErrNotFound("仓库")
		}
	}

	w, err := s.warehouseRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, types.ErrNotFound("仓库")
		}
		return nil, types.ErrSystem(err)
	}
	resp := ToWarehouseResponse(w)
	return &resp, nil
}

// List returns a paginated list of warehouses, filtered by user's access scope.
func (s *WarehouseService) List(ctx context.Context, userID uint, roleCode string, page types.PageRequest) (types.PageResult, error) {
	if roleCode == "admin" {
		warehouses, total, err := s.warehouseRepo.List(ctx, page)
		if err != nil {
			return types.PageResult{}, types.ErrSystem(err)
		}
		items := make([]WarehouseResponse, len(warehouses))
		for i := range warehouses {
			items[i] = ToWarehouseResponse(&warehouses[i])
		}
		return types.NewPageResult(page, items, total), nil
	}

	// Non-admin: only show bound warehouses
	bindings, err := s.userWarehouseRepo.ListByUserID(ctx, userID)
	if err != nil {
		return types.PageResult{}, types.ErrSystem(err)
	}

	if len(bindings) == 0 {
		return types.NewPageResult(page, []WarehouseResponse{}, 0), nil
	}

	ids := make([]uint, len(bindings))
	for i, b := range bindings {
		ids[i] = b.WarehouseID
	}

	warehouses, total, err := s.warehouseRepo.ListByIDs(ctx, ids)
	if err != nil {
		return types.PageResult{}, types.ErrSystem(err)
	}

	items := make([]WarehouseResponse, len(warehouses))
	for i := range warehouses {
		items[i] = ToWarehouseResponse(&warehouses[i])
	}
	return types.NewPageResult(page, items, total), nil
}

// Update modifies an existing warehouse.
func (s *WarehouseService) Update(ctx context.Context, userID uint, id uint, req UpdateWarehouseRequest) (*WarehouseResponse, error) {
	w, err := s.warehouseRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, types.ErrNotFound("仓库")
		}
		return nil, types.ErrSystem(err)
	}

	if req.Name != nil {
		w.Name = strings.TrimSpace(*req.Name)
	}
	if req.Status != nil {
		w.Status = *req.Status
	}
	if req.Address != nil {
		w.Address = strings.TrimSpace(*req.Address)
	}
	if req.ContactPerson != nil {
		w.ContactPerson = strings.TrimSpace(*req.ContactPerson)
	}
	if req.ContactPhone != nil {
		w.ContactPhone = strings.TrimSpace(*req.ContactPhone)
	}
	w.UpdatedBy = userID

	if err := s.warehouseRepo.Update(ctx, w); err != nil {
		return nil, types.ErrSystem(err)
	}

	resp := ToWarehouseResponse(w)
	return &resp, nil
}

// Delete soft-deletes a warehouse and all its user bindings.
func (s *WarehouseService) Delete(ctx context.Context, id uint) error {
	if _, err := s.warehouseRepo.GetByID(ctx, id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return types.ErrNotFound("仓库")
		}
		return types.ErrSystem(err)
	}

	return s.txRunner(ctx, func(txCtx context.Context) error {
		if err := s.userWarehouseRepo.DeleteByWarehouseID(txCtx, id); err != nil {
			return types.ErrSystem(err)
		}
		if err := s.warehouseRepo.Delete(txCtx, id); err != nil {
			return types.ErrSystem(err)
		}
		return nil
	})
}

// BindUsers binds a list of users to a warehouse.
func (s *WarehouseService) BindUsers(ctx context.Context, warehouseID uint, req BindUsersRequest) error {
	w, err := s.warehouseRepo.GetByID(ctx, warehouseID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return types.ErrNotFound("仓库")
		}
		return types.ErrSystem(err)
	}
	if w.Status != 1 {
		return types.ErrInvalidState("仓库已禁用")
	}

	return s.txRunner(ctx, func(txCtx context.Context) error {
		for _, uid := range req.UserIDs {
			// Skip if already bound
			existing, err := s.userWarehouseRepo.GetByUserAndWarehouse(txCtx, uid, warehouseID)
			if err == nil && existing != nil {
				continue
			}
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return types.ErrSystem(err)
			}

			// Check single-warehouse constraint for non-admin users
			roleCode, err := s.userWarehouseRepo.GetUserRoleCode(txCtx, uid)
			if err != nil {
				return types.ErrSystem(err)
			}
			var count int64
			countLoaded := false
			if roleCode != "admin" {
				count, err = s.userWarehouseRepo.CountByUserID(txCtx, uid)
				if err != nil {
					return types.ErrSystem(err)
				}
				countLoaded = true
				if count > 0 {
					return types.ErrValidation("普通用户只能绑定一个仓库")
				}
			}

			// Auto-set as default if first binding
			if !countLoaded {
				count, err = s.userWarehouseRepo.CountByUserID(txCtx, uid)
				if err != nil {
					return types.ErrSystem(err)
				}
			}

			uw := &UserWarehouse{
				UserID:      uid,
				WarehouseID: warehouseID,
				IsDefault:   count == 0,
			}
			if err := s.userWarehouseRepo.Create(txCtx, uw); err != nil {
				return types.ErrSystem(err)
			}
		}
		return nil
	})
}

// UnbindUser removes a user's binding from a warehouse.
func (s *WarehouseService) UnbindUser(ctx context.Context, warehouseID, userID uint) error {
	binding, err := s.userWarehouseRepo.GetByUserAndWarehouse(ctx, userID, warehouseID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return types.ErrNotFound("用户仓库绑定")
		}
		return types.ErrSystem(err)
	}

	wasDefault := binding.IsDefault

	return s.txRunner(ctx, func(txCtx context.Context) error {
		if err := s.userWarehouseRepo.Delete(txCtx, userID, warehouseID); err != nil {
			return types.ErrSystem(err)
		}

		// If the deleted binding was the default, assign a new default
		if wasDefault {
			remaining, err := s.userWarehouseRepo.ListByUserID(txCtx, userID)
			if err != nil {
				return types.ErrSystem(err)
			}
			if len(remaining) > 0 {
				if err := s.userWarehouseRepo.SetDefault(txCtx, userID, remaining[0].WarehouseID); err != nil {
					return types.ErrSystem(err)
				}
			}
		}
		return nil
	})
}

// ListWarehouseUsers returns a paginated list of users bound to a warehouse.
func (s *WarehouseService) ListWarehouseUsers(ctx context.Context, warehouseID uint, page types.PageRequest) (types.PageResult, error) {
	if _, err := s.warehouseRepo.GetByID(ctx, warehouseID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return types.PageResult{}, types.ErrNotFound("仓库")
		}
		return types.PageResult{}, types.ErrSystem(err)
	}

	users, total, err := s.userWarehouseRepo.ListByWarehouseID(ctx, warehouseID, page)
	if err != nil {
		return types.PageResult{}, types.ErrSystem(err)
	}
	return types.NewPageResult(page, users, total), nil
}

// GetMyWarehouses returns the current user's warehouse bindings.
func (s *WarehouseService) GetMyWarehouses(ctx context.Context, userID uint) ([]UserWarehouseResponse, error) {
	bindings, err := s.userWarehouseRepo.ListByUserID(ctx, userID)
	if err != nil {
		return nil, types.ErrSystem(err)
	}

	result := make([]UserWarehouseResponse, len(bindings))
	for i := range bindings {
		result[i] = ToUserWarehouseResponse(&bindings[i])
	}
	return result, nil
}

// SetDefaultWarehouse sets a warehouse as the user's default.
func (s *WarehouseService) SetDefaultWarehouse(ctx context.Context, userID uint, req SetDefaultWarehouseRequest) error {
	_, err := s.userWarehouseRepo.GetByUserAndWarehouse(ctx, userID, req.WarehouseID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return types.ErrForbidden()
		}
		return types.ErrSystem(err)
	}

	return s.txRunner(ctx, func(txCtx context.Context) error {
		if err := s.userWarehouseRepo.ClearDefault(txCtx, userID); err != nil {
			return types.ErrSystem(err)
		}
		if err := s.userWarehouseRepo.SetDefault(txCtx, userID, req.WarehouseID); err != nil {
			return types.ErrSystem(err)
		}
		return nil
	})
}

// SwitchCurrentWarehouse validates the user can switch to the given warehouse.
func (s *WarehouseService) SwitchCurrentWarehouse(ctx context.Context, userID uint, req SwitchWarehouseRequest) (*WarehouseResponse, error) {
	binding, err := s.userWarehouseRepo.GetByUserAndWarehouse(ctx, userID, req.WarehouseID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, types.ErrForbidden()
		}
		return nil, types.ErrSystem(err)
	}

	if binding.Warehouse != nil && binding.Warehouse.Status != 1 {
		return nil, types.ErrInvalidState("仓库已禁用")
	}

	w, err := s.warehouseRepo.GetByID(ctx, req.WarehouseID)
	if err != nil {
		return nil, types.ErrSystem(err)
	}

	resp := ToWarehouseResponse(w)
	return &resp, nil
}
