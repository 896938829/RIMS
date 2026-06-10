// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package document

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"rims-go/internal/db"
	"rims-go/internal/modules/audit"
	"rims-go/internal/modules/product"
	"rims-go/internal/types"
)

// AuditLogger is the narrow contract the document service depends on for
// emitting audit records. It is satisfied structurally by *audit.AuditService,
// so this package does not take a hard dependency on the concrete type.
type AuditLogger interface {
	Log(ctx context.Context, e audit.Entry) error
}

// DocumentService handles document and inventory transaction business logic.
type DocumentService struct {
	docRepo     DocumentRepository
	lineRepo    DocumentLineRepository
	txnRepo     InventoryTransactionRepository
	invRepo     product.InventoryRepository
	nonStdRepo  product.NonStdInventoryRepository
	productRepo product.ProductRepository
	txRunner    db.TxRunner
	audit       AuditLogger
}

// NewDocumentService creates a new DocumentService. The auditLogger is
// required: Complete emits an audit record from inside its business
// transaction and cannot silently drop it.
func NewDocumentService(
	docRepo DocumentRepository,
	lineRepo DocumentLineRepository,
	txnRepo InventoryTransactionRepository,
	invRepo product.InventoryRepository,
	nonStdRepo product.NonStdInventoryRepository,
	productRepo product.ProductRepository,
	txRunner db.TxRunner,
	auditLogger AuditLogger,
) *DocumentService {
	return &DocumentService{
		docRepo:     docRepo,
		lineRepo:    lineRepo,
		txnRepo:     txnRepo,
		invRepo:     invRepo,
		nonStdRepo:  nonStdRepo,
		productRepo: productRepo,
		txRunner:    txRunner,
		audit:       auditLogger,
	}
}

// Create creates a new draft document with its line items.
func (s *DocumentService) Create(ctx context.Context, userID, warehouseID uint, req CreateDocumentRequest) (*DocumentResponse, error) {
	if err := s.validateCreateRequest(req); err != nil {
		return nil, err
	}

	var resp *DocumentResponse
	err := s.txRunner(ctx, func(txCtx context.Context) error {
		// Generate document number
		docNo, err := s.generateDocNo(txCtx, req.DocType)
		if err != nil {
			return types.ErrSystem(err)
		}

		// Resolve ref doc number for display
		var refDocNo string
		if req.RefDocID > 0 {
			refDoc, err := s.docRepo.GetByID(txCtx, req.RefDocID)
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return types.ErrNotFound("关联单据")
				}
				return types.ErrSystem(err)
			}
			refDocNo = refDoc.DocNo
		}

		doc := &Document{
			DocNo:         docNo,
			DocType:       req.DocType,
			Status:        StatusDraft,
			WarehouseID:   warehouseID,
			ToWarehouseID: req.ToWarehouseID,
			RefDocID:      req.RefDocID,
			RefDocNo:      refDocNo,
			Remark:        strings.TrimSpace(req.Remark),
		}
		doc.CreatedBy = userID
		doc.UpdatedBy = userID

		if err := s.docRepo.Create(txCtx, doc); err != nil {
			return types.ErrSystem(err)
		}

		// Build line items with denormalized product info
		lines, err := s.buildLines(txCtx, doc, req.Lines, warehouseID)
		if err != nil {
			return err
		}

		if err := s.lineRepo.CreateBatch(txCtx, lines); err != nil {
			return types.ErrSystem(err)
		}

		docID := doc.ID
		if err := s.audit.Log(txCtx, audit.Entry{
			Actor: audit.Actor{
				UserID:      userID,
				WarehouseID: warehouseID,
			},
			Action:      audit.ActionCreate,
			Resource:    audit.ResourceDocument,
			ResourceID:  &docID,
			DocNo:       doc.DocNo,
			Description: fmt.Sprintf("创建单据 %s", doc.DocNo),
			Before: map[string]any{
				"status":      int8(0),
				"docType":     doc.DocType,
				"warehouseID": doc.WarehouseID,
			},
			After: map[string]any{
				"status":      doc.Status,
				"docType":     doc.DocType,
				"warehouseID": doc.WarehouseID,
				"lineCount":   len(lines),
			},
		}); err != nil {
			return err
		}

		r := ToDocumentResponse(doc)
		resp = &r
		return nil
	})
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// Get returns a document with all its line items.
func (s *DocumentService) Get(ctx context.Context, warehouseID, id uint) (*DocumentDetailResponse, error) {
	doc, err := s.docRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, types.ErrNotFound("单据")
		}
		return nil, types.ErrSystem(err)
	}
	if doc.WarehouseID != warehouseID {
		return nil, types.ErrNotFound("单据")
	}

	lines, err := s.lineRepo.ListByDocumentID(ctx, doc.ID)
	if err != nil {
		return nil, types.ErrSystem(err)
	}

	lineResps := make([]DocumentLineResponse, len(lines))
	for i := range lines {
		lineResps[i] = ToDocumentLineResponse(&lines[i])
	}

	return &DocumentDetailResponse{
		DocumentResponse: ToDocumentResponse(doc),
		Lines:            lineResps,
	}, nil
}

// List returns a paginated list of documents filtered by warehouse and optional type.
func (s *DocumentService) List(ctx context.Context, warehouseID uint, docType int8, page types.PageRequest) (types.PageResult, error) {
	docs, total, err := s.docRepo.List(ctx, warehouseID, docType, page)
	if err != nil {
		return types.PageResult{}, types.ErrSystem(err)
	}

	resps := make([]DocumentResponse, len(docs))
	for i := range docs {
		resps[i] = ToDocumentResponse(&docs[i])
	}

	return types.NewPageResult(page, resps, total), nil
}

// Complete transitions a draft document to completed and executes inventory
// changes. The audit record written at the end of the tx is guaranteed to
// commit or roll back atomically with the business write (requirements §5.4 /
// §8.2): if any step here — or the audit insert itself — returns an error,
// the entire transaction is rolled back and no audit row persists.
func (s *DocumentService) Complete(ctx context.Context, actor audit.Actor, warehouseID, id uint, isAdmin bool) error {
	userID := actor.UserID
	return s.txRunner(ctx, func(txCtx context.Context) error {
		doc, err := s.docRepo.GetByIDForUpdate(txCtx, id)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return types.ErrNotFound("单据")
			}
			return types.ErrSystem(err)
		}
		if doc.WarehouseID != warehouseID {
			return types.ErrNotFound("单据")
		}
		if doc.Status != StatusDraft {
			return types.ErrInvalidState("单据状态不允许此操作")
		}

		lines, err := s.lineRepo.ListByDocumentID(txCtx, doc.ID)
		if err != nil {
			return types.ErrSystem(err)
		}
		if len(lines) == 0 {
			return types.ErrValidation("单据无明细行")
		}

		now := time.Now()
		beforeStatus := doc.Status

		switch doc.DocType {
		case DocTypeInbound:
			if !isAdmin {
				return types.ErrForbidden()
			}
			if err := s.executeInbound(txCtx, doc, lines, userID, now); err != nil {
				return err
			}
		case DocTypeSales:
			if err := s.executeSales(txCtx, doc, lines, userID, now); err != nil {
				return err
			}
		case DocTypeReturn:
			if err := s.executeReturn(txCtx, doc, lines, userID, now); err != nil {
				return err
			}
		case DocTypeTransfer:
			if !isAdmin {
				return types.ErrForbidden()
			}
			if err := s.executeTransfer(txCtx, doc, lines, userID, now); err != nil {
				return err
			}
		case DocTypeConversion:
			if !isAdmin {
				return types.ErrForbidden()
			}
			if err := s.executeConversion(txCtx, doc, lines, userID, now); err != nil {
				return err
			}
		case DocTypeStocktake:
			return types.ErrInvalidState("盘点单请使用确认和结转操作")
		default:
			return types.ErrValidation("未知单据类型")
		}

		doc.Status = StatusCompleted
		doc.OperatedAt = &now
		doc.UpdatedBy = userID
		if err := s.docRepo.Update(txCtx, doc); err != nil {
			return types.ErrSystem(err)
		}

		docID := doc.ID
		return s.audit.Log(txCtx, audit.Entry{
			Actor:       actor,
			Action:      audit.ActionComplete,
			Resource:    audit.ResourceDocument,
			ResourceID:  &docID,
			DocNo:       doc.DocNo,
			Description: fmt.Sprintf("完成单据 %s", doc.DocNo),
			Before: map[string]any{
				"status":    beforeStatus,
				"docType":   doc.DocType,
				"lineCount": len(lines),
			},
			After: map[string]any{
				"status":     doc.Status,
				"operatedAt": now,
			},
		})
	})
}

// ConfirmStocktake transitions a stocktake document from recording to confirmed.
func (s *DocumentService) ConfirmStocktake(ctx context.Context, userID, warehouseID, id uint) error {
	return s.txRunner(ctx, func(txCtx context.Context) error {
		doc, err := s.docRepo.GetByIDForUpdate(txCtx, id)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return types.ErrNotFound("单据")
			}
			return types.ErrSystem(err)
		}
		if doc.WarehouseID != warehouseID {
			return types.ErrNotFound("单据")
		}
		if doc.DocType != DocTypeStocktake {
			return types.ErrValidation("非盘点单不支持此操作")
		}
		if doc.Status != StatusStRecording {
			return types.ErrInvalidState("单据状态不允许确认")
		}

		beforeStatus := doc.Status
		doc.Status = StatusStConfirmed
		doc.UpdatedBy = userID
		if err := s.docRepo.Update(txCtx, doc); err != nil {
			return types.ErrSystem(err)
		}

		docID := doc.ID
		return s.audit.Log(txCtx, audit.Entry{
			Actor: audit.Actor{
				UserID:      userID,
				WarehouseID: warehouseID,
			},
			Action:      audit.ActionConfirm,
			Resource:    audit.ResourceDocument,
			ResourceID:  &docID,
			DocNo:       doc.DocNo,
			Description: fmt.Sprintf("确认盘点单 %s", doc.DocNo),
			Before: map[string]any{
				"status":      beforeStatus,
				"docType":     doc.DocType,
				"warehouseID": doc.WarehouseID,
			},
			After: map[string]any{
				"status":      doc.Status,
				"docType":     doc.DocType,
				"warehouseID": doc.WarehouseID,
			},
		})
	})
}

// SettleStocktake transitions a confirmed stocktake to settled and applies inventory diffs.
func (s *DocumentService) SettleStocktake(ctx context.Context, userID, warehouseID, id uint) error {
	return s.txRunner(ctx, func(txCtx context.Context) error {
		doc, err := s.docRepo.GetByIDForUpdate(txCtx, id)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return types.ErrNotFound("单据")
			}
			return types.ErrSystem(err)
		}
		if doc.WarehouseID != warehouseID {
			return types.ErrNotFound("单据")
		}
		if doc.DocType != DocTypeStocktake {
			return types.ErrValidation("非盘点单不支持此操作")
		}
		if doc.Status != StatusStConfirmed {
			return types.ErrInvalidState("单据状态不允许结转")
		}

		lines, err := s.lineRepo.ListByDocumentID(txCtx, doc.ID)
		if err != nil {
			return types.ErrSystem(err)
		}

		now := time.Now()
		for _, line := range lines {
			if line.ProductID == 0 {
				continue
			}

			inv, err := s.getInventoryForUpdate(txCtx, doc.WarehouseID, line.ProductID)
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					if line.SystemQty != 0 {
						return types.ErrInvalidState("库存已变化，请重新盘点后再结转")
					}
					if line.DiffQty == 0 {
						continue
					}
					inv = &product.Inventory{
						WarehouseID: doc.WarehouseID,
						ProductID:   line.ProductID,
						Quantity:    0,
						Status:      1,
					}
					inv.CreatedBy = userID
					inv.UpdatedBy = userID
					if err := s.invRepo.Create(txCtx, inv); err != nil {
						return types.ErrSystem(err)
					}
				} else {
					return types.ErrSystem(err)
				}
			}

			beforeQty := inv.Quantity
			if beforeQty != line.SystemQty {
				return types.ErrInvalidState("库存已变化，请重新盘点后再结转")
			}
			if line.DiffQty == 0 {
				continue
			}
			afterQty := beforeQty + line.DiffQty // DiffQty can be negative (loss) or positive (gain)
			if afterQty < 0 {
				return types.ErrInvalidState("盘点差异会导致库存为负，请重新盘点后再结转")
			}
			inv.Quantity = afterQty
			inv.UpdatedBy = userID
			s.updateInventoryStatus(inv)

			if err := s.invRepo.Update(txCtx, inv); err != nil {
				return types.ErrSystem(err)
			}

			dir := DirectionIn
			qty := line.DiffQty
			if line.DiffQty < 0 {
				dir = DirectionOut
				qty = -line.DiffQty
			}

			if err := s.txnRepo.Create(txCtx, &InventoryTransaction{
				WarehouseID: doc.WarehouseID,
				ProductID:   line.ProductID,
				DocID:       doc.ID,
				DocNo:       doc.DocNo,
				DocType:     doc.DocType,
				Direction:   dir,
				Quantity:    qty,
				BeforeQty:   beforeQty,
				AfterQty:    inv.Quantity,
				OperatorID:  userID,
				OperatedAt:  now,
			}); err != nil {
				return types.ErrSystem(err)
			}
		}

		doc.Status = StatusStSettled
		doc.OperatedAt = &now
		doc.UpdatedBy = userID
		beforeStatus := StatusStConfirmed
		if err := s.docRepo.Update(txCtx, doc); err != nil {
			return types.ErrSystem(err)
		}

		docID := doc.ID
		return s.audit.Log(txCtx, audit.Entry{
			Actor: audit.Actor{
				UserID:      userID,
				WarehouseID: warehouseID,
			},
			Action:      audit.ActionSettle,
			Resource:    audit.ResourceDocument,
			ResourceID:  &docID,
			DocNo:       doc.DocNo,
			Description: fmt.Sprintf("结转盘点单 %s", doc.DocNo),
			Before: map[string]any{
				"status":      beforeStatus,
				"docType":     doc.DocType,
				"warehouseID": doc.WarehouseID,
				"lineCount":   len(lines),
			},
			After: map[string]any{
				"status":      doc.Status,
				"docType":     doc.DocType,
				"warehouseID": doc.WarehouseID,
				"operatedAt":  now,
			},
		})
	})
}

// ListTransactions returns a paginated inventory transaction log for a warehouse.
func (s *DocumentService) ListTransactions(ctx context.Context, warehouseID uint, page types.PageRequest) (types.PageResult, error) {
	txns, total, err := s.txnRepo.ListByWarehouse(ctx, warehouseID, page)
	if err != nil {
		return types.PageResult{}, types.ErrSystem(err)
	}

	resps := make([]TransactionResponse, len(txns))
	for i := range txns {
		resps[i] = ToTransactionResponse(&txns[i])
	}

	return types.NewPageResult(page, resps, total), nil
}

// --- Internal Execution Methods ---

func (s *DocumentService) executeInbound(ctx context.Context, doc *Document, lines []DocumentLine, userID uint, now time.Time) error {
	for _, line := range lines {
		inv, err := s.getOrCreateInventory(ctx, doc.WarehouseID, line.ProductID, userID)
		if err != nil {
			return err
		}

		beforeQty := inv.Quantity
		inv.Quantity += line.Quantity
		inv.UpdatedBy = userID
		s.updateInventoryStatus(inv)

		if err := s.invRepo.Update(ctx, inv); err != nil {
			return types.ErrSystem(err)
		}

		if err := s.txnRepo.Create(ctx, &InventoryTransaction{
			WarehouseID: doc.WarehouseID,
			ProductID:   line.ProductID,
			DocID:       doc.ID,
			DocNo:       doc.DocNo,
			DocType:     doc.DocType,
			Direction:   DirectionIn,
			Quantity:    line.Quantity,
			BeforeQty:   beforeQty,
			AfterQty:    inv.Quantity,
			OperatorID:  userID,
			OperatedAt:  now,
		}); err != nil {
			return types.ErrSystem(err)
		}
	}
	return nil
}

func (s *DocumentService) executeSales(ctx context.Context, doc *Document, lines []DocumentLine, userID uint, now time.Time) error {
	for _, line := range lines {
		inv, err := s.getInventoryForUpdate(ctx, doc.WarehouseID, line.ProductID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return types.ErrInsufficientStock()
			}
			return types.ErrSystem(err)
		}
		if inv.Quantity < line.Quantity {
			return types.ErrInsufficientStock()
		}

		beforeQty := inv.Quantity
		inv.Quantity -= line.Quantity
		inv.UpdatedBy = userID
		s.updateInventoryStatus(inv)

		if err := s.invRepo.Update(ctx, inv); err != nil {
			return types.ErrSystem(err)
		}

		if err := s.txnRepo.Create(ctx, &InventoryTransaction{
			WarehouseID: doc.WarehouseID,
			ProductID:   line.ProductID,
			DocID:       doc.ID,
			DocNo:       doc.DocNo,
			DocType:     doc.DocType,
			Direction:   DirectionOut,
			Quantity:    line.Quantity,
			BeforeQty:   beforeQty,
			AfterQty:    inv.Quantity,
			OperatorID:  userID,
			OperatedAt:  now,
		}); err != nil {
			return types.ErrSystem(err)
		}
	}
	return nil
}

func (s *DocumentService) executeReturn(ctx context.Context, doc *Document, lines []DocumentLine, userID uint, now time.Time) error {
	if doc.RefDocID == 0 {
		return types.ErrValidation("退货单必须关联原销售单")
	}

	refDoc, err := s.docRepo.GetByID(ctx, doc.RefDocID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return types.ErrNotFound("原销售单")
		}
		return types.ErrSystem(err)
	}
	if refDoc.DocType != DocTypeSales {
		return types.ErrValidation("关联单据不是销售单")
	}
	if refDoc.Status != StatusCompleted {
		return types.ErrInvalidState("原销售单未完成")
	}
	if refDoc.WarehouseID != doc.WarehouseID {
		return types.ErrValidation("退货仓库与原销售单仓库不一致")
	}

	// Get original sales lines for quantity validation
	refLines, err := s.lineRepo.ListByDocumentID(ctx, refDoc.ID)
	if err != nil {
		return types.ErrSystem(err)
	}
	refLineMap := make(map[uint]int) // productID -> original quantity
	for _, rl := range refLines {
		refLineMap[rl.ProductID] += rl.Quantity
	}

	for _, line := range lines {
		originalQty, ok := refLineMap[line.ProductID]
		if !ok {
			return types.ErrValidation(fmt.Sprintf("商品(%s)不在原销售单中", line.ProductCode))
		}

		if err := s.docRepo.LockReturnQuantity(ctx, doc.RefDocID, line.ProductID); err != nil {
			return types.ErrSystem(err)
		}

		alreadyReturned, err := s.lineRepo.SumReturnedQty(ctx, doc.RefDocID, line.ProductID)
		if err != nil {
			return types.ErrSystem(err)
		}
		if alreadyReturned+line.Quantity > originalQty {
			return types.ErrValidation(fmt.Sprintf("商品(%s)退货数量(%d)超过可退数量(%d)",
				line.ProductCode, line.Quantity, originalQty-alreadyReturned))
		}

		inv, err := s.getOrCreateInventory(ctx, doc.WarehouseID, line.ProductID, userID)
		if err != nil {
			return err
		}

		beforeQty := inv.Quantity
		inv.Quantity += line.Quantity
		inv.UpdatedBy = userID
		s.updateInventoryStatus(inv)

		if err := s.invRepo.Update(ctx, inv); err != nil {
			return types.ErrSystem(err)
		}

		if err := s.txnRepo.Create(ctx, &InventoryTransaction{
			WarehouseID: doc.WarehouseID,
			ProductID:   line.ProductID,
			DocID:       doc.ID,
			DocNo:       doc.DocNo,
			DocType:     doc.DocType,
			Direction:   DirectionIn,
			Quantity:    line.Quantity,
			BeforeQty:   beforeQty,
			AfterQty:    inv.Quantity,
			OperatorID:  userID,
			OperatedAt:  now,
		}); err != nil {
			return types.ErrSystem(err)
		}
	}
	return nil
}

func (s *DocumentService) executeTransfer(ctx context.Context, doc *Document, lines []DocumentLine, userID uint, now time.Time) error {
	if doc.ToWarehouseID == 0 {
		return types.ErrValidation("调拨单必须指定目标仓库")
	}
	if doc.ToWarehouseID == doc.WarehouseID {
		return types.ErrValidation("调拨目标仓库不能与源仓库相同")
	}

	if err := s.lockInventoryItems(ctx, transferInventoryLockKeys(doc, lines)...); err != nil {
		return types.ErrSystem(err)
	}

	for _, line := range lines {
		// Deduct from source warehouse
		srcInv, err := s.getInventoryForUpdate(ctx, doc.WarehouseID, line.ProductID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return types.ErrInsufficientStock()
			}
			return types.ErrSystem(err)
		}
		if srcInv.Quantity < line.Quantity {
			return types.ErrInsufficientStock()
		}

		srcBefore := srcInv.Quantity
		srcInv.Quantity -= line.Quantity
		srcInv.UpdatedBy = userID
		s.updateInventoryStatus(srcInv)

		if err := s.invRepo.Update(ctx, srcInv); err != nil {
			return types.ErrSystem(err)
		}

		if err := s.txnRepo.Create(ctx, &InventoryTransaction{
			WarehouseID: doc.WarehouseID,
			ProductID:   line.ProductID,
			DocID:       doc.ID,
			DocNo:       doc.DocNo,
			DocType:     doc.DocType,
			Direction:   DirectionOut,
			Quantity:    line.Quantity,
			BeforeQty:   srcBefore,
			AfterQty:    srcInv.Quantity,
			OperatorID:  userID,
			OperatedAt:  now,
		}); err != nil {
			return types.ErrSystem(err)
		}

		// Add to target warehouse
		dstInv, err := s.getOrCreateInventory(ctx, doc.ToWarehouseID, line.ProductID, userID)
		if err != nil {
			return err
		}

		dstBefore := dstInv.Quantity
		dstInv.Quantity += line.Quantity
		dstInv.UpdatedBy = userID
		s.updateInventoryStatus(dstInv)

		if err := s.invRepo.Update(ctx, dstInv); err != nil {
			return types.ErrSystem(err)
		}

		if err := s.txnRepo.Create(ctx, &InventoryTransaction{
			WarehouseID: doc.ToWarehouseID,
			ProductID:   line.ProductID,
			DocID:       doc.ID,
			DocNo:       doc.DocNo,
			DocType:     doc.DocType,
			Direction:   DirectionIn,
			Quantity:    line.Quantity,
			BeforeQty:   dstBefore,
			AfterQty:    dstInv.Quantity,
			OperatorID:  userID,
			OperatedAt:  now,
		}); err != nil {
			return types.ErrSystem(err)
		}
	}
	return nil
}

func (s *DocumentService) executeConversion(ctx context.Context, doc *Document, lines []DocumentLine, userID uint, now time.Time) error {
	for _, line := range lines {
		// Validate non-std inventory
		ns, err := s.nonStdRepo.GetByIDForUpdate(ctx, line.NonStdInvID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return types.ErrNotFound("非标库存")
			}
			return types.ErrSystem(err)
		}
		if ns.WarehouseID != doc.WarehouseID {
			return types.ErrNotFound("非标库存")
		}
		if ns.Status == 0 || ns.Status == 3 {
			return types.ErrInvalidState("非标库存状态不允许转换")
		}

		remaining := ns.Quantity - ns.ConvertedQty
		if line.Quantity > remaining {
			return types.ErrValidation(fmt.Sprintf("转换数量(%d)超过剩余量(%d)", line.Quantity, remaining))
		}

		// Validate target product
		p, err := s.productRepo.GetByID(ctx, line.ProductID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return types.ErrNotFound("目标商品")
			}
			return types.ErrSystem(err)
		}
		if p.Status != 1 {
			return types.ErrInvalidState("目标商品已停用")
		}

		// Get or create standard inventory
		inv, err := s.getOrCreateInventory(ctx, doc.WarehouseID, line.ProductID, userID)
		if err != nil {
			return err
		}

		beforeQty := inv.Quantity
		inv.Quantity += line.Quantity
		inv.UpdatedBy = userID
		s.updateInventoryStatus(inv)

		if err := s.invRepo.Update(ctx, inv); err != nil {
			return types.ErrSystem(err)
		}

		// Update non-std inventory
		ns.ConvertedQty += line.Quantity
		ns.UpdatedBy = userID
		if ns.ConvertedQty == ns.Quantity {
			ns.Status = 3 // fully converted
		} else {
			ns.Status = 2 // partially converted
		}
		if err := s.nonStdRepo.Update(ctx, ns); err != nil {
			return types.ErrSystem(err)
		}

		if err := s.txnRepo.Create(ctx, &InventoryTransaction{
			WarehouseID: doc.WarehouseID,
			ProductID:   line.ProductID,
			DocID:       doc.ID,
			DocNo:       doc.DocNo,
			DocType:     doc.DocType,
			Direction:   DirectionIn,
			Quantity:    line.Quantity,
			BeforeQty:   beforeQty,
			AfterQty:    inv.Quantity,
			OperatorID:  userID,
			OperatedAt:  now,
		}); err != nil {
			return types.ErrSystem(err)
		}
	}
	return nil
}

// --- Helper Methods ---

func (s *DocumentService) validateCreateRequest(req CreateDocumentRequest) error {
	switch req.DocType {
	case DocTypeReturn:
		if req.RefDocID == 0 {
			return types.ErrValidation("退货单必须关联原销售单")
		}
	case DocTypeTransfer:
		if req.ToWarehouseID == 0 {
			return types.ErrValidation("调拨单必须指定目标仓库")
		}
	}

	for i, line := range req.Lines {
		switch req.DocType {
		case DocTypeStocktake:
			if line.ProductID == 0 {
				return types.ErrValidation(fmt.Sprintf("第%d行: 商品不能为空", i+1))
			}
		case DocTypeConversion:
			if line.NonStdInvID == 0 {
				return types.ErrValidation(fmt.Sprintf("第%d行: 非标库存不能为空", i+1))
			}
			if line.ProductID == 0 {
				return types.ErrValidation(fmt.Sprintf("第%d行: 目标商品不能为空", i+1))
			}
			if line.Quantity <= 0 {
				return types.ErrValidation(fmt.Sprintf("第%d行: 转换数量必须大于0", i+1))
			}
		default:
			if line.ProductID == 0 {
				return types.ErrValidation(fmt.Sprintf("第%d行: 商品不能为空", i+1))
			}
			if line.Quantity <= 0 {
				return types.ErrValidation(fmt.Sprintf("第%d行: 数量必须大于0", i+1))
			}
		}
	}
	return nil
}

func (s *DocumentService) buildLines(ctx context.Context, doc *Document, reqLines []CreateDocumentLineRequest, warehouseID uint) ([]DocumentLine, error) {
	lines := make([]DocumentLine, 0, len(reqLines))
	for _, rl := range reqLines {
		line := DocumentLine{
			DocumentID:  doc.ID,
			ProductID:   rl.ProductID,
			NonStdInvID: rl.NonStdInvID,
			Quantity:    rl.Quantity,
			CostPrice:   rl.CostPrice,
			RetailPrice: rl.RetailPrice,
			Remark:      strings.TrimSpace(rl.Remark),
		}

		// Denormalize product info
		if rl.ProductID > 0 {
			p, err := s.productRepo.GetByID(ctx, rl.ProductID)
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil, types.ErrNotFound("商品")
				}
				return nil, types.ErrSystem(err)
			}
			line.ProductCode = p.Code
			line.ProductName = p.Name
			line.Unit = p.Unit
			if line.CostPrice == 0 {
				line.CostPrice = p.CostPrice
			}
			if line.RetailPrice == 0 {
				line.RetailPrice = p.RetailPrice
			}
		}

		// For stocktake: capture system quantity at record time
		if doc.DocType == DocTypeStocktake && rl.ProductID > 0 {
			inv, err := s.invRepo.GetByWarehouseAndProduct(ctx, warehouseID, rl.ProductID)
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, types.ErrSystem(err)
			}
			if inv != nil {
				line.SystemQty = inv.Quantity
			}
			line.ActualQty = rl.ActualQty
			line.DiffQty = rl.ActualQty - line.SystemQty
			line.Quantity = line.DiffQty
		}

		lines = append(lines, line)
	}
	return lines, nil
}

func (s *DocumentService) generateDocNo(ctx context.Context, docType int8) (string, error) {
	prefix := docNoPrefixes[docType]
	dateStr := time.Now().Format("20060102")

	if err := s.docRepo.LockDocNoSequence(ctx, prefix, dateStr); err != nil {
		return "", err
	}

	maxDocNo, err := s.docRepo.GetMaxDocNo(ctx, prefix, dateStr)
	if err != nil {
		return "", err
	}

	seq := 1
	if maxDocNo != "" {
		// Extract the 3-digit sequence from the end
		seqStr := maxDocNo[len(prefix)+8:] // prefix + 8-digit date
		if n, err := strconv.Atoi(seqStr); err == nil {
			seq = n + 1
		}
	}

	return fmt.Sprintf("%s%s%03d", prefix, dateStr, seq), nil
}

type inventoryLockKey struct {
	warehouseID uint
	productID   uint
}

func transferInventoryLockKeys(doc *Document, lines []DocumentLine) []inventoryLockKey {
	keys := make([]inventoryLockKey, 0, len(lines)*2)
	for _, line := range lines {
		keys = append(keys,
			inventoryLockKey{warehouseID: doc.WarehouseID, productID: line.ProductID},
			inventoryLockKey{warehouseID: doc.ToWarehouseID, productID: line.ProductID},
		)
	}
	return keys
}

func (s *DocumentService) lockInventoryItems(ctx context.Context, keys ...inventoryLockKey) error {
	keys = sortedUniqueInventoryLockKeys(keys)
	for _, key := range keys {
		if err := s.invRepo.LockItem(ctx, key.warehouseID, key.productID); err != nil {
			return err
		}
	}
	return nil
}

func sortedUniqueInventoryLockKeys(keys []inventoryLockKey) []inventoryLockKey {
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].warehouseID == keys[j].warehouseID {
			return keys[i].productID < keys[j].productID
		}
		return keys[i].warehouseID < keys[j].warehouseID
	})

	unique := keys[:0]
	for _, key := range keys {
		if len(unique) == 0 || unique[len(unique)-1] != key {
			unique = append(unique, key)
		}
	}
	return unique
}

func (s *DocumentService) getInventoryForUpdate(ctx context.Context, warehouseID, productID uint) (*product.Inventory, error) {
	if err := s.invRepo.LockItem(ctx, warehouseID, productID); err != nil {
		return nil, err
	}
	return s.invRepo.GetByWarehouseAndProductForUpdate(ctx, warehouseID, productID)
}

func (s *DocumentService) getOrCreateInventory(ctx context.Context, warehouseID, productID, userID uint) (*product.Inventory, error) {
	inv, err := s.getInventoryForUpdate(ctx, warehouseID, productID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			inv = &product.Inventory{
				WarehouseID: warehouseID,
				ProductID:   productID,
				Quantity:    0,
				Status:      1,
			}
			inv.CreatedBy = userID
			inv.UpdatedBy = userID
			if err := s.invRepo.Create(ctx, inv); err != nil {
				return nil, types.ErrSystem(err)
			}
			return inv, nil
		}
		return nil, types.ErrSystem(err)
	}
	return inv, nil
}

func (s *DocumentService) updateInventoryStatus(inv *product.Inventory) {
	if inv.AlertThreshold > 0 && inv.Quantity <= inv.AlertThreshold {
		inv.Status = 2 // low stock
	} else if inv.Status == 2 && (inv.AlertThreshold == 0 || inv.Quantity > inv.AlertThreshold) {
		inv.Status = 1 // normal
	}
}
