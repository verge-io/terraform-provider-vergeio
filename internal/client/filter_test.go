// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package vergeio

import "testing"

func TestEscapeFilterValue(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "apostrophe", in: "O'Brien", want: `O\'Brien`},
		{name: "braces", in: "Core{x}", want: `Core\{x}`},
		{name: "backslash", in: `Core\x`, want: `Core\\x`},
		// Brace is escaped after backslash, so the new slash is not doubled.
		{name: "backslash then brace", in: `\{`, want: `\\\{`},
		{name: "all three", in: `O'Brien{x}\`, want: `O\'Brien\{x}\\`},
		{name: "plain", in: "Core", want: "Core"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := EscapeFilterValue(tt.in); got != tt.want {
				t.Fatalf("EscapeFilterValue(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestKeepExact(t *testing.T) {
	rows := []string{"Core", "O'Brien", "Core{x}", `Core\x`}
	for _, want := range []string{"O'Brien", "Core{x}", `Core\x`} {
		t.Run(want, func(t *testing.T) {
			got := KeepExact(rows, want, func(s string) string { return s })
			if len(got) != 1 || got[0] != want {
				t.Fatalf("KeepExact(%q) = %#v, want [%q]", want, got, want)
			}
		})
	}

	t.Run("brace stripped name is dropped", func(t *testing.T) {
		got := KeepExact([]string{"Core"}, "Core{x}", func(s string) string { return s })
		if len(got) != 0 {
			t.Fatalf("KeepExact returned %#v, want none", got)
		}
	})

	t.Run("empty filter keeps every row", func(t *testing.T) {
		got := KeepExact(rows, "", func(s string) string { return s })
		if len(got) != len(rows) {
			t.Fatalf("KeepExact empty = %#v", got)
		}
	})
}
