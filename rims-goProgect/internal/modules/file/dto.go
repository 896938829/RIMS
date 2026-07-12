// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package file

import "time"

// ListFilesRequest holds filter and pagination params for listing files.
type ListFilesRequest struct {
	BusinessType string `form:"businessType" binding:"omitempty,max=32"`
	BusinessID   uint   `form:"businessId" binding:"omitempty,min=1"`
	Page         int    `form:"page" binding:"omitempty,min=1"`
	PageSize     int    `form:"pageSize" binding:"omitempty,min=1,max=100"`
}

type ReorderRequest struct {
	BusinessType string `json:"businessType" binding:"required,max=32"`
	BusinessID   uint   `json:"businessId" binding:"required,min=1"`
	FileIDs      []uint `json:"fileIds" binding:"required,min=1,dive,min=1"`
}

// FileResponse is the file attachment representation returned to clients.
type FileResponse struct {
	ID           uint      `json:"id"`
	BusinessType string    `json:"businessType"`
	BusinessID   *uint     `json:"businessId,omitempty"`
	FileURL      string    `json:"fileUrl"`
	OriginalName string    `json:"originalName"`
	FileSize     int64     `json:"fileSize"`
	MimeType     string    `json:"mimeType"`
	FileHash     string    `json:"fileHash"`
	IsPublic     bool      `json:"isPublic"`
	ObjectKey    string    `json:"objectKey,omitempty"` // admin-only
	CreatedBy    uint      `json:"createdBy"`
	CreatedAt    time.Time `json:"uploadedAt"`
	Position     int       `json:"position"`
}

// ToFileResponse converts a FileAttachment model to a FileResponse.
// objectKey is only populated for admin viewers.
func ToFileResponse(f *FileAttachment, includeObjectKey bool) FileResponse {
	resp := FileResponse{
		ID:           f.ID,
		BusinessType: f.BusinessType,
		BusinessID:   f.BusinessID,
		FileURL:      f.FileURL,
		OriginalName: f.OriginalName,
		FileSize:     f.FileSize,
		MimeType:     f.MimeType,
		FileHash:     f.FileHash,
		IsPublic:     f.IsPublic,
		CreatedBy:    f.CreatedBy,
		CreatedAt:    f.CreatedAt,
		Position:     f.Position,
	}
	if includeObjectKey {
		resp.ObjectKey = f.ObjectKey
	}
	return resp
}
