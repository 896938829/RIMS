// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package file

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"rims-go/internal/modules/audit"
	"rims-go/internal/types"
)

type fileHandlerAuditLogger struct {
	entries []audit.Entry
}

func (l *fileHandlerAuditLogger) Log(ctx context.Context, e audit.Entry) error {
	l.entries = append(l.entries, e)
	return nil
}

func TestFileHandlerAuditsDelete(t *testing.T) {
	repo := &aclFileRepoStub{files: map[uint]*FileAttachment{
		1: privateFile(1, 10),
	}}
	logger := &fileHandlerAuditLogger{}
	handler := NewHandler(NewFileService(repo, &aclStorageStub{}, 0, "", "/files/%d/download", nil), logger)
	c, rec := fileHandlerTestContext(http.MethodDelete, "/files/1", "id", "1")
	c.Set(types.CtxKeyUserID, uint(10))
	c.Set(types.CtxKeyUsername, "uploader")
	c.Set(types.CtxKeyRoleCode, "staff")
	c.Set(types.CtxKeyTraceID, "trace-file-audit")

	handler.Delete(c)

	if c.Writer.Status() != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body=%s", c.Writer.Status(), http.StatusNoContent, rec.Body.String())
	}
	if len(logger.entries) != 1 {
		t.Fatalf("audit entries = %d, want 1", len(logger.entries))
	}
	got := logger.entries[0]
	if got.Action != audit.ActionDelete || got.Resource != audit.ResourceFile {
		t.Fatalf("entry action/resource = %q/%q, want delete/file", got.Action, got.Resource)
	}
	if got.ResourceID == nil || *got.ResourceID != 1 {
		t.Fatalf("resourceID = %v, want 1", got.ResourceID)
	}
	if got.Actor.UserID != 10 || got.Actor.Username != "uploader" {
		t.Fatalf("actor = %#v, want uploader", got.Actor)
	}
}

func TestFileHandlerAuditsUpload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &aclFileRepoStub{}
	storage := &aclStorageStub{}
	checker := &aclCheckerStub{allowed: true}
	logger := &fileHandlerAuditLogger{}
	handler := NewHandler(NewFileService(repo, storage, 0, ".txt", "/files/%d/download", checker), logger)

	body, contentType := fileUploadMultipartBody(t, map[string]string{
		"businessType": BusinessTypeDocAttachment,
		"businessId":   "42",
	}, "file", "receipt.txt", "file body")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/files/upload", body)
	c.Request.Header.Set("Content-Type", contentType)
	c.Set(types.CtxKeyUserID, uint(10))
	c.Set(types.CtxKeyUsername, "uploader")
	c.Set(types.CtxKeyRoleCode, "staff")
	c.Set(types.CtxKeyTraceID, "trace-file-upload-audit")

	handler.Upload(c)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if len(logger.entries) != 1 {
		t.Fatalf("audit entries = %d, want 1", len(logger.entries))
	}
	got := logger.entries[0]
	if got.Action != audit.ActionCreate || got.Resource != audit.ResourceFile {
		t.Fatalf("entry action/resource = %q/%q, want create/file", got.Action, got.Resource)
	}
	if got.ResourceID == nil || *got.ResourceID != 100 {
		t.Fatalf("resourceID = %v, want 100", got.ResourceID)
	}
	if got.Actor.UserID != 10 || got.Actor.Username != "uploader" {
		t.Fatalf("actor = %#v, want uploader", got.Actor)
	}
	if got.After["filename"] != "receipt.txt" || got.After["businessType"] != BusinessTypeDocAttachment {
		t.Fatalf("upload details = %#v, want filename/businessType", got.After)
	}
	if got.After["businessID"] != uint(42) || got.After["isPublic"] != false {
		t.Fatalf("upload details = %#v, want businessID/private flag", got.After)
	}
}
