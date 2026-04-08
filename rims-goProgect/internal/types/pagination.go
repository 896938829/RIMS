// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package types

// PageRequest holds pagination and search parameters from query string.
type PageRequest struct {
	Page     int    `form:"page" binding:"omitempty,min=1"`
	PageSize int    `form:"pageSize" binding:"omitempty,min=1,max=100"`
	Sort     string `form:"sort"`
	Keyword  string `form:"keyword"`
}

// Defaults fills zero values with sensible defaults.
func (p *PageRequest) Defaults() {
	if p.Page <= 0 {
		p.Page = 1
	}
	if p.PageSize <= 0 {
		p.PageSize = 20
	}
}

// Offset returns the SQL offset based on page and page size.
func (p *PageRequest) Offset() int {
	p.Defaults()
	return (p.Page - 1) * p.PageSize
}

// PageResult is the paginated response payload.
type PageResult struct {
	List     interface{} `json:"list"`
	Total    int64       `json:"total"`
	Page     int         `json:"page"`
	PageSize int         `json:"pageSize"`
}

// NewPageResult creates a PageResult from the request and query results.
func NewPageResult(req PageRequest, list interface{}, total int64) PageResult {
	req.Defaults()
	return PageResult{
		List:     list,
		Total:    total,
		Page:     req.Page,
		PageSize: req.PageSize,
	}
}
