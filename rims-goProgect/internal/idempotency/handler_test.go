// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package idempotency

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"rims-go/internal/types"
)

type statusServiceStub struct {
	status *OperationStatus
	err    error
	calls  int
	userID uint
	scope  string
	key    string
}

func (s *statusServiceStub) Status(_ context.Context, userID uint, scope, key string) (*OperationStatus, error) {
	s.calls++
	s.userID = userID
	s.scope = scope
	s.key = key
	return s.status, s.err
}

func TestHandlerUsesCurrentJWTUserForStatusLookup(t *testing.T) {
	svc := &statusServiceStub{status: &OperationStatus{
		State:     StateProcessing,
		ExpiresAt: time.Now().Add(time.Hour),
	}}
	router := statusTestRouter(svc, 17)

	response := performStatusRequest(router, "key-1", "POST /api/v1/documents", "&user_id=99")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if svc.userID != 17 || svc.scope != "POST /api/v1/documents" || svc.key != "key-1" {
		t.Fatalf("lookup = user %d scope %q key %q", svc.userID, svc.scope, svc.key)
	}
}

func TestHandlerRejectsScopeOutsideRegisteredIdempotentMutations(t *testing.T) {
	svc := &statusServiceStub{}
	router := statusTestRouter(svc, 7)

	response := performStatusRequest(router, "key-1", "POST /api/v1/products", "")

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if svc.calls != 0 {
		t.Fatalf("service calls = %d, want 0", svc.calls)
	}
}

func TestHandlerReturnsNotFoundForAbsentOperation(t *testing.T) {
	svc := &statusServiceStub{err: ErrRecordNotFound}
	router := statusTestRouter(svc, 7)

	response := performStatusRequest(router, "missing", "POST /api/v1/documents", "")

	assertNotFoundResponse(t, response)
}

func TestHandlerReturnsNotFoundForExpiredOperation(t *testing.T) {
	svc := &statusServiceStub{err: ErrRecordNotFound}
	router := statusTestRouter(svc, 7)

	response := performStatusRequest(router, "expired", "POST /api/v1/documents", "")

	assertNotFoundResponse(t, response)
}

func TestHandlerReturnsProcessingStatus(t *testing.T) {
	expiresAt := time.Date(2026, 7, 14, 1, 2, 3, 0, time.UTC)
	svc := &statusServiceStub{status: &OperationStatus{
		State:      StateProcessing,
		StatusCode: 0,
		ExpiresAt:  expiresAt,
	}}
	router := statusTestRouter(svc, 7)

	response := performStatusRequest(router, "key-1", "POST /api/v1/files/upload", "")

	data := assertStatusResponse(t, response)
	if data["state"] != StateProcessing || data["status_code"] != float64(0) || data["expires_at"] != expiresAt.Format(time.RFC3339) {
		t.Fatalf("data = %#v", data)
	}
}

func TestHandlerReturnsCompletedStatusWithoutSensitiveFields(t *testing.T) {
	expiresAt := time.Date(2026, 7, 14, 1, 2, 3, 0, time.UTC)
	svc := &statusServiceStub{status: &OperationStatus{
		State:      StateCompleted,
		StatusCode: http.StatusCreated,
		ExpiresAt:  expiresAt,
	}}
	router := statusTestRouter(svc, 7)

	response := performStatusRequest(router, "key-1", "POST /api/v1/documents/:id/complete", "")

	data := assertStatusResponse(t, response)
	if len(data) != 3 {
		t.Fatalf("data keys = %#v, want only state/status_code/expires_at", data)
	}
	if data["state"] != StateCompleted || data["status_code"] != float64(http.StatusCreated) {
		t.Fatalf("data = %#v", data)
	}
	body := response.Body.String()
	for _, forbidden := range []string{"request_hash", "response_body", "stored-secret"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("response leaked %q: %s", forbidden, body)
		}
	}
}

func statusTestRouter(service StatusService, userID uint) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := router.Group("/api/v1")
	RegisterRoutes(api, NewHandler(service), func(c *gin.Context) {
		c.Set(types.CtxKeyUserID, userID)
		c.Next()
	})
	return router
}

func performStatusRequest(router http.Handler, key, scope, suffix string) *httptest.ResponseRecorder {
	path := "/api/v1/operations/idempotency/" + url.PathEscape(key) + "?scope=" + url.QueryEscape(scope) + suffix
	request := httptest.NewRequest(http.MethodGet, path, nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func assertNotFoundResponse(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var envelope struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Code != types.ErrCodeNotFound {
		t.Fatalf("code = %d, want %d", envelope.Code, types.ErrCodeNotFound)
	}
}

func assertStatusResponse(t *testing.T, response *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var envelope struct {
		Code int                    `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Code != types.ErrCodeOK {
		t.Fatalf("code = %d, want %d", envelope.Code, types.ErrCodeOK)
	}
	return envelope.Data
}
