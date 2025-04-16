// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package vergeio

import "github.com/hashicorp/terraform-plugin-framework/types"

func StringToNil(planValue types.String, stateValue types.String, defaultValue string) string {
	if planValue.IsUnknown() {
		if stateValue.IsUnknown() {
			return defaultValue
		}
		return stateValue.ValueString()
	}
	return planValue.ValueString()
}
func BoolToNil(planValue types.Bool, stateValue types.Bool, defaultValue bool) bool {
	if planValue.IsUnknown() {
		if stateValue.IsUnknown() {
			return defaultValue
		}
		return stateValue.ValueBool()
	}
	return planValue.ValueBool()
}

func Int32ToNil(planValue types.Int32, stateValue types.Int32, defaultValue int32) int32 {
	if planValue.IsUnknown() {
		if stateValue.IsUnknown() {
			return defaultValue
		}
		return stateValue.ValueInt32()
	}
	return planValue.ValueInt32()
}
func Int64ToNil(planValue types.Int64, stateValue types.Int64, defaultValue int64) int64 {
	if planValue.IsUnknown() {
		if stateValue.IsUnknown() {
			return defaultValue
		}
		return stateValue.ValueInt64()
	}
	return planValue.ValueInt64()
}
