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
	"gorm.io/gorm"

	"rims-go/internal/auth"
	"rims-go/internal/types"
)

type fakeAuthUserProvider struct {
	userID      uint
	username    string
	roleID      uint
	roleCode    string
	status      int8
	err         error
	calls       int
	requestedID uint
}

func (f *fakeAuthUserProvider) GetAuthUser(ctx context.Context, userID uint) (uint, string, uint, string, int8, error) {
	f.calls++
	f.requestedID = userID
	if f.err != nil {
		return 0, "", 0, "", 0, f.err
	}
	return f.userID, f.username, f.roleID, f.roleCode, f.status, nil
}

func TestJWTAuthRejectsDisabledUserFromProvider(t *testing.T) {
	tokenSvc := auth.NewTokenService("test-secret", 1)
	token, _, err := tokenSvc.GenerateToken(9, "old-name", 3, "admin")
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	provider := &fakeAuthUserProvider{
		userID:   9,
		username: "current-name",
		roleID:   4,
		roleCode: "operator",
		status:   0,
	}
	called := false
	r := newJWTTestRouter(tokenSvc, provider, func(c *gin.Context) {
		called = true
		c.Status(http.StatusNoContent)
	})

	w := performJWTRequest(r, token)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body=%s", w.Code, w.Body.String())
	}
	assertJWTResponseCode(t, w, types.ErrCodeAuthFailed)
	if called {
		t.Fatal("handler should not be called")
	}
	if provider.calls != 1 || provider.requestedID != 9 {
		t.Fatalf("provider calls=%d requestedID=%d, want calls=1 requestedID=9", provider.calls, provider.requestedID)
	}
}

func TestJWTAuthUsesCurrentProviderIdentity(t *testing.T) {
	tokenSvc := auth.NewTokenService("test-secret", 1)
	token, _, err := tokenSvc.GenerateToken(9, "old-name", 3, "staff")
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	provider := &fakeAuthUserProvider{
		userID:   9,
		username: "current-name",
		roleID:   4,
		roleCode: "admin",
		status:   1,
	}
	r := newJWTTestRouter(tokenSvc, provider, func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"userID":   types.GetUserID(c),
			"username": types.GetUsername(c),
			"roleID":   types.GetRoleID(c),
			"roleCode": types.GetRoleCode(c),
		})
	})

	w := performJWTRequest(r, token)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var got struct {
		UserID   uint   `json:"userID"`
		Username string `json:"username"`
		RoleID   uint   `json:"roleID"`
		RoleCode string `json:"roleCode"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v; body=%s", err, w.Body.String())
	}
	if got.UserID != 9 || got.Username != "current-name" || got.RoleID != 4 || got.RoleCode != "admin" {
		t.Fatalf("context identity = %#v, want current provider values", got)
	}
}

func TestJWTAuthRejectsMissingUserFromProvider(t *testing.T) {
	tokenSvc := auth.NewTokenService("test-secret", 1)
	token, _, err := tokenSvc.GenerateToken(9, "old-name", 3, "admin")
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	provider := &fakeAuthUserProvider{err: gorm.ErrRecordNotFound}
	called := false
	r := newJWTTestRouter(tokenSvc, provider, func(c *gin.Context) {
		called = true
		c.Status(http.StatusNoContent)
	})

	w := performJWTRequest(r, token)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body=%s", w.Code, w.Body.String())
	}
	assertJWTResponseCode(t, w, types.ErrCodeAuthFailed)
	if called {
		t.Fatal("handler should not be called")
	}
}

func TestJWTAuthRejectsMissingRoleFromProvider(t *testing.T) {
	tokenSvc := auth.NewTokenService("test-secret", 1)
	token, _, err := tokenSvc.GenerateToken(9, "old-name", 3, "admin")
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	provider := &fakeAuthUserProvider{
		userID:   9,
		username: "current-name",
		status:   1,
	}
	r := newJWTTestRouter(tokenSvc, provider, func(c *gin.Context) {
		t.Fatal("handler should not be called")
	})

	w := performJWTRequest(r, token)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body=%s", w.Code, w.Body.String())
	}
	assertJWTResponseCode(t, w, types.ErrCodeAuthFailed)
}

func TestJWTAuthRejectsProviderFailure(t *testing.T) {
	tokenSvc := auth.NewTokenService("test-secret", 1)
	token, _, err := tokenSvc.GenerateToken(9, "old-name", 3, "admin")
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	provider := &fakeAuthUserProvider{err: errors.New("database unavailable")}
	r := newJWTTestRouter(tokenSvc, provider, func(c *gin.Context) {
		t.Fatal("handler should not be called")
	})

	w := performJWTRequest(r, token)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body=%s", w.Code, w.Body.String())
	}
	assertJWTResponseCode(t, w, types.ErrCodeAuthFailed)
}

func newJWTTestRouter(tokenSvc *auth.TokenService, provider *fakeAuthUserProvider, handler gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/protected", JWTAuth(tokenSvc, provider), handler)
	return r
}

func performJWTRequest(r http.Handler, token string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	return w
}

func assertJWTResponseCode(t *testing.T, w *httptest.ResponseRecorder, want int) {
	t.Helper()
	var resp types.Response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response JSON = %q, unmarshal error: %v", w.Body.String(), err)
	}
	if resp.Code != want {
		t.Fatalf("response code = %d, want %d; body=%s", resp.Code, want, w.Body.String())
	}
}
