package file

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"rims-go/internal/types"
)

type mutationFileRepo struct {
	aclFileRepoStub
	binding     []FileAttachment
	positionIDs []uint
	positionErr error
	updateErr   error
	maxPosition int
}

func (r *mutationFileRepo) ListAllByBinding(context.Context, string, uint) ([]FileAttachment, error) {
	return append([]FileAttachment(nil), r.binding...), nil
}

func (r *mutationFileRepo) MaxPositionByBinding(context.Context, string, uint) (int, error) {
	return r.maxPosition, nil
}

func (r *mutationFileRepo) UpdatePositions(_ context.Context, _ string, _ uint, ids []uint) error {
	r.positionIDs = append([]uint(nil), ids...)
	return r.positionErr
}

func (r *mutationFileRepo) Update(_ context.Context, f *FileAttachment) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	copy := *f
	r.updated = append(r.updated, &copy)
	delete(r.cleanup, f.ObjectKey)
	return nil
}

func (r *mutationFileRepo) ReplaceObject(ctx context.Context, f *FileAttachment, previousObjectKey, prepareToken string) error {
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

type mutationStorage struct {
	saved     []string
	deleted   []string
	content   []byte
	deleteErr error
}

func (s *mutationStorage) Save(_ context.Context, key string, reader io.Reader) error {
	s.saved = append(s.saved, key)
	s.content, _ = io.ReadAll(reader)
	return nil
}
func (s *mutationStorage) Open(context.Context, string) (io.ReadCloser, error) {
	return nil, errors.New("not used")
}
func (s *mutationStorage) Delete(_ context.Context, key string) error {
	s.deleted = append(s.deleted, key)
	return s.deleteErr
}
func (s *mutationStorage) PublicURL(key string) string { return "/uploads/" + key }

func TestFileServiceReorderRequiresExactBindingSetAndPreservesRequestedOrder(t *testing.T) {
	businessID := uint(9)
	repo := &mutationFileRepo{binding: []FileAttachment{
		mutationFile(1, businessID, 7, 0), mutationFile(2, businessID, 7, 1), mutationFile(3, businessID, 7, 2),
	}}
	svc := NewFileService(repo, &mutationStorage{}, 10, ".txt", "/files/%d/download", nil)

	items, err := svc.Reorder(context.Background(), FileActor{UserID: 7}, ReorderRequest{
		BusinessType: BusinessTypeDocAttachment, BusinessID: businessID, FileIDs: []uint{3, 1, 2},
	})

	if err != nil {
		t.Fatalf("Reorder() error = %v", err)
	}
	if got := repo.positionIDs; len(got) != 3 || got[0] != 3 || got[1] != 1 || got[2] != 2 {
		t.Fatalf("position IDs = %v, want [3 1 2]", got)
	}
	for index, item := range items {
		if item.ID != []uint{3, 1, 2}[index] || item.Position != index {
			t.Fatalf("item[%d] = %#v", index, item)
		}
	}
}

func TestFileServiceReorderRejectsDuplicateMissingForeignAndNonOwner(t *testing.T) {
	businessID := uint(9)
	base := []FileAttachment{mutationFile(1, businessID, 7, 0), mutationFile(2, businessID, 7, 1)}
	tests := []struct {
		name  string
		actor FileActor
		ids   []uint
		code  int
	}{
		{name: "duplicate", actor: FileActor{UserID: 7}, ids: []uint{1, 1}, code: types.ErrCodeValidation},
		{name: "missing", actor: FileActor{UserID: 7}, ids: []uint{1}, code: types.ErrCodeValidation},
		{name: "foreign", actor: FileActor{UserID: 7}, ids: []uint{1, 99}, code: types.ErrCodeValidation},
		{name: "non-owner", actor: FileActor{UserID: 8}, ids: []uint{1, 2}, code: types.ErrCodePermissionDenied},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &mutationFileRepo{binding: base}
			svc := NewFileService(repo, &mutationStorage{}, 10, ".txt", "/files/%d/download", nil)
			_, err := svc.Reorder(context.Background(), tc.actor, ReorderRequest{BusinessType: BusinessTypeDocAttachment, BusinessID: businessID, FileIDs: tc.ids})
			var appErr *types.AppError
			if !errors.As(err, &appErr) || appErr.Code != tc.code || len(repo.positionIDs) != 0 {
				t.Fatalf("Reorder() error/updates = %v/%v, want code %d/no updates", err, repo.positionIDs, tc.code)
			}
		})
	}
}

func TestFileServiceReplacePreservesIdentityBindingPositionAndCreator(t *testing.T) {
	businessID := uint(9)
	original := mutationFile(4, businessID, 7, 3)
	original.ObjectKey = "old/object.txt"
	repo := &mutationFileRepo{aclFileRepoStub: aclFileRepoStub{files: map[uint]*FileAttachment{4: &original}}}
	storage := &mutationStorage{}
	svc := NewFileService(repo, storage, 10, ".txt", "/files/%d/download", nil)

	replaced, err := svc.Replace(context.Background(), 4, FileActor{UserID: 7}, UploadRequest{OriginalName: "new.txt", Reader: strings.NewReader("new body")})
	if err != nil {
		t.Fatalf("Replace() error = %v", err)
	}
	if replaced.ID != 4 || replaced.BusinessID == nil || *replaced.BusinessID != businessID || replaced.BusinessType != BusinessTypeDocAttachment || replaced.Position != 3 || replaced.CreatedBy != 7 {
		t.Fatalf("Replace() identity fields = %#v", replaced)
	}
	if replaced.OriginalName != "new.txt" || replaced.ObjectKey == original.ObjectKey || len(repo.updated) != 1 {
		t.Fatalf("Replace() metadata = %#v updated=%d", replaced, len(repo.updated))
	}
	if len(storage.deleted) != 1 || storage.deleted[0] != original.ObjectKey {
		t.Fatalf("deleted = %v, want old object", storage.deleted)
	}
	if len(repo.commitTokens) != 1 || repo.commitTokens[0] == "" {
		t.Fatalf("replacement commit tokens = %v, want one non-empty preparation owner", repo.commitTokens)
	}
}

func TestFileServiceReplaceUpdateFailureRollsBackNewObjectAndKeepsOld(t *testing.T) {
	businessID := uint(9)
	original := mutationFile(4, businessID, 7, 3)
	original.ObjectKey = "old/object.txt"
	repo := &mutationFileRepo{aclFileRepoStub: aclFileRepoStub{files: map[uint]*FileAttachment{4: &original}}, updateErr: errors.New("update failed")}
	storage := &mutationStorage{}
	svc := NewFileService(repo, storage, 10, ".txt", "/files/%d/download", nil)

	replaced, err := svc.Replace(context.Background(), 4, FileActor{UserID: 7}, UploadRequest{OriginalName: "new.txt", Reader: strings.NewReader("new body")})
	if replaced != nil || err == nil || len(storage.saved) != 1 || len(storage.deleted) != 1 || storage.deleted[0] != storage.saved[0] {
		t.Fatalf("Replace() record/error/save/delete = %#v/%v/%v/%v", replaced, err, storage.saved, storage.deleted)
	}
	if storage.deleted[0] == original.ObjectKey {
		t.Fatal("old object was deleted during rollback")
	}
}

func TestFileServiceReplaceRetainsCleanupResponsibilityWhenUpdateAndRollbackDeleteFail(t *testing.T) {
	businessID := uint(9)
	original := mutationFile(4, businessID, 7, 3)
	original.ObjectKey = "old/object.txt"
	repo := &mutationFileRepo{
		aclFileRepoStub: aclFileRepoStub{files: map[uint]*FileAttachment{4: &original}},
		updateErr:       errors.New("replace metadata failed"),
	}
	storage := &mutationStorage{deleteErr: errors.New("rollback disk unavailable")}
	svc := NewFileService(repo, storage, 10, ".txt", "/files/%d/download", nil)

	replaced, err := svc.Replace(context.Background(), 4, FileActor{UserID: 7}, UploadRequest{
		OriginalName: "new.txt",
		Reader:       strings.NewReader("new body"),
	})

	if replaced != nil || err == nil {
		t.Fatalf("Replace() record/error = %#v/%v, want nil/combined error", replaced, err)
	}
	if !strings.Contains(err.Error(), "replace metadata failed") ||
		!strings.Contains(err.Error(), "rollback disk unavailable") {
		t.Fatalf("Replace() error = %v, want primary and rollback failures", err)
	}
	if len(storage.saved) != 1 || len(repo.cleanup) != 1 {
		t.Fatalf("saved/cleanup responsibility = %v/%v, want one durable pending object", storage.saved, repo.cleanup)
	}
	task := repo.cleanup[storage.saved[0]]
	if task.operation != "replace" || task.prepareToken == "" || task.state != "ready" || task.primaryError != "replace metadata failed" || task.cleanupError != "rollback disk unavailable" {
		t.Fatalf("cleanup task = %#v, want replace failure evidence", task)
	}
}

func TestFileServiceListForReadPaginatesAuthorizedRowsAndReportsAuthorizedTotal(t *testing.T) {
	businessID := uint(9)
	first := mutationFile(1, businessID, 10, 0)
	second := mutationFile(2, businessID, 10, 1)
	repo := &mutationFileRepo{aclFileRepoStub: aclFileRepoStub{listItems: []FileAttachment{first, second}}}
	checker := &aclCheckerStub{allowedByID: map[uint]bool{1: false, 2: true}}
	svc := NewFileService(repo, &mutationStorage{}, 10, ".txt", "/files/%d/download", checker)

	result, err := svc.ListForRead(context.Background(), ListFilter{BusinessType: BusinessTypeDocAttachment, BusinessID: &businessID}, types.PageRequest{Page: 1, PageSize: 1}, false, FileActor{UserID: 20})
	if err != nil {
		t.Fatalf("ListForRead() error = %v", err)
	}
	items, ok := result.List.([]FileResponse)
	if !ok || result.Total != 1 || len(items) != 1 || items[0].ID != 2 || result.PageSize != 1 {
		t.Fatalf("ListForRead() = %#v items=%#v, want authorized ID2 total1", result, items)
	}
}

func mutationFile(id, businessID, creator uint, position int) FileAttachment {
	return FileAttachment{
		AuditableModel: types.AuditableModel{BaseModel: types.BaseModel{ID: id}, CreatedBy: creator, UpdatedBy: creator},
		BusinessType:   BusinessTypeDocAttachment, BusinessID: &businessID, ObjectKey: "object.txt", OriginalName: "old.txt", MimeType: "text/plain", Position: position,
	}
}
