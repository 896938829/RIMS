// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package file

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"rims-go/internal/types"
)

type aclFileRepoStub struct {
	files        map[uint]*FileAttachment
	listItems    []FileAttachment
	softDelete   []uint
	created      []*FileAttachment
	updated      []*FileAttachment
	cleanup      map[string]storageCleanupTestTask
	commitTokens []string
}

type storageCleanupTestTask struct {
	operation    string
	prepareToken string
	state        string
	primaryError string
	cleanupError string
}

func (r *aclFileRepoStub) PrepareStorageCleanup(_ context.Context, objectKey, operation, prepareToken string) error {
	if r.cleanup == nil {
		r.cleanup = make(map[string]storageCleanupTestTask)
	}
	r.cleanup[objectKey] = storageCleanupTestTask{operation: operation, prepareToken: prepareToken, state: "prepared"}
	return nil
}

func (r *aclFileRepoStub) ClearStorageCleanup(_ context.Context, objectKey, prepareToken string) error {
	task, exists := r.cleanup[objectKey]
	if exists && (prepareToken == "" || task.prepareToken == prepareToken) {
		delete(r.cleanup, objectKey)
	}
	return nil
}

func (r *aclFileRepoStub) RecordStorageCleanupFailure(_ context.Context, objectKey, prepareToken, primaryError, cleanupError string) error {
	task := r.cleanup[objectKey]
	if task.prepareToken != prepareToken {
		return errors.New("storage preparation token mismatch")
	}
	task.state = "ready"
	task.primaryError = primaryError
	task.cleanupError = cleanupError
	r.cleanup[objectKey] = task
	return nil
}

func (r *aclFileRepoStub) Create(ctx context.Context, f *FileAttachment, prepareToken string) error {
	task := r.cleanup[f.ObjectKey]
	if prepareToken == "" || task.prepareToken != prepareToken || task.state != "prepared" {
		return errors.New("storage preparation token mismatch")
	}
	if f.ID == 0 {
		f.ID = uint(100 + len(r.created))
	}
	copy := *f
	r.created = append(r.created, &copy)
	r.commitTokens = append(r.commitTokens, prepareToken)
	delete(r.cleanup, f.ObjectKey)
	return nil
}

func (r *aclFileRepoStub) Update(ctx context.Context, f *FileAttachment) error {
	copy := *f
	r.updated = append(r.updated, &copy)
	return nil
}

func (r *aclFileRepoStub) ReplaceObject(ctx context.Context, f *FileAttachment, previousObjectKey, prepareToken string) error {
	task := r.cleanup[f.ObjectKey]
	if prepareToken == "" || task.prepareToken != prepareToken || task.state != "prepared" {
		return errors.New("storage preparation token mismatch")
	}
	if err := r.Update(ctx, f); err != nil {
		return err
	}
	delete(r.cleanup, f.ObjectKey)
	r.commitTokens = append(r.commitTokens, prepareToken)
	if r.cleanup == nil {
		r.cleanup = make(map[string]storageCleanupTestTask)
	}
	r.cleanup[previousObjectKey] = storageCleanupTestTask{operation: "replace_previous", state: "ready"}
	return nil
}

func (r *aclFileRepoStub) GetByID(ctx context.Context, id uint) (*FileAttachment, error) {
	f, ok := r.files[id]
	if !ok {
		return nil, errors.New("not found")
	}
	copy := *f
	return &copy, nil
}

func (r *aclFileRepoStub) GetByHash(ctx context.Context, hash string) (*FileAttachment, error) {
	return nil, errors.New("not implemented")
}

func (r *aclFileRepoStub) List(ctx context.Context, filter ListFilter, page types.PageRequest) ([]FileAttachment, int64, error) {
	if r.listItems != nil {
		return r.listItems, int64(len(r.listItems)), nil
	}
	list := make([]FileAttachment, 0, len(r.files))
	for _, f := range r.files {
		list = append(list, *f)
	}
	return list, int64(len(list)), nil
}

func (r *aclFileRepoStub) SoftDelete(ctx context.Context, id uint) error {
	r.softDelete = append(r.softDelete, id)
	return nil
}

type aclStorageStub struct {
	opened int
	saved  int
}

func (s *aclStorageStub) Save(ctx context.Context, objectKey string, r io.Reader) error {
	s.saved++
	return nil
}

func (s *aclStorageStub) Open(ctx context.Context, objectKey string) (io.ReadCloser, error) {
	s.opened++
	return io.NopCloser(strings.NewReader("file body")), nil
}

func (s *aclStorageStub) Delete(ctx context.Context, objectKey string) error {
	return nil
}

func (s *aclStorageStub) PublicURL(objectKey string) string {
	return "/uploads/" + objectKey
}

type aclReadCountingReader struct {
	reads int
}

func (r *aclReadCountingReader) Read(p []byte) (int, error) {
	r.reads++
	return 0, errors.New("reader should not be read")
}

type aclCheckerStub struct {
	allowed     bool
	allowedByID map[uint]bool
	err         error
	calls       int
	action      FileAction
	actor       FileActor
	file        FileAttachment
}

func (c *aclCheckerStub) CanAccessFile(ctx context.Context, actor FileActor, f *FileAttachment, action FileAction) (bool, error) {
	c.calls++
	c.action = action
	c.actor = actor
	c.file = *f
	if c.allowedByID != nil {
		return c.allowedByID[f.ID], c.err
	}
	return c.allowed, c.err
}

func TestFileServiceUploadWithBusinessIDAuthorizesBeforeStorageAndCreate(t *testing.T) {
	businessID := uint(99)
	repo := &aclFileRepoStub{}
	storage := &aclStorageStub{}
	checker := &aclCheckerStub{allowed: true}
	svc := NewFileService(repo, storage, 0, ".txt", "/files/%d/download", checker)

	record, err := svc.Upload(context.Background(), FileActor{UserID: 20}, UploadRequest{
		BusinessType: BusinessTypeDocAttachment,
		BusinessID:   &businessID,
		OriginalName: "doc.txt",
		Reader:       strings.NewReader("file body"),
	})

	if err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	if record == nil || record.CreatedBy != 20 || record.BusinessID == nil || *record.BusinessID != businessID {
		t.Fatalf("Upload() record = %#v, want creator 20 and businessID %d", record, businessID)
	}
	if checker.calls != 1 || checker.action != FileActionCreate {
		t.Fatalf("checker calls/action = %d/%q, want 1/%q", checker.calls, checker.action, FileActionCreate)
	}
	if checker.actor.UserID != 20 || checker.actor.IsAdmin {
		t.Fatalf("checker actor = %#v, want user 20 non-admin", checker.actor)
	}
	if checker.file.BusinessID == nil || *checker.file.BusinessID != businessID || checker.file.BusinessType != BusinessTypeDocAttachment {
		t.Fatalf("checker file = %#v, want doc attachment businessID %d", checker.file, businessID)
	}
	if storage.saved != 1 {
		t.Fatalf("storage saves = %d, want 1", storage.saved)
	}
	if len(repo.created) != 1 {
		t.Fatalf("repo creates = %d, want 1", len(repo.created))
	}
}

func TestFileServiceUploadWithBusinessIDDenySkipsStorageAndCreate(t *testing.T) {
	businessID := uint(99)
	repo := &aclFileRepoStub{}
	storage := &aclStorageStub{}
	checker := &aclCheckerStub{allowed: false}
	svc := NewFileService(repo, storage, 0, ".txt", "/files/%d/download", checker)

	record, err := svc.Upload(context.Background(), FileActor{UserID: 20}, UploadRequest{
		BusinessType: BusinessTypeDocAttachment,
		BusinessID:   &businessID,
		OriginalName: "doc.txt",
		Reader:       strings.NewReader("file body"),
	})

	if record != nil {
		t.Fatalf("Upload() record = %#v, want nil", record)
	}
	appErr, ok := err.(*types.AppError)
	if !ok || appErr.Code != types.ErrCodePermissionDenied {
		t.Fatalf("Upload() error = %v, want permission denied", err)
	}
	if checker.calls != 1 || checker.action != FileActionCreate {
		t.Fatalf("checker calls/action = %d/%q, want 1/%q", checker.calls, checker.action, FileActionCreate)
	}
	if storage.saved != 0 {
		t.Fatalf("storage saves = %d, want 0", storage.saved)
	}
	if len(repo.created) != 0 {
		t.Fatalf("repo creates = %d, want 0", len(repo.created))
	}
}

func TestFileServiceUploadProductImageWithoutBusinessIDRejectsBeforeStorageAndCreate(t *testing.T) {
	repo := &aclFileRepoStub{}
	storage := &aclStorageStub{}
	reader := &aclReadCountingReader{}
	checker := &aclCheckerStub{err: errors.New("checker should not be called")}
	svc := NewFileService(repo, storage, 0, ".jpg", "/files/%d/download", checker)

	record, err := svc.Upload(context.Background(), FileActor{UserID: 20}, UploadRequest{
		BusinessType: BusinessTypeProductImage,
		BusinessID:   nil,
		OriginalName: "product.jpg",
		Reader:       reader,
	})

	if record != nil {
		t.Errorf("Upload() record = %#v, want nil", record)
	}
	appErr, ok := err.(*types.AppError)
	if !ok || appErr.Code != types.ErrCodeValidation {
		t.Errorf("Upload() error = %v, want validation error", err)
	}
	if storage.saved != 0 {
		t.Errorf("storage saves = %d, want 0", storage.saved)
	}
	if len(repo.created) != 0 {
		t.Errorf("repo creates = %d, want 0", len(repo.created))
	}
	if checker.calls != 0 {
		t.Errorf("checker calls = %d, want 0", checker.calls)
	}
	if reader.reads != 0 {
		t.Errorf("reader reads = %d, want 0", reader.reads)
	}
}

func TestFileServiceUploadProductImageWithZeroBusinessIDRejectsBeforeReadStorageAndCreate(t *testing.T) {
	businessID := uint(0)
	repo := &aclFileRepoStub{}
	storage := &aclStorageStub{}
	reader := &aclReadCountingReader{}
	checker := &aclCheckerStub{allowed: true}
	svc := NewFileService(repo, storage, 0, ".jpg", "/files/%d/download", checker)

	record, err := svc.Upload(context.Background(), FileActor{UserID: 20}, UploadRequest{
		BusinessType: BusinessTypeProductImage,
		BusinessID:   &businessID,
		OriginalName: "product.jpg",
		Reader:       reader,
	})

	if record != nil {
		t.Errorf("Upload() record = %#v, want nil", record)
	}
	appErr, ok := err.(*types.AppError)
	if !ok || appErr.Code != types.ErrCodeValidation {
		t.Errorf("Upload() error = %v, want validation error", err)
	}
	if storage.saved != 0 {
		t.Errorf("storage saves = %d, want 0", storage.saved)
	}
	if len(repo.created) != 0 {
		t.Errorf("repo creates = %d, want 0", len(repo.created))
	}
	if checker.calls != 0 {
		t.Errorf("checker calls = %d, want 0", checker.calls)
	}
	if reader.reads != 0 {
		t.Errorf("reader reads = %d, want 0", reader.reads)
	}
}

func TestFileServiceUploadProductImageWithBusinessIDAuthorizesAndReturnsPublicURL(t *testing.T) {
	businessID := uint(99)
	repo := &aclFileRepoStub{}
	storage := &aclStorageStub{}
	checker := &aclCheckerStub{allowed: true}
	svc := NewFileService(repo, storage, 0, ".jpg", "/files/%d/download", checker)

	record, err := svc.Upload(context.Background(), FileActor{UserID: 20}, UploadRequest{
		BusinessType: BusinessTypeProductImage,
		BusinessID:   &businessID,
		OriginalName: "product.jpg",
		Reader:       bytes.NewReader([]byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00}),
	})

	if err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	if record == nil {
		t.Fatal("Upload() record = nil, want file attachment")
	}
	if checker.calls != 1 || checker.action != FileActionCreate {
		t.Fatalf("checker calls/action = %d/%q, want 1/%q", checker.calls, checker.action, FileActionCreate)
	}
	if checker.file.BusinessID == nil || *checker.file.BusinessID != businessID {
		t.Fatalf("checker file businessID = %v, want %d", checker.file.BusinessID, businessID)
	}
	if checker.file.BusinessType != BusinessTypeProductImage || !checker.file.IsPublic {
		t.Fatalf("checker file = %#v, want public product image", checker.file)
	}
	if storage.saved != 1 {
		t.Fatalf("storage saves = %d, want 1", storage.saved)
	}
	if len(repo.created) != 1 {
		t.Fatalf("repo creates = %d, want 1", len(repo.created))
	}
	if !record.IsPublic {
		t.Fatalf("record IsPublic = false, want true")
	}
	if !strings.HasPrefix(record.FileURL, "/uploads/") {
		t.Fatalf("record FileURL = %q, want /uploads/... URL", record.FileURL)
	}
}

func TestFileServiceUploadWithoutBusinessIDKeepsUnscopedBehavior(t *testing.T) {
	repo := &aclFileRepoStub{}
	storage := &aclStorageStub{}
	checker := &aclCheckerStub{err: errors.New("checker should not be called for unscoped upload")}
	svc := NewFileService(repo, storage, 0, ".txt", "/files/%d/download", checker)

	record, err := svc.Upload(context.Background(), FileActor{UserID: 20}, UploadRequest{
		BusinessType: BusinessTypeOther,
		OriginalName: "loose.txt",
		Reader:       strings.NewReader("file body"),
	})

	if err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	if record == nil || record.CreatedBy != 20 {
		t.Fatalf("Upload() record = %#v, want creator 20", record)
	}
	if checker.calls != 0 {
		t.Fatalf("checker calls = %d, want 0", checker.calls)
	}
	if storage.saved != 1 {
		t.Fatalf("storage saves = %d, want 1", storage.saved)
	}
}

func TestFileServiceDeleteRejectsBusinessAccessWhenUploaderDiffers(t *testing.T) {
	repo := &aclFileRepoStub{files: map[uint]*FileAttachment{
		1: privateFile(1, 10),
	}}
	checker := &aclCheckerStub{allowed: true}
	svc := NewFileService(repo, &aclStorageStub{}, 0, "", "/files/%d/download", checker)

	err := svc.Delete(context.Background(), 1, FileActor{UserID: 20})

	appErr, ok := err.(*types.AppError)
	if !ok || appErr.Code != types.ErrCodePermissionDenied {
		t.Fatalf("Delete() error = %v, want permission denied", err)
	}
	if len(repo.softDelete) != 0 {
		t.Fatalf("soft deletes = %v, want none", repo.softDelete)
	}
	if checker.calls != 0 {
		t.Fatalf("checker calls = %d, want 0", checker.calls)
	}
}

func TestFileServiceDeleteRejectsBusinessAccessDenied(t *testing.T) {
	repo := &aclFileRepoStub{files: map[uint]*FileAttachment{
		1: privateFile(1, 10),
	}}
	checker := &aclCheckerStub{allowed: false}
	svc := NewFileService(repo, &aclStorageStub{}, 0, "", "/files/%d/download", checker)

	err := svc.Delete(context.Background(), 1, FileActor{UserID: 20})

	appErr, ok := err.(*types.AppError)
	if !ok || appErr.Code != types.ErrCodePermissionDenied {
		t.Fatalf("Delete() error = %v, want permission denied", err)
	}
	if len(repo.softDelete) != 0 {
		t.Fatalf("soft deletes = %v, want none", repo.softDelete)
	}
}

func TestFileServiceOpenForDownloadRejectsPrivateFileWithoutBusinessAccess(t *testing.T) {
	repo := &aclFileRepoStub{files: map[uint]*FileAttachment{
		1: privateFile(1, 10),
	}}
	storage := &aclStorageStub{}
	checker := &aclCheckerStub{allowed: false}
	svc := NewFileService(repo, storage, 0, "", "/files/%d/download", checker)

	rc, f, err := svc.OpenForDownload(context.Background(), 1, FileActor{UserID: 20})

	if rc != nil || f != nil {
		t.Fatalf("OpenForDownload() rc/file = %v/%v, want nil/nil", rc, f)
	}
	appErr, ok := err.(*types.AppError)
	if !ok || appErr.Code != types.ErrCodePermissionDenied {
		t.Fatalf("OpenForDownload() error = %v, want permission denied", err)
	}
	if storage.opened != 0 {
		t.Fatalf("storage opened = %d, want 0", storage.opened)
	}
}

func TestFileServiceOpenForDownloadAllowsAdmin(t *testing.T) {
	repo := &aclFileRepoStub{files: map[uint]*FileAttachment{
		1: privateFile(1, 10),
	}}
	storage := &aclStorageStub{}
	svc := NewFileService(repo, storage, 0, "", "/files/%d/download", nil)

	rc, f, err := svc.OpenForDownload(context.Background(), 1, FileActor{UserID: 20, IsAdmin: true})
	if err != nil {
		t.Fatalf("OpenForDownload() error = %v", err)
	}
	defer rc.Close()
	if f == nil || f.ID != 1 {
		t.Fatalf("OpenForDownload() file = %#v, want ID 1", f)
	}
	if storage.opened != 1 {
		t.Fatalf("storage opened = %d, want 1", storage.opened)
	}
}

func TestFileServiceOpenForDownloadPublicFileBypassesChecker(t *testing.T) {
	repo := &aclFileRepoStub{files: map[uint]*FileAttachment{
		1: publicFile(1, 10),
	}}
	storage := &aclStorageStub{}
	checker := &aclCheckerStub{err: errors.New("checker should not be called")}
	svc := NewFileService(repo, storage, 0, "", "/files/%d/download", checker)

	rc, f, err := svc.OpenForDownload(context.Background(), 1, FileActor{UserID: 20})
	if err != nil {
		t.Fatalf("OpenForDownload() error = %v", err)
	}
	defer rc.Close()
	if f == nil || f.ID != 1 {
		t.Fatalf("OpenForDownload() file = %#v, want ID 1", f)
	}
	if checker.calls != 0 {
		t.Fatalf("checker calls = %d, want 0", checker.calls)
	}
	if storage.opened != 1 {
		t.Fatalf("storage opened = %d, want 1", storage.opened)
	}
}

func TestFileServiceCheckerErrorMapsToSystemError(t *testing.T) {
	repo := &aclFileRepoStub{files: map[uint]*FileAttachment{
		1: privateFile(1, 10),
	}}
	storage := &aclStorageStub{}
	checker := &aclCheckerStub{err: errors.New("acl backend unavailable")}
	svc := NewFileService(repo, storage, 0, "", "/files/%d/download", checker)

	rc, f, err := svc.OpenForDownload(context.Background(), 1, FileActor{UserID: 20})

	if rc != nil || f != nil {
		t.Fatalf("OpenForDownload() rc/file = %v/%v, want nil/nil", rc, f)
	}
	appErr, ok := err.(*types.AppError)
	if !ok || appErr.Code != types.ErrCodeSystemError {
		t.Fatalf("OpenForDownload() error = %v, want system error", err)
	}
	if storage.opened != 0 {
		t.Fatalf("storage opened = %d, want 0", storage.opened)
	}
}

func TestFileServiceUploaderBypassesCheckerForReadAndDelete(t *testing.T) {
	repo := &aclFileRepoStub{files: map[uint]*FileAttachment{
		1: privateFile(1, 10),
		2: privateFile(2, 10),
	}}
	storage := &aclStorageStub{}
	checker := &aclCheckerStub{err: errors.New("checker should not be called")}
	svc := NewFileService(repo, storage, 0, "", "/files/%d/download", checker)

	rc, f, err := svc.OpenForDownload(context.Background(), 1, FileActor{UserID: 10})
	if err != nil {
		t.Fatalf("OpenForDownload() error = %v", err)
	}
	defer rc.Close()
	if f == nil || f.ID != 1 {
		t.Fatalf("OpenForDownload() file = %#v, want ID 1", f)
	}

	if err := svc.Delete(context.Background(), 2, FileActor{UserID: 10}); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if checker.calls != 0 {
		t.Fatalf("checker calls = %d, want 0", checker.calls)
	}
	if len(repo.softDelete) != 1 || repo.softDelete[0] != 2 {
		t.Fatalf("soft deletes = %v, want [2]", repo.softDelete)
	}
}

func TestFileServiceNilCheckerUsesOwnerAdminDefault(t *testing.T) {
	repo := &aclFileRepoStub{files: map[uint]*FileAttachment{
		1: privateFile(1, 10),
		2: privateFile(2, 10),
		3: privateFile(3, 10),
	}}
	storage := &aclStorageStub{}
	svc := NewFileService(repo, storage, 0, "", "/files/%d/download", nil)

	rc, f, err := svc.OpenForDownload(context.Background(), 1, FileActor{UserID: 20})
	if rc != nil || f != nil {
		t.Fatalf("non-owner OpenForDownload() rc/file = %v/%v, want nil/nil", rc, f)
	}
	appErr, ok := err.(*types.AppError)
	if !ok || appErr.Code != types.ErrCodePermissionDenied {
		t.Fatalf("non-owner OpenForDownload() error = %v, want permission denied", err)
	}

	rc, f, err = svc.OpenForDownload(context.Background(), 2, FileActor{UserID: 10})
	if err != nil {
		t.Fatalf("owner OpenForDownload() error = %v", err)
	}
	defer rc.Close()
	if f == nil || f.ID != 2 {
		t.Fatalf("owner OpenForDownload() file = %#v, want ID 2", f)
	}

	if err := svc.Delete(context.Background(), 3, FileActor{UserID: 20, IsAdmin: true}); err != nil {
		t.Fatalf("admin Delete() error = %v", err)
	}
	if len(repo.softDelete) != 1 || repo.softDelete[0] != 3 {
		t.Fatalf("soft deletes = %v, want [3]", repo.softDelete)
	}
}

func TestFileServiceGetForReadRejectsPrivateFileWhenCheckerDenies(t *testing.T) {
	repo := &aclFileRepoStub{files: map[uint]*FileAttachment{
		1: privateFile(1, 10),
	}}
	checker := &aclCheckerStub{allowed: false}
	svc := NewFileService(repo, &aclStorageStub{}, 0, "", "/files/%d/download", checker)

	f, err := svc.GetForRead(context.Background(), 1, FileActor{UserID: 20})

	if f != nil {
		t.Fatalf("GetForRead() file = %#v, want nil", f)
	}
	appErr, ok := err.(*types.AppError)
	if !ok || appErr.Code != types.ErrCodePermissionDenied {
		t.Fatalf("GetForRead() error = %v, want permission denied", err)
	}
	if checker.calls != 1 || checker.action != FileActionRead {
		t.Fatalf("checker calls/action = %d/%q, want 1/%q", checker.calls, checker.action, FileActionRead)
	}
}

func TestFileServiceGetForReadAllowsBusinessAccessWhenCheckerAllows(t *testing.T) {
	repo := &aclFileRepoStub{files: map[uint]*FileAttachment{
		1: privateFile(1, 10),
	}}
	checker := &aclCheckerStub{allowed: true}
	svc := NewFileService(repo, &aclStorageStub{}, 0, "", "/files/%d/download", checker)

	f, err := svc.GetForRead(context.Background(), 1, FileActor{UserID: 20})

	if err != nil {
		t.Fatalf("GetForRead() error = %v", err)
	}
	if f == nil || f.ID != 1 {
		t.Fatalf("GetForRead() file = %#v, want ID 1", f)
	}
	if checker.calls != 1 || checker.action != FileActionRead {
		t.Fatalf("checker calls/action = %d/%q, want 1/%q", checker.calls, checker.action, FileActionRead)
	}
}

func TestFileServiceListForReadFiltersUnauthorizedCandidates(t *testing.T) {
	unauthorized := privateFile(1, 10)
	authorized := privateFile(2, 10)
	owner := privateFile(3, 20)
	public := publicFile(4, 10)
	adminVisible := privateFile(5, 30)
	repo := &aclFileRepoStub{
		listItems: []FileAttachment{*unauthorized, *authorized, *owner, *public, *adminVisible},
	}
	checker := &aclCheckerStub{allowedByID: map[uint]bool{2: true}}
	svc := NewFileService(repo, &aclStorageStub{}, 0, "", "/files/%d/download", checker)

	result, err := svc.ListForRead(context.Background(), ListFilter{}, types.PageRequest{Page: 1, PageSize: 20}, false, FileActor{UserID: 20})
	if err != nil {
		t.Fatalf("ListForRead() error = %v", err)
	}
	items, ok := result.List.([]FileResponse)
	if !ok {
		t.Fatalf("ListForRead() list type = %T, want []FileResponse", result.List)
	}
	gotIDs := make([]uint, 0, len(items))
	for _, item := range items {
		gotIDs = append(gotIDs, item.ID)
	}
	wantIDs := []uint{2, 3, 4}
	if !equalUintSlices(gotIDs, wantIDs) {
		t.Fatalf("ListForRead() IDs = %v, want %v", gotIDs, wantIDs)
	}
	if result.Total != int64(len(wantIDs)) {
		t.Fatalf("ListForRead() total = %d, want %d", result.Total, len(wantIDs))
	}

	result, err = svc.ListForRead(context.Background(), ListFilter{}, types.PageRequest{Page: 1, PageSize: 20}, false, FileActor{UserID: 99, IsAdmin: true})
	if err != nil {
		t.Fatalf("admin ListForRead() error = %v", err)
	}
	items, ok = result.List.([]FileResponse)
	if !ok {
		t.Fatalf("admin ListForRead() list type = %T, want []FileResponse", result.List)
	}
	gotIDs = gotIDs[:0]
	for _, item := range items {
		gotIDs = append(gotIDs, item.ID)
	}
	wantIDs = []uint{1, 2, 3, 4, 5}
	if !equalUintSlices(gotIDs, wantIDs) {
		t.Fatalf("admin ListForRead() IDs = %v, want %v", gotIDs, wantIDs)
	}
	if result.Total != int64(len(wantIDs)) {
		t.Fatalf("admin ListForRead() total = %d, want %d", result.Total, len(wantIDs))
	}
}

func privateFile(id, uploaderID uint) *FileAttachment {
	f := &FileAttachment{
		BusinessType: BusinessTypeDocAttachment,
		ObjectKey:    "private.txt",
		OriginalName: "private.txt",
		FileSize:     9,
		MimeType:     "text/plain",
		IsPublic:     false,
	}
	f.ID = id
	f.CreatedBy = uploaderID
	return f
}

func equalUintSlices(a, b []uint) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func publicFile(id, uploaderID uint) *FileAttachment {
	f := &FileAttachment{
		BusinessType: BusinessTypeProductImage,
		ObjectKey:    "public.jpg",
		OriginalName: "public.jpg",
		FileSize:     9,
		MimeType:     "image/jpeg",
		IsPublic:     true,
	}
	f.ID = id
	f.CreatedBy = uploaderID
	return f
}
