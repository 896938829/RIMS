// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package file

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"gorm.io/gorm"

	"rims-go/internal/types"
)

// UploadRequest is the service-level input for uploading a file.
type UploadRequest struct {
	BusinessType string
	BusinessID   *uint
	OriginalName string
	Reader       io.Reader
	DeclaredSize int64 // size reported by the multipart form (may be 0 if unknown)
}

// FileService orchestrates file upload, retrieval and deletion logic.
type FileService struct {
	repo              FileRepository
	storage           Storage
	maxSize           int64
	allowedExt        map[string]bool
	downloadURLFormat string // e.g. "/api/v1/files/%d/download"
}

// NewFileService constructs a FileService.
//
//   - maxUploadMB: from cfg.MaxUploadMB (<=0 means no limit).
//   - allowedExts: comma-separated extension list from cfg.AllowedExts. Entries
//     are case-insensitive and may include or omit the leading dot.
//   - downloadURLFormat: template like "/api/v1/files/%d/download" used for
//     building URLs of private (non-public) files once the record ID is known.
func NewFileService(repo FileRepository, storage Storage, maxUploadMB int, allowedExts string, downloadURLFormat string) *FileService {
	var maxSize int64
	if maxUploadMB > 0 {
		maxSize = int64(maxUploadMB) * 1024 * 1024
	}
	return &FileService{
		repo:              repo,
		storage:           storage,
		maxSize:           maxSize,
		allowedExt:        parseAllowedExts(allowedExts),
		downloadURLFormat: downloadURLFormat,
	}
}

func parseAllowedExts(raw string) map[string]bool {
	out := make(map[string]bool)
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(strings.ToLower(part))
		if part == "" {
			continue
		}
		if !strings.HasPrefix(part, ".") {
			part = "." + part
		}
		out[part] = true
	}
	return out
}

// Upload validates, stores, and records metadata for a new file. The returned
// FileAttachment has its FileURL populated.
func (s *FileService) Upload(ctx context.Context, uploaderID uint, req UploadRequest) (*FileAttachment, error) {
	businessType := strings.TrimSpace(req.BusinessType)
	if businessType == "" {
		businessType = BusinessTypeOther
	}
	if !IsValidBusinessType(businessType) {
		return nil, types.ErrValidation("不支持的业务类型")
	}

	originalName := strings.TrimSpace(req.OriginalName)
	if originalName == "" {
		return nil, types.ErrValidation("文件名不能为空")
	}
	ext := strings.ToLower(path.Ext(originalName))
	if ext == "" {
		return nil, types.ErrValidation("文件必须包含扩展名")
	}
	if len(s.allowedExt) > 0 && !s.allowedExt[ext] {
		return nil, types.ErrValidation(fmt.Sprintf("不允许的文件类型：%s", ext))
	}

	if req.Reader == nil {
		return nil, types.ErrValidation("文件内容为空")
	}

	// Enforce size limit at read time so we never buffer more than allowed,
	// even if the multipart header lied about the size.
	var reader io.Reader = req.Reader
	if s.maxSize > 0 {
		// +1 so we can detect overflow vs. an exactly-max-size file.
		reader = io.LimitReader(req.Reader, s.maxSize+1)
	}

	// Buffer the whole file once to compute hash + detect MIME + let the
	// storage backend consume it. 10MB default keeps memory bounded.
	hasher := sha256.New()
	var buf bytes.Buffer
	tee := io.TeeReader(reader, hasher)
	written, err := io.Copy(&buf, tee)
	if err != nil {
		return nil, types.ErrSystem(fmt.Errorf("read upload: %w", err))
	}
	if s.maxSize > 0 && written > s.maxSize {
		return nil, types.ErrValidation(fmt.Sprintf("文件超过大小限制 (%d MB)", s.maxSize/(1024*1024)))
	}
	if written == 0 {
		return nil, types.ErrValidation("文件内容为空")
	}

	hash := hex.EncodeToString(hasher.Sum(nil))

	// MIME sniff from first 512 bytes; falls back to application/octet-stream.
	sniffN := 512
	if buf.Len() < sniffN {
		sniffN = buf.Len()
	}
	mimeType := http.DetectContentType(buf.Bytes()[:sniffN])

	// Generate object key: <yyyy>/<mm>/<random>.<ext>
	now := time.Now().UTC()
	randHex, err := randomHex(16)
	if err != nil {
		return nil, types.ErrSystem(err)
	}
	objectKey := fmt.Sprintf("%04d/%02d/%s%s", now.Year(), int(now.Month()), randHex, ext)

	if err := s.storage.Save(ctx, objectKey, bytes.NewReader(buf.Bytes())); err != nil {
		return nil, types.ErrSystem(err)
	}

	isPublic := IsPublicBusinessType(businessType)

	record := &FileAttachment{
		BusinessType: businessType,
		BusinessID:   req.BusinessID,
		ObjectKey:    objectKey,
		OriginalName: originalName,
		FileSize:     written,
		FileHash:     hash,
		MimeType:     mimeType,
		IsPublic:     isPublic,
	}
	record.CreatedBy = uploaderID
	record.UpdatedBy = uploaderID

	if isPublic {
		record.FileURL = s.storage.PublicURL(objectKey)
	} else {
		// Placeholder: real URL needs the ID, filled in after Create.
		record.FileURL = ""
	}

	if err := s.repo.Create(ctx, record); err != nil {
		// Best-effort rollback of the stored object so we don't orphan it.
		_ = s.storage.Delete(ctx, objectKey)
		return nil, types.ErrSystem(err)
	}

	if !isPublic {
		record.FileURL = fmt.Sprintf(s.downloadURLFormat, record.ID)
		if err := s.repo.Update(ctx, record); err != nil {
			return nil, types.ErrSystem(err)
		}
	}

	return record, nil
}

// Get retrieves a file attachment record by ID.
func (s *FileService) Get(ctx context.Context, id uint) (*FileAttachment, error) {
	f, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, types.ErrNotFound("文件")
		}
		return nil, types.ErrSystem(err)
	}
	return f, nil
}

// List returns a paginated list of file attachments matching the filter.
func (s *FileService) List(ctx context.Context, filter ListFilter, page types.PageRequest, includeObjectKey bool) (types.PageResult, error) {
	list, total, err := s.repo.List(ctx, filter, page)
	if err != nil {
		return types.PageResult{}, types.ErrSystem(err)
	}
	items := make([]FileResponse, len(list))
	for i := range list {
		items[i] = ToFileResponse(&list[i], includeObjectKey)
	}
	return types.NewPageResult(page, items, total), nil
}

// Delete soft-deletes a file record. The underlying object is retained for
// later cleanup. Permission: uploader or admin.
func (s *FileService) Delete(ctx context.Context, id, operatorID uint, isAdmin bool) error {
	f, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return types.ErrNotFound("文件")
		}
		return types.ErrSystem(err)
	}
	if !isAdmin && f.CreatedBy != operatorID {
		return types.ErrForbidden()
	}
	if err := s.repo.SoftDelete(ctx, id); err != nil {
		return types.ErrSystem(err)
	}
	return nil
}

// OpenForDownload returns a reader for a file's content alongside its metadata.
// Callers must close the returned ReadCloser.
func (s *FileService) OpenForDownload(ctx context.Context, id uint) (io.ReadCloser, *FileAttachment, error) {
	f, err := s.Get(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	rc, err := s.storage.Open(ctx, f.ObjectKey)
	if err != nil {
		if errors.Is(err, ErrObjectNotFound) {
			return nil, nil, types.ErrNotFound("文件")
		}
		return nil, nil, types.ErrSystem(err)
	}
	return rc, f, nil
}

// randomHex returns a cryptographically random hex string of the given byte length.
func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("random: %w", err)
	}
	return hex.EncodeToString(b), nil
}
