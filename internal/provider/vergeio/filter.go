// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package vergeio

import "strings"

// EscapeFilterValue prepares a value for a single-quoted VergeOS filter
// literal. Backslash, apostrophe, and an opening brace are reserved inside
// that literal. A balanced {...} that is not escaped is dropped, so the
// filter matches a different object. Backslash is escaped first so the
// escapes added for apostrophe and brace stay intact.
func EscapeFilterValue(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `'`, `\'`)
	value = strings.ReplaceAll(value, `{`, `\{`)
	return value
}

// KeepExact returns rows whose field equals want. An empty want keeps every
// row. Rows that do not match are discarded so a filter the platform
// rewrites returns nothing instead of a different object.
func KeepExact[T any](rows []T, want string, field func(T) string) []T {
	if want == "" {
		return rows
	}
	kept := make([]T, 0, len(rows))
	for _, row := range rows {
		if field(row) == want {
			kept = append(kept, row)
		}
	}
	return kept
}
