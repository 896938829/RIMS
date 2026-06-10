// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package warehouse

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

type warehouseHandlerAuditLogger struct {
	entries []audit.Entry
}

func (l *warehouseHandlerAuditLogger) Log(ctx context.Context, e audit.Entry) error {
	l.entries = append(l.entries, e)
	return nil
}

type auditWarehouseRepoStub struct {
	warehouses map[uint]*Warehouse
	next       uint
}

func (r *auditWarehouseRepoStub) Create(ctx context.Context, w *Warehouse) error {
	if r.warehouses == nil {
		r.warehouses = make(map[uint]*Warehouse)
	}
	if w.ID == 0 {
		if r.next == 0 {
			r.next = 100
		}
		w.ID = r.next
		r.next++
	}
	copy := *w
	r.warehouses[w.ID] = &copy
	return nil
}
func (r *auditWarehouseRepoStub) GetByID(ctx context.Context, id uint) (*Warehouse, error) {
	w, ok := r.warehouses[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	copy := *w
	return &copy, nil
}
func (r *auditWarehouseRepoStub) GetByCode(ctx context.Context, code string) (*Warehouse, error) {
	return nil, gorm.ErrRecordNotFound
}
func (r *auditWarehouseRepoStub) List(ctx context.Context, page types.PageRequest) ([]Warehouse, int64, error) {
	return nil, 0, nil
}
func (r *auditWarehouseRepoStub) ListByIDs(ctx context.Context, ids []uint) ([]Warehouse, int64, error) {
	return nil, 0, nil
}
func (r *auditWarehouseRepoStub) Update(ctx context.Context, w *Warehouse) error {
	copy := *w
	r.warehouses[w.ID] = &copy
	return nil
}
func (r *auditWarehouseRepoStub) Delete(ctx context.Context, id uint) error {
	delete(r.warehouses, id)
	return nil
}

type auditUserWarehouseRepoStub struct {
	bindings map[uint]map[uint]*UserWarehouse
}

func (r *auditUserWarehouseRepoStub) Create(ctx context.Context, uw *UserWarehouse) error {
	if r.bindings[uw.UserID] == nil {
		r.bindings[uw.UserID] = map[uint]*UserWarehouse{}
	}
	copy := *uw
	r.bindings[uw.UserID][uw.WarehouseID] = &copy
	return nil
}
func (r *auditUserWarehouseRepoStub) Delete(ctx context.Context, userID, warehouseID uint) error {
	delete(r.bindings[userID], warehouseID)
	return nil
}
func (r *auditUserWarehouseRepoStub) DeleteByWarehouseID(ctx context.Context, warehouseID uint) error {
	return nil
}
func (r *auditUserWarehouseRepoStub) GetByUserAndWarehouse(ctx context.Context, userID, warehouseID uint) (*UserWarehouse, error) {
	if byWarehouse := r.bindings[userID]; byWarehouse != nil {
		if uw, ok := byWarehouse[warehouseID]; ok {
			copy := *uw
			return &copy, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}
func (r *auditUserWarehouseRepoStub) ListByUserID(ctx context.Context, userID uint) ([]UserWarehouse, error) {
	out := make([]UserWarehouse, 0, len(r.bindings[userID]))
	for _, uw := range r.bindings[userID] {
		out = append(out, *uw)
	}
	return out, nil
}
func (r *auditUserWarehouseRepoStub) ListByWarehouseID(ctx context.Context, warehouseID uint, page types.PageRequest) ([]WarehouseUserInfo, int64, error) {
	return nil, 0, nil
}
func (r *auditUserWarehouseRepoStub) GetDefaultByUserID(ctx context.Context, userID uint) (*UserWarehouse, error) {
	return nil, gorm.ErrRecordNotFound
}
func (r *auditUserWarehouseRepoStub) ClearDefault(ctx context.Context, userID uint) error {
	for _, uw := range r.bindings[userID] {
		uw.IsDefault = false
	}
	return nil
}
func (r *auditUserWarehouseRepoStub) SetDefault(ctx context.Context, userID, warehouseID uint) error {
	if r.bindings[userID] == nil {
		r.bindings[userID] = map[uint]*UserWarehouse{}
	}
	if r.bindings[userID][warehouseID] == nil {
		r.bindings[userID][warehouseID] = &UserWarehouse{UserID: userID, WarehouseID: warehouseID}
	}
	r.bindings[userID][warehouseID].IsDefault = true
	return nil
}
func (r *auditUserWarehouseRepoStub) CountByUserID(ctx context.Context, userID uint) (int64, error) {
	return int64(len(r.bindings[userID])), nil
}
func (r *auditUserWarehouseRepoStub) GetUserRoleCode(ctx context.Context, userID uint) (string, error) {
	return "admin", nil
}
func (r *auditUserWarehouseRepoStub) GetDefaultWarehouseID(ctx context.Context, userID uint) (uint, error) {
	return 0, nil
}
func (r *auditUserWarehouseRepoStub) HasAccess(ctx context.Context, userID, warehouseID uint) (bool, error) {
	return true, nil
}

func TestWarehouseHandlerAuditsBindUnbindAndSetDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := &Warehouse{Code: "WH", Name: "Warehouse", Status: 1}
	w.ID = 10
	uwRepo := &auditUserWarehouseRepoStub{bindings: map[uint]map[uint]*UserWarehouse{
		30: {
			10: {UserID: 30, WarehouseID: 10, IsDefault: true},
		},
		40: {
			10: {UserID: 40, WarehouseID: 10},
		},
	}}
	logger := &warehouseHandlerAuditLogger{}
	handler := NewHandler(
		NewWarehouseService(&auditWarehouseRepoStub{warehouses: map[uint]*Warehouse{10: w}}, uwRepo, passThroughWarehouseTx),
		logger,
	)

	runWarehouseAuditRequest(t, handler.BindUsers, http.MethodPost, "/warehouses/10/users", []gin.Param{{Key: "id", Value: "10"}}, `{"userIds":[20,21]}`)
	runWarehouseAuditRequest(t, handler.UnbindUser, http.MethodDelete, "/warehouses/10/users/30", []gin.Param{{Key: "id", Value: "10"}, {Key: "userId", Value: "30"}}, "")
	runWarehouseAuditRequest(t, handler.SetDefaultWarehouse, http.MethodPut, "/users/me/warehouses/default", nil, `{"warehouseId":10}`)

	assertWarehouseAuditEntry(t, logger.entries[0], audit.ActionBind, audit.ResourceUserWarehouse, 10)
	assertWarehouseAuditEntry(t, logger.entries[1], audit.ActionUnbind, audit.ResourceUserWarehouse, 10)
	assertWarehouseAuditEntry(t, logger.entries[2], audit.ActionUpdate, audit.ResourceUserWarehouse, 10)
}

func TestWarehouseHandlerAuditsCRUD(t *testing.T) {
	gin.SetMode(gin.TestMode)
	warehouseRepo := &auditWarehouseRepoStub{warehouses: map[uint]*Warehouse{}, next: 12}
	uwRepo := &auditUserWarehouseRepoStub{bindings: map[uint]map[uint]*UserWarehouse{}}
	logger := &warehouseHandlerAuditLogger{}
	handler := NewHandler(NewWarehouseService(warehouseRepo, uwRepo, passThroughWarehouseTx), logger)

	runWarehouseAuditRequest(t, handler.CreateWarehouse, http.MethodPost, "/warehouses", nil, `{"code":"WH12","name":"North Store","status":1}`)
	runWarehouseAuditRequest(t, handler.UpdateWarehouse, http.MethodPut, "/warehouses/12", []gin.Param{{Key: "id", Value: "12"}}, `{"name":"North Store Updated","status":1}`)
	runWarehouseAuditRequest(t, handler.DeleteWarehouse, http.MethodDelete, "/warehouses/12", []gin.Param{{Key: "id", Value: "12"}}, "")

	if len(logger.entries) != 3 {
		t.Fatalf("audit entries = %d, want 3", len(logger.entries))
	}
	assertWarehouseAuditEntry(t, logger.entries[0], audit.ActionCreate, audit.ResourceWarehouse, 12)
	if logger.entries[0].After["code"] != "WH12" || logger.entries[0].After["status"] != int8(1) {
		t.Fatalf("create details = %#v, want code/status", logger.entries[0].After)
	}
	assertWarehouseAuditEntry(t, logger.entries[1], audit.ActionUpdate, audit.ResourceWarehouse, 12)
	if logger.entries[1].After["name"] != "North Store Updated" || logger.entries[1].After["status"] != int8(1) {
		t.Fatalf("update details = %#v, want name/status", logger.entries[1].After)
	}
	assertWarehouseAuditEntry(t, logger.entries[2], audit.ActionDelete, audit.ResourceWarehouse, 12)
	if logger.entries[2].After["warehouseID"] != uint(12) {
		t.Fatalf("delete details = %#v, want warehouseID 12", logger.entries[2].After)
	}
}

func passThroughWarehouseTx(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

func runWarehouseAuditRequest(t *testing.T, fn gin.HandlerFunc, method, target string, params []gin.Param, body string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(method, target, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(types.CtxKeyUserID, uint(40))
	c.Set(types.CtxKeyUsername, "admin")
	c.Set(types.CtxKeyRoleCode, "admin")
	c.Set(types.CtxKeyTraceID, "trace-warehouse-audit")
	c.Params = params

	fn(c)
	if rec.Code < 200 || rec.Code >= 300 {
		t.Fatalf("%s %s status = %d; body=%s", method, target, rec.Code, rec.Body.String())
	}
	return c, rec
}

func assertWarehouseAuditEntry(t *testing.T, got audit.Entry, action, resource string, warehouseID uint) {
	t.Helper()
	if got.Action != action || got.Resource != resource {
		t.Fatalf("entry action/resource = %q/%q, want %q/%q", got.Action, got.Resource, action, resource)
	}
	if got.Actor.UserID != 40 || got.Actor.RoleCode != "admin" {
		t.Fatalf("actor = %#v, want admin user 40", got.Actor)
	}
	if got.After["warehouseID"] != warehouseID {
		t.Fatalf("after warehouseID = %#v, want %d", got.After["warehouseID"], warehouseID)
	}
}

var _ db.TxRunner = passThroughWarehouseTx
