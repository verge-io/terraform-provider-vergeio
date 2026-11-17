// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package nas

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
)

func boolplanmodifierUseState() planmodifier.Bool {
	return boolplanmodifier.UseStateForUnknown()
}

func int64planmodifierUseState() planmodifier.Int64 {
	return int64planmodifier.UseStateForUnknown()
}

// useStateForUnknownDescription is the framework text for that modifier.
// Tests compare against it so a timestamp cannot keep a stale value.
func useStateForUnknownDescription(ctx context.Context) string {
	return int64planmodifier.UseStateForUnknown().Description(ctx)
}
