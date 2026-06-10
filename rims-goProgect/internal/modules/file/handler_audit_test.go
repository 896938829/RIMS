// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package file

import (
	"context"
	"net/http"
	"testing"

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
