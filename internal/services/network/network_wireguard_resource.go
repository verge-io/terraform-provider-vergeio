// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package network

import (
	"context"

	"terraform-provider-vergeio/internal/shared"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/verge-io/govergeos"
)

var (
	_ resource.Resource                = &NetworkWireGuardResource{}
	_ resource.ResourceWithConfigure   = &NetworkWireGuardResource{}
	_ resource.ResourceWithImportState = &NetworkWireGuardResource{}
	_ resource.ResourceWithIdentity    = &NetworkWireGuardResource{}
)

func NewNetworkWireGuardResource() resource.Resource {
	return &NetworkWireGuardResource{}
}

// NetworkWireGuardResource is one WireGuard interface on a network.
type NetworkWireGuardResource struct {
	api *vpnAPI
}

func (r *NetworkWireGuardResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network_wireguard"
}

func (r *NetworkWireGuardResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A WireGuard interface on a VergeOS network. Creating the interface stages firewall rules named Accept WireGuard and leaves them unapplied. apply defaults to true and refreshes a running network so the tunnel is not left half configured. Set apply to false to stage the rules and refresh later with vergeio_network_apply or vergeio_network_rules. A stopped network is not refreshed. It loads staged rules when it starts. Set network_id to vergeio_network.id. Destroy is refused while a peer still exists. " + vpnDeleteOrder,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "WireGuard interface id.",
				Computed:            true,
				PlanModifiers:       stringState(),
			},
			"network_id": schema.StringAttribute{
				MarkdownDescription: "Network id, the same value as vergeio_network.id. Changing it replaces this resource.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Interface name.",
				Required:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Interface description. Omit to leave the current description unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       stringState(),
			},
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the interface is enabled. Defaults to true.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
			"ip": schema.StringAttribute{
				MarkdownDescription: "Interface address with a prefix length, for example 192.168.255.1/24.",
				Required:            true,
			},
			"listen_port": schema.Int64Attribute{
				MarkdownDescription: "UDP listen port. Omit to leave the current port unchanged. VergeOS defaults a new interface to 51820.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       intState(),
			},
			"mtu": schema.Int64Attribute{
				MarkdownDescription: "Interface MTU. 0 lets VergeOS choose. Omit to leave the current MTU unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       intState(),
			},
			"private_key": schema.StringAttribute{
				MarkdownDescription: "Interface private key. Leave unset and VergeOS generates one. The API read used here does not return it, so Terraform keeps the configured value.",
				Optional:            true,
				Sensitive:           true,
			},
			"endpoint_ip": schema.StringAttribute{
				MarkdownDescription: "Address peers use to reach this interface. Omit to leave the current value unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       stringState(),
			},
			"configure_firewall": schema.BoolAttribute{
				MarkdownDescription: "Create a PAT rule on the external network for this interface. Accept WireGuard rules on this network are staged either way and are applied when apply is true.",
				Optional:            true,
			},
			"external_ip": schema.StringAttribute{
				MarkdownDescription: "External IP address id used when configure_firewall creates the PAT rule.",
				Optional:            true,
			},
			"apply": schema.BoolAttribute{
				MarkdownDescription: "Refresh the network after a change so staged Accept WireGuard rules take effect. Defaults to true. Set to false to leave the rules staged.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
			"public_key": schema.StringAttribute{
				MarkdownDescription: "Interface public key assigned by VergeOS.",
				Computed:            true,
				PlanModifiers:       stringState(),
			},
		},
	}
}

func (r *NetworkWireGuardResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	api, diags := configureVPN(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	r.api = api
}

func (r *NetworkWireGuardResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data wireGuardModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	notice, err := r.api.createWireGuard(ctx, &data)
	if err != nil {
		resp.Diagnostics.AddError("Error Creating Network WireGuard", err.Error())
		return
	}
	addFirewallNotice(&resp.Diagnostics, notice)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, data.ID)
}

func (r *NetworkWireGuardResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data wireGuardModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.readWireGuard(ctx, &data); err != nil {
		if vergeos.IsNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error Reading Network WireGuard", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, data.ID)
}

func (r *NetworkWireGuardResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data wireGuardModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	notice, err := r.api.updateWireGuard(ctx, &data)
	if err != nil {
		resp.Diagnostics.AddError("Error Updating Network WireGuard", err.Error())
		return
	}
	addFirewallNotice(&resp.Diagnostics, notice)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, data.ID)
}

func (r *NetworkWireGuardResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data wireGuardModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	notice, err := r.api.deleteWireGuard(ctx, &data)
	if err != nil {
		resp.Diagnostics.AddError("Error Deleting Network WireGuard", err.Error())
		return
	}
	addFirewallNotice(&resp.Diagnostics, notice)
}

func (r *NetworkWireGuardResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = shared.KeyIdentitySchema("WireGuard interface id. Import vergeio_network_wireguard with this value.")
}

func (r *NetworkWireGuardResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importVPN(ctx, req, resp, "Invalid Network WireGuard Import ID", "Import vergeio_network_wireguard with the interface id.")
}
