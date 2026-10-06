// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package network

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"

	"terraform-provider-vergeio/internal/client"
	"terraform-provider-vergeio/internal/shared"
)

func configureVPN(providerData any) (*vpnAPI, diag.Diagnostics) {
	var diags diag.Diagnostics
	if providerData == nil {
		return nil, diags
	}
	client, ok := providerData.(*vergeio.Client)
	if !ok {
		diags.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *vergeio.Client, got: %T. Please report this issue to the provider developers.", providerData),
		)
		return nil, diags
	}
	api, err := newVPNAPI(client)
	if err != nil {
		diags.AddError("Unable to Create VergeOS API Client", err.Error())
		return nil, diags
	}
	return api, diags
}

func addFirewallNotice(diags *diag.Diagnostics, notice *firewallNotice) {
	if diags == nil || notice == nil || notice.Detail == "" {
		return
	}
	diags.AddWarning(notice.Summary, notice.Detail)
}

func importVPN(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse, summary, detail string) {
	if strings.TrimSpace(req.ID) != "" {
		if _, err := parsePositiveID(req.ID); err != nil {
			resp.Diagnostics.AddError(summary, detail)
			return
		}
	}
	shared.ImportByID(ctx, req, resp, summary, detail)
}
