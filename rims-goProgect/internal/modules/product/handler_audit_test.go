// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package product

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"rims-go/internal/db"
	"rims-go/internal/modules/audit"
	"rims-go/internal/types"
)

type productHandlerAuditLogger struct {
	entries []audit.Entry
}

func (l *productHandlerAuditLogger) Log(ctx context.Context, e audit.Entry) error {
	l.entries = append(l.entries, e)
	return nil
}

type auditProductRepoStub struct {
	products map[uint]*Product
}

func (r *auditProductRepoStub) Create(ctx context.Context, p *Product) error { return nil }
func (r *auditProductRepoStub) GetByID(ctx context.Context, id uint) (*Product, error) {
	p, ok := r.products[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	copy := *p
	return &copy, nil
}
func (r *auditProductRepoStub) GetByCode(ctx context.Context, code string) (*Product, error) {
	return nil, gorm.ErrRecordNotFound
}
func (r *auditProductRepoStub) GetByBarcode(ctx context.Context, barcode string) (*Product, error) {
	return nil, gorm.ErrRecordNotFound
}
func (r *auditProductRepoStub) List(ctx context.Context, page types.PageRequest) ([]Product, int64, error) {
	return nil, 0, nil
}
func (r *auditProductRepoStub) Update(ctx context.Context, p *Product) error {
	copy := *p
	r.products[p.ID] = &copy
	return nil
}
func (r *auditProductRepoStub) Delete(ctx context.Context, id uint) error { return nil }

type auditInventoryRepoStub struct {
	items map[uint]*Inventory
	byKey map[uint]*Inventory
}

func (r *auditInventoryRepoStub) Create(ctx context.Context, inv *Inventory) error {
	copy := *inv
	r.byKey[inv.ProductID] = &copy
	return nil
}
func (r *auditInventoryRepoStub) GetByID(ctx context.Context, id uint) (*Inventory, error) {
	inv, ok := r.items[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	copy := *inv
	return &copy, nil
}
func (r *auditInventoryRepoStub) LockItem(ctx context.Context, warehouseID, productID uint) error {
	return nil
}
func (r *auditInventoryRepoStub) GetByWarehouseAndProductForUpdate(ctx context.Context, warehouseID, productID uint) (*Inventory, error) {
	inv, ok := r.byKey[productID]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	copy := *inv
	return &copy, nil
}
func (r *auditInventoryRepoStub) GetByWarehouseAndProduct(ctx context.Context, warehouseID, productID uint) (*Inventory, error) {
	return r.GetByWarehouseAndProductForUpdate(ctx, warehouseID, productID)
}
func (r *auditInventoryRepoStub) ExistsByProductID(ctx context.Context, productID uint) (bool, error) {
	return false, nil
}
func (r *auditInventoryRepoStub) ListByWarehouse(ctx context.Context, warehouseID uint, page types.PageRequest) ([]Inventory, int64, error) {
	return nil, 0, nil
}
func (r *auditInventoryRepoStub) ListAlerts(ctx context.Context, warehouseID uint, page types.PageRequest) ([]Inventory, int64, error) {
	return nil, 0, nil
}
func (r *auditInventoryRepoStub) Update(ctx context.Context, inv *Inventory) error {
	copy := *inv
	r.byKey[inv.ProductID] = &copy
	return nil
}
func (r *auditInventoryRepoStub) UpdateSettings(ctx context.Context, id uint, alertThreshold *int, status *int8, updatedBy uint) error {
	inv := r.items[id]
	if alertThreshold != nil {
		inv.AlertThreshold = *alertThreshold
	}
	if status != nil {
		inv.Status = *status
	}
	inv.UpdatedBy = updatedBy
	return nil
}
func (r *auditInventoryRepoStub) Delete(ctx context.Context, id uint) error { return nil }

type auditNonStdRepoStub struct {
	items map[uint]*NonStdInventory
}

func (r *auditNonStdRepoStub) Create(ctx context.Context, ns *NonStdInventory) error { return nil }
func (r *auditNonStdRepoStub) GetByID(ctx context.Context, id uint) (*NonStdInventory, error) {
	ns, ok := r.items[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	copy := *ns
	return &copy, nil
}
func (r *auditNonStdRepoStub) GetByIDForUpdate(ctx context.Context, id uint) (*NonStdInventory, error) {
	return r.GetByID(ctx, id)
}
func (r *auditNonStdRepoStub) GetByTempLabel(ctx context.Context, tempLabel string) (*NonStdInventory, error) {
	return nil, gorm.ErrRecordNotFound
}
func (r *auditNonStdRepoStub) ListByWarehouse(ctx context.Context, warehouseID uint, page types.PageRequest) ([]NonStdInventory, int64, error) {
	return nil, 0, nil
}
func (r *auditNonStdRepoStub) Update(ctx context.Context, ns *NonStdInventory) error {
	copy := *ns
	r.items[ns.ID] = &copy
	return nil
}
func (r *auditNonStdRepoStub) Delete(ctx context.Context, id uint) error { return nil }

func TestProductHandlerAuditsCostInventoryAndNonStdConversion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	p := &Product{Code: "P1", Name: "Product", Unit: "pcs", Status: 1, CostPrice: 10}
	p.ID = 5
	target := &Product{Code: "P2", Name: "Target", Unit: "pcs", Status: 1}
	target.ID = 6
	inv := &Inventory{WarehouseID: 3, ProductID: 5, Quantity: 9, AlertThreshold: 2, Status: 1}
	inv.ID = 8
	targetInv := &Inventory{WarehouseID: 3, ProductID: 6, Quantity: 1, Status: 1}
	targetInv.ID = 9
	ns := &NonStdInventory{WarehouseID: 3, TempLabel: "TMP", Description: "raw", Unit: "kg", Quantity: 10, ConvertedQty: 2, Status: 1}
	ns.ID = 11
	productRepo := &auditProductRepoStub{products: map[uint]*Product{5: p, 6: target}}
	invRepo := &auditInventoryRepoStub{
		items: map[uint]*Inventory{8: inv},
		byKey: map[uint]*Inventory{
			6: targetInv,
		},
	}
	nonStdRepo := &auditNonStdRepoStub{items: map[uint]*NonStdInventory{11: ns}}
	logger := &productHandlerAuditLogger{}
	handler := NewHandler(NewProductService(productRepo, invRepo, nonStdRepo, passThroughProductTx), logger)

	runProductAuditRequest(t, handler.UpdateProduct, http.MethodPut, "/products/5", []gin.Param{{Key: "id", Value: "5"}}, `{"costPrice":12.5}`)
	runProductAuditRequest(t, handler.UpdateInventory, http.MethodPut, "/inventory/8", []gin.Param{{Key: "id", Value: "8"}}, `{"alertThreshold":4,"status":1}`)
	runProductAuditRequest(t, handler.ConvertNonStd, http.MethodPost, "/non-std-inventory/11/convert", []gin.Param{{Key: "id", Value: "11"}}, `{"productId":6,"quantity":3}`)

	assertProductAuditEntry(t, logger.entries[0], audit.ActionUpdate, audit.ResourceProduct, 5)
	if logger.entries[0].After["costPrice"] != 12.5 {
		t.Fatalf("costPrice detail = %#v, want 12.5", logger.entries[0].After["costPrice"])
	}
	assertProductAuditEntry(t, logger.entries[1], audit.ActionUpdate, audit.ResourceInventory, 8)
	if logger.entries[1].After["alertThreshold"] != 4 || logger.entries[1].After["status"] != int8(1) {
		t.Fatalf("inventory details = %#v, want alertThreshold/status", logger.entries[1].After)
	}
	assertProductAuditEntry(t, logger.entries[2], audit.ActionConvert, audit.ResourceNonStdInventory, 11)
	if logger.entries[2].After["productID"] != uint(6) || logger.entries[2].After["quantity"] != 3 {
		t.Fatalf("convert details = %#v, want productID 6 quantity 3", logger.entries[2].After)
	}
}

func passThroughProductTx(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

func runProductAuditRequest(t *testing.T, fn gin.HandlerFunc, method, target string, params []gin.Param, body string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(method, target, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(types.CtxKeyUserID, uint(2))
	c.Set(types.CtxKeyUsername, "admin")
	c.Set(types.CtxKeyRoleCode, "admin")
	c.Set(types.CtxKeyWarehouseID, uint(3))
	c.Set(types.CtxKeyTraceID, "trace-product-audit")
	c.Params = params

	fn(c)
	if rec.Code < 200 || rec.Code >= 300 {
		t.Fatalf("%s %s status = %d; body=%s", method, target, rec.Code, rec.Body.String())
	}
	return c, rec
}

func assertProductAuditEntry(t *testing.T, got audit.Entry, action, resource string, resourceID uint) {
	t.Helper()
	if got.Action != action || got.Resource != resource {
		t.Fatalf("entry action/resource = %q/%q, want %q/%q", got.Action, got.Resource, action, resource)
	}
	if got.ResourceID == nil || *got.ResourceID != resourceID {
		t.Fatalf("entry resourceID = %v, want %d", got.ResourceID, resourceID)
	}
	if got.Actor.UserID != 2 || got.Actor.WarehouseID != 3 {
		t.Fatalf("actor = %#v, want user 2 warehouse 3", got.Actor)
	}
}

var _ db.TxRunner = passThroughProductTx
