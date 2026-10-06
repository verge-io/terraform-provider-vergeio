// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package network

import (
	"context"
	"fmt"

	"terraform-provider-vergeio/internal/shared"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

const (
	networkApplyRules = "rules"
	networkApplyDNS   = "dns"
	networkApplyAll   = "all"
)

var (
	_ action.Action                   = &NetworkApplyAction{}
	_ action.ActionWithConfigure      = &NetworkApplyAction{}
	_ action.ActionWithValidateConfig = &NetworkApplyAction{}
)

func NewNetworkApplyAction() action.Action {
	return &NetworkApplyAction{}
}

// NetworkApplyAction applies staged firewall rules, DNS, or both on a running network.
type NetworkApplyAction struct {
	sdk *vergeos.Client
}

type networkApplyActionModel struct {
	NetworkID types.String `tfsdk:"network_id"`
	Target    types.String `tfsdk:"target"`
}

func (a *NetworkApplyAction) Metadata(ctx context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network_apply"
}

func (a *NetworkApplyAction) Schema(ctx context.Context, req action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Apply staged firewall rules or DNS changes on a running VergeOS network. Use it when vergeio_network_rule resources set apply to false. rules refreshes the network so staged firewall rules take effect. dns refreshes DNS only. all does rules, then dns. The action does not wait for need_fw_apply to clear. A stopped network returns an error and is not refreshed. It loads staged rules when it starts. Requires Terraform 1.14 or later. OpenTofu does not implement actions, and no resource behavior depends on this action.",
		Attributes: map[string]schema.Attribute{
			"network_id": schema.StringAttribute{
				MarkdownDescription: "Network id, the same value as vergeio_network.id.",
				Required:            true,
			},
			"target": schema.StringAttribute{
				MarkdownDescription: "rules, dns, or all. When omitted, rules are applied.",
				Optional:            true,
				Validators: []validator.String{
					stringvalidator.OneOf(networkApplyRules, networkApplyDNS, networkApplyAll),
				},
			},
		},
	}
}

func (a *NetworkApplyAction) Configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	sdk, diags := shared.ActionSDK(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	a.sdk = sdk
}

func (a *NetworkApplyAction) ValidateConfig(ctx context.Context, req action.ValidateConfigRequest, resp *action.ValidateConfigResponse) {
	var data networkApplyActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if _, _, err := shared.KnownPositiveID(data.NetworkID, "network_id"); err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("network_id"), "Invalid Network ID", err.Error())
	}
}

func (a *NetworkApplyAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var data networkApplyActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if a.sdk == nil {
		resp.Diagnostics.AddError("VergeOS Client Missing", "The provider has not been configured.")
		return
	}
	networkID, known, err := shared.KnownPositiveID(data.NetworkID, "network_id")
	if err != nil || !known {
		if err == nil {
			err = fmt.Errorf("network_id is required")
		}
		resp.Diagnostics.AddAttributeError(path.Root("network_id"), "Invalid Network ID", err.Error())
		return
	}
	target := networkApplyRules
	if !data.Target.IsNull() && !data.Target.IsUnknown() && data.Target.ValueString() != "" {
		target = data.Target.ValueString()
	}
	network, err := a.sdk.Networks.Get(ctx, networkID)
	if err != nil {
		resp.Diagnostics.AddError("Error Applying Network Changes", err.Error())
		return
	}
	if !networkIsRunning(network) {
		resp.Diagnostics.AddError(
			"Network Is Stopped",
			fmt.Sprintf("Network %s is not running, so Terraform did not apply its firewall or DNS changes. A stopped network loads staged rules when it starts.", networkLabel(network, networkID)),
		)
		return
	}
	shared.ReportProgress(resp, fmt.Sprintf("Applying %s on network %d", target, networkID))
	tflog.Debug(ctx, fmt.Sprintf("Applying %s on network %d", target, networkID))
	switch target {
	case networkApplyRules:
		err = a.sdk.Networks.ApplyRules(ctx, networkID)
	case networkApplyDNS:
		err = a.sdk.Networks.ApplyDNS(ctx, networkID)
	case networkApplyAll:
		if err = a.sdk.Networks.ApplyRules(ctx, networkID); err == nil {
			err = a.sdk.Networks.ApplyDNS(ctx, networkID)
		}
	default:
		err = fmt.Errorf("target %q is not rules, dns, or all", target)
	}
	if err != nil {
		resp.Diagnostics.AddError("Error Applying Network Changes", err.Error())
	}
}
