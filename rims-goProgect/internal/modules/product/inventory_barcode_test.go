// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package product

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"rims-go/internal/types"
)

func TestGetInventoryByBarcodeReturnsOnlyCurrentWarehouseInventory(t *testing.T) {
	product := barcodeTestProduct(10, 1)
	inventory := barcodeTestInventory(20, 88, 10, 8, 2, 1)
	inventoryRepo := &barcodeInventoryRepo{inventory: inventory}
	service := NewProductService(
		&barcodeProductRepo{product: product},
		inventoryRepo,
		nil,
		nil,
	)

	got, err := service.GetInventoryByBarcode(context.Background(), 88, " CODE-1 ")
	if err != nil {
		t.Fatalf("GetInventoryByBarcode() error = %v", err)
	}
	if inventoryRepo.warehouseID != 88 || inventoryRepo.productID != 10 {
		t.Fatalf("inventory lookup = warehouse %d product %d, want 88/10", inventoryRepo.warehouseID, inventoryRepo.productID)
	}
	if got.WarehouseID != 88 || got.Quantity != 8 || got.LockedQty != 2 {
		t.Fatalf("response = %+v, want current warehouse inventory", got)
	}
	if got.Product == nil || got.Product.Barcode != "CODE-1" {
		t.Fatalf("response product = %+v, want active barcode product", got.Product)
	}
}

func TestGetInventoryByBarcodeRejectsUnavailableProductOrInventory(t *testing.T) {
	tests := []struct {
		name               string
		product            *Product
		productErr         error
		inventory          *Inventory
		inventoryErr       error
		wantCode           int
		wantInventoryCalls int
	}{
		{
			name:       "unknown barcode",
			productErr: gorm.ErrRecordNotFound,
			wantCode:   types.ErrCodeNotFound,
		},
		{
			name:     "disabled product",
			product:  barcodeTestProduct(10, 0),
			wantCode: types.ErrCodeInvalidState,
		},
		{
			name:               "absent from current warehouse",
			product:            barcodeTestProduct(10, 1),
			inventoryErr:       gorm.ErrRecordNotFound,
			wantCode:           types.ErrCodeInvalidState,
			wantInventoryCalls: 1,
		},
		{
			name:               "disabled current warehouse inventory",
			product:            barcodeTestProduct(10, 1),
			inventory:          barcodeTestInventory(20, 88, 10, 8, 0, 0),
			wantCode:           types.ErrCodeInvalidState,
			wantInventoryCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inventoryRepo := &barcodeInventoryRepo{
				inventory: tt.inventory,
				err:       tt.inventoryErr,
			}
			service := NewProductService(
				&barcodeProductRepo{product: tt.product, err: tt.productErr},
				inventoryRepo,
				nil,
				nil,
			)

			_, err := service.GetInventoryByBarcode(context.Background(), 88, "CODE-1")
			var appErr *types.AppError
			if !errors.As(err, &appErr) || appErr.Code != tt.wantCode {
				t.Fatalf("error = %v, want app code %d", err, tt.wantCode)
			}
			if inventoryRepo.calls != tt.wantInventoryCalls {
				t.Fatalf("inventory calls = %d, want %d", inventoryRepo.calls, tt.wantInventoryCalls)
			}
		})
	}
}

func TestInventoryBarcodeRouteUsesWarehouseScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	product := barcodeTestProduct(10, 1)
	inventoryRepo := &barcodeInventoryRepo{
		inventory: barcodeTestInventory(20, 88, 10, 8, 2, 1),
	}
	handler := NewHandler(NewProductService(
		&barcodeProductRepo{product: product},
		inventoryRepo,
		nil,
		nil,
	))
	router := gin.New()
	api := router.Group("/api/v1")
	RegisterRoutes(
		api,
		handler,
		func(c *gin.Context) { c.Set(types.CtxKeyUserID, uint(42)); c.Next() },
		func(c *gin.Context) { c.Set(types.CtxKeyWarehouseID, uint(88)); c.Next() },
		func(c *gin.Context) { c.Next() },
		func(string) gin.HandlerFunc { return func(c *gin.Context) { c.Next() } },
	)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/inventory/barcode/CODE-1", nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	var envelope struct {
		Code int               `json:"code"`
		Data InventoryResponse `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Code != 0 || envelope.Data.WarehouseID != 88 || envelope.Data.Quantity != 8 {
		t.Fatalf("response = %+v, want warehouse-scoped inventory", envelope)
	}
}

type barcodeProductRepo struct {
	ProductRepository
	product *Product
	err     error
}

func (r *barcodeProductRepo) GetByBarcode(context.Context, string) (*Product, error) {
	return r.product, r.err
}

type barcodeInventoryRepo struct {
	InventoryRepository
	inventory   *Inventory
	err         error
	calls       int
	warehouseID uint
	productID   uint
}

func (r *barcodeInventoryRepo) GetByWarehouseAndProduct(_ context.Context, warehouseID, productID uint) (*Inventory, error) {
	r.calls++
	r.warehouseID = warehouseID
	r.productID = productID
	return r.inventory, r.err
}

func barcodeTestProduct(id uint, status int8) *Product {
	product := &Product{
		Code:    "P-1",
		Name:    "测试商品",
		Unit:    "件",
		Barcode: "CODE-1",
		Status:  status,
	}
	product.ID = id
	return product
}

func barcodeTestInventory(id, warehouseID, productID uint, quantity, locked int, status int8) *Inventory {
	inventory := &Inventory{
		WarehouseID: warehouseID,
		ProductID:   productID,
		Quantity:    quantity,
		LockedQty:   locked,
		Status:      status,
	}
	inventory.ID = id
	return inventory
}
