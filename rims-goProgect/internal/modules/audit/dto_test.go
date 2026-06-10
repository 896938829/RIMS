// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package audit

import (
	"testing"
	"unicode/utf8"
)

func TestTruncatePreservesUTF8Boundaries(t *testing.T) {
	tests := []struct {
		name string
		s    string
		n    int
		want string
	}{
		{
			name: "chinese runes",
			s:    "你你你",
			n:    2,
			want: "你你",
		},
		{
			name: "mixed ascii and chinese runes",
			s:    "a你b",
			n:    2,
			want: "a你",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncate(tt.s, tt.n)
			if got != tt.want {
				t.Fatalf("truncate(%q, %d) = %q, want %q", tt.s, tt.n, got, tt.want)
			}
			if !utf8.ValidString(got) {
				t.Fatalf("truncate(%q, %d) returned invalid UTF-8: %q", tt.s, tt.n, got)
			}
		})
	}
}

func TestTruncateNonPositiveLimitKeepsCurrentSemantics(t *testing.T) {
	const input = "你abc"

	if got := truncate(input, 0); got != input {
		t.Fatalf("truncate(%q, 0) = %q, want input unchanged", input, got)
	}
	if got := truncate(input, -1); got != input {
		t.Fatalf("truncate(%q, -1) = %q, want input unchanged", input, got)
	}
}
