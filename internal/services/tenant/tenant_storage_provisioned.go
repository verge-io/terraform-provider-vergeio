// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tenant

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// provisionedGiB is 1 GiB in bytes. VergeOS types tenant_storage.provisioned
// as disksize and floors the stored quota to a multiple of this value.
const provisionedGiB int64 = 1 << 30

func provisionedAligned(bytes int64) bool {
	return bytes > 0 && bytes%provisionedGiB == 0
}

// provisionedMultipleOfGiB rejects quotas VergeOS would floor, so applying a
// non-aligned value cannot taint the allocation (#197).
type provisionedMultipleOfGiB struct{}

func (v provisionedMultipleOfGiB) Description(_ context.Context) string {
	return fmt.Sprintf("value must be a positive multiple of %d (1 GiB); VergeOS floors provisioned to whole GiB", provisionedGiB)
}

func (v provisionedMultipleOfGiB) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v provisionedMultipleOfGiB) ValidateInt64(ctx context.Context, req validator.Int64Request, resp *validator.Int64Response) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	value := req.ConfigValue.ValueInt64()
	if provisionedAligned(value) {
		return
	}
	resp.Diagnostics.AddAttributeError(
		req.Path,
		"Invalid provisioned value",
		fmt.Sprintf("provisioned must be a positive multiple of %d (1 GiB); VergeOS floors non-aligned values, which previously tainted the allocation. Got: %d", provisionedGiB, value),
	)
}
