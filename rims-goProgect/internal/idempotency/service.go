// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package idempotency

import (
	"context"
	"errors"
	"fmt"
	"time"

	"rims-go/internal/types"
)

const (
	defaultTTL         = 24 * time.Hour
	defaultReplayLease = 2 * time.Minute
	maxReplayLease     = 5 * time.Minute
)

var ErrKeyReusedWithDifferentRequest = errors.New("idempotency key reused with different request")

type DecisionType int

const (
	DecisionProceed DecisionType = iota + 1
	DecisionReplay
	DecisionProcessing
)

// Decision tells callers how to handle an idempotency key.
type Decision struct {
	Type         DecisionType
	StatusCode   int
	ResponseBody []byte
}

// OperationStatus is the safe, public view of an idempotency record.
type OperationStatus struct {
	State      string    `json:"state"`
	StatusCode int       `json:"status_code"`
	ExpiresAt  time.Time `json:"expires_at"`
}

// Service coordinates idempotency key state transitions.
type Service struct {
	repo        Repository
	ttl         time.Duration
	now         func() time.Time
	replayLease time.Duration
}

// NewService creates an idempotency service. Non-positive TTL values default
// to 24 hours.
func NewService(repo Repository, ttl time.Duration) *Service {
	if ttl <= 0 {
		ttl = defaultTTL
	}
	return &Service{
		repo:        repo,
		ttl:         ttl,
		now:         time.Now,
		replayLease: defaultReplayLease,
	}
}

// Begin reserves a key or returns the cached state for an existing key.
func (s *Service) Begin(ctx context.Context, userID uint, scope, key, requestHash string) (Decision, error) {
	now := s.now()
	record, err := s.repo.Get(ctx, userID, scope, key)
	if errors.Is(err, ErrRecordNotFound) {
		if createErr := s.createProcessing(ctx, userID, scope, key, requestHash, now); createErr != nil {
			return s.beginExisting(ctx, userID, scope, key, requestHash, createErr)
		}
		return Decision{Type: DecisionProceed}, nil
	}
	if err != nil {
		return Decision{}, err
	}
	return s.decisionForRecord(ctx, record, requestHash, now)
}

func (s *Service) beginExisting(ctx context.Context, userID uint, scope, key, requestHash string, createErr error) (Decision, error) {
	record, err := s.repo.Get(ctx, userID, scope, key)
	if err != nil {
		return Decision{}, fmt.Errorf("create idempotency processing record: %w; fallback get failed: %v", createErr, err)
	}
	decision, err := s.decisionForRecord(ctx, record, requestHash, s.now())
	if err != nil {
		return Decision{}, err
	}
	return decision, nil
}

func (s *Service) createProcessing(ctx context.Context, userID uint, scope, key, requestHash string, now time.Time) error {
	return s.repo.Create(ctx, &Record{
		UserID:         userID,
		Scope:          scope,
		IdempotencyKey: key,
		RequestHash:    requestHash,
		State:          StateProcessing,
		ExpiresAt:      now.Add(s.ttl),
	})
}

func (s *Service) decisionForRecord(ctx context.Context, record *Record, requestHash string, now time.Time) (Decision, error) {
	if !record.ExpiresAt.IsZero() && !record.ExpiresAt.After(now) {
		if err := s.repo.Delete(ctx, record.UserID, record.Scope, record.IdempotencyKey); err != nil {
			return Decision{}, err
		}
		if err := s.createProcessing(ctx, record.UserID, record.Scope, record.IdempotencyKey, requestHash, now); err != nil {
			return Decision{}, err
		}
		return Decision{Type: DecisionProceed}, nil
	}

	if record.RequestHash != requestHash {
		return Decision{}, types.NewAppError(
			types.ErrCodeValidation,
			"幂等键已用于不同请求",
			ErrKeyReusedWithDifferentRequest,
		)
	}

	switch record.State {
	case StateCompleted:
		return Decision{
			Type:         DecisionReplay,
			StatusCode:   record.StatusCode,
			ResponseBody: append([]byte(nil), record.ResponseBody...),
		}, nil
	case StateProcessing:
		return Decision{Type: DecisionProcessing}, nil
	default:
		return Decision{}, fmt.Errorf("unknown idempotency state %q", record.State)
	}
}

// Complete stores the successful response for future replays.
func (s *Service) Complete(ctx context.Context, userID uint, scope, key string, statusCode int, responseBody []byte) error {
	return s.repo.Complete(ctx, userID, scope, key, statusCode, responseBody)
}

// Release deletes an in-flight key so failed requests can be retried.
func (s *Service) Release(ctx context.Context, userID uint, scope, key string) error {
	return s.repo.DeleteProcessing(ctx, userID, scope, key)
}

// Status returns safe metadata for an unexpired idempotency record.
func (s *Service) Status(ctx context.Context, userID uint, scope, key string) (*OperationStatus, error) {
	now := s.now()
	status, err := s.repo.GetStatus(ctx, userID, scope, key)
	if err != nil {
		return nil, err
	}
	if !status.ExpiresAt.IsZero() && !status.ExpiresAt.After(now) {
		return nil, ErrRecordNotFound
	}
	if status.State == StateCompleted {
		leaseUntil := now.Add(boundedReplayLease(s.replayLease))
		if status.ExpiresAt.After(leaseUntil) {
			leaseUntil = status.ExpiresAt
		}
		if err := s.repo.ExtendCompletedReplayLease(
			ctx, userID, scope, key, now, leaseUntil,
		); err != nil {
			return nil, err
		}
		status.ExpiresAt = leaseUntil
	}
	return status, nil
}

func boundedReplayLease(lease time.Duration) time.Duration {
	if lease <= 0 {
		return defaultReplayLease
	}
	if lease > maxReplayLease {
		return maxReplayLease
	}
	return lease
}
