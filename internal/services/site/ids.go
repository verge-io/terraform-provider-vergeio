// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package site

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-vergeio/internal/shared"
)

var regexpPositive = regexp.MustCompile(`^[1-9][0-9]*$`)

func idString(id int) types.String {
	if id <= 0 {
		return types.StringNull()
	}
	return types.StringValue(strconv.Itoa(id))
}

func parseID(id types.String, what string) (int, error) {
	if id.IsNull() || id.IsUnknown() {
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

func knownInt(v types.Int64) (*int, error) {
	if v.IsNull() || v.IsUnknown() {
		return nil, nil
	}
	return intPtr(v.ValueInt64())
}

func changedInt(plan, state types.Int64) (*int, error) {
	if plan.IsNull() || plan.IsUnknown() {
		return nil, nil
	}
	if !state.IsNull() && !state.IsUnknown() && state.ValueInt64() == plan.ValueInt64() {
		return nil, nil
	}
	return intPtr(plan.ValueInt64())
}

// importPositive accepts the resource key. A name or an empty id is rejected
// before the value is written into state.
func importPositive(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse, summary, detail string) {
	id := strings.TrimSpace(req.ID)
	if id == "" && req.Identity != nil {
		var got types.String
		resp.Diagnostics.Append(req.Identity.GetAttribute(ctx, path.Root("id"), &got)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if !got.IsNull() && !got.IsUnknown() {
			id = strings.TrimSpace(got.ValueString())
		}
	}
	if _, err := parseIDText(id, "resource"); err != nil {
		resp.Diagnostics.AddError(summary, detail)
		return
	}
	shared.ImportByID(ctx, req, resp, summary, detail)
}
