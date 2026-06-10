// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package document

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"rims-go/internal/modules/audit"
	"rims-go/internal/modules/product"
	"rims-go/internal/types"
)

type docRepoConcurrencyStub struct {
	calls               []string
	document            *Document
	created             []*Document
	next                uint
	maxDocNo            string
	lockPrefix          string
	lockDateStr         string
	maxPrefix           string
	maxDateStr          string
	lockReturnRefDocID  uint
	lockReturnProductID uint
}

func (r *docRepoConcurrencyStub) Create(ctx context.Context, doc *Document) error {
	r.calls = append(r.calls, "create-doc")
	if doc.ID == 0 {
		if r.next == 0 {
			r.next = 100
		}
		doc.ID = r.next
		r.next++
	}
	copy := *doc
	r.created = append(r.created, &copy)
	r.document = &copy
	return nil
}

func (r *docRepoConcurrencyStub) GetByID(ctx context.Context, id uint) (*Document, error) {
	r.calls = append(r.calls, "get-doc")
	if r.document != nil {
		return r.document, nil
	}
	return nil, gorm.ErrRecordNotFound
}

func (r *docRepoConcurrencyStub) GetByIDForUpdate(ctx context.Context, id uint) (*Document, error) {
	r.calls = append(r.calls, "get-doc-for-update")
	if r.document != nil {
		return r.document, nil
	}
	return nil, gorm.ErrRecordNotFound
}

func (r *docRepoConcurrencyStub) GetByDocNo(ctx context.Context, docNo string) (*Document, error) {
	return nil, gorm.ErrRecordNotFound
}

func (r *docRepoConcurrencyStub) List(ctx context.Context, warehouseID uint, docType int8, page types.PageRequest) ([]Document, int64, error) {
	return nil, 0, nil
}

func (r *docRepoConcurrencyStub) Update(ctx context.Context, doc *Document) error {
	r.calls = append(r.calls, "update-doc")
	r.document = doc
	return nil
}

func (r *docRepoConcurrencyStub) LockDocNoSequence(ctx context.Context, prefix string, dateStr string) error {
	r.calls = append(r.calls, "lock")
	r.lockPrefix = prefix
	r.lockDateStr = dateStr
	return nil
}

func (r *docRepoConcurrencyStub) LockReturnQuantity(ctx context.Context, refDocID uint, productID uint) error {
	r.calls = append(r.calls, "lock-return")
	r.lockReturnRefDocID = refDocID
	r.lockReturnProductID = productID
	return nil
}

func (r *docRepoConcurrencyStub) GetMaxDocNo(ctx context.Context, prefix string, dateStr string) (string, error) {
	r.calls = append(r.calls, "max")
	r.maxPrefix = prefix
	r.maxDateStr = dateStr
	return r.maxDocNo, nil
}

type inventoryRepoConcurrencyStub struct {
	calls         []string
	detailedCalls []string
	lockKeys      []inventoryLockCall
	inventory     *product.Inventory
	inventories   map[string]*product.Inventory
}

type inventoryLockCall struct {
	warehouseID uint
	productID   uint
}

func inventoryKey(warehouseID, productID uint) string {
	return fmt.Sprintf("%d:%d", warehouseID, productID)
}

func (r *inventoryRepoConcurrencyStub) Create(ctx context.Context, inv *product.Inventory) error {
	r.calls = append(r.calls, "create")
	r.detailedCalls = append(r.detailedCalls, fmt.Sprintf("create:%d:%d", inv.WarehouseID, inv.ProductID))
	r.inventory = inv
	if r.inventories != nil {
		r.inventories[inventoryKey(inv.WarehouseID, inv.ProductID)] = inv
	}
	return nil
}

func (r *inventoryRepoConcurrencyStub) GetByID(ctx context.Context, id uint) (*product.Inventory, error) {
	return r.inventory, nil
}

func (r *inventoryRepoConcurrencyStub) LockItem(ctx context.Context, warehouseID, productID uint) error {
	r.calls = append(r.calls, "lock")
	r.detailedCalls = append(r.detailedCalls, fmt.Sprintf("lock:%d:%d", warehouseID, productID))
	r.lockKeys = append(r.lockKeys, inventoryLockCall{warehouseID: warehouseID, productID: productID})
	return nil
}

func (r *inventoryRepoConcurrencyStub) GetByWarehouseAndProductForUpdate(ctx context.Context, warehouseID, productID uint) (*product.Inventory, error) {
	r.calls = append(r.calls, "get-for-update")
	r.detailedCalls = append(r.detailedCalls, fmt.Sprintf("get-for-update:%d:%d", warehouseID, productID))
	if r.inventories != nil {
		if inv := r.inventories[inventoryKey(warehouseID, productID)]; inv != nil {
			return inv, nil
		}
		return nil, gorm.ErrRecordNotFound
	}
	if r.inventory == nil {
		return nil, gorm.ErrRecordNotFound
	}
	return r.inventory, nil
}

func (r *inventoryRepoConcurrencyStub) GetByWarehouseAndProduct(ctx context.Context, warehouseID, productID uint) (*product.Inventory, error) {
	r.calls = append(r.calls, "get")
	r.detailedCalls = append(r.detailedCalls, fmt.Sprintf("get:%d:%d", warehouseID, productID))
	if r.inventories != nil {
		if inv := r.inventories[inventoryKey(warehouseID, productID)]; inv != nil {
			return inv, nil
		}
		return nil, gorm.ErrRecordNotFound
	}
	if r.inventory == nil {
		return nil, gorm.ErrRecordNotFound
	}
	return r.inventory, nil
}

func (r *inventoryRepoConcurrencyStub) ExistsByProductID(ctx context.Context, productID uint) (bool, error) {
	return false, nil
}

func (r *inventoryRepoConcurrencyStub) ListByWarehouse(ctx context.Context, warehouseID uint, page types.PageRequest) ([]product.Inventory, int64, error) {
	return nil, 0, nil
}

func (r *inventoryRepoConcurrencyStub) ListAlerts(ctx context.Context, warehouseID uint, page types.PageRequest) ([]product.Inventory, int64, error) {
	return nil, 0, nil
}

func (r *inventoryRepoConcurrencyStub) Update(ctx context.Context, inv *product.Inventory) error {
	r.calls = append(r.calls, "update")
	r.detailedCalls = append(r.detailedCalls, fmt.Sprintf("update:%d:%d", inv.WarehouseID, inv.ProductID))
	r.inventory = inv
	if r.inventories != nil {
		r.inventories[inventoryKey(inv.WarehouseID, inv.ProductID)] = inv
	}
	return nil
}

func (r *inventoryRepoConcurrencyStub) UpdateSettings(ctx context.Context, id uint, alertThreshold *int, status *int8, updatedBy uint) error {
	return nil
}

func (r *inventoryRepoConcurrencyStub) Delete(ctx context.Context, id uint) error {
	return nil
}

type inventoryTransactionRepoStub struct {
	created []*InventoryTransaction
}

func (r *inventoryTransactionRepoStub) Create(ctx context.Context, txn *InventoryTransaction) error {
	r.created = append(r.created, txn)
	return nil
}

func (r *inventoryTransactionRepoStub) CreateBatch(ctx context.Context, txns []InventoryTransaction) error {
	return nil
}

func (r *inventoryTransactionRepoStub) ListByWarehouse(ctx context.Context, warehouseID uint, page types.PageRequest) ([]InventoryTransaction, int64, error) {
	return nil, 0, nil
}

func (r *inventoryTransactionRepoStub) ListByDocument(ctx context.Context, docID uint) ([]InventoryTransaction, error) {
	return nil, nil
}

type documentLineRepoStub struct {
	lines       []DocumentLine
	created     []DocumentLine
	calls       *[]string
	returnedQty int
}

func (r *documentLineRepoStub) CreateBatch(ctx context.Context, lines []DocumentLine) error {
	r.created = append(r.created, lines...)
	return nil
}

func (r *documentLineRepoStub) ListByDocumentID(ctx context.Context, documentID uint) ([]DocumentLine, error) {
	return r.lines, nil
}

func (r *documentLineRepoStub) SumReturnedQty(ctx context.Context, refDocID uint, productID uint) (int, error) {
	if r.calls != nil {
		*r.calls = append(*r.calls, "sum-returned")
	}
	return r.returnedQty, nil
}

type auditLoggerStub struct{}

func (auditLoggerStub) Log(ctx context.Context, e audit.Entry) error {
	return nil
}

type documentAuditTxKey struct{}

type documentAuditTxRunner struct {
	calls      int
	committed  bool
	rolledBack bool
}

func (r *documentAuditTxRunner) run(ctx context.Context, fn func(context.Context) error) error {
	r.calls++
	err := fn(context.WithValue(ctx, documentAuditTxKey{}, true))
	if err != nil {
		r.rolledBack = true
		return err
	}
	r.committed = true
	return nil
}

type recordingDocumentAuditLogger struct {
	entries []audit.Entry
	txSeen  []bool
	err     error
}

func (l *recordingDocumentAuditLogger) Log(ctx context.Context, e audit.Entry) error {
	l.entries = append(l.entries, e)
	l.txSeen = append(l.txSeen, ctx.Value(documentAuditTxKey{}) == true)
	return l.err
}

type documentProductRepoStub struct {
	products map[uint]*product.Product
}

func (r *documentProductRepoStub) Create(ctx context.Context, p *product.Product) error { return nil }
func (r *documentProductRepoStub) GetByID(ctx context.Context, id uint) (*product.Product, error) {
	p, ok := r.products[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	copy := *p
	return &copy, nil
}
func (r *documentProductRepoStub) GetByCode(ctx context.Context, code string) (*product.Product, error) {
	return nil, gorm.ErrRecordNotFound
}
func (r *documentProductRepoStub) GetByBarcode(ctx context.Context, barcode string) (*product.Product, error) {
	return nil, gorm.ErrRecordNotFound
}
func (r *documentProductRepoStub) List(ctx context.Context, page types.PageRequest) ([]product.Product, int64, error) {
	return nil, 0, nil
}
func (r *documentProductRepoStub) Update(ctx context.Context, p *product.Product) error { return nil }
func (r *documentProductRepoStub) Delete(ctx context.Context, id uint) error            { return nil }

func TestDocumentServiceAuditsCreateInsideTransaction(t *testing.T) {
	auditErr := errors.New("audit insert failed")
	txRunner := &documentAuditTxRunner{}
	logger := &recordingDocumentAuditLogger{err: auditErr}
	docRepo := &docRepoConcurrencyStub{next: 700}
	lineRepo := &documentLineRepoStub{}
	productRepo := &documentProductRepoStub{products: map[uint]*product.Product{
		55: {Code: "P55", Name: "Product 55", Unit: "pcs", CostPrice: 3, RetailPrice: 5},
	}}
	productRepo.products[55].ID = 55
	service := &DocumentService{
		docRepo:     docRepo,
		lineRepo:    lineRepo,
		invRepo:     &inventoryRepoConcurrencyStub{},
		productRepo: productRepo,
		txRunner:    txRunner.run,
		audit:       logger,
	}

	resp, err := service.Create(context.Background(), 77, 12, CreateDocumentRequest{
		DocType: DocTypeSales,
		Lines: []CreateDocumentLineRequest{
			{ProductID: 55, Quantity: 2},
		},
	})

	if !errors.Is(err, auditErr) {
		t.Fatalf("Create() error = %v, want audit error", err)
	}
	if resp != nil {
		t.Fatalf("Create() response = %#v, want nil on audit rollback", resp)
	}
	if !txRunner.rolledBack || txRunner.committed {
		t.Fatalf("tx committed=%v rolledBack=%v, want rollback only", txRunner.committed, txRunner.rolledBack)
	}
	if len(logger.entries) != 1 || !logger.txSeen[0] {
		t.Fatalf("audit entries/txSeen = %d/%v, want one in tx", len(logger.entries), logger.txSeen)
	}
	got := logger.entries[0]
	assertDocumentAuditEntry(t, got, audit.ActionCreate, 700, docRepo.created[0].DocNo, 77, 12)
	if got.Before["status"] != int8(0) || got.After["status"] != StatusDraft {
		t.Fatalf("create status snapshot before/after = %#v/%#v, want 0/draft", got.Before, got.After)
	}
	if got.After["docType"] != DocTypeSales || got.After["warehouseID"] != uint(12) {
		t.Fatalf("create audit details = %#v, want docType/warehouseID", got.After)
	}
}

func TestDocumentServiceAuditsConfirmStocktake(t *testing.T) {
	txRunner := &documentAuditTxRunner{}
	logger := &recordingDocumentAuditLogger{}
	doc := &Document{
		AuditableModel: types.AuditableModel{BaseModel: types.BaseModel{ID: 801}},
		DocNo:          "PD20260610001",
		DocType:        DocTypeStocktake,
		Status:         StatusStRecording,
		WarehouseID:    12,
	}
	docRepo := &docRepoConcurrencyStub{document: doc}
	service := &DocumentService{
		docRepo:  docRepo,
		txRunner: txRunner.run,
		audit:    logger,
	}

	err := service.ConfirmStocktake(context.Background(), 77, 12, 801)

	if err != nil {
		t.Fatalf("ConfirmStocktake() error = %v", err)
	}
	if !txRunner.committed || txRunner.rolledBack {
		t.Fatalf("tx committed=%v rolledBack=%v, want commit", txRunner.committed, txRunner.rolledBack)
	}
	if len(logger.entries) != 1 || !logger.txSeen[0] {
		t.Fatalf("audit entries/txSeen = %d/%v, want one in tx", len(logger.entries), logger.txSeen)
	}
	got := logger.entries[0]
	assertDocumentAuditEntry(t, got, audit.ActionConfirm, 801, "PD20260610001", 77, 12)
	if got.Before["status"] != StatusStRecording || got.After["status"] != StatusStConfirmed {
		t.Fatalf("confirm status snapshot before/after = %#v/%#v, want recording/confirmed", got.Before, got.After)
	}
	if got.After["docType"] != DocTypeStocktake || got.After["warehouseID"] != uint(12) {
		t.Fatalf("confirm audit details = %#v, want docType/warehouseID", got.After)
	}
}

func TestDocumentServiceAuditsSettleStocktakeInsideTransaction(t *testing.T) {
	auditErr := errors.New("audit insert failed")
	txRunner := &documentAuditTxRunner{}
	logger := &recordingDocumentAuditLogger{err: auditErr}
	doc := &Document{
		AuditableModel: types.AuditableModel{BaseModel: types.BaseModel{ID: 901}},
		DocNo:          "PD20260610002",
		DocType:        DocTypeStocktake,
		Status:         StatusStConfirmed,
		WarehouseID:    12,
	}
	docRepo := &docRepoConcurrencyStub{document: doc}
	lineRepo := &documentLineRepoStub{
		lines: []DocumentLine{{ProductID: 55, SystemQty: 4, DiffQty: 3}},
	}
	invRepo := &inventoryRepoConcurrencyStub{
		inventory: &product.Inventory{WarehouseID: 12, ProductID: 55, Quantity: 4, Status: 1},
	}
	service := &DocumentService{
		docRepo:  docRepo,
		lineRepo: lineRepo,
		txnRepo:  &inventoryTransactionRepoStub{},
		invRepo:  invRepo,
		txRunner: txRunner.run,
		audit:    logger,
	}

	err := service.SettleStocktake(context.Background(), 77, 12, 901)

	if !errors.Is(err, auditErr) {
		t.Fatalf("SettleStocktake() error = %v, want audit error", err)
	}
	if !txRunner.rolledBack || txRunner.committed {
		t.Fatalf("tx committed=%v rolledBack=%v, want rollback only", txRunner.committed, txRunner.rolledBack)
	}
	if len(logger.entries) != 1 || !logger.txSeen[0] {
		t.Fatalf("audit entries/txSeen = %d/%v, want one in tx", len(logger.entries), logger.txSeen)
	}
	got := logger.entries[0]
	assertDocumentAuditEntry(t, got, audit.ActionSettle, 901, "PD20260610002", 77, 12)
	if got.Before["status"] != StatusStConfirmed || got.After["status"] != StatusStSettled {
		t.Fatalf("settle status snapshot before/after = %#v/%#v, want confirmed/settled", got.Before, got.After)
	}
	if got.Before["docType"] != DocTypeStocktake || got.After["warehouseID"] != uint(12) {
		t.Fatalf("settle audit details before/after = %#v/%#v, want docType/warehouseID", got.Before, got.After)
	}
}

func TestSettleStocktakeRejectsWhenCurrentInventoryDiffersFromSystemQty(t *testing.T) {
	doc := &Document{
		AuditableModel: types.AuditableModel{BaseModel: types.BaseModel{ID: 902}},
		DocNo:          "PD20260610003",
		DocType:        DocTypeStocktake,
		Status:         StatusStConfirmed,
		WarehouseID:    12,
	}
	docRepo := &docRepoConcurrencyStub{document: doc}
	lineRepo := &documentLineRepoStub{
		lines: []DocumentLine{{DocumentID: 902, ProductID: 55, SystemQty: 10, ActualQty: 8, DiffQty: -2}},
	}
	txnRepo := &inventoryTransactionRepoStub{}
	inv := &product.Inventory{WarehouseID: 12, ProductID: 55, Quantity: 8, Status: 1}
	invRepo := &inventoryRepoConcurrencyStub{inventory: inv}
	service := &DocumentService{
		docRepo:  docRepo,
		lineRepo: lineRepo,
		txnRepo:  txnRepo,
		invRepo:  invRepo,
		txRunner: func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) },
		audit:    auditLoggerStub{},
	}

	err := service.SettleStocktake(context.Background(), 77, 12, 902)

	appErr := assertAppErrorCode(t, err, types.ErrCodeInvalidState)
	if !strings.Contains(appErr.Message, "库存") {
		t.Fatalf("SettleStocktake() error message = %q, want stock-change message", appErr.Message)
	}
	if inv.Quantity != 8 {
		t.Fatalf("inventory quantity = %d, want unchanged 8", inv.Quantity)
	}
	if containsString(invRepo.calls, "update") {
		t.Fatalf("inventory update was called on stale stocktake snapshot, calls %v", invRepo.calls)
	}
	if len(txnRepo.created) != 0 {
		t.Fatalf("created %d transactions, want none", len(txnRepo.created))
	}
	if containsString(docRepo.calls, "update-doc") {
		t.Fatalf("document update was called on stale stocktake snapshot, calls %v", docRepo.calls)
	}
	if doc.Status != StatusStConfirmed {
		t.Fatalf("document status = %d, want still confirmed", doc.Status)
	}
}

func TestSettleStocktakeRejectsStaleZeroDiffLine(t *testing.T) {
	doc := &Document{
		AuditableModel: types.AuditableModel{BaseModel: types.BaseModel{ID: 904}},
		DocNo:          "PD20260610005",
		DocType:        DocTypeStocktake,
		Status:         StatusStConfirmed,
		WarehouseID:    12,
	}
	docRepo := &docRepoConcurrencyStub{document: doc}
	lineRepo := &documentLineRepoStub{
		lines: []DocumentLine{{DocumentID: 904, ProductID: 55, SystemQty: 10, ActualQty: 10, DiffQty: 0}},
	}
	txnRepo := &inventoryTransactionRepoStub{}
	inv := &product.Inventory{WarehouseID: 12, ProductID: 55, Quantity: 8, Status: 1}
	invRepo := &inventoryRepoConcurrencyStub{inventory: inv}
	service := &DocumentService{
		docRepo:  docRepo,
		lineRepo: lineRepo,
		txnRepo:  txnRepo,
		invRepo:  invRepo,
		txRunner: func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) },
		audit:    auditLoggerStub{},
	}

	err := service.SettleStocktake(context.Background(), 77, 12, 904)

	assertAppErrorCode(t, err, types.ErrCodeInvalidState)
	if inv.Quantity != 8 {
		t.Fatalf("inventory quantity = %d, want unchanged 8", inv.Quantity)
	}
	if containsString(invRepo.calls, "update") {
		t.Fatalf("inventory update was called on stale zero-diff line, calls %v", invRepo.calls)
	}
	if len(txnRepo.created) != 0 {
		t.Fatalf("created %d transactions, want none", len(txnRepo.created))
	}
	if containsString(docRepo.calls, "update-doc") {
		t.Fatalf("document update was called on stale zero-diff line, calls %v", docRepo.calls)
	}
	if doc.Status != StatusStConfirmed {
		t.Fatalf("document status = %d, want still confirmed", doc.Status)
	}
}

func TestSettleStocktakeZeroDiffMissingInventoryDoesNotCreateInventory(t *testing.T) {
	doc := &Document{
		AuditableModel: types.AuditableModel{BaseModel: types.BaseModel{ID: 905}},
		DocNo:          "PD20260610006",
		DocType:        DocTypeStocktake,
		Status:         StatusStConfirmed,
		WarehouseID:    12,
	}
	docRepo := &docRepoConcurrencyStub{document: doc}
	lineRepo := &documentLineRepoStub{
		lines: []DocumentLine{{DocumentID: 905, ProductID: 55, SystemQty: 0, ActualQty: 0, DiffQty: 0}},
	}
	txnRepo := &inventoryTransactionRepoStub{}
	invRepo := &inventoryRepoConcurrencyStub{}
	service := &DocumentService{
		docRepo:  docRepo,
		lineRepo: lineRepo,
		txnRepo:  txnRepo,
		invRepo:  invRepo,
		txRunner: func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) },
		audit:    auditLoggerStub{},
	}

	err := service.SettleStocktake(context.Background(), 77, 12, 905)

	if err != nil {
		t.Fatalf("SettleStocktake() error = %v, want nil", err)
	}
	if containsString(invRepo.calls, "create") {
		t.Fatalf("inventory create was called for zero-diff missing inventory, calls %v", invRepo.calls)
	}
	if containsString(invRepo.calls, "update") {
		t.Fatalf("inventory update was called for zero-diff missing inventory, calls %v", invRepo.calls)
	}
	if len(txnRepo.created) != 0 {
		t.Fatalf("created %d transactions, want none", len(txnRepo.created))
	}
	if !containsString(docRepo.calls, "update-doc") {
		t.Fatalf("document update was not called, calls %v", docRepo.calls)
	}
	if doc.Status != StatusStSettled {
		t.Fatalf("document status = %d, want settled", doc.Status)
	}
}

func TestSettleStocktakePositiveDiffMissingInventoryCreatesInventory(t *testing.T) {
	doc := &Document{
		AuditableModel: types.AuditableModel{BaseModel: types.BaseModel{ID: 906}},
		DocNo:          "PD20260610007",
		DocType:        DocTypeStocktake,
		Status:         StatusStConfirmed,
		WarehouseID:    12,
	}
	docRepo := &docRepoConcurrencyStub{document: doc}
	lineRepo := &documentLineRepoStub{
		lines: []DocumentLine{{DocumentID: 906, ProductID: 55, SystemQty: 0, ActualQty: 5, DiffQty: 5}},
	}
	txnRepo := &inventoryTransactionRepoStub{}
	invRepo := &inventoryRepoConcurrencyStub{}
	service := &DocumentService{
		docRepo:  docRepo,
		lineRepo: lineRepo,
		txnRepo:  txnRepo,
		invRepo:  invRepo,
		txRunner: func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) },
		audit:    auditLoggerStub{},
	}

	err := service.SettleStocktake(context.Background(), 77, 12, 906)

	if err != nil {
		t.Fatalf("SettleStocktake() error = %v, want nil", err)
	}
	if !containsString(invRepo.calls, "create") {
		t.Fatalf("inventory create was not called for positive stocktake gain, calls %v", invRepo.calls)
	}
	if invRepo.inventory == nil || invRepo.inventory.Quantity != 5 {
		t.Fatalf("inventory = %#v, want quantity 5", invRepo.inventory)
	}
	if len(txnRepo.created) != 1 {
		t.Fatalf("created %d transactions, want one", len(txnRepo.created))
	}
	if txnRepo.created[0].BeforeQty != 0 || txnRepo.created[0].AfterQty != 5 || txnRepo.created[0].Quantity != 5 {
		t.Fatalf("transaction = %#v, want 0 -> 5 quantity 5", txnRepo.created[0])
	}
}

func TestSettleStocktakeRejectsWhenDiffWouldMakeInventoryNegative(t *testing.T) {
	doc := &Document{
		AuditableModel: types.AuditableModel{BaseModel: types.BaseModel{ID: 903}},
		DocNo:          "PD20260610004",
		DocType:        DocTypeStocktake,
		Status:         StatusStConfirmed,
		WarehouseID:    12,
	}
	docRepo := &docRepoConcurrencyStub{document: doc}
	lineRepo := &documentLineRepoStub{
		lines: []DocumentLine{{DocumentID: 903, ProductID: 55, SystemQty: 2, ActualQty: -3, DiffQty: -5}},
	}
	txnRepo := &inventoryTransactionRepoStub{}
	inv := &product.Inventory{WarehouseID: 12, ProductID: 55, Quantity: 2, Status: 1}
	invRepo := &inventoryRepoConcurrencyStub{inventory: inv}
	service := &DocumentService{
		docRepo:  docRepo,
		lineRepo: lineRepo,
		txnRepo:  txnRepo,
		invRepo:  invRepo,
		txRunner: func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) },
		audit:    auditLoggerStub{},
	}

	err := service.SettleStocktake(context.Background(), 77, 12, 903)

	assertAppErrorCode(t, err, types.ErrCodeInvalidState)
	if inv.Quantity != 2 {
		t.Fatalf("inventory quantity = %d, want unchanged 2", inv.Quantity)
	}
	if containsString(invRepo.calls, "update") {
		t.Fatalf("inventory update was called for negative stocktake result, calls %v", invRepo.calls)
	}
	if len(txnRepo.created) != 0 {
		t.Fatalf("created %d transactions, want none", len(txnRepo.created))
	}
	if containsString(docRepo.calls, "update-doc") {
		t.Fatalf("document update was called for negative stocktake result, calls %v", docRepo.calls)
	}
	if doc.Status != StatusStConfirmed {
		t.Fatalf("document status = %d, want still confirmed", doc.Status)
	}
}

func assertAppErrorCode(t *testing.T, err error, wantCode int) *types.AppError {
	t.Helper()
	var appErr *types.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("error = %v, want AppError code %d", err, wantCode)
	}
	if appErr.Code != wantCode {
		t.Fatalf("error code = %d, want %d (error %v)", appErr.Code, wantCode, err)
	}
	return appErr
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func assertDocumentAuditEntry(t *testing.T, got audit.Entry, action string, resourceID uint, docNo string, userID, warehouseID uint) {
	t.Helper()
	if got.Action != action || got.Resource != audit.ResourceDocument {
		t.Fatalf("entry action/resource = %q/%q, want %q/%q", got.Action, got.Resource, action, audit.ResourceDocument)
	}
	if got.ResourceID == nil || *got.ResourceID != resourceID {
		t.Fatalf("entry resourceID = %v, want %d", got.ResourceID, resourceID)
	}
	if got.DocNo != docNo {
		t.Fatalf("entry docNo = %q, want %q", got.DocNo, docNo)
	}
	if got.Actor.UserID != userID || got.Actor.WarehouseID != warehouseID {
		t.Fatalf("actor = %#v, want user %d warehouse %d", got.Actor, userID, warehouseID)
	}
}

func TestGenerateDocNoLocksSequenceBeforeReadingMax(t *testing.T) {
	dateStr := time.Now().Format("20060102")
	repo := &docRepoConcurrencyStub{
		maxDocNo: fmt.Sprintf("XS%s007", dateStr),
	}
	service := &DocumentService{docRepo: repo}

	docNo, err := service.generateDocNo(context.Background(), DocTypeSales)
	if err != nil {
		t.Fatalf("generateDocNo returned error: %v", err)
	}

	if docNo != fmt.Sprintf("XS%s008", dateStr) {
		t.Fatalf("expected next doc no XS%s008, got %s", dateStr, docNo)
	}
	if !reflect.DeepEqual(repo.calls, []string{"lock", "max"}) {
		t.Fatalf("expected lock before max lookup, got calls %v", repo.calls)
	}
	if repo.lockPrefix != "XS" || repo.lockDateStr != dateStr {
		t.Fatalf("unexpected lock key prefix/date: %q %q", repo.lockPrefix, repo.lockDateStr)
	}
}

func TestCompleteLocksDocumentBeforeStatusCheck(t *testing.T) {
	docRepo := &docRepoConcurrencyStub{
		document: &Document{
			DocNo:       "XS20260609001",
			DocType:     DocTypeSales,
			Status:      StatusDraft,
			WarehouseID: 10,
		},
	}
	lineRepo := &documentLineRepoStub{
		lines: []DocumentLine{{ProductID: 20, Quantity: 2}},
	}
	invRepo := &inventoryRepoConcurrencyStub{
		inventory: &product.Inventory{WarehouseID: 10, ProductID: 20, Quantity: 5},
	}
	txRunner := func(ctx context.Context, fn func(context.Context) error) error {
		return fn(ctx)
	}
	service := &DocumentService{
		docRepo:  docRepo,
		lineRepo: lineRepo,
		txnRepo:  &inventoryTransactionRepoStub{},
		invRepo:  invRepo,
		txRunner: txRunner,
		audit:    auditLoggerStub{},
	}

	err := service.Complete(context.Background(), audit.Actor{UserID: 99}, 10, 1, false)
	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}

	if len(docRepo.calls) == 0 || docRepo.calls[0] != "get-doc-for-update" {
		t.Fatalf("expected Complete to lock document before status check, got calls %v", docRepo.calls)
	}
}

func TestGetOrCreateInventoryLocksItemBeforeReading(t *testing.T) {
	invRepo := &inventoryRepoConcurrencyStub{
		inventory: &product.Inventory{WarehouseID: 10, ProductID: 20, Quantity: 5},
	}
	service := &DocumentService{invRepo: invRepo}

	inv, err := service.getOrCreateInventory(context.Background(), 10, 20, 99)
	if err != nil {
		t.Fatalf("getOrCreateInventory returned error: %v", err)
	}

	if inv.Quantity != 5 {
		t.Fatalf("expected existing quantity 5, got %d", inv.Quantity)
	}
	if !reflect.DeepEqual(invRepo.calls, []string{"lock", "get-for-update"}) {
		t.Fatalf("expected lock and FOR UPDATE read, got calls %v", invRepo.calls)
	}
}

func TestExecuteSalesLocksInventoryBeforeDeducting(t *testing.T) {
	invRepo := &inventoryRepoConcurrencyStub{
		inventory: &product.Inventory{WarehouseID: 10, ProductID: 20, Quantity: 5},
	}
	txnRepo := &inventoryTransactionRepoStub{}
	service := &DocumentService{invRepo: invRepo, txnRepo: txnRepo}

	err := service.executeSales(
		context.Background(),
		&Document{WarehouseID: 10, DocNo: "XS20260609001"},
		[]DocumentLine{{ProductID: 20, Quantity: 2}},
		99,
		time.Now(),
	)
	if err != nil {
		t.Fatalf("executeSales returned error: %v", err)
	}

	if invRepo.inventory.Quantity != 3 {
		t.Fatalf("expected quantity 3 after sale, got %d", invRepo.inventory.Quantity)
	}
	if !reflect.DeepEqual(invRepo.calls, []string{"lock", "get-for-update", "update"}) {
		t.Fatalf("expected lock, FOR UPDATE read, and update, got calls %v", invRepo.calls)
	}
	if len(txnRepo.created) != 1 {
		t.Fatalf("expected 1 inventory transaction, got %d", len(txnRepo.created))
	}
}

func TestExecuteReturnLocksReturnQuantityBeforeSummingReturnedQty(t *testing.T) {
	docRepo := &docRepoConcurrencyStub{
		document: &Document{
			AuditableModel: types.AuditableModel{BaseModel: types.BaseModel{ID: 100}},
			DocType:        DocTypeSales,
			Status:         StatusCompleted,
			WarehouseID:    10,
		},
	}
	lineRepo := &documentLineRepoStub{
		lines: []DocumentLine{{DocumentID: 100, ProductID: 20, Quantity: 5}},
		calls: &docRepo.calls,
	}
	invRepo := &inventoryRepoConcurrencyStub{
		inventory: &product.Inventory{WarehouseID: 10, ProductID: 20, Quantity: 1},
	}
	txnRepo := &inventoryTransactionRepoStub{}
	service := &DocumentService{
		docRepo:  docRepo,
		lineRepo: lineRepo,
		invRepo:  invRepo,
		txnRepo:  txnRepo,
	}

	err := service.executeReturn(
		context.Background(),
		&Document{
			AuditableModel: types.AuditableModel{BaseModel: types.BaseModel{ID: 200}},
			DocNo:          "XT20260609001",
			DocType:        DocTypeReturn,
			RefDocID:       100,
			WarehouseID:    10,
		},
		[]DocumentLine{{ProductID: 20, ProductCode: "P20", Quantity: 2}},
		99,
		time.Now(),
	)
	if err != nil {
		t.Fatalf("executeReturn returned error: %v", err)
	}

	if docRepo.lockReturnRefDocID != 100 || docRepo.lockReturnProductID != 20 {
		t.Fatalf("expected return quantity lock for ref doc 100 product 20, got ref doc %d product %d",
			docRepo.lockReturnRefDocID, docRepo.lockReturnProductID)
	}
	if !reflect.DeepEqual(docRepo.calls, []string{"get-doc", "lock-return", "sum-returned"}) {
		t.Fatalf("expected return quantity lock before returned quantity sum, got calls %v", docRepo.calls)
	}
}

func TestExecuteTransferLocksInventoryItemsInDeterministicOrderBeforeReading(t *testing.T) {
	invRepo := &inventoryRepoConcurrencyStub{
		inventories: map[string]*product.Inventory{
			inventoryKey(20, 30): {WarehouseID: 20, ProductID: 30, Quantity: 5},
			inventoryKey(10, 30): {WarehouseID: 10, ProductID: 30, Quantity: 1},
		},
	}
	txnRepo := &inventoryTransactionRepoStub{}
	service := &DocumentService{invRepo: invRepo, txnRepo: txnRepo}

	err := service.executeTransfer(
		context.Background(),
		&Document{
			AuditableModel: types.AuditableModel{BaseModel: types.BaseModel{ID: 300}},
			DocNo:          "DB20260609001",
			DocType:        DocTypeTransfer,
			WarehouseID:    20,
			ToWarehouseID:  10,
		},
		[]DocumentLine{{ProductID: 30, Quantity: 2}},
		99,
		time.Now(),
	)
	if err != nil {
		t.Fatalf("executeTransfer returned error: %v", err)
	}

	expectedFirstCalls := []string{"lock:10:30", "lock:20:30"}
	if len(invRepo.detailedCalls) < len(expectedFirstCalls) {
		t.Fatalf("expected at least %d inventory calls, got %v", len(expectedFirstCalls), invRepo.detailedCalls)
	}
	if !reflect.DeepEqual(invRepo.detailedCalls[:2], expectedFirstCalls) {
		t.Fatalf("expected deterministic pre-lock order %v before inventory reads, got calls %v",
			expectedFirstCalls, invRepo.detailedCalls)
	}
}
