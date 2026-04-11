// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package file

import "rims-go/internal/types"

// Business type constants for file attachments.
const (
	BusinessTypeProductImage   = "product_image"
	BusinessTypeDocAttachment  = "doc_attachment"
	BusinessTypeImportTemplate = "import_template"
	BusinessTypeExportResult   = "export_result"
	BusinessTypeOther          = "other"
)

// FileAttachment stores metadata for uploaded files. The actual object lives
// in the configured storage backend (local disk in v1, object storage later).
type FileAttachment struct {
	types.AuditableModel
	BusinessType string `gorm:"size:32;not null;index:idx_file_business,priority:1"`
	BusinessID   *uint  `gorm:"index:idx_file_business,priority:2"`
	ObjectKey    string `gorm:"size:512;not null;uniqueIndex:idx_file_object_key"`
	FileURL      string `gorm:"size:1024;not null"`
	OriginalName string `gorm:"size:255;not null"`
	FileSize     int64  `gorm:"not null;default:0"`
	FileHash     string `gorm:"size:64;not null;default:'';index"`
	MimeType     string `gorm:"size:128;not null;default:''"`
	IsPublic     bool   `gorm:"not null;default:false"`
}

// TableName overrides the default table name.
func (FileAttachment) TableName() string { return "file_attachments" }

// IsValidBusinessType returns true if the given business type is in the allowed set.
func IsValidBusinessType(t string) bool {
	switch t {
	case BusinessTypeProductImage, BusinessTypeDocAttachment,
		BusinessTypeImportTemplate, BusinessTypeExportResult, BusinessTypeOther:
		return true
	}
	return false
}

// IsPublicBusinessType returns whether files of the given business type are
// served via the static public URL (true) or via the controlled download
// endpoint (false).
func IsPublicBusinessType(t string) bool {
	return t == BusinessTypeProductImage
}
