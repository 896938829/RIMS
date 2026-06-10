// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package document

import (
	"context"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"rims-go/internal/db"
	"rims-go/internal/types"
)

// --- Document Repository ---

// DocumentRepository defines data access operations for documents.
type DocumentRepository interface {
	Create(ctx context.Context, doc *Document) error
	GetByID(ctx context.Context, id uint) (*Document, error)
	GetByIDForUpdate(ctx context.Context, id uint) (*Document, error)
	GetByDocNo(ctx context.Context, docNo string) (*Document, error)
	List(ctx context.Context, warehouseID uint, docType int8, page types.PageRequest) ([]Document, int64, error)
	Update(ctx context.Context, doc *Document) error
	LockDocNoSequence(ctx context.Context, prefix string, dateStr string) error
	LockReturnQuantity(ctx context.Context, refDocID uint, productID uint) error
	GetMaxDocNo(ctx context.Context, prefix string, dateStr string) (string, error)
}

type documentRepo struct {
	gormDB *gorm.DB
}

// NewDocumentRepository creates a new DocumentRepository backed by GORM.
func NewDocumentRepository(gormDB *gorm.DB) DocumentRepository {
	return &documentRepo{gormDB: gormDB}
}

func (r *documentRepo) getDB(ctx context.Context) *gorm.DB {
	return db.FromCtx(ctx, r.gormDB)
}

func (r *documentRepo) Create(ctx context.Context, doc *Document) error {
	return r.getDB(ctx).Create(doc).Error
}

func (r *documentRepo) GetByID(ctx context.Context, id uint) (*Document, error) {
	var doc Document
	if err := r.getDB(ctx).First(&doc, id).Error; err != nil {
		return nil, err
	}
	return &doc, nil
}

func (r *documentRepo) GetByIDForUpdate(ctx context.Context, id uint) (*Document, error) {
	var doc Document
	if err := r.getDB(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&doc, id).Error; err != nil {
		return nil, err
	}
	return &doc, nil
}

func (r *documentRepo) GetByDocNo(ctx context.Context, docNo string) (*Document, error) {
	var doc Document
	if err := r.getDB(ctx).Where("doc_no = ?", docNo).First(&doc).Error; err != nil {
		return nil, err
	}
	return &doc, nil
}

func (r *documentRepo) List(ctx context.Context, warehouseID uint, docType int8, page types.PageRequest) ([]Document, int64, error) {
	page.Defaults()
	d := r.getDB(ctx).Model(&Document{}).Where("warehouse_id = ?", warehouseID)

	if docType > 0 {
		d = d.Where("doc_type = ?", docType)
	}
	if page.Keyword != "" {
		kw := "%" + page.Keyword + "%"
		d = d.Where("doc_no LIKE ?", kw)
	}

	var total int64
	if err := d.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var docs []Document
	if err := d.Offset(page.Offset()).Limit(page.PageSize).
		Order("id DESC").
		Find(&docs).Error; err != nil {
		return nil, 0, err
	}

	return docs, total, nil
}

func (r *documentRepo) Update(ctx context.Context, doc *Document) error {
	return r.getDB(ctx).Save(doc).Error
}

func (r *documentRepo) LockDocNoSequence(ctx context.Context, prefix string, dateStr string) error {
	return r.getDB(ctx).
		Exec("SELECT pg_advisory_xact_lock(hashtext(?), hashtext(?))", "rims_document_no", prefix+dateStr).
		Error
}

func (r *documentRepo) LockReturnQuantity(ctx context.Context, refDocID uint, productID uint) error {
	lockKey := fmt.Sprintf("%d:%d", refDocID, productID)
	return r.getDB(ctx).
		Exec("SELECT pg_advisory_xact_lock(hashtext(?), hashtext(?))", "rims_return_quantity", lockKey).
		Error
}

func (r *documentRepo) GetMaxDocNo(ctx context.Context, prefix string, dateStr string) (string, error) {
	var docNo string
	pattern := prefix + dateStr + "%"
	err := r.getDB(ctx).Model(&Document{}).
		Where("doc_no LIKE ?", pattern).
		Order("doc_no DESC").
		Limit(1).
		Pluck("doc_no", &docNo).Error
	if err != nil {
		return "", err
	}
	return docNo, nil
}

// --- DocumentLine Repository ---

// DocumentLineRepository defines data access operations for document lines.
type DocumentLineRepository interface {
	CreateBatch(ctx context.Context, lines []DocumentLine) error
	ListByDocumentID(ctx context.Context, documentID uint) ([]DocumentLine, error)
	SumReturnedQty(ctx context.Context, refDocID uint, productID uint) (int, error)
}

type documentLineRepo struct {
	gormDB *gorm.DB
}

// NewDocumentLineRepository creates a new DocumentLineRepository backed by GORM.
func NewDocumentLineRepository(gormDB *gorm.DB) DocumentLineRepository {
	return &documentLineRepo{gormDB: gormDB}
}

func (r *documentLineRepo) getDB(ctx context.Context) *gorm.DB {
	return db.FromCtx(ctx, r.gormDB)
}

func (r *documentLineRepo) CreateBatch(ctx context.Context, lines []DocumentLine) error {
	return r.getDB(ctx).Create(&lines).Error
}

func (r *documentLineRepo) ListByDocumentID(ctx context.Context, documentID uint) ([]DocumentLine, error) {
	var lines []DocumentLine
	if err := r.getDB(ctx).Where("document_id = ?", documentID).
		Order("id ASC").
		Find(&lines).Error; err != nil {
		return nil, err
	}
	return lines, nil
}

// SumReturnedQty sums the quantity of a product across completed return documents
// that reference the given sales document.
func (r *documentLineRepo) SumReturnedQty(ctx context.Context, refDocID uint, productID uint) (int, error) {
	var total int
	err := r.getDB(ctx).Model(&DocumentLine{}).
		Select("COALESCE(SUM(document_lines.quantity), 0)").
		Joins("JOIN documents ON documents.id = document_lines.document_id").
		Where("documents.ref_doc_id = ? AND documents.doc_type = ? AND documents.status = ? AND documents.deleted_at IS NULL",
			refDocID, DocTypeReturn, StatusCompleted).
		Where("document_lines.product_id = ? AND document_lines.deleted_at IS NULL", productID).
		Scan(&total).Error
	if err != nil {
		return 0, err
	}
	return total, nil
}

// --- InventoryTransaction Repository ---

// InventoryTransactionRepository defines data access operations for inventory transactions.
type InventoryTransactionRepository interface {
	Create(ctx context.Context, txn *InventoryTransaction) error
	CreateBatch(ctx context.Context, txns []InventoryTransaction) error
	ListByWarehouse(ctx context.Context, warehouseID uint, page types.PageRequest) ([]InventoryTransaction, int64, error)
	ListByDocument(ctx context.Context, docID uint) ([]InventoryTransaction, error)
}

type inventoryTransactionRepo struct {
	gormDB *gorm.DB
}

// NewInventoryTransactionRepository creates a new InventoryTransactionRepository backed by GORM.
func NewInventoryTransactionRepository(gormDB *gorm.DB) InventoryTransactionRepository {
	return &inventoryTransactionRepo{gormDB: gormDB}
}

func (r *inventoryTransactionRepo) getDB(ctx context.Context) *gorm.DB {
	return db.FromCtx(ctx, r.gormDB)
}

func (r *inventoryTransactionRepo) Create(ctx context.Context, txn *InventoryTransaction) error {
	return r.getDB(ctx).Create(txn).Error
}

func (r *inventoryTransactionRepo) CreateBatch(ctx context.Context, txns []InventoryTransaction) error {
	return r.getDB(ctx).Create(&txns).Error
}

func (r *inventoryTransactionRepo) ListByWarehouse(ctx context.Context, warehouseID uint, page types.PageRequest) ([]InventoryTransaction, int64, error) {
	page.Defaults()
	d := r.getDB(ctx).Model(&InventoryTransaction{}).Where("warehouse_id = ?", warehouseID)

	if page.Keyword != "" {
		kw := "%" + page.Keyword + "%"
		d = d.Where("doc_no LIKE ?", kw)
	}

	var total int64
	if err := d.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var txns []InventoryTransaction
	if err := d.Offset(page.Offset()).Limit(page.PageSize).
		Order("id DESC").
		Find(&txns).Error; err != nil {
		return nil, 0, err
	}

	return txns, total, nil
}

func (r *inventoryTransactionRepo) ListByDocument(ctx context.Context, docID uint) ([]InventoryTransaction, error) {
	var txns []InventoryTransaction
	if err := r.getDB(ctx).Where("doc_id = ?", docID).
		Order("id ASC").
		Find(&txns).Error; err != nil {
		return nil, err
	}
	return txns, nil
}
