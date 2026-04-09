// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package warehouse

import "time"

// --- Warehouse CRUD DTOs ---

// CreateWarehouseRequest holds data for creating a new warehouse.
type CreateWarehouseRequest struct {
	Code          string `json:"code" binding:"required,min=2,max=32"`
	Name          string `json:"name" binding:"required,max=128"`
	Status        *int8  `json:"status" binding:"omitempty,oneof=0 1"`
	Address       string `json:"address" binding:"max=255"`
	ContactPerson string `json:"contactPerson" binding:"max=64"`
	ContactPhone  string `json:"contactPhone" binding:"max=20"`
}

// UpdateWarehouseRequest holds data for updating an existing warehouse.
type UpdateWarehouseRequest struct {
	Name          *string `json:"name" binding:"omitempty,max=128"`
	Status        *int8   `json:"status" binding:"omitempty,oneof=0 1"`
	Address       *string `json:"address" binding:"omitempty,max=255"`
	ContactPerson *string `json:"contactPerson" binding:"omitempty,max=64"`
	ContactPhone  *string `json:"contactPhone" binding:"omitempty,max=20"`
}

// WarehouseResponse is the warehouse representation in API responses.
type WarehouseResponse struct {
	ID            uint      `json:"id"`
	Code          string    `json:"code"`
	Name          string    `json:"name"`
	Status        int8      `json:"status"`
	Address       string    `json:"address"`
	ContactPerson string    `json:"contactPerson"`
	ContactPhone  string    `json:"contactPhone"`
	CreatedBy     uint      `json:"createdBy"`
	UpdatedBy     uint      `json:"updatedBy"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// ToWarehouseResponse converts a Warehouse model to a WarehouseResponse DTO.
func ToWarehouseResponse(w *Warehouse) WarehouseResponse {
	return WarehouseResponse{
		ID:            w.ID,
		Code:          w.Code,
		Name:          w.Name,
		Status:        w.Status,
		Address:       w.Address,
		ContactPerson: w.ContactPerson,
		ContactPhone:  w.ContactPhone,
		CreatedBy:     w.CreatedBy,
		UpdatedBy:     w.UpdatedBy,
		CreatedAt:     w.CreatedAt,
		UpdatedAt:     w.UpdatedAt,
	}
}

// --- User-Warehouse Binding DTOs ---

// BindUsersRequest holds a list of user IDs to bind to a warehouse.
type BindUsersRequest struct {
	UserIDs []uint `json:"userIds" binding:"required,min=1"`
}

// SetDefaultWarehouseRequest holds the warehouse ID to set as default.
type SetDefaultWarehouseRequest struct {
	WarehouseID uint `json:"warehouseId" binding:"required"`
}

// SwitchWarehouseRequest holds the warehouse ID to switch to.
type SwitchWarehouseRequest struct {
	WarehouseID uint `json:"warehouseId" binding:"required"`
}

// UserWarehouseResponse represents a user's warehouse binding in API responses.
type UserWarehouseResponse struct {
	ID          uint               `json:"id"`
	WarehouseID uint               `json:"warehouseId"`
	IsDefault   bool               `json:"isDefault"`
	Warehouse   *WarehouseResponse `json:"warehouse,omitempty"`
}

// ToUserWarehouseResponse converts a UserWarehouse model to a UserWarehouseResponse DTO.
func ToUserWarehouseResponse(uw *UserWarehouse) UserWarehouseResponse {
	resp := UserWarehouseResponse{
		ID:          uw.ID,
		WarehouseID: uw.WarehouseID,
		IsDefault:   uw.IsDefault,
	}
	if uw.Warehouse != nil {
		whResp := ToWarehouseResponse(uw.Warehouse)
		resp.Warehouse = &whResp
	}
	return resp
}
