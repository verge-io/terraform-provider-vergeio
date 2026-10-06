// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package site

import (
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

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

func knownInt64(v types.Int64) types.Int64 {
	if v.IsNull() || v.IsUnknown() {
		return types.Int64Null()
	}
	return v
}

func knownFloat(v types.Float64) types.Float64 {
	if v.IsNull() || v.IsUnknown() {
		return types.Float64Null()
	}
	return v
}

// stringFromAPI stores a platform string. An empty value stays null when the
// configuration has not set one, so an omitted argument is not planned as "".
func stringFromAPI(api string, prior types.String) types.String {
	if api == "" && (prior.IsNull() || prior.IsUnknown()) {
		return types.StringNull()
	}
	if api == "" && !prior.IsNull() && !prior.IsUnknown() && prior.ValueString() == "" {
		return types.StringValue("")
	}
	if api == "" {
		return types.StringNull()
	}
	return types.StringValue(api)
}

// intFromAPI stores a platform integer. Zero stays null when the configuration
// has not set a number, so an omitted argument is not planned as 0. An
// explicit 0 is kept.
func intFromAPI(api int, prior types.Int64) types.Int64 {
	if api == 0 && (prior.IsNull() || prior.IsUnknown()) {
		return types.Int64Null()
	}
	return types.Int64Value(int64(api))
}

// boolFromAPI stores a platform bool. False stays null when the configuration
// has not set a value, so an omitted argument is not planned as false. An
// explicit false is kept.
func boolFromAPI(api bool, prior types.Bool) types.Bool {
	if !api && (prior.IsNull() || prior.IsUnknown()) {
		return types.BoolNull()
	}
	return types.BoolValue(api)
}

func floatFromAPI(api float64, prior types.Float64) types.Float64 {
	if api == 0 && (prior.IsNull() || prior.IsUnknown()) {
		return types.Float64Null()
	}
	return types.Float64Value(api)
}

func versionChanged(plan, state types.Int64) bool {
	if plan.IsNull() || plan.IsUnknown() {
		return false
	}
	if state.IsNull() || state.IsUnknown() {
		return true
	}
	return plan.ValueInt64() != state.ValueInt64()
}

func secret(v types.String) string {
	if v.IsNull() || v.IsUnknown() {
		return ""
	}
	return v.ValueString()
}

func stringValue(v types.String) string {
	return secret(v)
}

func knownFloatPtr(v types.Float64) *float64 {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	n := v.ValueFloat64()
	return &n
}

func changedFloat(plan, state types.Float64) *float64 {
	if plan.IsNull() || plan.IsUnknown() {
		return nil
	}
	if !state.IsNull() && !state.IsUnknown() && state.ValueFloat64() == plan.ValueFloat64() {
		return nil
	}
	n := plan.ValueFloat64()
	return &n
}

func volatileStringValue(v string) types.String {
	if v == "" {
		return types.StringNull()
	}
	return types.StringValue(v)
}

func setString(v types.String) bool {
	return !v.IsNull() && !v.IsUnknown() && strings.TrimSpace(v.ValueString()) != ""
}
