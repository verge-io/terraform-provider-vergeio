// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package platform

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-vergeio/internal/shared"
)

func idString(id int) types.String {
	if id <= 0 {
		return types.StringNull()
	}
	return types.StringValue(strconv.Itoa(id))
}

func parseID(id types.String, what string) (int, error) {
	if !knownString(id) {
		return 0, fmt.Errorf("%s id is empty", what)
	}
	return parseIDText(id.ValueString(), what)
}

func parseIDText(text, what string) (int, error) {
	text = strings.TrimSpace(text)
	n, err := strconv.Atoi(text)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%s id %q is not a positive integer", what, text)
	}
	return n, nil
}

func intPtr(n int64) (*int, error) {
	if n > int64(math.MaxInt) || n < int64(math.MinInt) {
		return nil, fmt.Errorf("value %d does not fit in an integer", n)
	}
	v := int(n)
	return &v, nil
}

func knownIntPtr(v types.Int64) (*int, error) {
	if !knownInt(v) {
		return nil, nil
	}
	return intPtr(v.ValueInt64())
}

func changedIntPtr(plan, state types.Int64) (*int, error) {
	if !knownInt(plan) {
		return nil, nil
	}
	if knownInt(state) && state.ValueInt64() == plan.ValueInt64() {
		return nil, nil
	}
	return intPtr(plan.ValueInt64())
}

func changedString(plan, state types.String) *string {
	if !knownString(plan) {
		return nil
	}
	if knownString(state) && state.ValueString() == plan.ValueString() {
		return nil
	}
	s := plan.ValueString()
	return &s
}

func changedBool(plan, state types.Bool) *bool {
	if !knownBool(plan) {
		return nil
	}
	if knownBool(state) && state.ValueBool() == plan.ValueBool() {
		return nil
	}
	b := plan.ValueBool()
	return &b
}

func importPositive(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse, summary, detail string) {
	id := strings.TrimSpace(req.ID)
	if id == "" && req.Identity != nil {
		var got types.String
		resp.Diagnostics.Append(req.Identity.GetAttribute(ctx, path.Root("id"), &got)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if knownString(got) {
			id = strings.TrimSpace(got.ValueString())
		}
	}
	if _, err := parseIDText(id, "resource"); err != nil {
		resp.Diagnostics.AddError(summary, detail)
		return
	}
	shared.ImportByID(ctx, req, resp, summary, detail)
}

func validSettingKey(key string) error {
	key = strings.TrimSpace(key)
	if key == "" || strings.ContainsAny(key, "/\\?# \t\r\n") {
		return fmt.Errorf("setting key %q is empty or contains a character that cannot be used in the settings path", key)
	}
	return nil
}
