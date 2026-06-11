// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func performCORSRequest(origins, requestOrigin string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(CORS(origins))
	router.GET("/ping", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set("Origin", requestOrigin)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func TestCORSWildcardReturnsLiteralStar(t *testing.T) {
	recorder := performCORSRequest("*", "https://untrusted.example")

	got := recorder.Header().Get("Access-Control-Allow-Origin")
	if got != "*" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want %q", got, "*")
	}
}

func TestCORSAllowlistEchoesMatchedOrigin(t *testing.T) {
	recorder := performCORSRequest("https://app.example, https://admin.example", "https://admin.example")

	got := recorder.Header().Get("Access-Control-Allow-Origin")
	if got != "https://admin.example" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want matched origin", got)
	}
}

func TestCORSAllowlistMismatchOmitsAllowOrigin(t *testing.T) {
	recorder := performCORSRequest("https://app.example", "https://untrusted.example")

	got := recorder.Header().Get("Access-Control-Allow-Origin")
	if got != "" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want no header", got)
	}
}
