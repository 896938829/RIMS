// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package document

import "time"

// --- Request DTOs ---

// CreateDocumentRequest is the unified creation request for all document types.
type CreateDocumentRequest struct {
	DocType       int8                        `json:"docType" binding:"required,oneof=1 2 3 4 5 6"`
	ToWarehouseID uint                        `json:"toWarehouseId"`
	RefDocID      uint                        `json:"refDocId"`
	Remark        string                      `json:"remark" binding:"max=512"`
	Lines         []CreateDocumentLineRequest `json:"lines" binding:"required,min=1,dive"`
}

// CreateDocumentLineRequest represents a single line item in the creation request.
type CreateDocumentLineRequest struct {
	ProductID   uint    `json:"productId"`
	NonStdInvID uint    `json:"nonStdInvId"`
	Quantity    int     `json:"quantity" binding:"min=0"`
	CostPrice   float64 `json:"costPrice"`
	RetailPrice float64 `json:"retailPrice"`
	ActualQty   int     `json:"actualQty"`
	Remark      string  `json:"remark" binding:"max=255"`
}

// --- Response DTOs ---

// DocumentResponse is the document header in API responses.
type DocumentResponse struct {
	ID            uint       `json:"id"`
	DocNo         string     `json:"docNo"`
	DocType       int8       `json:"docType"`
	DocTypeName   string     `json:"docTypeName"`
	Status        int8       `json:"status"`
	StatusName    string     `json:"statusName"`
	WarehouseID   uint       `json:"warehouseId"`
	ToWarehouseID uint       `json:"toWarehouseId,omitempty"`
	RefDocID      uint       `json:"refDocId,omitempty"`
	RefDocNo      string     `json:"refDocNo,omitempty"`
	Remark        string     `json:"remark"`
	OperatedAt    *time.Time `json:"operatedAt,omitempty"`
	CreatedBy     uint       `json:"createdBy"`
	UpdatedBy     uint       `json:"updatedBy"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
}

// DocumentDetailResponse includes document header and all its lines.
type DocumentDetailResponse struct {
	DocumentResponse
	Lines []DocumentLineResponse `json:"lines"`
}

// DocumentLineResponse represents a line item in API responses.
type DocumentLineResponse struct {
	ID          uint    `json:"id"`
	ProductID   uint    `json:"productId,omitempty"`
	NonStdInvID uint    `json:"nonStdInvId,omitempty"`
	ProductCode string  `json:"productCode"`
	ProductName string  `json:"productName"`
	Quantity    int     `json:"quantity"`
	Unit        string  `json:"unit"`
	CostPrice   float64 `json:"costPrice,omitempty"`
	RetailPrice float64 `json:"retailPrice,omitempty"`
	SystemQty   int     `json:"systemQty,omitempty"`
	ActualQty   int     `json:"actualQty,omitempty"`
	DiffQty     int     `json:"diffQty,omitempty"`
	Remark      string  `json:"remark"`
}

// TransactionResponse represents an inventory transaction in API responses.
type TransactionResponse struct {
	ID          uint      `json:"id"`
	WarehouseID uint      `json:"warehouseId"`
	ProductID   uint      `json:"productId"`
	DocID       uint      `json:"docId"`
	DocNo       string    `json:"docNo"`
	DocType     int8      `json:"docType"`
	DocTypeName string    `json:"docTypeName"`
	Direction   int8      `json:"direction"`
	Quantity    int       `json:"quantity"`
	BeforeQty   int       `json:"beforeQty"`
	AfterQty    int       `json:"afterQty"`
	OperatorID  uint      `json:"operatorId"`
	OperatedAt  time.Time `json:"operatedAt"`
	CreatedAt   time.Time `json:"createdAt"`
}

// --- Converter Functions ---

// ToDocumentResponse converts a Document model to a DocumentResponse.
func ToDocumentResponse(doc *Document) DocumentResponse {
	return DocumentResponse{
		ID:            doc.ID,
		DocNo:         doc.DocNo,
		DocType:       doc.DocType,
		DocTypeName:   DocTypeName(doc.DocType),
		Status:        doc.Status,
		StatusName:    StatusName(doc.DocType, doc.Status),
		WarehouseID:   doc.WarehouseID,
		ToWarehouseID: doc.ToWarehouseID,
		RefDocID:      doc.RefDocID,
		RefDocNo:      doc.RefDocNo,
		Remark:        doc.Remark,
		OperatedAt:    doc.OperatedAt,
		CreatedBy:     doc.CreatedBy,
		UpdatedBy:     doc.UpdatedBy,
		CreatedAt:     doc.CreatedAt,
		UpdatedAt:     doc.UpdatedAt,
	}
}

// ToDocumentLineResponse converts a DocumentLine model to a DocumentLineResponse.
func ToDocumentLineResponse(line *DocumentLine) DocumentLineResponse {
	return DocumentLineResponse{
		ID:          line.ID,
		ProductID:   line.ProductID,
		NonStdInvID: line.NonStdInvID,
		ProductCode: line.ProductCode,
		ProductName: line.ProductName,
		Quantity:    line.Quantity,
		Unit:        line.Unit,
		CostPrice:   line.CostPrice,
		RetailPrice: line.RetailPrice,
		SystemQty:   line.SystemQty,
		ActualQty:   line.ActualQty,
		DiffQty:     line.DiffQty,
		Remark:      line.Remark,
	}
}

// ToTransactionResponse converts an InventoryTransaction model to a TransactionResponse.
func ToTransactionResponse(txn *InventoryTransaction) TransactionResponse {
	return TransactionResponse{
		ID:          txn.ID,
		WarehouseID: txn.WarehouseID,
		ProductID:   txn.ProductID,
		DocID:       txn.DocID,
		DocNo:       txn.DocNo,
		DocType:     txn.DocType,
		DocTypeName: DocTypeName(txn.DocType),
		Direction:   txn.Direction,
		Quantity:    txn.Quantity,
		BeforeQty:   txn.BeforeQty,
		AfterQty:    txn.AfterQty,
		OperatorID:  txn.OperatorID,
		OperatedAt:  txn.OperatedAt,
		CreatedAt:   txn.CreatedAt,
	}
}

// --- Name Helpers ---

// DocTypeName returns the Chinese display name for a document type.
func DocTypeName(t int8) string {
	switch t {
	case DocTypeInbound:
		return "入库单"
	case DocTypeSales:
		return "销售单"
	case DocTypeReturn:
		return "退货单"
	case DocTypeTransfer:
		return "调拨单"
	case DocTypeStocktake:
		return "盘点单"
	case DocTypeConversion:
		return "非标转换单"
	default:
		return "未知"
	}
}

// StatusName returns the Chinese display name for a document status.
func StatusName(docType, status int8) string {
	if docType == DocTypeStocktake {
		switch status {
		case StatusStRecording:
			return "盘点中"
		case StatusStConfirmed:
			return "差异已确认"
		case StatusStSettled:
			return "已结转"
		default:
			return "未知"
		}
	}
	switch status {
	case StatusDraft:
		return "草稿"
	case StatusCompleted:
		return "已完成"
	default:
		return "未知"
	}
}
