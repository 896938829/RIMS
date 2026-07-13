// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package idempotency

import "errors"

const MaxKeyLength = 255

var ErrInvalidKey = errors.New("invalid idempotency key")

// ValidateKey enforces the shared header and status-path key contract.
func ValidateKey(key string) error {
	if len(key) == 0 || len(key) > MaxKeyLength {
		return ErrInvalidKey
	}
	for i := 0; i < len(key); i++ {
		c := key[i]
		if (c >= 'a' && c <= 'z') ||
			(c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') ||
			c == '.' || c == '_' || c == '~' || c == '-' {
			continue
		}
		return ErrInvalidKey
	}
	return nil
}
