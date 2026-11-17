// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tags

import "github.com/hashicorp/terraform-plugin-framework/types"

// knownString drops unknown values. State cannot store them. A null value
// stays null so an omitted argument is not written as an empty string.
func knownString(v types.String) types.String {
	if v.IsNull() || v.IsUnknown() {
		return types.StringNull()
	}
	return v
}

func knownBool(v types.Bool) types.Bool {
	if v.IsNull() || v.IsUnknown() {
		return types.BoolNull()
	}
	return v
}

func knownInt32(v types.Int32) types.Int32 {
	if v.IsNull() || v.IsUnknown() {
		return types.Int32Null()
	}
	return v
}
