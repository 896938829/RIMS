// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package file

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"rims-go/internal/types"
)

func TestFileHandlerUploadDelegatesActorForBusinessACL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	businessID := uint(99)
	repo := &aclFileRepoStub{}
	storage := &aclStorageStub{}
	checker := &aclCheckerStub{allowed: true}
	handler := NewHandler(NewFileService(repo, storage, 0, ".txt", "/files/%d/download", checker))
	body, contentType := fileUploadMultipartBody(t, map[string]string{
		"businessType": BusinessTypeDocAttachment,
		"businessId":   "99",
	}, "file", "doc.txt", "file body")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/files/upload", body)
	c.Request.Header.Set("Content-Type", contentType)
	c.Set(types.CtxKeyUserID, uint(33))
	c.Set(types.CtxKeyRoleCode, "staff")

	handler.Upload(c)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if checker.calls != 1 || checker.action != FileActionCreate {
		t.Fatalf("checker calls/action = %d/%q, want 1/%q", checker.calls, checker.action, FileActionCreate)
	}
	if checker.actor.UserID != 33 || checker.actor.IsAdmin {
		t.Fatalf("checker actor = %#v, want user 33 non-admin", checker.actor)
	}
	if checker.file.BusinessID == nil || *checker.file.BusinessID != businessID {
		t.Fatalf("checker businessID = %v, want %d", checker.file.BusinessID, businessID)
	}
}

func TestFileHandlerDownloadDelegatesActorAndDoesNotApplyUploaderOnlyACL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &aclFileRepoStub{files: map[uint]*FileAttachment{
		1: privateFile(1, 10),
	}}
	storage := &aclStorageStub{}
	checker := &aclCheckerStub{allowed: true}
	handler := NewHandler(NewFileService(repo, storage, 0, "", "/files/%d/download", checker))
	c, rec := fileHandlerTestContext(http.MethodGet, "/files/1/download", "id", "1")
	c.Set(types.CtxKeyUserID, uint(20))
	c.Set(types.CtxKeyRoleCode, "staff")

	handler.Download(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if rec.Body.String() != "file body" {
		t.Fatalf("body = %q, want file body", rec.Body.String())
	}
	if checker.calls != 1 || checker.action != FileActionRead {
		t.Fatalf("checker calls/action = %d/%q, want 1/%q", checker.calls, checker.action, FileActionRead)
	}
	if checker.actor.UserID != 20 || checker.actor.IsAdmin {
		t.Fatalf("checker actor = %#v, want user 20 non-admin", checker.actor)
	}
}

func TestFileHandlerDeleteRejectsNonOwnerEvenWithBusinessAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &aclFileRepoStub{files: map[uint]*FileAttachment{
		1: privateFile(1, 10),
	}}
	checker := &aclCheckerStub{allowed: true}
	handler := NewHandler(NewFileService(repo, &aclStorageStub{}, 0, "", "/files/%d/download", checker))
	c, rec := fileHandlerTestContext(http.MethodDelete, "/files/1", "id", "1")
	c.Set(types.CtxKeyUserID, uint(77))
	c.Set(types.CtxKeyRoleCode, "staff")

	handler.Delete(c)

	if c.Writer.Status() != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", c.Writer.Status(), http.StatusForbidden, rec.Body.String())
	}
	if checker.calls != 0 {
		t.Fatalf("checker calls = %d, want 0", checker.calls)
	}
	if len(repo.softDelete) != 0 {
		t.Fatalf("soft deletes = %v, want none", repo.softDelete)
	}
}

func TestFileHandlerDeleteUsesAdminRoleInActor(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &aclFileRepoStub{files: map[uint]*FileAttachment{
		1: privateFile(1, 10),
	}}
	checker := &aclCheckerStub{allowed: false}
	handler := NewHandler(NewFileService(repo, &aclStorageStub{}, 0, "", "/files/%d/download", checker))
	c, rec := fileHandlerTestContext(http.MethodDelete, "/files/1", "id", "1")
	c.Set(types.CtxKeyUserID, uint(77))
	c.Set(types.CtxKeyRoleCode, "admin")

	handler.Delete(c)

	if c.Writer.Status() != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body=%s", c.Writer.Status(), http.StatusNoContent, rec.Body.String())
	}
	if checker.calls != 0 {
		t.Fatalf("checker calls = %d, want 0 for admin bypass", checker.calls)
	}
	if len(repo.softDelete) != 1 || repo.softDelete[0] != 1 {
		t.Fatalf("soft deletes = %v, want [1]", repo.softDelete)
	}
}

func TestFileHandlerGetDelegatesActorAndDoesNotLeakPrivateMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &aclFileRepoStub{files: map[uint]*FileAttachment{
		1: privateFile(1, 10),
	}}
	checker := &aclCheckerStub{allowed: false}
	handler := NewHandler(NewFileService(repo, &aclStorageStub{}, 0, "", "/files/%d/download", checker))
	c, rec := fileHandlerTestContext(http.MethodGet, "/files/1", "id", "1")
	c.Set(types.CtxKeyUserID, uint(20))
	c.Set(types.CtxKeyRoleCode, "staff")

	handler.Get(c)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
	if checker.calls != 1 || checker.action != FileActionRead {
		t.Fatalf("checker calls/action = %d/%q, want 1/%q", checker.calls, checker.action, FileActionRead)
	}
	if checker.actor.UserID != 20 || checker.actor.IsAdmin {
		t.Fatalf("checker actor = %#v, want user 20 non-admin", checker.actor)
	}
	if rec.Body.String() == "" || contains(rec.Body.String(), "private.txt") {
		t.Fatalf("response body leaked private metadata: %s", rec.Body.String())
	}
}

func TestFileHandlerListUsesActorAwareReadPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	private := privateFile(1, 10)
	public := publicFile(2, 10)
	repo := &aclFileRepoStub{
		listItems: []FileAttachment{*private, *public},
	}
	checker := &aclCheckerStub{allowed: false}
	handler := NewHandler(NewFileService(repo, &aclStorageStub{}, 0, "", "/files/%d/download", checker))
	c, rec := fileHandlerTestContext(http.MethodGet, "/files", "", "")
	c.Set(types.CtxKeyUserID, uint(20))
	c.Set(types.CtxKeyRoleCode, "staff")

	handler.List(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var resp types.Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v; body=%s", err, rec.Body.String())
	}
	if resp.Code != types.ErrCodeOK {
		t.Fatalf("response code = %d, want %d", resp.Code, types.ErrCodeOK)
	}
	body := rec.Body.String()
	if contains(body, "private.txt") {
		t.Fatalf("response body leaked private metadata: %s", body)
	}
	if !contains(body, "public.jpg") {
		t.Fatalf("response body missing public file: %s", body)
	}
	if checker.calls != 1 || checker.action != FileActionRead {
		t.Fatalf("checker calls/action = %d/%q, want 1/%q", checker.calls, checker.action, FileActionRead)
	}
	if checker.actor.UserID != 20 || checker.actor.IsAdmin {
		t.Fatalf("checker actor = %#v, want user 20 non-admin", checker.actor)
	}
}

func fileHandlerTestContext(method, target, paramKey, paramValue string) (*gin.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(method, target, nil)
	if paramKey != "" {
		c.Params = gin.Params{{Key: paramKey, Value: paramValue}}
	}
	return c, rec
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

func fileUploadMultipartBody(t *testing.T, fields map[string]string, fileField, filename, content string) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatalf("write multipart field %s: %v", key, err)
		}
	}
	part, err := writer.CreateFormFile(fileField, filename)
	if err != nil {
		t.Fatalf("create multipart file: %v", err)
	}
	if _, err := part.Write([]byte(content)); err != nil {
		t.Fatalf("write multipart file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	return body, writer.FormDataContentType()
}
