// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"rims-go/internal/types"
)

type fakePermissionChecker struct {
	allowed bool
	err     error
	calls   int
	roleID  uint
	code    string
}

func (f *fakePermissionChecker) HasPermission(ctx context.Context, roleID uint, code string) (bool, error) {
	f.calls++
	f.roleID = roleID
	f.code = code
	return f.allowed, f.err
}

func newPermissionTestRouter(
	checker PermissionChecker,
	code string,
	setup func(*gin.Context),
	called *bool,
) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/protected", func(c *gin.Context) {
		if setup != nil {
			setup(c)
		}
		c.Next()
	}, Permission(checker, code), func(c *gin.Context) {
		*called = true
		c.Status(http.StatusNoContent)
	})
	return r
}

func performPermissionRequest(r http.Handler) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	r.ServeHTTP(w, req)
	return w
}

func assertPermissionResponseCode(t *testing.T, w *httptest.ResponseRecorder, want int) {
	t.Helper()
	var resp types.Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response JSON = %q, unmarshal error: %v", w.Body.String(), err)
	}
	if resp.Code != want {
		t.Fatalf("response code = %d, want %d; body=%s", resp.Code, want, w.Body.String())
	}
}

func TestPermissionAllowsAdminWithoutChecker(t *testing.T) {
	called := false
	r := newPermissionTestRouter(nil, "product:create", func(c *gin.Context) {
		c.Set(types.CtxKeyRoleCode, "admin")
	}, &called)

	w := performPermissionRequest(r)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body=%s", w.Code, w.Body.String())
	}
	if !called {
		t.Fatal("handler was not called")
	}
}

func TestPermissionAllowsRoleWithPermission(t *testing.T) {
	checker := &fakePermissionChecker{allowed: true}
	called := false
	r := newPermissionTestRouter(checker, "product:create", func(c *gin.Context) {
		c.Set(types.CtxKeyRoleID, uint(3))
		c.Set(types.CtxKeyRoleCode, "staff")
	}, &called)

	w := performPermissionRequest(r)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body=%s", w.Code, w.Body.String())
	}
	if !called {
		t.Fatal("handler was not called")
	}
	if checker.calls != 1 {
		t.Fatalf("checker calls = %d, want 1", checker.calls)
	}
	if checker.roleID != 3 || checker.code != "product:create" {
		t.Fatalf("checker saw roleID=%d code=%q, want roleID=3 code=product:create", checker.roleID, checker.code)
	}
}

func TestPermissionDeniesRoleWithoutPermission(t *testing.T) {
	checker := &fakePermissionChecker{allowed: false}
	called := false
	r := newPermissionTestRouter(checker, "product:create", func(c *gin.Context) {
		c.Set(types.CtxKeyRoleID, uint(3))
		c.Set(types.CtxKeyRoleCode, "staff")
	}, &called)

	w := performPermissionRequest(r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", w.Code, w.Body.String())
	}
	assertPermissionResponseCode(t, w, types.ErrCodePermissionDenied)
	if called {
		t.Fatal("handler should not be called")
	}
	if checker.calls != 1 {
		t.Fatalf("checker calls = %d, want 1", checker.calls)
	}
}

func TestPermissionRejectsMissingRoleID(t *testing.T) {
	checker := &fakePermissionChecker{allowed: true}
	called := false
	r := newPermissionTestRouter(checker, "product:create", func(c *gin.Context) {
		c.Set(types.CtxKeyRoleCode, "staff")
	}, &called)

	w := performPermissionRequest(r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", w.Code, w.Body.String())
	}
	assertPermissionResponseCode(t, w, types.ErrCodePermissionDenied)
	if called {
		t.Fatal("handler should not be called")
	}
	if checker.calls != 0 {
		t.Fatalf("checker calls = %d, want 0", checker.calls)
	}
}

func TestPermissionReturnsSystemErrorOnCheckerFailure(t *testing.T) {
	checker := &fakePermissionChecker{err: errors.New("db down")}
	called := false
	r := newPermissionTestRouter(checker, "product:create", func(c *gin.Context) {
		c.Set(types.CtxKeyRoleID, uint(3))
		c.Set(types.CtxKeyRoleCode, "staff")
	}, &called)

	w := performPermissionRequest(r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body=%s", w.Code, w.Body.String())
	}
	assertPermissionResponseCode(t, w, types.ErrCodeSystemError)
	if called {
		t.Fatal("handler should not be called")
	}
	if checker.calls != 1 {
		t.Fatalf("checker calls = %d, want 1", checker.calls)
	}
}
