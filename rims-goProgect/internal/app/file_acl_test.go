// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package app

import (
	"context"
	"errors"
	"testing"

	"rims-go/internal/modules/document"
	"rims-go/internal/modules/file"
	"rims-go/internal/modules/warehouse"
	"rims-go/internal/types"
)

type fileACLDocumentRepoStub struct {
	docs map[uint]*document.Document
}

func (r *fileACLDocumentRepoStub) Create(ctx context.Context, doc *document.Document) error {
	return nil
}

func (r *fileACLDocumentRepoStub) GetByID(ctx context.Context, id uint) (*document.Document, error) {
	doc, ok := r.docs[id]
	if !ok {
		return nil, errors.New("not found")
	}
	copy := *doc
	return &copy, nil
}

func (r *fileACLDocumentRepoStub) GetByIDForUpdate(ctx context.Context, id uint) (*document.Document, error) {
	return nil, errors.New("not implemented")
}

func (r *fileACLDocumentRepoStub) GetByDocNo(ctx context.Context, docNo string) (*document.Document, error) {
	return nil, errors.New("not implemented")
}

func (r *fileACLDocumentRepoStub) List(ctx context.Context, warehouseID uint, docType int8, page types.PageRequest) ([]document.Document, int64, error) {
	return nil, 0, nil
}

func (r *fileACLDocumentRepoStub) Update(ctx context.Context, doc *document.Document) error {
	return nil
}

func (r *fileACLDocumentRepoStub) LockDocNoSequence(ctx context.Context, prefix string, dateStr string) error {
	return nil
}

func (r *fileACLDocumentRepoStub) LockReturnQuantity(ctx context.Context, refDocID uint, productID uint) error {
	return nil
}

func (r *fileACLDocumentRepoStub) GetMaxDocNo(ctx context.Context, prefix string, dateStr string) (string, error) {
	return "", nil
}

type fileACLUserWarehouseRepoStub struct {
	allowed bool
	calls   int
	userID  uint
	whID    uint
}

func (r *fileACLUserWarehouseRepoStub) Create(ctx context.Context, uw *warehouse.UserWarehouse) error {
	return nil
}

func (r *fileACLUserWarehouseRepoStub) Delete(ctx context.Context, userID, warehouseID uint) error {
	return nil
}

func (r *fileACLUserWarehouseRepoStub) DeleteByWarehouseID(ctx context.Context, warehouseID uint) error {
	return nil
}

func (r *fileACLUserWarehouseRepoStub) GetByUserAndWarehouse(ctx context.Context, userID, warehouseID uint) (*warehouse.UserWarehouse, error) {
	return nil, errors.New("not implemented")
}

func (r *fileACLUserWarehouseRepoStub) ListByUserID(ctx context.Context, userID uint) ([]warehouse.UserWarehouse, error) {
	return nil, nil
}

func (r *fileACLUserWarehouseRepoStub) ListByWarehouseID(ctx context.Context, warehouseID uint, page types.PageRequest) ([]warehouse.WarehouseUserInfo, int64, error) {
	return nil, 0, nil
}

func (r *fileACLUserWarehouseRepoStub) GetDefaultByUserID(ctx context.Context, userID uint) (*warehouse.UserWarehouse, error) {
	return nil, errors.New("not implemented")
}

func (r *fileACLUserWarehouseRepoStub) ClearDefault(ctx context.Context, userID uint) error {
	return nil
}

func (r *fileACLUserWarehouseRepoStub) SetDefault(ctx context.Context, userID, warehouseID uint) error {
	return nil
}

func (r *fileACLUserWarehouseRepoStub) CountByUserID(ctx context.Context, userID uint) (int64, error) {
	return 0, nil
}

func (r *fileACLUserWarehouseRepoStub) GetUserRoleCode(ctx context.Context, userID uint) (string, error) {
	return "", nil
}

func (r *fileACLUserWarehouseRepoStub) GetDefaultWarehouseID(ctx context.Context, userID uint) (uint, error) {
	return 0, nil
}

func (r *fileACLUserWarehouseRepoStub) HasAccess(ctx context.Context, userID, warehouseID uint) (bool, error) {
	r.calls++
	r.userID = userID
	r.whID = warehouseID
	return r.allowed, nil
}

func TestFileAccessCheckerAllowsDocumentAttachmentWhenUserHasDocumentWarehouse(t *testing.T) {
	businessID := uint(99)
	docRepo := &fileACLDocumentRepoStub{docs: map[uint]*document.Document{
		businessID: {WarehouseID: 7},
	}}
	whRepo := &fileACLUserWarehouseRepoStub{allowed: true}
	checker := fileAccessChecker{docRepo: docRepo, whRepo: whRepo}

	allowed, err := checker.CanAccessFile(context.Background(), file.FileActor{UserID: 42}, &file.FileAttachment{
		BusinessType: file.BusinessTypeDocAttachment,
		BusinessID:   &businessID,
	}, file.FileActionRead)

	if err != nil {
		t.Fatalf("CanAccessFile() error = %v", err)
	}
	if !allowed {
		t.Fatal("CanAccessFile() allowed = false, want true")
	}
	if whRepo.calls != 1 || whRepo.userID != 42 || whRepo.whID != 7 {
		t.Fatalf("HasAccess calls/user/warehouse = %d/%d/%d, want 1/42/7", whRepo.calls, whRepo.userID, whRepo.whID)
	}
}

func TestFileAccessCheckerCreateDocumentAttachmentChecksDocumentWarehouseWithoutOwnerBypass(t *testing.T) {
	businessID := uint(99)
	f := &file.FileAttachment{
		BusinessType: file.BusinessTypeDocAttachment,
		BusinessID:   &businessID,
	}
	f.CreatedBy = 42
	docRepo := &fileACLDocumentRepoStub{docs: map[uint]*document.Document{
		businessID: {WarehouseID: 7},
	}}
	whRepo := &fileACLUserWarehouseRepoStub{allowed: false}
	checker := fileAccessChecker{docRepo: docRepo, whRepo: whRepo}

	allowed, err := checker.CanAccessFile(context.Background(), file.FileActor{UserID: 42}, f, file.FileActionCreate)

	if err != nil {
		t.Fatalf("CanAccessFile() error = %v", err)
	}
	if allowed {
		t.Fatal("CanAccessFile() allowed = true, want false")
	}
	if whRepo.calls != 1 || whRepo.userID != 42 || whRepo.whID != 7 {
		t.Fatalf("HasAccess calls/user/warehouse = %d/%d/%d, want 1/42/7", whRepo.calls, whRepo.userID, whRepo.whID)
	}
}

func TestFileAccessCheckerRejectsDocumentAttachmentDeleteForWarehouseUser(t *testing.T) {
	businessID := uint(99)
	f := &file.FileAttachment{
		BusinessType: file.BusinessTypeDocAttachment,
		BusinessID:   &businessID,
	}
	f.CreatedBy = 10
	docRepo := &fileACLDocumentRepoStub{docs: map[uint]*document.Document{
		businessID: {WarehouseID: 7},
	}}
	whRepo := &fileACLUserWarehouseRepoStub{allowed: true}
	checker := fileAccessChecker{docRepo: docRepo, whRepo: whRepo}

	allowed, err := checker.CanAccessFile(context.Background(), file.FileActor{UserID: 42}, f, file.FileActionDelete)

	if err != nil {
		t.Fatalf("CanAccessFile() error = %v", err)
	}
	if allowed {
		t.Fatal("CanAccessFile() allowed = true, want false")
	}
	if whRepo.calls != 0 {
		t.Fatalf("HasAccess calls = %d, want 0 for delete", whRepo.calls)
	}
}

func TestFileAccessCheckerRejectsDocumentAttachmentWhenUserLacksDocumentWarehouse(t *testing.T) {
	businessID := uint(99)
	docRepo := &fileACLDocumentRepoStub{docs: map[uint]*document.Document{
		businessID: {WarehouseID: 7},
	}}
	whRepo := &fileACLUserWarehouseRepoStub{allowed: false}
	checker := fileAccessChecker{docRepo: docRepo, whRepo: whRepo}

	allowed, err := checker.CanAccessFile(context.Background(), file.FileActor{UserID: 42}, &file.FileAttachment{
		BusinessType: file.BusinessTypeDocAttachment,
		BusinessID:   &businessID,
	}, file.FileActionRead)

	if err != nil {
		t.Fatalf("CanAccessFile() error = %v", err)
	}
	if allowed {
		t.Fatal("CanAccessFile() allowed = true, want false")
	}
}

func TestFileAccessCheckerFallsBackToOwnerAdminDefaultBehaviorForUnscopedBusinessTypes(t *testing.T) {
	checker := fileAccessChecker{}
	f := &file.FileAttachment{BusinessType: file.BusinessTypeOther}
	f.CreatedBy = 42

	ownerAllowed, err := checker.CanAccessFile(context.Background(), file.FileActor{UserID: 42}, f, file.FileActionRead)
	if err != nil {
		t.Fatalf("owner CanAccessFile() error = %v", err)
	}
	if !ownerAllowed {
		t.Fatal("owner CanAccessFile() allowed = false, want true")
	}

	adminAllowed, err := checker.CanAccessFile(context.Background(), file.FileActor{UserID: 7, IsAdmin: true}, f, file.FileActionDelete)
	if err != nil {
		t.Fatalf("admin CanAccessFile() error = %v", err)
	}
	if !adminAllowed {
		t.Fatal("admin CanAccessFile() allowed = false, want true")
	}

	otherAllowed, err := checker.CanAccessFile(context.Background(), file.FileActor{UserID: 7}, f, file.FileActionRead)
	if err != nil {
		t.Fatalf("other CanAccessFile() error = %v", err)
	}
	if otherAllowed {
		t.Fatal("other CanAccessFile() allowed = true, want false")
	}
}

func TestFileAccessCheckerRejectsNilBusinessID(t *testing.T) {
	checker := fileAccessChecker{}

	allowed, err := checker.CanAccessFile(context.Background(), file.FileActor{UserID: 7}, &file.FileAttachment{
		BusinessType: file.BusinessTypeDocAttachment,
	}, file.FileActionRead)

	if err != nil {
		t.Fatalf("CanAccessFile() error = %v", err)
	}
	if allowed {
		t.Fatal("CanAccessFile() allowed = true, want false")
	}
}

func TestFileAccessCheckerAllowsProductImageRead(t *testing.T) {
	businessID := uint(99)
	checker := fileAccessChecker{}

	allowed, err := checker.CanAccessFile(context.Background(), file.FileActor{UserID: 7}, &file.FileAttachment{
		BusinessType: file.BusinessTypeProductImage,
		BusinessID:   &businessID,
	}, file.FileActionRead)

	if err != nil {
		t.Fatalf("CanAccessFile() error = %v", err)
	}
	if !allowed {
		t.Fatal("CanAccessFile() allowed = false, want true")
	}
}

func TestFileAccessCheckerRejectsProductImageDeleteForNonOwnerNonAdmin(t *testing.T) {
	businessID := uint(99)
	checker := fileAccessChecker{}

	allowed, err := checker.CanAccessFile(context.Background(), file.FileActor{UserID: 7}, &file.FileAttachment{
		BusinessType: file.BusinessTypeProductImage,
		BusinessID:   &businessID,
	}, file.FileActionDelete)

	if err != nil {
		t.Fatalf("CanAccessFile() error = %v", err)
	}
	if allowed {
		t.Fatal("CanAccessFile() allowed = true, want false")
	}
}

func TestFileAccessCheckerDefaultsDenyForUnknownBusinessTypeWithBusinessID(t *testing.T) {
	businessID := uint(99)
	checker := fileAccessChecker{}

	allowed, err := checker.CanAccessFile(context.Background(), file.FileActor{UserID: 7}, &file.FileAttachment{
		BusinessType: "unknown",
		BusinessID:   &businessID,
	}, file.FileActionRead)

	if err != nil {
		t.Fatalf("CanAccessFile() error = %v", err)
	}
	if allowed {
		t.Fatal("CanAccessFile() allowed = true, want false")
	}
}
