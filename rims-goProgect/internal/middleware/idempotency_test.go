// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package middleware

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"rims-go/internal/idempotency"
	"rims-go/internal/types"
)

type fakeIdempotencyService struct {
	beginFunc    func(ctx context.Context, userID uint, scope, key, requestHash string) (idempotency.Decision, error)
	completeFunc func(ctx context.Context, userID uint, scope, key string, statusCode int, responseBody []byte) error
	releaseFunc  func(ctx context.Context, userID uint, scope, key string) error
}

func (s *fakeIdempotencyService) Begin(ctx context.Context, userID uint, scope, key, requestHash string) (idempotency.Decision, error) {
	return s.beginFunc(ctx, userID, scope, key, requestHash)
}

func (s *fakeIdempotencyService) Complete(ctx context.Context, userID uint, scope, key string, statusCode int, responseBody []byte) error {
	if s.completeFunc == nil {
		return nil
	}
	return s.completeFunc(ctx, userID, scope, key, statusCode, responseBody)
}

func (s *fakeIdempotencyService) Release(ctx context.Context, userID uint, scope, key string) error {
	if s.releaseFunc == nil {
		return nil
	}
	return s.releaseFunc(ctx, userID, scope, key)
}

func newIdempotencyTestRouter(svc *fakeIdempotencyService, handler gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/documents", func(c *gin.Context) {
		c.Set(types.CtxKeyUserID, uint(7))
		c.Set(types.CtxKeyWarehouseID, uint(11))
		c.Next()
	}, Idempotency(svc, 0), handler)
	return r
}

func newIdempotencyObserverRouter(svc *fakeIdempotencyService, handler gin.HandlerFunc) (*gin.Engine, *int) {
	gin.SetMode(gin.TestMode)
	errCount := 0
	r := gin.New()
	r.POST("/documents", func(c *gin.Context) {
		c.Next()
		errCount = len(c.Errors)
	}, func(c *gin.Context) {
		c.Set(types.CtxKeyUserID, uint(7))
		c.Set(types.CtxKeyWarehouseID, uint(11))
		c.Next()
	}, Idempotency(svc, 0), handler)
	return r, &errCount
}

func TestRequestHashIncludesIdentityComponents(t *testing.T) {
	base := requestHash(
		http.MethodPost,
		"/documents/:id/complete",
		"/documents/1/complete",
		"dryRun=false",
		7,
		11,
		[]byte(`{"confirm":true}`),
	)

	tests := []struct {
		name         string
		method       string
		fullPath     string
		concretePath string
		rawQuery     string
		userID       uint
		warehouseID  uint
		body         []byte
	}{
		{
			name:         "body",
			method:       http.MethodPost,
			fullPath:     "/documents/:id/complete",
			concretePath: "/documents/1/complete",
			rawQuery:     "dryRun=false",
			userID:       7,
			warehouseID:  11,
			body:         []byte(`{"confirm":false}`),
		},
		{
			name:         "raw query",
			method:       http.MethodPost,
			fullPath:     "/documents/:id/complete",
			concretePath: "/documents/1/complete",
			rawQuery:     "dryRun=true",
			userID:       7,
			warehouseID:  11,
			body:         []byte(`{"confirm":true}`),
		},
		{
			name:         "user ID",
			method:       http.MethodPost,
			fullPath:     "/documents/:id/complete",
			concretePath: "/documents/1/complete",
			rawQuery:     "dryRun=false",
			userID:       8,
			warehouseID:  11,
			body:         []byte(`{"confirm":true}`),
		},
		{
			name:         "warehouse ID",
			method:       http.MethodPost,
			fullPath:     "/documents/:id/complete",
			concretePath: "/documents/1/complete",
			rawQuery:     "dryRun=false",
			userID:       7,
			warehouseID:  12,
			body:         []byte(`{"confirm":true}`),
		},
		{
			name:         "concrete path",
			method:       http.MethodPost,
			fullPath:     "/documents/:id/complete",
			concretePath: "/documents/2/complete",
			rawQuery:     "dryRun=false",
			userID:       7,
			warehouseID:  11,
			body:         []byte(`{"confirm":true}`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := requestHash(tt.method, tt.fullPath, tt.concretePath, tt.rawQuery, tt.userID, tt.warehouseID, tt.body)
			if got == base {
				t.Fatalf("hash did not change when only %s changed", tt.name)
			}
		})
	}
}

func TestIdempotencyPassesThroughWithoutHeader(t *testing.T) {
	calledBegin := false
	svc := &fakeIdempotencyService{
		beginFunc: func(ctx context.Context, userID uint, scope, key, requestHash string) (idempotency.Decision, error) {
			calledBegin = true
			return idempotency.Decision{Type: idempotency.DecisionProceed}, nil
		},
	}
	r := newIdempotencyTestRouter(svc, func(c *gin.Context) {
		c.JSON(http.StatusCreated, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/documents", strings.NewReader(`{"docType":2}`))
	r.ServeHTTP(w, req)

	if calledBegin {
		t.Fatal("Begin should not be called without Idempotency-Key")
	}
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", w.Code)
	}
}

func TestIdempotencyCachesSuccessfulResponse(t *testing.T) {
	var completedStatus int
	var completedBody string
	svc := &fakeIdempotencyService{
		beginFunc: func(ctx context.Context, userID uint, scope, key, requestHash string) (idempotency.Decision, error) {
			if userID != 7 {
				t.Fatalf("userID = %d, want 7", userID)
			}
			if scope != "POST /documents" {
				t.Fatalf("scope = %q, want POST /documents", scope)
			}
			return idempotency.Decision{Type: idempotency.DecisionProceed}, nil
		},
		completeFunc: func(ctx context.Context, userID uint, scope, key string, statusCode int, responseBody []byte) error {
			completedStatus = statusCode
			completedBody = string(responseBody)
			return nil
		},
	}
	r := newIdempotencyTestRouter(svc, func(c *gin.Context) {
		var body map[string]any
		if err := c.ShouldBindJSON(&body); err != nil {
			t.Fatalf("request body was not restored: %v", err)
		}
		c.JSON(http.StatusCreated, gin.H{"id": 123})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/documents", strings.NewReader(`{"docType":2}`))
	req.Header.Set("Idempotency-Key", "key-1")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", w.Code)
	}
	if completedStatus != http.StatusCreated {
		t.Fatalf("completed status = %d, want 201", completedStatus)
	}
	if !strings.Contains(completedBody, `"id":123`) {
		t.Fatalf("cached body = %s", completedBody)
	}
}

func TestIdempotencyCanonicalizesMultipartFormDataBoundary(t *testing.T) {
	hashes := make([]string, 0, 2)
	svc := &fakeIdempotencyService{
		beginFunc: func(ctx context.Context, userID uint, scope, key, requestHash string) (idempotency.Decision, error) {
			hashes = append(hashes, requestHash)
			return idempotency.Decision{Type: idempotency.DecisionProceed}, nil
		},
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/files/upload", func(c *gin.Context) {
		c.Set(types.CtxKeyUserID, uint(7))
		c.Next()
	}, Idempotency(svc, 0), func(c *gin.Context) {
		if c.PostForm("businessType") != "doc_attachment" {
			t.Fatalf("businessType = %q, want doc_attachment", c.PostForm("businessType"))
		}
		fh, err := c.FormFile("file")
		if err != nil {
			t.Fatalf("request body was not restored as multipart: %v", err)
		}
		c.JSON(http.StatusCreated, gin.H{"filename": fh.Filename})
	})

	for _, boundary := range []string{"boundary-one", "boundary-two"} {
		body, contentType := idempotencyMultipartBody(t, boundary, map[string]string{
			"businessType": "doc_attachment",
			"businessId":   "9",
		}, "file", "receipt.txt", "same file body")
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/files/upload", body)
		req.Header.Set("Content-Type", contentType)
		req.Header.Set("Idempotency-Key", "upload-key")
		r.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
		}
	}

	if len(hashes) != 2 {
		t.Fatalf("hash count = %d, want 2", len(hashes))
	}
	if hashes[0] != hashes[1] {
		t.Fatalf("multipart hashes differ for equivalent bodies: %q != %q", hashes[0], hashes[1])
	}
}

func TestIdempotencyMultipartHashPreservesDuplicateFieldOrder(t *testing.T) {
	hashes := make([]string, 0, 2)
	svc := &fakeIdempotencyService{
		beginFunc: func(ctx context.Context, userID uint, scope, key, requestHash string) (idempotency.Decision, error) {
			hashes = append(hashes, requestHash)
			return idempotency.Decision{Type: idempotency.DecisionProceed}, nil
		},
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/files/upload", func(c *gin.Context) {
		c.Set(types.CtxKeyUserID, uint(7))
		c.Next()
	}, Idempotency(svc, 0), func(c *gin.Context) {
		c.JSON(http.StatusCreated, gin.H{"businessId": c.PostForm("businessId")})
	})

	requests := [][]idempotencyMultipartField{
		{{key: "businessId", value: "1"}, {key: "businessId", value: "2"}},
		{{key: "businessId", value: "2"}, {key: "businessId", value: "1"}},
	}
	for _, fields := range requests {
		body, contentType := idempotencyMultipartBodyWithFields(t, "same-boundary", fields, "file", "receipt.txt", "same file body")
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/files/upload", body)
		req.Header.Set("Content-Type", contentType)
		req.Header.Set("Idempotency-Key", "upload-key")
		r.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
		}
	}

	if len(hashes) != 2 {
		t.Fatalf("hash count = %d, want 2", len(hashes))
	}
	if hashes[0] == hashes[1] {
		t.Fatal("multipart hashes matched for requests with different duplicate field order")
	}
}

func TestIdempotencyReplaysCompletedResponse(t *testing.T) {
	svc := &fakeIdempotencyService{
		beginFunc: func(ctx context.Context, userID uint, scope, key, requestHash string) (idempotency.Decision, error) {
			return idempotency.Decision{
				Type:         idempotency.DecisionReplay,
				StatusCode:   http.StatusCreated,
				ResponseBody: []byte(`{"code":0,"message":"success","data":{"id":123}}`),
			}, nil
		},
		completeFunc: func(ctx context.Context, userID uint, scope, key string, statusCode int, responseBody []byte) error {
			t.Fatal("Complete should not be called when replaying")
			return nil
		},
	}
	r := newIdempotencyTestRouter(svc, func(c *gin.Context) {
		t.Fatal("handler should not run on replay")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/documents", strings.NewReader(`{"docType":2}`))
	req.Header.Set("Idempotency-Key", "key-1")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", w.Code)
	}
	if strings.TrimSpace(w.Body.String()) != `{"code":0,"message":"success","data":{"id":123}}` {
		t.Fatalf("body = %s", w.Body.String())
	}
}

func TestIdempotencyRejectsSameKeyAndBodyOnDifferentConcretePath(t *testing.T) {
	var firstHash string
	svc := &fakeIdempotencyService{
		beginFunc: func(ctx context.Context, userID uint, scope, key, requestHash string) (idempotency.Decision, error) {
			if scope != "POST /documents/:id/complete" {
				t.Fatalf("scope = %q, want route pattern scope", scope)
			}
			if firstHash == "" {
				firstHash = requestHash
				return idempotency.Decision{Type: idempotency.DecisionProceed}, nil
			}
			if requestHash != firstHash {
				return idempotency.Decision{}, idempotency.ErrKeyReusedWithDifferentRequest
			}
			return idempotency.Decision{
				Type:         idempotency.DecisionReplay,
				StatusCode:   http.StatusOK,
				ResponseBody: []byte(`{"wrong":"replay"}`),
			}, nil
		},
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/documents/:id/complete", func(c *gin.Context) {
		c.Set(types.CtxKeyUserID, uint(7))
		c.Set(types.CtxKeyWarehouseID, uint(11))
		c.Next()
	}, Idempotency(svc, 0), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"completed": c.Param("id")})
	})

	first := httptest.NewRecorder()
	firstReq := httptest.NewRequest(http.MethodPost, "/documents/1/complete", strings.NewReader(`{"confirm":true}`))
	firstReq.Header.Set("Idempotency-Key", "key-1")
	r.ServeHTTP(first, firstReq)
	if first.Code != http.StatusOK {
		t.Fatalf("first status = %d, want 200", first.Code)
	}

	second := httptest.NewRecorder()
	secondReq := httptest.NewRequest(http.MethodPost, "/documents/2/complete", strings.NewReader(`{"confirm":true}`))
	secondReq.Header.Set("Idempotency-Key", "key-1")
	r.ServeHTTP(second, secondReq)
	if second.Code != http.StatusBadRequest {
		t.Fatalf("second status = %d, want 400; body=%s", second.Code, second.Body.String())
	}
	if !strings.Contains(second.Body.String(), `"code":10003`) {
		t.Fatalf("second body = %s", second.Body.String())
	}
}

func TestIdempotencyRejectsDifferentBodyForSameKey(t *testing.T) {
	svc := &fakeIdempotencyService{
		beginFunc: func(ctx context.Context, userID uint, scope, key, requestHash string) (idempotency.Decision, error) {
			return idempotency.Decision{}, idempotency.ErrKeyReusedWithDifferentRequest
		},
	}
	r := newIdempotencyTestRouter(svc, func(c *gin.Context) {
		t.Fatal("handler should not run when key is reused with different body")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/documents", strings.NewReader(`{"docType":3}`))
	req.Header.Set("Idempotency-Key", "key-1")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"code":10003`) {
		t.Fatalf("body = %s", w.Body.String())
	}
}

func TestIdempotencyRejectsBodyLargerThanConfiguredLimit(t *testing.T) {
	calledBegin := false
	svc := &fakeIdempotencyService{
		beginFunc: func(ctx context.Context, userID uint, scope, key, requestHash string) (idempotency.Decision, error) {
			calledBegin = true
			return idempotency.Decision{Type: idempotency.DecisionProceed}, nil
		},
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/documents", func(c *gin.Context) {
		c.Set(types.CtxKeyUserID, uint(7))
		c.Set(types.CtxKeyWarehouseID, uint(11))
		c.Next()
	}, Idempotency(svc, 1), func(c *gin.Context) {
		t.Fatal("handler should not run for oversized body")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/documents", io.LimitReader(strings.NewReader(strings.Repeat("x", 1024*1024+1)), 1024*1024+1))
	req.Header.Set("Idempotency-Key", "key-1")
	r.ServeHTTP(w, req)

	if calledBegin {
		t.Fatal("Begin should not be called for oversized body")
	}
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", w.Code)
	}
}

func TestIdempotencyAllowsMultipartEnvelopeOverheadAroundConfiguredFileLimit(t *testing.T) {
	calledBegin := false
	svc := &fakeIdempotencyService{
		beginFunc: func(ctx context.Context, userID uint, scope, key, requestHash string) (idempotency.Decision, error) {
			calledBegin = true
			return idempotency.Decision{Type: idempotency.DecisionProceed}, nil
		},
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/files/upload", func(c *gin.Context) {
		c.Set(types.CtxKeyUserID, uint(7))
		c.Next()
	}, Idempotency(svc, 1), func(c *gin.Context) {
		if _, err := c.FormFile("file"); err != nil {
			t.Fatalf("request body was not restored as multipart: %v", err)
		}
		c.JSON(http.StatusCreated, gin.H{"ok": true})
	})

	body, contentType := idempotencyMultipartBody(t, "boundary-one", map[string]string{
		"businessType": "doc_attachment",
	}, "file", "receipt.txt", strings.Repeat("x", 1024*1024))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/files/upload", body)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Idempotency-Key", "upload-key")
	r.ServeHTTP(w, req)

	if !calledBegin {
		t.Fatal("Begin should be called for multipart body whose file content is at the configured limit")
	}
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
	}
}

func TestIdempotencyReturnsConflictForProcessingDuplicate(t *testing.T) {
	svc := &fakeIdempotencyService{
		beginFunc: func(ctx context.Context, userID uint, scope, key, requestHash string) (idempotency.Decision, error) {
			return idempotency.Decision{Type: idempotency.DecisionProcessing}, nil
		},
	}
	r := newIdempotencyTestRouter(svc, func(c *gin.Context) {
		t.Fatal("handler should not run for processing duplicate")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/documents", strings.NewReader(`{"docType":2}`))
	req.Header.Set("Idempotency-Key", "key-1")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"code":20002`) {
		t.Fatalf("body = %s", w.Body.String())
	}
}

func TestIdempotencyReleasesKeyForServerError(t *testing.T) {
	released := false
	svc := &fakeIdempotencyService{
		beginFunc: func(ctx context.Context, userID uint, scope, key, requestHash string) (idempotency.Decision, error) {
			return idempotency.Decision{Type: idempotency.DecisionProceed}, nil
		},
		completeFunc: func(ctx context.Context, userID uint, scope, key string, statusCode int, responseBody []byte) error {
			t.Fatal("Complete should not be called for 5xx responses")
			return nil
		},
		releaseFunc: func(ctx context.Context, userID uint, scope, key string) error {
			released = true
			return nil
		},
	}
	r := newIdempotencyTestRouter(svc, func(c *gin.Context) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "boom"})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/documents", strings.NewReader(`{"docType":2}`))
	req.Header.Set("Idempotency-Key", "key-1")
	r.ServeHTTP(w, req)

	if !released {
		t.Fatal("expected key to be released for 5xx response")
	}
}

func TestIdempotencyReleasesKeyForBadRequestAndDoesNotComplete(t *testing.T) {
	released := false
	svc := &fakeIdempotencyService{
		beginFunc: func(ctx context.Context, userID uint, scope, key, requestHash string) (idempotency.Decision, error) {
			return idempotency.Decision{Type: idempotency.DecisionProceed}, nil
		},
		completeFunc: func(ctx context.Context, userID uint, scope, key string, statusCode int, responseBody []byte) error {
			t.Fatal("Complete should not be called for 4xx responses")
			return nil
		},
		releaseFunc: func(ctx context.Context, userID uint, scope, key string) error {
			released = true
			return nil
		},
	}
	r := newIdempotencyTestRouter(svc, func(c *gin.Context) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad request"})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/documents", strings.NewReader(`{"docType":2}`))
	req.Header.Set("Idempotency-Key", "key-1")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if !released {
		t.Fatal("expected key to be released for 4xx response")
	}
}

func TestIdempotencySurfacesCompleteErrorWithoutChangingClientResponse(t *testing.T) {
	svc := &fakeIdempotencyService{
		beginFunc: func(ctx context.Context, userID uint, scope, key, requestHash string) (idempotency.Decision, error) {
			return idempotency.Decision{Type: idempotency.DecisionProceed}, nil
		},
		completeFunc: func(ctx context.Context, userID uint, scope, key string, statusCode int, responseBody []byte) error {
			return errors.New("complete failed")
		},
	}
	r, errCount := newIdempotencyObserverRouter(svc, func(c *gin.Context) {
		c.JSON(http.StatusCreated, gin.H{"id": 123})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/documents", strings.NewReader(`{"docType":2}`))
	req.Header.Set("Idempotency-Key", "key-1")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", w.Code)
	}
	if *errCount != 1 {
		t.Fatalf("c.Errors count = %d, want 1", *errCount)
	}
}

func TestIdempotencySurfacesReleaseErrorWithoutChangingClientResponse(t *testing.T) {
	svc := &fakeIdempotencyService{
		beginFunc: func(ctx context.Context, userID uint, scope, key, requestHash string) (idempotency.Decision, error) {
			return idempotency.Decision{Type: idempotency.DecisionProceed}, nil
		},
		releaseFunc: func(ctx context.Context, userID uint, scope, key string) error {
			return errors.New("release failed")
		},
	}
	r, errCount := newIdempotencyObserverRouter(svc, func(c *gin.Context) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad request"})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/documents", strings.NewReader(`{"docType":2}`))
	req.Header.Set("Idempotency-Key", "key-1")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if *errCount != 1 {
		t.Fatalf("c.Errors count = %d, want 1", *errCount)
	}
}

func TestIdempotencyReturnsSystemErrorWhenBeginFails(t *testing.T) {
	svc := &fakeIdempotencyService{
		beginFunc: func(ctx context.Context, userID uint, scope, key, requestHash string) (idempotency.Decision, error) {
			return idempotency.Decision{}, errors.New("db down")
		},
	}
	r := newIdempotencyTestRouter(svc, func(c *gin.Context) {
		t.Fatal("handler should not run when Begin fails")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/documents", strings.NewReader(`{"docType":2}`))
	req.Header.Set("Idempotency-Key", "key-1")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}

type idempotencyMultipartField struct {
	key   string
	value string
}

func idempotencyMultipartBody(t *testing.T, boundary string, fields map[string]string, fileField, filename, content string) (*bytes.Buffer, string) {
	t.Helper()
	ordered := make([]idempotencyMultipartField, 0, len(fields))
	for key, value := range fields {
		ordered = append(ordered, idempotencyMultipartField{key: key, value: value})
	}
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].key < ordered[j].key
	})
	return idempotencyMultipartBodyWithFields(t, boundary, ordered, fileField, filename, content)
}

func idempotencyMultipartBodyWithFields(t *testing.T, boundary string, fields []idempotencyMultipartField, fileField, filename, content string) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if err := writer.SetBoundary(boundary); err != nil {
		t.Fatalf("set multipart boundary: %v", err)
	}
	for _, field := range fields {
		if err := writer.WriteField(field.key, field.value); err != nil {
			t.Fatalf("write multipart field %s: %v", field.key, err)
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
