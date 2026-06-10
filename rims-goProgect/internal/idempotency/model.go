// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package idempotency

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"rims-go/internal/types"
)

const (
	StateProcessing = "processing"
	StateCompleted  = "completed"
)

// JSONB stores raw JSON response bodies in PostgreSQL jsonb columns.
type JSONB []byte

// Value implements driver.Valuer.
func (j JSONB) Value() (driver.Value, error) {
	if len(j) == 0 {
		return nil, nil
	}
	if !json.Valid(j) {
		return nil, fmt.Errorf("invalid jsonb value")
	}
	return string(j), nil
}

// Scan implements sql.Scanner.
func (j *JSONB) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}
	switch v := value.(type) {
	case []byte:
		*j = append((*j)[0:0], v...)
	case string:
		*j = append((*j)[0:0], v...)
	default:
		return fmt.Errorf("unsupported jsonb scan type %T", value)
	}
	return nil
}

// Record stores the processing or completed response for an idempotent request.
type Record struct {
	types.BaseModel
	UserID         uint      `gorm:"not null"`
	Scope          string    `gorm:"size:255;not null"`
	IdempotencyKey string    `gorm:"size:255;not null"`
	RequestHash    string    `gorm:"size:64;not null"`
	State          string    `gorm:"size:32;not null"`
	StatusCode     int       `gorm:"column:status_code"`
	ResponseBody   JSONB     `gorm:"type:jsonb"`
	ExpiresAt      time.Time `gorm:"not null"`
}

// TableName overrides GORM's default table name.
func (Record) TableName() string { return "idempotency_keys" }
