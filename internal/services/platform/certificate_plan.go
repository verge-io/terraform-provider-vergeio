// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package platform

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/verge-io/govergeos"
)

func (r *certificateResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Config.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}
	var config certificateModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if selfSignedIgnoresKeySettings(config) {
		resp.Diagnostics.AddWarning(
			"VergeOS ignores key settings on a self_signed certificate",
			"VergeOS ignores key_type and rsa_key_size for self_signed certificates and always issues RSA 2048. The row still stores the configured values.",
		)
	}
}

func selfSignedIgnoresKeySettings(config certificateModel) bool {
	if !knownString(config.Type) || config.Type.ValueString() != vergeos.CertificateTypeSelfSigned {
		return false
	}
	return !config.KeyType.IsNull() || !config.RSAKeySize.IsNull()
}
