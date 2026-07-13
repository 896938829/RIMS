// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package idempotency

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeRepository struct {
	record       *Record
	status       *OperationStatus
	getErrs      []error
	statusErr    error
	createErr    error
	created      *Record
	deleted      bool
	statusUserID uint
	statusScope  string
	statusKey    string
}

func (r *fakeRepository) Get(ctx context.Context, userID uint, scope, key string) (*Record, error) {
	if len(r.getErrs) > 0 {
		err := r.getErrs[0]
		r.getErrs = r.getErrs[1:]
		if err != nil {
			return nil, err
		}
	}
	if r.record == nil {
		return nil, ErrRecordNotFound
	}
	return r.record, nil
}

func (r *fakeRepository) Create(ctx context.Context, record *Record) error {
	if r.createErr != nil {
		return r.createErr
	}
	copy := *record
	r.created = &copy
	r.record = &copy
	return nil
}

func (r *fakeRepository) GetStatus(ctx context.Context, userID uint, scope, key string) (*OperationStatus, error) {
	r.statusUserID = userID
	r.statusScope = scope
	r.statusKey = key
	if r.statusErr != nil {
		return nil, r.statusErr
	}
	if r.status == nil {
		return nil, ErrRecordNotFound
	}
	copy := *r.status
	return &copy, nil
}

func (r *fakeRepository) Complete(ctx context.Context, userID uint, scope, key string, statusCode int, responseBody []byte) error {
	return nil
}

func (r *fakeRepository) DeleteProcessing(ctx context.Context, userID uint, scope, key string) error {
	return nil
}

func (r *fakeRepository) Delete(ctx context.Context, userID uint, scope, key string) error {
	r.deleted = true
	r.record = nil
	return nil
}

func TestBeginCreatesProcessingRecordForNewKey(t *testing.T) {
	repo := &fakeRepository{}
	svc := NewService(repo, time.Hour)

	decision, err := svc.Begin(context.Background(), 7, "POST /api/v1/documents", "key-1", "hash-1")
	if err != nil {
		t.Fatalf("Begin returned error: %v", err)
	}

	if decision.Type != DecisionProceed {
		t.Fatalf("decision = %v, want %v", decision.Type, DecisionProceed)
	}
	if repo.created == nil {
		t.Fatal("expected processing record to be created")
	}
	if repo.created.UserID != 7 || repo.created.Scope != "POST /api/v1/documents" ||
		repo.created.IdempotencyKey != "key-1" || repo.created.RequestHash != "hash-1" {
		t.Fatalf("created record = %+v", repo.created)
	}
	if repo.created.State != StateProcessing {
		t.Fatalf("state = %q, want %q", repo.created.State, StateProcessing)
	}
	if time.Until(repo.created.ExpiresAt) <= 0 {
		t.Fatalf("expires_at was not set in the future: %v", repo.created.ExpiresAt)
	}
}

func TestNewServiceDefaultsNonPositiveTTLToTwentyFourHours(t *testing.T) {
	repo := &fakeRepository{}
	svc := NewService(repo, 0)
	before := time.Now()

	decision, err := svc.Begin(context.Background(), 7, "POST /api/v1/documents", "key-1", "hash-1")
	after := time.Now()
	if err != nil {
		t.Fatalf("Begin returned error: %v", err)
	}

	if decision.Type != DecisionProceed {
		t.Fatalf("decision = %v, want %v", decision.Type, DecisionProceed)
	}
	if repo.created == nil {
		t.Fatal("expected processing record to be created")
	}

	minExpiresAt := before.Add(23 * time.Hour)
	maxExpiresAt := after.Add(25 * time.Hour)
	if repo.created.ExpiresAt.Before(minExpiresAt) || repo.created.ExpiresAt.After(maxExpiresAt) {
		t.Fatalf("expires_at = %v, want between %v and %v", repo.created.ExpiresAt, minExpiresAt, maxExpiresAt)
	}
}

func TestBeginReplaysCompletedRecordWithSameHash(t *testing.T) {
	repo := &fakeRepository{record: &Record{
		UserID:         7,
		Scope:          "POST /api/v1/documents",
		IdempotencyKey: "key-1",
		RequestHash:    "hash-1",
		State:          StateCompleted,
		StatusCode:     201,
		ResponseBody:   []byte(`{"code":0,"message":"success"}`),
	}}
	svc := NewService(repo, time.Hour)

	decision, err := svc.Begin(context.Background(), 7, "POST /api/v1/documents", "key-1", "hash-1")
	if err != nil {
		t.Fatalf("Begin returned error: %v", err)
	}

	if decision.Type != DecisionReplay {
		t.Fatalf("decision = %v, want %v", decision.Type, DecisionReplay)
	}
	if decision.StatusCode != 201 {
		t.Fatalf("status = %d, want 201", decision.StatusCode)
	}
	if string(decision.ResponseBody) != `{"code":0,"message":"success"}` {
		t.Fatalf("body = %s", decision.ResponseBody)
	}
}

func TestBeginRejectsSameKeyWithDifferentHash(t *testing.T) {
	repo := &fakeRepository{record: &Record{
		UserID:         7,
		Scope:          "POST /api/v1/documents",
		IdempotencyKey: "key-1",
		RequestHash:    "hash-1",
		State:          StateCompleted,
	}}
	svc := NewService(repo, time.Hour)

	_, err := svc.Begin(context.Background(), 7, "POST /api/v1/documents", "key-1", "hash-2")
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !errors.Is(err, ErrKeyReusedWithDifferentRequest) {
		t.Fatalf("error = %v, want ErrKeyReusedWithDifferentRequest", err)
	}
}

func TestBeginReturnsProcessingForDuplicateInFlightRequest(t *testing.T) {
	repo := &fakeRepository{record: &Record{
		UserID:         7,
		Scope:          "POST /api/v1/documents",
		IdempotencyKey: "key-1",
		RequestHash:    "hash-1",
		State:          StateProcessing,
	}}
	svc := NewService(repo, time.Hour)

	decision, err := svc.Begin(context.Background(), 7, "POST /api/v1/documents", "key-1", "hash-1")
	if err != nil {
		t.Fatalf("Begin returned error: %v", err)
	}

	if decision.Type != DecisionProcessing {
		t.Fatalf("decision = %v, want %v", decision.Type, DecisionProcessing)
	}
}

func TestBeginDeletesExpiredCompletedRecordAndCreatesFreshProcessingRecord(t *testing.T) {
	repo := &fakeRepository{record: &Record{
		UserID:         7,
		Scope:          "POST /api/v1/documents",
		IdempotencyKey: "key-1",
		RequestHash:    "hash-1",
		State:          StateCompleted,
		StatusCode:     201,
		ResponseBody:   []byte(`{"code":0}`),
		ExpiresAt:      time.Now().Add(-time.Minute),
	}}
	svc := NewService(repo, time.Hour)

	decision, err := svc.Begin(context.Background(), 7, "POST /api/v1/documents", "key-1", "hash-2")
	if err != nil {
		t.Fatalf("Begin returned error: %v", err)
	}

	if decision.Type != DecisionProceed {
		t.Fatalf("decision = %v, want %v", decision.Type, DecisionProceed)
	}
	if !repo.deleted {
		t.Fatal("expected expired record to be deleted")
	}
	if repo.created == nil {
		t.Fatal("expected fresh processing record to be created")
	}
	if repo.created.RequestHash != "hash-2" {
		t.Fatalf("fresh request hash = %q, want hash-2", repo.created.RequestHash)
	}
	if repo.created.State != StateProcessing {
		t.Fatalf("fresh state = %q, want %q", repo.created.State, StateProcessing)
	}
}

func TestBeginReturnsCreateErrorWithFallbackGetErrorWhenRaceRecoveryFails(t *testing.T) {
	createErr := errors.New("create processing failed")
	getErr := errors.New("fallback get failed")
	repo := &fakeRepository{
		getErrs:   []error{ErrRecordNotFound, getErr},
		createErr: createErr,
	}
	svc := NewService(repo, time.Hour)

	_, err := svc.Begin(context.Background(), 7, "POST /api/v1/documents", "key-1", "hash-1")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, createErr) {
		t.Fatalf("error = %v, want wrapped create error %v", err, createErr)
	}
	if !strings.Contains(err.Error(), getErr.Error()) {
		t.Fatalf("error = %v, want fallback get error %v in message", err, getErr)
	}
}

func TestStatusUsesCurrentUserScopeAndKey(t *testing.T) {
	expiresAt := time.Now().Add(time.Hour)
	repo := &fakeRepository{status: &OperationStatus{
		State:      StateProcessing,
		StatusCode: 0,
		ExpiresAt:  expiresAt,
	}}
	svc := NewService(repo, time.Hour)

	status, err := svc.Status(context.Background(), 17, "POST /api/v1/documents/:id/complete", "key-1")
	if err != nil {
		t.Fatalf("Status returned error: %v", err)
	}
	if repo.statusUserID != 17 || repo.statusScope != "POST /api/v1/documents/:id/complete" || repo.statusKey != "key-1" {
		t.Fatalf("lookup = user %d scope %q key %q", repo.statusUserID, repo.statusScope, repo.statusKey)
	}
	if status.State != StateProcessing || status.StatusCode != 0 || !status.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("status = %+v", status)
	}
}

func TestStatusReturnsCompletedMetadataWithoutStoredRequestOrResponse(t *testing.T) {
	expiresAt := time.Now().Add(time.Hour)
	repo := &fakeRepository{status: &OperationStatus{
		State:      StateCompleted,
		StatusCode: 201,
		ExpiresAt:  expiresAt,
	}}
	svc := NewService(repo, time.Hour)

	status, err := svc.Status(context.Background(), 7, "POST /api/v1/documents", "key-1")
	if err != nil {
		t.Fatalf("Status returned error: %v", err)
	}
	if status.State != StateCompleted || status.StatusCode != 201 || !status.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("status = %+v", status)
	}
}

func TestStatusReturnsNotFoundForMissingRecord(t *testing.T) {
	svc := NewService(&fakeRepository{}, time.Hour)

	_, err := svc.Status(context.Background(), 7, "POST /api/v1/documents", "missing")
	if !errors.Is(err, ErrRecordNotFound) {
		t.Fatalf("error = %v, want ErrRecordNotFound", err)
	}
}

func TestStatusReturnsNotFoundForExpiredRecord(t *testing.T) {
	svc := NewService(&fakeRepository{status: &OperationStatus{
		State:      StateCompleted,
		StatusCode: 201,
		ExpiresAt:  time.Now().Add(-time.Second),
	}}, time.Hour)

	_, err := svc.Status(context.Background(), 7, "POST /api/v1/documents", "expired")
	if !errors.Is(err, ErrRecordNotFound) {
		t.Fatalf("error = %v, want ErrRecordNotFound", err)
	}
}
