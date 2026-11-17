// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package shared

import (
	"fmt"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/verge-io/govergeos"
)

// ActionSDK returns the govergeos client stored on the provider.
// A nil provider value means Terraform has not configured the provider yet.
func ActionSDK(data any) (*vergeos.Client, diag.Diagnostics) {
	var diags diag.Diagnostics
	if data == nil {
		return nil, diags
	}
	c, ok := data.(*vergeio.Client)
	if !ok {
		diags.AddError(
			"Unexpected Action Configure Type",
			fmt.Sprintf("Expected *vergeio.Client, got: %T. Please report this issue to the provider developers.", data),
		)
		return nil, diags
	}
	sdk, err := c.CachedVergeosClient()
	if err != nil {
		diags.AddError("Unable to Create VergeOS API Client", err.Error())
		return nil, diags
	}
	return sdk, diags
}

// ReportProgress sends an action progress message when Terraform supplied the callback.
func ReportProgress(resp *action.InvokeResponse, message string) {
	if resp == nil || resp.SendProgress == nil || message == "" {
		return
	}
	resp.SendProgress(action.InvokeProgressEvent{Message: message})
}
