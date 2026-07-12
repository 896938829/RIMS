package file

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"rims-go/internal/types"
)

type validationFileRepo struct {
	aclFileRepoStub
	count      int64
	countErr   error
	countCalls int
}

func (r *validationFileRepo) CountByBinding(context.Context, string, uint) (int64, error) {
	r.countCalls++
	return r.count, r.countErr
}

func TestFileServiceUploadRejectsMaximumAttachmentCountBeforeReadOrStorage(t *testing.T) {
	businessID := uint(7)
	repo := &validationFileRepo{count: 9}
	storage := &aclStorageStub{}
	reader := &aclReadCountingReader{}
	svc := NewFileServiceWithLimits(repo, storage, 10, 9, ".pdf", "/files/%d/download", &aclCheckerStub{allowed: true})

	record, err := svc.Upload(context.Background(), FileActor{UserID: 2}, UploadRequest{
		BusinessType: BusinessTypeDocAttachment,
		BusinessID:   &businessID,
		OriginalName: "proof.pdf",
		Reader:       reader,
	})

	if record != nil {
		t.Fatalf("Upload() record = %#v, want nil", record)
	}
	var appErr *types.AppError
	if !errors.As(err, &appErr) || appErr.Code != types.ErrCodeValidation {
		t.Fatalf("Upload() error = %v, want validation", err)
	}
	if repo.countCalls != 1 || reader.reads != 0 || storage.saved != 0 || len(repo.created) != 0 {
		t.Fatalf("side effects count/read/save/create = %d/%d/%d/%d, want 1/0/0/0", repo.countCalls, reader.reads, storage.saved, len(repo.created))
	}
}

func TestFileServiceUploadAttachmentCountErrorStopsUpload(t *testing.T) {
	businessID := uint(7)
	repo := &validationFileRepo{countErr: errors.New("count failed")}
	storage := &aclStorageStub{}
	svc := NewFileServiceWithLimits(repo, storage, 10, 9, ".pdf", "/files/%d/download", &aclCheckerStub{allowed: true})

	_, err := svc.Upload(context.Background(), FileActor{UserID: 2}, UploadRequest{
		BusinessType: BusinessTypeDocAttachment,
		BusinessID:   &businessID,
		OriginalName: "proof.pdf",
		Reader:       strings.NewReader("%PDF-1.7"),
	})

	var appErr *types.AppError
	if !errors.As(err, &appErr) || appErr.Code != types.ErrCodeSystemError || storage.saved != 0 {
		t.Fatalf("Upload() error/save = %v/%d, want system/0", err, storage.saved)
	}
}

func TestFileServiceUploadRejectsExtensionMIMEContentMismatch(t *testing.T) {
	tests := []struct {
		name    string
		file    string
		allowed string
		body    []byte
	}{
		{name: "html as jpeg", file: "photo.jpg", allowed: ".jpg", body: []byte("<html><script>alert(1)</script></html>")},
		{name: "executable as pdf", file: "proof.pdf", allowed: ".pdf", body: []byte("MZ executable payload")},
		{name: "html as csv", file: "rows.csv", allowed: ".csv", body: []byte("<!doctype html><title>x</title>")},
		{name: "html as xlsx", file: "rows.xlsx", allowed: ".xlsx", body: []byte("<html>not office</html>")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &validationFileRepo{}
			storage := &aclStorageStub{}
			svc := NewFileServiceWithLimits(repo, storage, 10, 9, tc.allowed, "/files/%d/download", nil)
			record, err := svc.Upload(context.Background(), FileActor{UserID: 2}, UploadRequest{
				BusinessType: BusinessTypeOther,
				OriginalName: tc.file,
				Reader:       bytes.NewReader(tc.body),
			})
			var appErr *types.AppError
			if record != nil || !errors.As(err, &appErr) || appErr.Code != types.ErrCodeValidation {
				t.Fatalf("Upload() record/error = %#v/%v, want nil validation", record, err)
			}
			if storage.saved != 0 || len(repo.created) != 0 {
				t.Fatalf("save/create = %d/%d, want 0/0", storage.saved, len(repo.created))
			}
		})
	}
}

func TestMimeMatchesExtensionFamilies(t *testing.T) {
	accepted := [][2]string{{".jpg", "image/jpeg"}, {".png", "image/png"}, {".gif", "image/gif"}, {".pdf", "application/pdf"}, {".csv", "text/plain; charset=utf-8"}, {".xlsx", "application/zip"}}
	for _, pair := range accepted {
		if !mimeMatchesExtension(pair[0], pair[1]) {
			t.Errorf("mimeMatchesExtension(%q, %q) = false, want true", pair[0], pair[1])
		}
	}
	for _, ext := range []string{".jpg", ".png", ".gif", ".pdf", ".csv", ".xlsx"} {
		if mimeMatchesExtension(ext, "text/html") {
			t.Errorf("mimeMatchesExtension(%q, text/html) = true, want false", ext)
		}
	}
}
