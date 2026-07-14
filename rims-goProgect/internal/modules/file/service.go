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

// FileAction describes an operation that needs file-level authorization.
type FileAction string

const (
	FileActionCreate FileAction = "create"
	FileActionRead   FileAction = "read"
	FileActionDelete FileAction = "delete"
)

// FileActor is the caller identity used for file authorization.
type FileActor struct {
	UserID  uint
	IsAdmin bool
}

// BusinessAccessChecker authorizes access based on a file's linked business object.
type BusinessAccessChecker interface {
	CanAccessFile(ctx context.Context, actor FileActor, f *FileAttachment, action FileAction) (bool, error)
}

type ownerAdminAccessChecker struct{}

func (ownerAdminAccessChecker) CanAccessFile(_ context.Context, actor FileActor, f *FileAttachment, _ FileAction) (bool, error) {
	return actor.IsAdmin || f.CreatedBy == actor.UserID, nil
}

// FileService orchestrates file upload, retrieval and deletion logic.
type FileService struct {
	repo                    FileRepository
	storage                 Storage
	maxSize                 int64
	allowedExt              map[string]bool
	downloadURLFormat       string // e.g. "/api/v1/files/%d/download"
	accessChecker           BusinessAccessChecker
	maxAttachmentsPerObject int
}

type bindingAttachmentCounter interface {
	CountByBinding(ctx context.Context, businessType string, businessID uint) (int64, error)
}

type fixtureAttachmentBindingClassifier interface {
	IsFixtureAttachmentBinding(ctx context.Context, businessType string, businessID uint) (bool, error)
}

type attachmentMutationRepository interface {
	ListAllByBinding(ctx context.Context, businessType string, businessID uint) ([]FileAttachment, error)
	MaxPositionByBinding(ctx context.Context, businessType string, businessID uint) (int, error)
	UpdatePositions(ctx context.Context, businessType string, businessID uint, fileIDs []uint) error
}

type preparedFileObject struct {
	objectKey    string
	originalName string
	content      []byte
	size         int64
	hash         string
	mimeType     string
}

// NewFileService constructs a FileService.
//
//   - maxUploadMB: from cfg.MaxUploadMB (<=0 means no limit).
//   - allowedExts: comma-separated extension list from cfg.AllowedExts. Entries
//     are case-insensitive and may include or omit the leading dot.
//   - downloadURLFormat: template like "/api/v1/files/%d/download" used for
//     building URLs of private (non-public) files once the record ID is known.
func NewFileService(repo FileRepository, storage Storage, maxUploadMB int, allowedExts string, downloadURLFormat string, accessChecker BusinessAccessChecker) *FileService {
	return NewFileServiceWithLimits(repo, storage, maxUploadMB, 9, allowedExts, downloadURLFormat, accessChecker)
}

// NewFileServiceWithLimits constructs a service with explicit attachment bounds.
func NewFileServiceWithLimits(repo FileRepository, storage Storage, maxUploadMB, maxAttachmentsPerObject int, allowedExts string, downloadURLFormat string, accessChecker BusinessAccessChecker) *FileService {
	var maxSize int64
	if maxUploadMB > 0 {
		maxSize = int64(maxUploadMB) * 1024 * 1024
	}
	if accessChecker == nil {
		accessChecker = ownerAdminAccessChecker{}
	}
	return &FileService{
		repo:                    repo,
		storage:                 storage,
		maxSize:                 maxSize,
		allowedExt:              parseAllowedExts(allowedExts),
		downloadURLFormat:       downloadURLFormat,
		accessChecker:           accessChecker,
		maxAttachmentsPerObject: maxAttachmentsPerObject,
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
func (s *FileService) Upload(ctx context.Context, actor FileActor, req UploadRequest) (*FileAttachment, error) {
	businessType := strings.TrimSpace(req.BusinessType)
	if businessType == "" {
		businessType = BusinessTypeOther
	}
	if !IsValidBusinessType(businessType) {
		return nil, types.ErrValidation("不支持的业务类型")
	}
	if businessType == BusinessTypeProductImage && (req.BusinessID == nil || *req.BusinessID == 0) {
		return nil, types.ErrValidation("product_image必须关联业务对象")
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

	isPublic := IsPublicBusinessType(businessType)
	position := 0
	fixtureBinding := false
	if req.BusinessID != nil {
		if err := s.authorizeBusinessBinding(ctx, actor, &FileAttachment{
			BusinessType: businessType,
			BusinessID:   req.BusinessID,
			IsPublic:     isPublic,
		}); err != nil {
			return nil, err
		}
		if err := s.enforceAttachmentLimit(ctx, businessType, *req.BusinessID); err != nil {
			return nil, err
		}
		if classifier, ok := s.repo.(fixtureAttachmentBindingClassifier); ok {
			isFixture, err := classifier.IsFixtureAttachmentBinding(ctx, businessType, *req.BusinessID)
			if err != nil {
				return nil, types.ErrSystem(err)
			}
			fixtureBinding = isFixture
		}
		if mutationRepo, ok := s.repo.(attachmentMutationRepository); ok {
			maxPosition, err := mutationRepo.MaxPositionByBinding(ctx, businessType, *req.BusinessID)
			if err != nil {
				return nil, types.ErrSystem(err)
			}
			position = maxPosition + 1
		}
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
	if !mimeMatchesExtension(ext, mimeType) {
		return nil, types.ErrValidation(fmt.Sprintf("文件内容与扩展名不匹配：%s", ext))
	}

	// Generate object key: <yyyy>/<mm>/<random>.<ext>
	now := time.Now().UTC()
	randHex, err := randomHex(16)
	if err != nil {
		return nil, types.ErrSystem(err)
	}
	objectKey := buildObjectKey(now, randHex, ext, fixtureBinding)

	if err := s.storage.Save(ctx, objectKey, bytes.NewReader(buf.Bytes())); err != nil {
		return nil, types.ErrSystem(err)
	}

	record := &FileAttachment{
		BusinessType: businessType,
		BusinessID:   req.BusinessID,
		ObjectKey:    objectKey,
		OriginalName: originalName,
		FileSize:     written,
		FileHash:     hash,
		MimeType:     mimeType,
		IsPublic:     isPublic,
		Position:     position,
	}
	record.CreatedBy = actor.UserID
	record.UpdatedBy = actor.UserID

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

// Reorder applies one exact order to all attachments in a business binding.
func (s *FileService) Reorder(ctx context.Context, actor FileActor, req ReorderRequest) ([]FileAttachment, error) {
	businessType := strings.TrimSpace(req.BusinessType)
	if !IsValidBusinessType(businessType) || req.BusinessID == 0 || len(req.FileIDs) == 0 {
		return nil, types.ErrValidation("无效的附件排序请求")
	}
	seen := make(map[uint]struct{}, len(req.FileIDs))
	for _, id := range req.FileIDs {
		if id == 0 {
			return nil, types.ErrValidation("文件ID无效")
		}
		if _, exists := seen[id]; exists {
			return nil, types.ErrValidation("文件ID不能重复")
		}
		seen[id] = struct{}{}
	}
	repo, ok := s.repo.(attachmentMutationRepository)
	if !ok {
		return nil, types.ErrSystem(errors.New("file repository does not support reorder"))
	}
	files, err := repo.ListAllByBinding(ctx, businessType, req.BusinessID)
	if err != nil {
		return nil, types.ErrSystem(err)
	}
	if len(files) != len(req.FileIDs) {
		return nil, types.ErrValidation("排序必须包含当前对象的全部附件")
	}
	byID := make(map[uint]FileAttachment, len(files))
	for _, file := range files {
		if !actor.IsAdmin && file.CreatedBy != actor.UserID {
			return nil, types.ErrForbidden()
		}
		byID[file.ID] = file
	}
	ordered := make([]FileAttachment, len(req.FileIDs))
	for index, id := range req.FileIDs {
		file, exists := byID[id]
		if !exists {
			return nil, types.ErrValidation("排序包含不属于当前对象的附件")
		}
		file.Position = index
		ordered[index] = file
	}
	if err := repo.UpdatePositions(ctx, businessType, req.BusinessID, req.FileIDs); err != nil {
		return nil, types.ErrSystem(err)
	}
	return ordered, nil
}

// Replace swaps file bytes and mutable metadata while preserving attachment identity.
func (s *FileService) Replace(ctx context.Context, id uint, actor FileActor, req UploadRequest) (*FileAttachment, error) {
	original, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if !actor.IsAdmin && original.CreatedBy != actor.UserID {
		return nil, types.ErrForbidden()
	}
	fixtureBinding := false
	if original.BusinessID != nil {
		if classifier, ok := s.repo.(fixtureAttachmentBindingClassifier); ok {
			fixtureBinding, err = classifier.IsFixtureAttachmentBinding(ctx, original.BusinessType, *original.BusinessID)
			if err != nil {
				return nil, types.ErrSystem(err)
			}
		}
	}
	prepared, err := s.prepareFileObject(ctx, req, fixtureBinding)
	if err != nil {
		return nil, err
	}
	if err := s.storage.Save(ctx, prepared.objectKey, bytes.NewReader(prepared.content)); err != nil {
		return nil, types.ErrSystem(err)
	}
	replacement := *original
	replacement.ObjectKey = prepared.objectKey
	replacement.OriginalName = prepared.originalName
	replacement.FileSize = prepared.size
	replacement.FileHash = prepared.hash
	replacement.MimeType = prepared.mimeType
	replacement.UpdatedBy = actor.UserID
	if replacement.IsPublic {
		replacement.FileURL = s.storage.PublicURL(prepared.objectKey)
	} else {
		replacement.FileURL = fmt.Sprintf(s.downloadURLFormat, replacement.ID)
	}
	if err := s.repo.Update(ctx, &replacement); err != nil {
		_ = s.storage.Delete(ctx, prepared.objectKey)
		return nil, types.ErrSystem(err)
	}
	_ = s.storage.Delete(ctx, original.ObjectKey)
	return &replacement, nil
}

func (s *FileService) prepareFileObject(ctx context.Context, req UploadRequest, fixtureBinding bool) (*preparedFileObject, error) {
	originalName := strings.TrimSpace(req.OriginalName)
	if originalName == "" {
		return nil, types.ErrValidation("文件名不能为空")
	}
	ext := strings.ToLower(path.Ext(originalName))
	if ext == "" || (len(s.allowedExt) > 0 && !s.allowedExt[ext]) {
		return nil, types.ErrValidation(fmt.Sprintf("不允许的文件类型：%s", ext))
	}
	if req.Reader == nil {
		return nil, types.ErrValidation("文件内容为空")
	}
	if s.maxSize > 0 && req.DeclaredSize > s.maxSize {
		return nil, types.ErrValidation(fmt.Sprintf("文件超过大小限制 (%d MB)", s.maxSize/(1024*1024)))
	}
	var reader io.Reader = &contextReader{ctx: ctx, reader: req.Reader}
	if s.maxSize > 0 {
		reader = io.LimitReader(reader, s.maxSize+1)
	}
	hasher := sha256.New()
	var buf bytes.Buffer
	written, err := io.Copy(&buf, io.TeeReader(reader, hasher))
	if err != nil {
		if ctx.Err() != nil {
			return nil, types.ErrSystem(ctx.Err())
		}
		return nil, types.ErrSystem(fmt.Errorf("read upload: %w", err))
	}
	if written == 0 {
		return nil, types.ErrValidation("文件内容为空")
	}
	if s.maxSize > 0 && written > s.maxSize {
		return nil, types.ErrValidation(fmt.Sprintf("文件超过大小限制 (%d MB)", s.maxSize/(1024*1024)))
	}
	sniffN := 512
	if buf.Len() < sniffN {
		sniffN = buf.Len()
	}
	mimeType := http.DetectContentType(buf.Bytes()[:sniffN])
	if !mimeMatchesExtension(ext, mimeType) {
		return nil, types.ErrValidation(fmt.Sprintf("文件内容与扩展名不匹配：%s", ext))
	}
	now := time.Now().UTC()
	randHex, err := randomHex(16)
	if err != nil {
		return nil, types.ErrSystem(err)
	}
	return &preparedFileObject{
		objectKey:    buildObjectKey(now, randHex, ext, fixtureBinding),
		originalName: originalName,
		content:      append([]byte(nil), buf.Bytes()...),
		size:         written,
		hash:         hex.EncodeToString(hasher.Sum(nil)),
		mimeType:     mimeType,
	}, nil
}

func buildObjectKey(now time.Time, randomSuffix, ext string, fixtureBinding bool) string {
	prefix := ""
	if fixtureBinding {
		prefix = "m9-e2e/"
	}
	return fmt.Sprintf("%s%04d/%02d/%s%s", prefix, now.Year(), int(now.Month()), randomSuffix, ext)
}

func (s *FileService) enforceAttachmentLimit(ctx context.Context, businessType string, businessID uint) error {
	if s.maxAttachmentsPerObject <= 0 {
		return nil
	}
	counter, ok := s.repo.(bindingAttachmentCounter)
	if !ok {
		return nil
	}
	count, err := counter.CountByBinding(ctx, businessType, businessID)
	if err != nil {
		return types.ErrSystem(err)
	}
	if count >= int64(s.maxAttachmentsPerObject) {
		return types.ErrValidation(fmt.Sprintf("附件数量不能超过%d个", s.maxAttachmentsPerObject))
	}
	return nil
}

func mimeMatchesExtension(ext, mimeType string) bool {
	mimeType = strings.ToLower(strings.TrimSpace(strings.Split(mimeType, ";")[0]))
	switch ext {
	case ".jpg", ".jpeg":
		return mimeType == "image/jpeg"
	case ".png":
		return mimeType == "image/png"
	case ".gif":
		return mimeType == "image/gif"
	case ".pdf":
		return mimeType == "application/pdf"
	case ".csv":
		return mimeType == "text/plain" || mimeType == "text/csv" || mimeType == "application/csv"
	case ".xlsx":
		return mimeType == "application/zip" ||
			mimeType == "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	default:
		return true
	}
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

// GetForRead retrieves a file attachment record and applies read authorization.
func (s *FileService) GetForRead(ctx context.Context, id uint, actor FileActor) (*FileAttachment, error) {
	f, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.authorize(ctx, actor, f, FileActionRead); err != nil {
		return nil, err
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

// ListForRead paginates after authorization so totals never describe hidden rows.
func (s *FileService) ListForRead(ctx context.Context, filter ListFilter, page types.PageRequest, includeObjectKey bool, actor FileActor) (types.PageResult, error) {
	page.Defaults()
	authorized := make([]FileResponse, 0)
	const candidatePageSize = 100
	for candidatePage := 1; ; candidatePage++ {
		list, total, err := s.repo.List(ctx, filter, types.PageRequest{Page: candidatePage, PageSize: candidatePageSize})
		if err != nil {
			return types.PageResult{}, types.ErrSystem(err)
		}
		for i := range list {
			if err := s.authorize(ctx, actor, &list[i], FileActionRead); err != nil {
				var appErr *types.AppError
				if errors.As(err, &appErr) && appErr.Code == types.ErrCodePermissionDenied {
					continue
				}
				return types.PageResult{}, err
			}
			authorized = append(authorized, ToFileResponse(&list[i], includeObjectKey))
		}
		if int64(candidatePage*candidatePageSize) >= total || len(list) == 0 {
			break
		}
	}
	start := page.Offset()
	if start > len(authorized) {
		start = len(authorized)
	}
	end := start + page.PageSize
	if end > len(authorized) {
		end = len(authorized)
	}
	return types.NewPageResult(page, authorized[start:end], int64(len(authorized))), nil
}

// Delete soft-deletes a file record. The underlying object is retained for
// later cleanup.
func (s *FileService) Delete(ctx context.Context, id uint, actor FileActor) error {
	f, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return types.ErrNotFound("文件")
		}
		return types.ErrSystem(err)
	}
	if err := s.authorize(ctx, actor, f, FileActionDelete); err != nil {
		return err
	}
	if err := s.repo.SoftDelete(ctx, id); err != nil {
		return types.ErrSystem(err)
	}
	return nil
}

// OpenForDownload returns a reader for a file's content alongside its metadata.
// Callers must close the returned ReadCloser.
func (s *FileService) OpenForDownload(ctx context.Context, id uint, actor FileActor) (io.ReadCloser, *FileAttachment, error) {
	f, err := s.GetForRead(ctx, id, actor)
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

func (s *FileService) authorize(ctx context.Context, actor FileActor, f *FileAttachment, action FileAction) error {
	if actor.IsAdmin || f.CreatedBy == actor.UserID {
		return nil
	}
	if action == FileActionDelete {
		return types.ErrForbidden()
	}
	if action == FileActionRead && f.IsPublic {
		return nil
	}
	allowed, err := s.accessChecker.CanAccessFile(ctx, actor, f, action)
	if err != nil {
		return types.ErrSystem(err)
	}
	if !allowed {
		return types.ErrForbidden()
	}
	return nil
}

func (s *FileService) authorizeBusinessBinding(ctx context.Context, actor FileActor, f *FileAttachment) error {
	allowed, err := s.accessChecker.CanAccessFile(ctx, actor, f, FileActionCreate)
	if err != nil {
		return types.ErrSystem(err)
	}
	if !allowed {
		return types.ErrForbidden()
	}
	return nil
}

// randomHex returns a cryptographically random hex string of the given byte length.
func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("random: %w", err)
	}
	return hex.EncodeToString(b), nil
}
