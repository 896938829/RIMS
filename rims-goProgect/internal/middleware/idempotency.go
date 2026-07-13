// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package middleware

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"rims-go/internal/idempotency"
	"rims-go/internal/types"
)

// IdempotencyService is the service surface required by the idempotency
// middleware.
type IdempotencyService interface {
	Begin(ctx context.Context, userID uint, scope, key, requestHash string) (idempotency.Decision, error)
	Complete(ctx context.Context, userID uint, scope, key string, statusCode int, responseBody []byte) error
	Release(ctx context.Context, userID uint, scope, key string) error
}

// Idempotency guards selected unsafe endpoints against duplicate submissions.
func Idempotency(service IdempotencyService, maxUploadMB int) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, hasKeyHeader := c.Request.Header[http.CanonicalHeaderKey("Idempotency-Key")]
		key := c.GetHeader("Idempotency-Key")
		if !hasKeyHeader {
			c.Next()
			return
		}
		if err := idempotency.ValidateKey(key); err != nil {
			types.Fail(c, http.StatusBadRequest, types.ErrValidation("无效的幂等键"))
			c.Abort()
			return
		}

		body, err := readRequestBody(c.Request.Body, maxUploadMB, c.GetHeader("Content-Type"))
		if err != nil {
			if errors.Is(err, errBodyTooLarge) {
				types.Fail(c, http.StatusRequestEntityTooLarge, types.ErrValidation("请求体过大"))
				c.Abort()
				return
			}
			types.Fail(c, http.StatusBadRequest, types.ErrValidation("请求体读取失败"))
			c.Abort()
			return
		}
		c.Request.Body = io.NopCloser(bytes.NewReader(body))

		scope := c.Request.Method + " " + c.FullPath()
		userID := types.GetUserID(c)
		hashBody := bodyForRequestHash(c.GetHeader("Content-Type"), body)
		hash := requestHash(
			c.Request.Method,
			c.FullPath(),
			c.Request.URL.EscapedPath(),
			c.Request.URL.RawQuery,
			userID,
			types.GetWarehouseID(c),
			hashBody,
		)

		decision, err := service.Begin(c.Request.Context(), userID, scope, key, hash)
		if err != nil {
			if errors.Is(err, idempotency.ErrKeyReusedWithDifferentRequest) {
				types.Fail(c, http.StatusBadRequest, types.ErrValidation("幂等键已用于不同请求"))
			} else {
				types.FailFromError(c, err)
			}
			c.Abort()
			return
		}

		switch decision.Type {
		case idempotency.DecisionReplay:
			c.Data(decision.StatusCode, "application/json; charset=utf-8", decision.ResponseBody)
			c.Abort()
			return
		case idempotency.DecisionProcessing:
			types.Fail(c, http.StatusConflict, types.ErrInvalidState("请求正在处理中，请稍后重试"))
			c.Abort()
			return
		case idempotency.DecisionProceed:
			capture := &responseCaptureWriter{ResponseWriter: c.Writer}
			c.Writer = capture
			c.Next()

			status := capture.Status()
			if status >= http.StatusOK && status < http.StatusMultipleChoices {
				if err := service.Complete(c.Request.Context(), userID, scope, key, status, capture.body.Bytes()); err != nil {
					_ = c.Error(err)
				}
			} else {
				if err := service.Release(c.Request.Context(), userID, scope, key); err != nil {
					_ = c.Error(err)
				}
			}
			return
		default:
			types.FailFromError(c, fmt.Errorf("unknown idempotency decision %d", decision.Type))
			c.Abort()
			return
		}
	}
}

var errBodyTooLarge = errors.New("idempotency request body too large")

const multipartEnvelopeOverheadBytes int64 = 1024 * 1024

func readRequestBody(body io.Reader, maxUploadMB int, contentType string) ([]byte, error) {
	if maxUploadMB <= 0 {
		return io.ReadAll(body)
	}

	limit := int64(maxUploadMB) * 1024 * 1024
	if isMultipartFormData(contentType) {
		limit += multipartEnvelopeOverheadBytes
	}
	limited, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(limited)) > limit {
		return nil, errBodyTooLarge
	}
	return limited, nil
}

func bodyForRequestHash(contentType string, body []byte) []byte {
	if !isMultipartFormData(contentType) {
		return body
	}
	_, params, _ := mime.ParseMediaType(contentType)
	boundary := params["boundary"]
	if boundary == "" {
		return body
	}
	canonical, err := canonicalMultipartBody(body, boundary)
	if err != nil {
		return body
	}
	return canonical
}

func isMultipartFormData(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	return err == nil && strings.EqualFold(mediaType, "multipart/form-data")
}

type multipartHashPart struct {
	fieldName   string
	fileName    string
	contentType string
	body        []byte
}

func canonicalMultipartBody(body []byte, boundary string) ([]byte, error) {
	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	parts := make([]multipartHashPart, 0)
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		partBody, err := io.ReadAll(part)
		if err != nil {
			_ = part.Close()
			return nil, err
		}
		_ = part.Close()
		parts = append(parts, multipartHashPart{
			fieldName:   part.FormName(),
			fileName:    part.FileName(),
			contentType: part.Header.Get("Content-Type"),
			body:        partBody,
		})
	}

	var canonical bytes.Buffer
	canonical.WriteString("multipart/form-data")
	canonical.WriteByte(0)
	for _, part := range parts {
		writeCanonicalString(&canonical, part.fieldName)
		writeCanonicalString(&canonical, part.fileName)
		writeCanonicalString(&canonical, part.contentType)
		writeCanonicalBytes(&canonical, part.body)
	}
	return canonical.Bytes(), nil
}

func writeCanonicalString(dst *bytes.Buffer, value string) {
	writeCanonicalBytes(dst, []byte(value))
}

func writeCanonicalBytes(dst *bytes.Buffer, value []byte) {
	dst.WriteString(strconv.Itoa(len(value)))
	dst.WriteByte(0)
	dst.Write(value)
	dst.WriteByte(0)
}

func requestHash(method, fullPath, concretePath, rawQuery string, userID, warehouseID uint, body []byte) string {
	h := sha256.New()
	h.Write([]byte(method))
	h.Write([]byte{0})
	h.Write([]byte(fullPath))
	h.Write([]byte{0})
	h.Write([]byte(concretePath))
	h.Write([]byte{0})
	h.Write([]byte(rawQuery))
	h.Write([]byte{0})
	h.Write([]byte(strconv.FormatUint(uint64(userID), 10)))
	h.Write([]byte{0})
	h.Write([]byte(strconv.FormatUint(uint64(warehouseID), 10)))
	h.Write([]byte{0})
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}

type responseCaptureWriter struct {
	gin.ResponseWriter
	body bytes.Buffer
}

func (w *responseCaptureWriter) Write(data []byte) (int, error) {
	w.body.Write(data)
	return w.ResponseWriter.Write(data)
}

func (w *responseCaptureWriter) WriteString(s string) (int, error) {
	w.body.WriteString(s)
	return w.ResponseWriter.WriteString(s)
}
