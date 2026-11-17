// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package platform

import (
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-vergeio/internal/shared"
)

func knownString(v types.String) bool {
	return !v.IsNull() && !v.IsUnknown()
}

func knownBool(v types.Bool) bool {
	return !v.IsNull() && !v.IsUnknown()
}

func knownInt(v types.Int64) bool {
	return !v.IsNull() && !v.IsUnknown()
}

func secret(v types.String) string {
	if !knownString(v) {
		return ""
	}
	return v.ValueString()
}

func stringValue(v types.String) string {
	return secret(v)
}

func versionChanged(plan, state types.Int64) bool {
	if !knownInt(plan) {
		return false
	}
	if !knownInt(state) {
		return true
	}
	return plan.ValueInt64() != state.ValueInt64()
}

func writeOnlyActive(v types.Int64) bool {
	return shared.WriteOnlyVersionSet(v)
}

// stringFromAPI stores a platform string. An empty value stays null when the
// configuration has not set one, so an omitted argument is not planned as "".
func stringFromAPI(api string, prior types.String) types.String {
	if api == "" && (prior.IsNull() || prior.IsUnknown()) {
		return types.StringNull()
	}
	if api == "" && knownString(prior) && prior.ValueString() == "" {
		return types.StringValue("")
	}
	if api == "" {
		return types.StringNull()
	}
	return types.StringValue(api)
}

// keepConfiguredString keeps a value VergeOS does not return on read.
// A non-empty API value replaces it.
func keepConfiguredString(api string, prior types.String) types.String {
	if api != "" {
		return types.StringValue(api)
	}
	if !knownString(prior) {
		return types.StringNull()
	}
	return prior
}

func intFromAPI(api int, prior types.Int64) types.Int64 {
	if api == 0 && !knownInt(prior) {
		return types.Int64Null()
	}
	return types.Int64Value(int64(api))
}

func boolFromAPI(api bool, prior types.Bool) types.Bool {
	if !api && !knownBool(prior) {
		return types.BoolNull()
	}
	return types.BoolValue(api)
}

func timestampFromAPI(api int64) types.Int64 {
	if api == 0 {
		return types.Int64Null()
	}
	return types.Int64Value(api)
}

func canonicalPEM(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	return s
}

func pemEqual(a, b string) bool {
	return canonicalPEM(a) == canonicalPEM(b)
}

// keepPEM stores the configured PEM when it matches the API text aside from
// line endings and surrounding space. A real change stores the API text.
func keepPEM(api string, prior types.String) types.String {
	if api == "" {
		return keepConfiguredString("", prior)
	}
	if knownString(prior) && pemEqual(prior.ValueString(), api) {
		return prior
	}
	return types.StringValue(api)
}
