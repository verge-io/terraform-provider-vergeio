// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package shared

import "github.com/hashicorp/terraform-plugin-framework/types"

// ApplyWriteOnlyString copies a write-only config value onto dest when the
// provider should send it. Terraform stores the write-only value as null, so
// a change to that value alone does not call Update. The paired version
// attribute is what changes. Create (hasState false) always copies a known
// value. Update copies it only when the version differs from state.
func ApplyWriteOnlyString(dest *types.String, planVersion, stateVersion types.Int64, config types.String, hasState bool) {
	if dest == nil || config.IsNull() || config.IsUnknown() {
		return
	}
	if hasState && planVersion.Equal(stateVersion) {
		return
	}
	*dest = types.StringValue(config.ValueString())
}

// WriteOnlyVersionSet reports whether a rotation version is present.
// A set version means the paired write-only secret was used, so the stored
// secret attribute must stay null.
func WriteOnlyVersionSet(v types.Int64) bool {
	return !v.IsNull() && !v.IsUnknown()
}
