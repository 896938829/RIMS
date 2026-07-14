// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package file

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type namespaceFileRepo struct {
	aclFileRepoStub
	fixture bool
}

type rejectingNamespaceRepo struct {
	namespaceFileRepo
	createErr error
}

func (r *rejectingNamespaceRepo) Create(context.Context, *FileAttachment) error {
	return r.createErr
}

func (r *namespaceFileRepo) IsFixtureAttachmentBinding(context.Context, string, uint) (bool, error) {
	return r.fixture, nil
}

func TestBuildObjectKeyPartitionsFixtureAndOrdinaryUploads(t *testing.T) {
	now := time.Date(2026, time.July, 14, 0, 0, 0, 0, time.UTC)
	const suffix = "0123456789abcdef0123456789abcdef"

	ordinary := buildObjectKey(now, suffix, ".bin", false)
	fixture := buildObjectKey(now, suffix, ".bin", true)

	if ordinary != "2026/07/"+suffix+".bin" {
		t.Fatalf("ordinary object key = %q", ordinary)
	}
	if fixture != "m9-e2e/2026/07/"+suffix+".bin" {
		t.Fatalf("fixture object key = %q", fixture)
	}
	if ordinary == fixture {
		t.Fatalf("object-key namespaces overlap: ordinary=%q fixture=%q", ordinary, fixture)
	}
}

func TestUploadUsesFixtureBindingNamespace(t *testing.T) {
	businessID := uint(42)
	repo := &namespaceFileRepo{fixture: true}
	storage := &mutationStorage{}
	svc := NewFileService(repo, storage, 10, ".txt", "/files/%d/download", &aclCheckerStub{allowed: true})

	record, err := svc.Upload(context.Background(), FileActor{UserID: 7}, UploadRequest{
		BusinessType: BusinessTypeDocAttachment,
		BusinessID:   &businessID,
		OriginalName: "fixture.txt",
		Reader:       strings.NewReader("fixture attachment"),
	})
	if err != nil {
		t.Fatalf("Upload returned error: %v", err)
	}
	if !strings.HasPrefix(record.ObjectKey, "m9-e2e/") {
		t.Fatalf("fixture object key = %q, want reserved namespace", record.ObjectKey)
	}
}

func TestUploadKeepsOrdinaryBindingOutsideFixtureNamespace(t *testing.T) {
	businessID := uint(43)
	repo := &namespaceFileRepo{fixture: false}
	storage := &mutationStorage{}
	svc := NewFileService(repo, storage, 10, ".txt", "/files/%d/download", &aclCheckerStub{allowed: true})

	record, err := svc.Upload(context.Background(), FileActor{UserID: 7}, UploadRequest{
		BusinessType: BusinessTypeDocAttachment,
		BusinessID:   &businessID,
		OriginalName: "ordinary.txt",
		Reader:       strings.NewReader("ordinary attachment"),
	})
	if err != nil {
		t.Fatalf("Upload returned error: %v", err)
	}
	if strings.HasPrefix(record.ObjectKey, "m9-e2e/") {
		t.Fatalf("ordinary object key entered fixture namespace: %q", record.ObjectKey)
	}
}

func TestUploadDeletesStoredObjectWhenCleanupHistoryRejectsAttachmentCreate(t *testing.T) {
	businessID := uint(45)
	repo := &rejectingNamespaceRepo{
		namespaceFileRepo: namespaceFileRepo{fixture: false},
		createErr:         errors.New("fixture cleanup history owns object key"),
	}
	storage := &mutationStorage{}
	svc := NewFileService(repo, storage, 10, ".txt", "/files/%d/download", &aclCheckerStub{allowed: true})

	_, err := svc.Upload(context.Background(), FileActor{UserID: 7}, UploadRequest{
		BusinessType: BusinessTypeDocAttachment,
		BusinessID:   &businessID,
		OriginalName: "ordinary.txt",
		Reader:       strings.NewReader("ordinary attachment"),
	})
	if err == nil {
		t.Fatal("Upload succeeded after repository rejected the tombstoned object key")
	}
	if len(storage.saved) != 1 || len(storage.deleted) != 1 || storage.saved[0] != storage.deleted[0] {
		t.Fatalf("stored-object rollback saved/deleted = %v/%v, want the rejected key deleted", storage.saved, storage.deleted)
	}
	if strings.HasPrefix(storage.saved[0], "m9-e2e/") {
		t.Fatalf("ordinary rollback test unexpectedly used fixture namespace: %q", storage.saved[0])
	}
}

func TestUploadRetainsCleanupResponsibilityWhenMetadataAndRollbackDeleteFail(t *testing.T) {
	businessID := uint(46)
	repo := &rejectingNamespaceRepo{
		namespaceFileRepo: namespaceFileRepo{fixture: false},
		createErr:         errors.New("fixture cleanup history owns object key"),
	}
	storage := &mutationStorage{deleteErr: errors.New("rollback disk unavailable")}
	svc := NewFileService(repo, storage, 10, ".txt", "/files/%d/download", &aclCheckerStub{allowed: true})

	_, err := svc.Upload(context.Background(), FileActor{UserID: 7}, UploadRequest{
		BusinessType: BusinessTypeDocAttachment,
		BusinessID:   &businessID,
		OriginalName: "ordinary.txt",
		Reader:       strings.NewReader("ordinary attachment"),
	})

	if err == nil || !strings.Contains(err.Error(), "fixture cleanup history owns object key") ||
		!strings.Contains(err.Error(), "rollback disk unavailable") {
		t.Fatalf("Upload() error = %v, want primary and rollback failures", err)
	}
	if len(storage.saved) != 1 || len(repo.cleanup) != 1 {
		t.Fatalf("saved/cleanup responsibility = %v/%v, want one durable pending object", storage.saved, repo.cleanup)
	}
	task := repo.cleanup[storage.saved[0]]
	if task.operation != "upload" || task.primaryError != "fixture cleanup history owns object key" || task.cleanupError != "rollback disk unavailable" {
		t.Fatalf("cleanup task = %#v, want upload failure evidence", task)
	}
}

func TestReplaceKeepsFixtureBindingInReservedNamespace(t *testing.T) {
	businessID := uint(44)
	original := mutationFile(4, businessID, 7, 0)
	original.ObjectKey = "m9-e2e/2026/07/old.txt"
	repo := &namespaceFileRepo{
		aclFileRepoStub: aclFileRepoStub{files: map[uint]*FileAttachment{4: &original}},
		fixture:         true,
	}
	storage := &mutationStorage{}
	svc := NewFileService(repo, storage, 10, ".txt", "/files/%d/download", nil)

	record, err := svc.Replace(context.Background(), 4, FileActor{UserID: 7}, UploadRequest{
		OriginalName: "replacement.txt",
		Reader:       strings.NewReader("replacement attachment"),
	})
	if err != nil {
		t.Fatalf("Replace returned error: %v", err)
	}
	if !strings.HasPrefix(record.ObjectKey, "m9-e2e/") {
		t.Fatalf("replacement object key = %q, want reserved namespace", record.ObjectKey)
	}
}

func TestSameRandomSuffixCannotCrossFixtureNamespace(t *testing.T) {
	now := time.Date(2026, time.July, 14, 0, 0, 0, 0, time.UTC)
	const suffix = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	checkedFixtureKey := buildObjectKey(now, suffix, ".bin", true)
	ordinaryUploadAfterCheck := buildObjectKey(now, suffix, ".bin", false)

	if checkedFixtureKey == ordinaryUploadAfterCheck {
		t.Fatalf("ordinary upload reused cleanup key %q after active-reference check", checkedFixtureKey)
	}
	if !strings.HasPrefix(checkedFixtureKey, "m9-e2e/") || strings.HasPrefix(ordinaryUploadAfterCheck, "m9-e2e/") {
		t.Fatalf("namespace invariant broken: fixture=%q ordinary=%q", checkedFixtureKey, ordinaryUploadAfterCheck)
	}
}
