// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package product

import (
	"context"
	"reflect"
	"testing"

	"gorm.io/gorm"

	"rims-go/internal/types"
)

type productRepoConcurrencyStub struct{}

func (productRepoConcurrencyStub) Create(ctx context.Context, p *Product) error {
	return nil
}

func (productRepoConcurrencyStub) GetByID(ctx context.Context, id uint) (*Product, error) {
	return &Product{Status: 1}, nil
}

func (productRepoConcurrencyStub) GetByCode(ctx context.Context, code string) (*Product, error) {
	return nil, gorm.ErrRecordNotFound
}

func (productRepoConcurrencyStub) GetByBarcode(ctx context.Context, barcode string) (*Product, error) {
	return nil, gorm.ErrRecordNotFound
}

func (productRepoConcurrencyStub) List(ctx context.Context, page types.PageRequest) ([]Product, int64, error) {
	return nil, 0, nil
}

func (productRepoConcurrencyStub) Update(ctx context.Context, p *Product) error {
	return nil
}

func (productRepoConcurrencyStub) Delete(ctx context.Context, id uint) error {
	return nil
}

type inventoryRepoConcurrencyStub struct {
	calls          []string
	inventory      *Inventory
	settingsUpdate *inventorySettingsUpdate
	savedInventory *Inventory
}

type inventorySettingsUpdate struct {
	id             uint
	alertThreshold *int
	status         *int8
	updatedBy      uint
}

func (r *inventoryRepoConcurrencyStub) Create(ctx context.Context, inv *Inventory) error {
	r.calls = append(r.calls, "create")
	r.inventory = inv
	return nil
}

func (r *inventoryRepoConcurrencyStub) GetByID(ctx context.Context, id uint) (*Inventory, error) {
	r.calls = append(r.calls, "get")
	return r.inventory, nil
}

func (r *inventoryRepoConcurrencyStub) LockItem(ctx context.Context, warehouseID, productID uint) error {
	r.calls = append(r.calls, "lock")
	return nil
}

func (r *inventoryRepoConcurrencyStub) GetByWarehouseAndProductForUpdate(ctx context.Context, warehouseID, productID uint) (*Inventory, error) {
	r.calls = append(r.calls, "get-for-update")
	if r.inventory == nil {
		return nil, gorm.ErrRecordNotFound
	}
	return r.inventory, nil
}

func (r *inventoryRepoConcurrencyStub) GetByWarehouseAndProduct(ctx context.Context, warehouseID, productID uint) (*Inventory, error) {
	r.calls = append(r.calls, "get")
	if r.inventory == nil {
		return nil, gorm.ErrRecordNotFound
	}
	return r.inventory, nil
}

func (r *inventoryRepoConcurrencyStub) ExistsByProductID(ctx context.Context, productID uint) (bool, error) {
	return false, nil
}

func (r *inventoryRepoConcurrencyStub) ListByWarehouse(ctx context.Context, warehouseID uint, page types.PageRequest) ([]Inventory, int64, error) {
	return nil, 0, nil
}

func (r *inventoryRepoConcurrencyStub) ListAlerts(ctx context.Context, warehouseID uint, page types.PageRequest) ([]Inventory, int64, error) {
	return nil, 0, nil
}

func (r *inventoryRepoConcurrencyStub) Update(ctx context.Context, inv *Inventory) error {
	r.calls = append(r.calls, "update")
	r.savedInventory = inv
	r.inventory = inv
	return nil
}

func (r *inventoryRepoConcurrencyStub) UpdateSettings(ctx context.Context, id uint, alertThreshold *int, status *int8, updatedBy uint) error {
	r.calls = append(r.calls, "update-settings")
	r.settingsUpdate = &inventorySettingsUpdate{
		id:             id,
		alertThreshold: alertThreshold,
		status:         status,
		updatedBy:      updatedBy,
	}
	if r.inventory != nil {
		if alertThreshold != nil {
			r.inventory.AlertThreshold = *alertThreshold
		}
		if status != nil {
			r.inventory.Status = *status
		} else if alertThreshold != nil {
			if *alertThreshold > 0 && r.inventory.Quantity <= *alertThreshold {
				r.inventory.Status = 2
			} else if r.inventory.Status == 2 {
				r.inventory.Status = 1
			}
		}
		r.inventory.UpdatedBy = updatedBy
	}
	return nil
}

func (r *inventoryRepoConcurrencyStub) Delete(ctx context.Context, id uint) error {
	return nil
}

type nonStdRepoConcurrencyStub struct {
	calls []string
	item  *NonStdInventory
}

func (r *nonStdRepoConcurrencyStub) Create(ctx context.Context, ns *NonStdInventory) error {
	return nil
}

func (r *nonStdRepoConcurrencyStub) GetByID(ctx context.Context, id uint) (*NonStdInventory, error) {
	r.calls = append(r.calls, "get")
	if r.item == nil {
		return nil, gorm.ErrRecordNotFound
	}
	return r.item, nil
}

func (r *nonStdRepoConcurrencyStub) GetByIDForUpdate(ctx context.Context, id uint) (*NonStdInventory, error) {
	r.calls = append(r.calls, "get-for-update")
	if r.item == nil {
		return nil, gorm.ErrRecordNotFound
	}
	return r.item, nil
}

func (r *nonStdRepoConcurrencyStub) GetByTempLabel(ctx context.Context, tempLabel string) (*NonStdInventory, error) {
	return nil, gorm.ErrRecordNotFound
}

func (r *nonStdRepoConcurrencyStub) ListByWarehouse(ctx context.Context, warehouseID uint, page types.PageRequest) ([]NonStdInventory, int64, error) {
	return nil, 0, nil
}

func (r *nonStdRepoConcurrencyStub) Update(ctx context.Context, ns *NonStdInventory) error {
	r.calls = append(r.calls, "update")
	r.item = ns
	return nil
}

func (r *nonStdRepoConcurrencyStub) Delete(ctx context.Context, id uint) error {
	return nil
}

func TestConvertNonStdLocksNonStdAndInventoryBeforeUpdating(t *testing.T) {
	invRepo := &inventoryRepoConcurrencyStub{
		inventory: &Inventory{WarehouseID: 10, ProductID: 20, Quantity: 5},
	}
	nonStdRepo := &nonStdRepoConcurrencyStub{
		item: &NonStdInventory{WarehouseID: 10, Quantity: 5, ConvertedQty: 1, Status: 1},
	}
	txRunner := func(ctx context.Context, fn func(context.Context) error) error {
		return fn(ctx)
	}
	service := NewProductService(productRepoConcurrencyStub{}, invRepo, nonStdRepo, txRunner)

	err := service.ConvertNonStd(context.Background(), 99, 10, 1, ConvertNonStdRequest{
		ProductID: 20,
		Quantity:  2,
	})
	if err != nil {
		t.Fatalf("ConvertNonStd returned error: %v", err)
	}

	if !reflect.DeepEqual(nonStdRepo.calls, []string{"get-for-update", "update"}) {
		t.Fatalf("expected locked non-standard read and update, got calls %v", nonStdRepo.calls)
	}
	if !reflect.DeepEqual(invRepo.calls, []string{"lock", "get-for-update", "update"}) {
		t.Fatalf("expected locked standard inventory read and update, got calls %v", invRepo.calls)
	}
	if invRepo.inventory.Quantity != 7 {
		t.Fatalf("expected standard inventory quantity 7, got %d", invRepo.inventory.Quantity)
	}
	if nonStdRepo.item.ConvertedQty != 3 {
		t.Fatalf("expected converted quantity 3, got %d", nonStdRepo.item.ConvertedQty)
	}
}

func TestUpdateInventoryUsesSettingsUpdateWithoutSavingQuantity(t *testing.T) {
	threshold := 10
	invRepo := &inventoryRepoConcurrencyStub{
		inventory: &Inventory{
			AuditableModel: types.AuditableModel{BaseModel: types.BaseModel{ID: 7}},
			WarehouseID:    10,
			ProductID:      20,
			Quantity:       3,
			AlertThreshold: 0,
			Status:         1,
		},
	}
	service := NewProductService(productRepoConcurrencyStub{}, invRepo, &nonStdRepoConcurrencyStub{}, nil)

	resp, err := service.UpdateInventory(context.Background(), 99, 10, 7, UpdateInventoryRequest{
		AlertThreshold: &threshold,
	})
	if err != nil {
		t.Fatalf("UpdateInventory returned error: %v", err)
	}

	if !reflect.DeepEqual(invRepo.calls, []string{"get", "update-settings", "get"}) {
		t.Fatalf("expected field-level settings update and reload, got calls %v", invRepo.calls)
	}
	if invRepo.savedInventory != nil {
		t.Fatalf("expected UpdateInventory not to call full-row Update/Save")
	}
	if invRepo.settingsUpdate == nil {
		t.Fatal("expected inventory settings update to be recorded")
	}
	if invRepo.settingsUpdate.id != 7 || invRepo.settingsUpdate.updatedBy != 99 {
		t.Fatalf("unexpected settings update metadata: %+v", invRepo.settingsUpdate)
	}
	if invRepo.settingsUpdate.alertThreshold == nil || *invRepo.settingsUpdate.alertThreshold != threshold {
		t.Fatalf("expected alert threshold %d, got %+v", threshold, invRepo.settingsUpdate.alertThreshold)
	}
	if invRepo.settingsUpdate.status != nil {
		t.Fatalf("expected nil status so repository can auto-calculate from current DB quantity, got %v", *invRepo.settingsUpdate.status)
	}
	if resp.Quantity != 3 || resp.AlertThreshold != threshold || resp.Status != 2 {
		t.Fatalf("unexpected inventory response: %+v", resp)
	}
}
