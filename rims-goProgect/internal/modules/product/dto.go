// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package product

import "time"

// --- Product DTOs ---

// CreateProductRequest holds data for creating a new product.
type CreateProductRequest struct {
	Code        string  `json:"code" binding:"required,min=2,max=32"`
	Name        string  `json:"name" binding:"required,max=128"`
	Category    string  `json:"category" binding:"max=64"`
	Spec        string  `json:"spec" binding:"max=128"`
	Unit        string  `json:"unit" binding:"required,max=16"`
	Barcode     string  `json:"barcode" binding:"max=64"`
	RetailPrice float64 `json:"retailPrice" binding:"omitempty,min=0"`
	CostPrice   float64 `json:"costPrice" binding:"omitempty,min=0"`
	ImageURL    string  `json:"imageUrl" binding:"max=512"`
	Status      *int8   `json:"status" binding:"omitempty,oneof=0 1"`
}

// UpdateProductRequest holds data for updating an existing product.
type UpdateProductRequest struct {
	Name        *string  `json:"name" binding:"omitempty,max=128"`
	Category    *string  `json:"category" binding:"omitempty,max=64"`
	Spec        *string  `json:"spec" binding:"omitempty,max=128"`
	Unit        *string  `json:"unit" binding:"omitempty,max=16"`
	Barcode     *string  `json:"barcode" binding:"omitempty,max=64"`
	RetailPrice *float64 `json:"retailPrice" binding:"omitempty,min=0"`
	CostPrice   *float64 `json:"costPrice" binding:"omitempty,min=0"`
	ImageURL    *string  `json:"imageUrl" binding:"omitempty,max=512"`
	Status      *int8    `json:"status" binding:"omitempty,oneof=0 1"`
}

// ProductResponse is the product representation for non-admin users (no cost price).
type ProductResponse struct {
	ID          uint      `json:"id"`
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	Category    string    `json:"category"`
	Spec        string    `json:"spec"`
	Unit        string    `json:"unit"`
	Barcode     string    `json:"barcode"`
	RetailPrice float64   `json:"retailPrice"`
	ImageURL    string    `json:"imageUrl"`
	Status      int8      `json:"status"`
	CreatedBy   uint      `json:"createdBy"`
	UpdatedBy   uint      `json:"updatedBy"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// AdminProductResponse extends ProductResponse with cost price for admin users.
type AdminProductResponse struct {
	ProductResponse
	CostPrice float64 `json:"costPrice"`
}

// ToProductResponse converts a Product model to a ProductResponse DTO.
func ToProductResponse(p *Product) ProductResponse {
	return ProductResponse{
		ID:          p.ID,
		Code:        p.Code,
		Name:        p.Name,
		Category:    p.Category,
		Spec:        p.Spec,
		Unit:        p.Unit,
		Barcode:     p.Barcode,
		RetailPrice: p.RetailPrice,
		ImageURL:    p.ImageURL,
		Status:      p.Status,
		CreatedBy:   p.CreatedBy,
		UpdatedBy:   p.UpdatedBy,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
	}
}

// ToAdminProductResponse converts a Product model to an AdminProductResponse DTO.
func ToAdminProductResponse(p *Product) AdminProductResponse {
	return AdminProductResponse{
		ProductResponse: ToProductResponse(p),
		CostPrice:       p.CostPrice,
	}
}

// --- Inventory DTOs ---

// UpdateInventoryRequest holds data for updating inventory settings.
// Quantity changes are handled by the document module, not directly.
type UpdateInventoryRequest struct {
	AlertThreshold *int  `json:"alertThreshold" binding:"omitempty,min=0"`
	Status         *int8 `json:"status" binding:"omitempty,oneof=0 1"`
}

// InventoryResponse is the inventory representation in API responses.
type InventoryResponse struct {
	ID             uint             `json:"id"`
	WarehouseID    uint             `json:"warehouseId"`
	ProductID      uint             `json:"productId"`
	Quantity       int              `json:"quantity"`
	LockedQty      int              `json:"lockedQty"`
	AlertThreshold int              `json:"alertThreshold"`
	Status         int8             `json:"status"`
	Product        *ProductResponse `json:"product,omitempty"`
	CreatedBy      uint             `json:"createdBy"`
	UpdatedBy      uint             `json:"updatedBy"`
	CreatedAt      time.Time        `json:"createdAt"`
	UpdatedAt      time.Time        `json:"updatedAt"`
}

// ToInventoryResponse converts an Inventory model to an InventoryResponse DTO.
func ToInventoryResponse(inv *Inventory) InventoryResponse {
	resp := InventoryResponse{
		ID:             inv.ID,
		WarehouseID:    inv.WarehouseID,
		ProductID:      inv.ProductID,
		Quantity:       inv.Quantity,
		LockedQty:      inv.LockedQty,
		AlertThreshold: inv.AlertThreshold,
		Status:         inv.Status,
		CreatedBy:      inv.CreatedBy,
		UpdatedBy:      inv.UpdatedBy,
		CreatedAt:      inv.CreatedAt,
		UpdatedAt:      inv.UpdatedAt,
	}
	if inv.Product != nil {
		pResp := ToProductResponse(inv.Product)
		resp.Product = &pResp
	}
	return resp
}

// InventoryAlertResponse is a lightweight inventory alert representation.
type InventoryAlertResponse struct {
	ID             uint   `json:"id"`
	WarehouseID    uint   `json:"warehouseId"`
	ProductID      uint   `json:"productId"`
	ProductCode    string `json:"productCode"`
	ProductName    string `json:"productName"`
	Quantity       int    `json:"quantity"`
	AlertThreshold int    `json:"alertThreshold"`
}

// ToInventoryAlertResponse converts an Inventory model to an InventoryAlertResponse DTO.
func ToInventoryAlertResponse(inv *Inventory) InventoryAlertResponse {
	resp := InventoryAlertResponse{
		ID:             inv.ID,
		WarehouseID:    inv.WarehouseID,
		ProductID:      inv.ProductID,
		Quantity:       inv.Quantity,
		AlertThreshold: inv.AlertThreshold,
	}
	if inv.Product != nil {
		resp.ProductCode = inv.Product.Code
		resp.ProductName = inv.Product.Name
	}
	return resp
}

// --- Non-Standard Inventory DTOs ---

// CreateNonStdInventoryRequest holds data for creating a non-standard inventory item.
type CreateNonStdInventoryRequest struct {
	TempLabel      string `json:"tempLabel" binding:"required,max=64"`
	Description    string `json:"description" binding:"required,max=255"`
	Unit           string `json:"unit" binding:"required,max=16"`
	Quantity       int    `json:"quantity" binding:"required,min=1"`
	SourceMethod   string `json:"sourceMethod" binding:"max=32"`
	SourceDocument string `json:"sourceDocument" binding:"max=64"`
}

// UpdateNonStdInventoryRequest holds data for updating a non-standard inventory item.
type UpdateNonStdInventoryRequest struct {
	Description    *string `json:"description" binding:"omitempty,max=255"`
	Quantity       *int    `json:"quantity" binding:"omitempty,min=1"`
	SourceMethod   *string `json:"sourceMethod" binding:"omitempty,max=32"`
	SourceDocument *string `json:"sourceDocument" binding:"omitempty,max=64"`
	Status         *int8   `json:"status" binding:"omitempty,oneof=0 1"`
}

// ConvertNonStdRequest holds data for converting non-standard inventory to standard.
type ConvertNonStdRequest struct {
	ProductID uint `json:"productId" binding:"required"`
	Quantity  int  `json:"quantity" binding:"required,min=1"`
}

// NonStdInventoryResponse is the non-standard inventory representation in API responses.
type NonStdInventoryResponse struct {
	ID             uint      `json:"id"`
	WarehouseID    uint      `json:"warehouseId"`
	TempLabel      string    `json:"tempLabel"`
	Description    string    `json:"description"`
	Unit           string    `json:"unit"`
	Quantity       int       `json:"quantity"`
	ConvertedQty   int       `json:"convertedQty"`
	RemainingQty   int       `json:"remainingQty"`
	SourceMethod   string    `json:"sourceMethod"`
	SourceDocument string    `json:"sourceDocument"`
	Status         int8      `json:"status"`
	CreatedBy      uint      `json:"createdBy"`
	UpdatedBy      uint      `json:"updatedBy"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

// ToNonStdInventoryResponse converts a NonStdInventory model to a NonStdInventoryResponse DTO.
func ToNonStdInventoryResponse(ns *NonStdInventory) NonStdInventoryResponse {
	return NonStdInventoryResponse{
		ID:             ns.ID,
		WarehouseID:    ns.WarehouseID,
		TempLabel:      ns.TempLabel,
		Description:    ns.Description,
		Unit:           ns.Unit,
		Quantity:       ns.Quantity,
		ConvertedQty:   ns.ConvertedQty,
		RemainingQty:   ns.Quantity - ns.ConvertedQty,
		SourceMethod:   ns.SourceMethod,
		SourceDocument: ns.SourceDocument,
		Status:         ns.Status,
		CreatedBy:      ns.CreatedBy,
		UpdatedBy:      ns.UpdatedBy,
		CreatedAt:      ns.CreatedAt,
		UpdatedAt:      ns.UpdatedAt,
	}
}
