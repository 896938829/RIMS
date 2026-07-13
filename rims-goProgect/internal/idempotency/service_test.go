// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package idempotency

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeRepository struct {
	record               *Record
	status               *OperationStatus
	getErrs              []error
	statusErr            error
	createErr            error
	created              *Record
	deleted              bool
	statusUserID         uint
	statusScope          string
	statusKey            string
	extendLeaseErr       error
	clearOnExtendFailure bool
	extendedFrom         time.Time
	extendedUntil        time.Time
}

func (r *fakeRepository) ExtendCompletedReplayLease(
	ctx context.Context,
	userID uint,
	scope, key string,
	now, expectedExpiresAt, leaseUntil time.Time,
) error {
	r.extendedFrom = now
	r.extendedUntil = leaseUntil
	if r.extendLeaseErr != nil && r.clearOnExtendFailure {
		r.status = nil
		r.record = nil
	}
	if r.extendLeaseErr == nil {
		if r.status != nil {
			r.status.ExpiresAt = leaseUntil
		}
		if r.record != nil {
			r.record.ExpiresAt = leaseUntil
		}
	}
	return r.extendLeaseErr
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

func (r *fakeRepository) DeleteExpired(
	ctx context.Context,
	userID uint,
	scope, key, state, requestHash string,
	expectedExpiresAt, now time.Time,
) error {
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
	now := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)
	expiresAt := now.Add(time.Second)
	repo := &fakeRepository{status: &OperationStatus{
		State:      StateCompleted,
		StatusCode: 201,
		ExpiresAt:  expiresAt,
	}}
	svc := NewService(repo, time.Hour)
	svc.now = func() time.Time { return now }
	svc.replayLease = 2 * time.Minute

	status, err := svc.Status(context.Background(), 7, "POST /api/v1/documents", "key-1")
	if err != nil {
		t.Fatalf("Status returned error: %v", err)
	}
	wantExpiry := now.Add(2 * time.Minute)
	if status.State != StateCompleted || status.StatusCode != 201 || !status.ExpiresAt.Equal(wantExpiry) {
		t.Fatalf("status = %+v", status)
	}
	if !repo.extendedFrom.Equal(now) || !repo.extendedUntil.Equal(wantExpiry) {
		t.Fatalf("lease from/until = %s/%s, want %s/%s", repo.extendedFrom, repo.extendedUntil, now, wantExpiry)
	}
}

func TestStatusDoesNotReturnCompletedWhenReplayLeaseCASLosesCleanupRace(t *testing.T) {
	now := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)
	repo := &fakeRepository{
		status: &OperationStatus{
			State: StateCompleted, StatusCode: 201, ExpiresAt: now.Add(time.Millisecond),
		},
		extendLeaseErr:       ErrRecordNotFound,
		clearOnExtendFailure: true,
	}
	svc := NewService(repo, time.Hour)
	svc.now = func() time.Time { return now }

	_, err := svc.Status(context.Background(), 7, "POST /api/v1/documents", "key-1")
	if !errors.Is(err, ErrRecordNotFound) {
		t.Fatalf("Status() error = %v, want ErrRecordNotFound after lost lease race", err)
	}
}

func TestStatusBoundsCompletedReplayLease(t *testing.T) {
	now := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)
	repo := &fakeRepository{status: &OperationStatus{
		State: StateCompleted, StatusCode: 201, ExpiresAt: now.Add(time.Second),
	}}
	svc := NewService(repo, time.Hour)
	svc.now = func() time.Time { return now }
	svc.replayLease = 24 * time.Hour

	status, err := svc.Status(context.Background(), 7, "POST /api/v1/documents", "key-1")
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.ExpiresAt.After(now.Add(maxReplayLease)) {
		t.Fatalf("lease expiry = %s, exceeds max %s", status.ExpiresAt, now.Add(maxReplayLease))
	}
}

func TestBeginExpiredDeleteLosesToConcurrentStatusLeaseAndReplays(t *testing.T) {
	expiresAt := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)
	repo := newLeaseRaceRepository(&Record{
		UserID: 7, Scope: "POST /api/v1/documents", IdempotencyKey: "key-1",
		RequestHash: "hash-1", State: StateCompleted, StatusCode: 201,
		ResponseBody: []byte(`{"code":0}`), ExpiresAt: expiresAt,
	})
	beginService := NewService(repo, time.Hour)
	beginService.now = func() time.Time { return expiresAt }
	statusService := NewService(repo, time.Hour)
	statusService.now = func() time.Time { return expiresAt.Add(-time.Millisecond) }

	beginResult := make(chan beginCallResult, 1)
	go func() {
		decision, err := beginService.Begin(
			context.Background(), 7, "POST /api/v1/documents", "key-1", "hash-1",
		)
		beginResult <- beginCallResult{decision: decision, err: err}
	}()
	<-repo.deleteEntered

	statusResult := make(chan error, 1)
	go func() {
		_, err := statusService.Status(
			context.Background(), 7, "POST /api/v1/documents", "key-1",
		)
		statusResult <- err
	}()
	if err := <-statusResult; err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	close(repo.allowDelete)

	result := <-beginResult
	if result.err != nil {
		t.Fatalf("Begin() error = %v", result.err)
	}
	if result.decision.Type != DecisionReplay {
		t.Fatalf("Begin() decision = %v, want replay", result.decision.Type)
	}
	if repo.createCalls != 0 {
		t.Fatalf("Create() calls = %d, want 0", repo.createCalls)
	}
}

func TestCleanupExpiredDeleteLosesToConcurrentStatusLease(t *testing.T) {
	expiresAt := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)
	repo := newLeaseRaceRepository(&Record{
		UserID: 7, Scope: "POST /api/v1/documents", IdempotencyKey: "key-1",
		RequestHash: "hash-1", State: StateCompleted, StatusCode: 201,
		ResponseBody: []byte(`{"code":0}`), ExpiresAt: expiresAt,
	})
	statusService := NewService(repo, time.Hour)
	statusService.now = func() time.Time { return expiresAt.Add(-time.Millisecond) }

	cleanupResult := make(chan bool, 1)
	go func() {
		cleanupResult <- repo.cleanupExpired(expiresAt.Add(time.Millisecond))
	}()
	<-repo.cleanupEntered

	statusResult := make(chan error, 1)
	go func() {
		_, err := statusService.Status(
			context.Background(), 7, "POST /api/v1/documents", "key-1",
		)
		statusResult <- err
	}()
	if err := <-statusResult; err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	close(repo.allowCleanup)

	if deleted := <-cleanupResult; deleted {
		t.Fatal("cleanup deleted a record whose completed replay lease was renewed")
	}
	if repo.snapshot() == nil {
		t.Fatal("renewed record was removed")
	}
}

type beginCallResult struct {
	decision Decision
	err      error
}

type leaseRaceRepository struct {
	mu             sync.Mutex
	record         *Record
	deleteEntered  chan struct{}
	allowDelete    chan struct{}
	cleanupEntered chan struct{}
	allowCleanup   chan struct{}
	createCalls    int
}

func newLeaseRaceRepository(record *Record) *leaseRaceRepository {
	copy := *record
	return &leaseRaceRepository{
		record: &copy, deleteEntered: make(chan struct{}), allowDelete: make(chan struct{}),
		cleanupEntered: make(chan struct{}), allowCleanup: make(chan struct{}),
	}
}

func (r *leaseRaceRepository) Get(context.Context, uint, string, string) (*Record, error) {
	return r.snapshotOrError()
}

func (r *leaseRaceRepository) GetStatus(context.Context, uint, string, string) (*OperationStatus, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.record == nil {
		return nil, ErrRecordNotFound
	}
	return &OperationStatus{
		State: r.record.State, StatusCode: r.record.StatusCode, ExpiresAt: r.record.ExpiresAt,
	}, nil
}

func (r *leaseRaceRepository) ExtendCompletedReplayLease(
	_ context.Context,
	_ uint,
	_, _ string,
	now, expectedExpiresAt, leaseUntil time.Time,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.record == nil || r.record.State != StateCompleted ||
		!r.record.ExpiresAt.Equal(expectedExpiresAt) || !r.record.ExpiresAt.After(now) {
		return ErrRecordNotFound
	}
	r.record.ExpiresAt = leaseUntil
	return nil
}

func (r *leaseRaceRepository) Create(_ context.Context, record *Record) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.createCalls++
	copy := *record
	r.record = &copy
	return nil
}

func (r *leaseRaceRepository) Complete(context.Context, uint, string, string, int, []byte) error {
	return nil
}

func (r *leaseRaceRepository) DeleteProcessing(context.Context, uint, string, string) error {
	return nil
}

func (r *leaseRaceRepository) Delete(context.Context, uint, string, string) error {
	return errors.New("unconditional delete must not be used")
}

func (r *leaseRaceRepository) DeleteExpired(
	_ context.Context,
	_ uint,
	_, _, state, requestHash string,
	expectedExpiresAt, now time.Time,
) error {
	close(r.deleteEntered)
	<-r.allowDelete
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.record == nil || r.record.State != state || r.record.RequestHash != requestHash ||
		!r.record.ExpiresAt.Equal(expectedExpiresAt) || r.record.ExpiresAt.After(now) {
		return ErrRecordNotFound
	}
	r.record = nil
	return nil
}

func (r *leaseRaceRepository) cleanupExpired(cutoff time.Time) bool {
	close(r.cleanupEntered)
	<-r.allowCleanup
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.record == nil || !r.record.ExpiresAt.Before(cutoff) {
		return false
	}
	r.record = nil
	return true
}

func (r *leaseRaceRepository) snapshotOrError() (*Record, error) {
	record := r.snapshot()
	if record == nil {
		return nil, ErrRecordNotFound
	}
	return record, nil
}

func (r *leaseRaceRepository) snapshot() *Record {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.record == nil {
		return nil
	}
	copy := *r.record
	return &copy
}

func TestCompletedStatusLeaseKeepsOriginalReplaySafePastPriorExpiry(t *testing.T) {
	now := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)
	current := now
	originalExpiry := now.Add(time.Second)
	repo := &fakeRepository{
		status: &OperationStatus{
			State: StateCompleted, StatusCode: 201, ExpiresAt: originalExpiry,
		},
		record: &Record{
			UserID: 7, Scope: "POST /api/v1/documents", IdempotencyKey: "key-1",
			RequestHash: "hash-1", State: StateCompleted, StatusCode: 201,
			ResponseBody: []byte(`{"code":0}`), ExpiresAt: originalExpiry,
		},
	}
	svc := NewService(repo, time.Hour)
	svc.now = func() time.Time { return current }
	svc.replayLease = 2 * time.Minute

	status, err := svc.Status(context.Background(), 7, "POST /api/v1/documents", "key-1")
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	current = originalExpiry.Add(time.Second)
	decision, err := svc.Begin(
		context.Background(), 7, "POST /api/v1/documents", "key-1", "hash-1",
	)
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	if decision.Type != DecisionReplay || !status.ExpiresAt.After(current) {
		t.Fatalf("decision/status = %#v/%#v, want replay inside leased expiry", decision, status)
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
