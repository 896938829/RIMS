// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package product

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"rims-go/internal/db"
	"rims-go/internal/types"
)

// ProductService handles product, inventory, and non-standard inventory business logic.
type ProductService struct {
	productRepo   ProductRepository
	inventoryRepo InventoryRepository
	nonStdRepo    NonStdInventoryRepository
	txRunner      db.TxRunner
}

// NewProductService creates a new ProductService.
func NewProductService(
	productRepo ProductRepository,
	inventoryRepo InventoryRepository,
	nonStdRepo NonStdInventoryRepository,
	txRunner db.TxRunner,
) *ProductService {
	return &ProductService{
		productRepo:   productRepo,
		inventoryRepo: inventoryRepo,
		nonStdRepo:    nonStdRepo,
		txRunner:      txRunner,
	}
}

// --- Product CRUD ---

// Create creates a new product.
func (s *ProductService) Create(ctx context.Context, userID uint, req CreateProductRequest) (*AdminProductResponse, error) {
	code := strings.TrimSpace(req.Code)
	existing, err := s.productRepo.GetByCode(ctx, code)
	if err == nil && existing != nil {
		return nil, types.ErrDuplicate("商品编码已存在")
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, types.ErrSystem(err)
	}

	barcode := strings.TrimSpace(req.Barcode)
	if barcode != "" {
		existing, err := s.productRepo.GetByBarcode(ctx, barcode)
		if err == nil && existing != nil {
			return nil, types.ErrDuplicate("条码已存在")
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, types.ErrSystem(err)
		}
	}

	status := int8(1)
	if req.Status != nil {
		status = *req.Status
	}

	p := &Product{
		Code:        code,
		Name:        strings.TrimSpace(req.Name),
		Category:    strings.TrimSpace(req.Category),
		Spec:        strings.TrimSpace(req.Spec),
		Unit:        strings.TrimSpace(req.Unit),
		Barcode:     barcode,
		RetailPrice: req.RetailPrice,
		CostPrice:   req.CostPrice,
		ImageURL:    strings.TrimSpace(req.ImageURL),
		Status:      status,
	}
	p.CreatedBy = userID
	p.UpdatedBy = userID

	if err := s.productRepo.Create(ctx, p); err != nil {
		return nil, types.ErrSystem(err)
	}

	resp := ToAdminProductResponse(p)
	return &resp, nil
}

// GetByID retrieves a product by ID.
func (s *ProductService) GetByID(ctx context.Context, id uint) (*Product, error) {
	p, err := s.productRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, types.ErrNotFound("商品")
		}
		return nil, types.ErrSystem(err)
	}
	return p, nil
}

// GetByBarcode retrieves a product by barcode.
func (s *ProductService) GetByBarcode(ctx context.Context, barcode string) (*Product, error) {
	p, err := s.productRepo.GetByBarcode(ctx, barcode)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, types.ErrNotFound("商品")
		}
		return nil, types.ErrSystem(err)
	}
	return p, nil
}

// List returns a paginated list of products.
func (s *ProductService) List(ctx context.Context, page types.PageRequest, isAdmin bool) (types.PageResult, error) {
	products, total, err := s.productRepo.List(ctx, page)
	if err != nil {
		return types.PageResult{}, types.ErrSystem(err)
	}

	if isAdmin {
		items := make([]AdminProductResponse, len(products))
		for i := range products {
			items[i] = ToAdminProductResponse(&products[i])
		}
		return types.NewPageResult(page, items, total), nil
	}

	items := make([]ProductResponse, len(products))
	for i := range products {
		items[i] = ToProductResponse(&products[i])
	}
	return types.NewPageResult(page, items, total), nil
}

// Update modifies an existing product.
func (s *ProductService) Update(ctx context.Context, userID, id uint, req UpdateProductRequest) (*AdminProductResponse, error) {
	p, err := s.productRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, types.ErrNotFound("商品")
		}
		return nil, types.ErrSystem(err)
	}

	if req.Barcode != nil {
		newBarcode := strings.TrimSpace(*req.Barcode)
		if newBarcode != "" && newBarcode != p.Barcode {
			existing, err := s.productRepo.GetByBarcode(ctx, newBarcode)
			if err == nil && existing != nil {
				return nil, types.ErrDuplicate("条码已存在")
			}
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, types.ErrSystem(err)
			}
		}
		p.Barcode = newBarcode
	}

	if req.Name != nil {
		p.Name = strings.TrimSpace(*req.Name)
	}
	if req.Category != nil {
		p.Category = strings.TrimSpace(*req.Category)
	}
	if req.Spec != nil {
		p.Spec = strings.TrimSpace(*req.Spec)
	}
	if req.Unit != nil {
		p.Unit = strings.TrimSpace(*req.Unit)
	}
	if req.RetailPrice != nil {
		p.RetailPrice = *req.RetailPrice
	}
	if req.CostPrice != nil {
		p.CostPrice = *req.CostPrice
	}
	if req.ImageURL != nil {
		p.ImageURL = strings.TrimSpace(*req.ImageURL)
	}
	if req.Status != nil {
		p.Status = *req.Status
	}
	p.UpdatedBy = userID

	if err := s.productRepo.Update(ctx, p); err != nil {
		return nil, types.ErrSystem(err)
	}

	resp := ToAdminProductResponse(p)
	return &resp, nil
}

// Delete soft-deletes a product. Refuses if inventory records exist.
func (s *ProductService) Delete(ctx context.Context, id uint) error {
	if _, err := s.productRepo.GetByID(ctx, id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return types.ErrNotFound("商品")
		}
		return types.ErrSystem(err)
	}

	exists, err := s.inventoryRepo.ExistsByProductID(ctx, id)
	if err != nil {
		return types.ErrSystem(err)
	}
	if exists {
		return types.ErrInvalidState("该商品存在库存记录，无法删除")
	}

	documentLineCount, err := s.productRepo.CountDocumentLinesByProductID(ctx, id)
	if err != nil {
		return types.ErrSystem(err)
	}
	if documentLineCount > 0 {
		return types.ErrInvalidState("该商品存在单据记录，无法删除")
	}

	if err := s.productRepo.Delete(ctx, id); err != nil {
		return types.ErrSystem(err)
	}
	return nil
}

// --- Standard Inventory ---

// ListInventory returns a paginated list of inventory for a warehouse.
func (s *ProductService) ListInventory(ctx context.Context, warehouseID uint, page types.PageRequest) (types.PageResult, error) {
	inventories, total, err := s.inventoryRepo.ListByWarehouse(ctx, warehouseID, page)
	if err != nil {
		return types.PageResult{}, types.ErrSystem(err)
	}
	items := make([]InventoryResponse, len(inventories))
	for i := range inventories {
		items[i] = ToInventoryResponse(&inventories[i])
	}
	return types.NewPageResult(page, items, total), nil
}

// GetInventory retrieves an inventory record by ID, validating warehouse ownership.
func (s *ProductService) GetInventory(ctx context.Context, warehouseID, id uint) (*InventoryResponse, error) {
	inv, err := s.inventoryRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, types.ErrNotFound("库存记录")
		}
		return nil, types.ErrSystem(err)
	}
	if inv.WarehouseID != warehouseID {
		return nil, types.ErrNotFound("库存记录")
	}
	resp := ToInventoryResponse(inv)
	return &resp, nil
}

// UpdateInventory updates inventory settings (alert threshold and status only).
func (s *ProductService) UpdateInventory(ctx context.Context, userID, warehouseID, id uint, req UpdateInventoryRequest) (*InventoryResponse, error) {
	inv, err := s.inventoryRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, types.ErrNotFound("库存记录")
		}
		return nil, types.ErrSystem(err)
	}
	if inv.WarehouseID != warehouseID {
		return nil, types.ErrNotFound("库存记录")
	}

	if err := s.inventoryRepo.UpdateSettings(ctx, inv.ID, req.AlertThreshold, req.Status, userID); err != nil {
		return nil, types.ErrSystem(err)
	}

	inv, err = s.inventoryRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, types.ErrNotFound("库存记录")
		}
		return nil, types.ErrSystem(err)
	}
	if inv.WarehouseID != warehouseID {
		return nil, types.ErrNotFound("库存记录")
	}

	resp := ToInventoryResponse(inv)
	return &resp, nil
}

// ListAlerts returns inventory records at or below their alert threshold.
func (s *ProductService) ListAlerts(ctx context.Context, warehouseID uint, page types.PageRequest) (types.PageResult, error) {
	inventories, total, err := s.inventoryRepo.ListAlerts(ctx, warehouseID, page)
	if err != nil {
		return types.PageResult{}, types.ErrSystem(err)
	}
	items := make([]InventoryAlertResponse, len(inventories))
	for i := range inventories {
		items[i] = ToInventoryAlertResponse(&inventories[i])
	}
	return types.NewPageResult(page, items, total), nil
}

// --- Non-Standard Inventory ---

// CreateNonStd creates a new non-standard inventory item.
func (s *ProductService) CreateNonStd(ctx context.Context, userID, warehouseID uint, req CreateNonStdInventoryRequest) (*NonStdInventoryResponse, error) {
	tempLabel := strings.TrimSpace(req.TempLabel)
	existing, err := s.nonStdRepo.GetByTempLabel(ctx, tempLabel)
	if err == nil && existing != nil {
		return nil, types.ErrDuplicate("临时标签号已存在")
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, types.ErrSystem(err)
	}

	ns := &NonStdInventory{
		WarehouseID:    warehouseID,
		TempLabel:      tempLabel,
		Description:    strings.TrimSpace(req.Description),
		Unit:           strings.TrimSpace(req.Unit),
		Quantity:       req.Quantity,
		ConvertedQty:   0,
		SourceMethod:   strings.TrimSpace(req.SourceMethod),
		SourceDocument: strings.TrimSpace(req.SourceDocument),
		Status:         1,
	}
	ns.CreatedBy = userID
	ns.UpdatedBy = userID

	if err := s.nonStdRepo.Create(ctx, ns); err != nil {
		return nil, types.ErrSystem(err)
	}

	resp := ToNonStdInventoryResponse(ns)
	return &resp, nil
}

// GetNonStd retrieves a non-standard inventory item by ID, validating warehouse ownership.
func (s *ProductService) GetNonStd(ctx context.Context, warehouseID, id uint) (*NonStdInventoryResponse, error) {
	ns, err := s.nonStdRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, types.ErrNotFound("非标库存")
		}
		return nil, types.ErrSystem(err)
	}
	if ns.WarehouseID != warehouseID {
		return nil, types.ErrNotFound("非标库存")
	}
	resp := ToNonStdInventoryResponse(ns)
	return &resp, nil
}

// ListNonStd returns a paginated list of non-standard inventory for a warehouse.
func (s *ProductService) ListNonStd(ctx context.Context, warehouseID uint, page types.PageRequest) (types.PageResult, error) {
	list, total, err := s.nonStdRepo.ListByWarehouse(ctx, warehouseID, page)
	if err != nil {
		return types.PageResult{}, types.ErrSystem(err)
	}
	items := make([]NonStdInventoryResponse, len(list))
	for i := range list {
		items[i] = ToNonStdInventoryResponse(&list[i])
	}
	return types.NewPageResult(page, items, total), nil
}

// UpdateNonStd modifies a non-standard inventory item.
func (s *ProductService) UpdateNonStd(ctx context.Context, userID, warehouseID, id uint, req UpdateNonStdInventoryRequest) (*NonStdInventoryResponse, error) {
	ns, err := s.nonStdRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, types.ErrNotFound("非标库存")
		}
		return nil, types.ErrSystem(err)
	}
	if ns.WarehouseID != warehouseID {
		return nil, types.ErrNotFound("非标库存")
	}

	if req.Quantity != nil {
		if *req.Quantity < ns.ConvertedQty {
			return nil, types.ErrValidation(fmt.Sprintf("数量不能小于已转换量(%d)", ns.ConvertedQty))
		}
		ns.Quantity = *req.Quantity
	}
	if req.Description != nil {
		ns.Description = strings.TrimSpace(*req.Description)
	}
	if req.SourceMethod != nil {
		ns.SourceMethod = strings.TrimSpace(*req.SourceMethod)
	}
	if req.SourceDocument != nil {
		ns.SourceDocument = strings.TrimSpace(*req.SourceDocument)
	}
	if req.Status != nil {
		ns.Status = *req.Status
	}
	ns.UpdatedBy = userID

	if err := s.nonStdRepo.Update(ctx, ns); err != nil {
		return nil, types.ErrSystem(err)
	}

	resp := ToNonStdInventoryResponse(ns)
	return &resp, nil
}

// DeleteNonStd soft-deletes a non-standard inventory item. Refuses if conversions have occurred.
func (s *ProductService) DeleteNonStd(ctx context.Context, warehouseID, id uint) error {
	ns, err := s.nonStdRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return types.ErrNotFound("非标库存")
		}
		return types.ErrSystem(err)
	}
	if ns.WarehouseID != warehouseID {
		return types.ErrNotFound("非标库存")
	}
	if ns.ConvertedQty > 0 {
		return types.ErrInvalidState("已有转换记录，无法删除")
	}

	if err := s.nonStdRepo.Delete(ctx, id); err != nil {
		return types.ErrSystem(err)
	}
	return nil
}

// ConvertNonStd converts a portion of non-standard inventory to standard inventory.
func (s *ProductService) ConvertNonStd(ctx context.Context, userID, warehouseID, nonStdID uint, req ConvertNonStdRequest) error {
	return s.txRunner(ctx, func(txCtx context.Context) error {
		// 1. Get and validate non-standard inventory
		ns, err := s.nonStdRepo.GetByIDForUpdate(txCtx, nonStdID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return types.ErrNotFound("非标库存")
			}
			return types.ErrSystem(err)
		}
		if ns.WarehouseID != warehouseID {
			return types.ErrNotFound("非标库存")
		}
		if ns.Status == 0 || ns.Status == 3 {
			return types.ErrInvalidState("非标库存状态不允许转换")
		}

		remaining := ns.Quantity - ns.ConvertedQty
		if req.Quantity > remaining {
			return types.ErrValidation(fmt.Sprintf("转换数量(%d)超过剩余量(%d)", req.Quantity, remaining))
		}

		// 2. Validate target product
		p, err := s.productRepo.GetByID(txCtx, req.ProductID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return types.ErrNotFound("目标商品")
			}
			return types.ErrSystem(err)
		}
		if p.Status != 1 {
			return types.ErrInvalidState("目标商品已停用")
		}

		// 3. Get or create inventory record
		// Advisory lock serializes the missing-row get-or-create key; FOR UPDATE locks the existing row.
		if err := s.inventoryRepo.LockItem(txCtx, warehouseID, req.ProductID); err != nil {
			return types.ErrSystem(err)
		}
		inv, err := s.inventoryRepo.GetByWarehouseAndProductForUpdate(txCtx, warehouseID, req.ProductID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				inv = &Inventory{
					WarehouseID: warehouseID,
					ProductID:   req.ProductID,
					Quantity:    0,
					Status:      1,
				}
				inv.CreatedBy = userID
				inv.UpdatedBy = userID
				if err := s.inventoryRepo.Create(txCtx, inv); err != nil {
					return types.ErrSystem(err)
				}
			} else {
				return types.ErrSystem(err)
			}
		}

		// 4. Increase standard inventory
		inv.Quantity += req.Quantity
		inv.UpdatedBy = userID
		if inv.AlertThreshold > 0 && inv.Quantity <= inv.AlertThreshold {
			inv.Status = 2
		} else if inv.Status == 2 && inv.Quantity > inv.AlertThreshold {
			inv.Status = 1
		}
		if err := s.inventoryRepo.Update(txCtx, inv); err != nil {
			return types.ErrSystem(err)
		}

		// 5. Update non-standard inventory
		ns.ConvertedQty += req.Quantity
		ns.UpdatedBy = userID
		if ns.ConvertedQty == ns.Quantity {
			ns.Status = 3 // fully converted
		} else {
			ns.Status = 2 // partially converted
		}
		if err := s.nonStdRepo.Update(txCtx, ns); err != nil {
			return types.ErrSystem(err)
		}

		return nil
	})
}
