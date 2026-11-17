// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package network

import (
	"context"

	"terraform-provider-vergeio/internal/shared"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/verge-io/govergeos"
)

var (
	_ resource.Resource                = &NetworkWireGuardPeerResource{}
	_ resource.ResourceWithConfigure   = &NetworkWireGuardPeerResource{}
	_ resource.ResourceWithImportState = &NetworkWireGuardPeerResource{}
	_ resource.ResourceWithIdentity    = &NetworkWireGuardPeerResource{}
)

func NewNetworkWireGuardPeerResource() resource.Resource {
	return &NetworkWireGuardPeerResource{}
}

// NetworkWireGuardPeerResource is one peer on a WireGuard interface.
type NetworkWireGuardPeerResource struct {
	api *vpnAPI
}

func (r *NetworkWireGuardPeerResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network_wireguard_peer"
}

func (r *NetworkWireGuardPeerResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A WireGuard peer on a vergeio_network_wireguard interface. Set wireguard_id to that interface id so Terraform destroys the peer first. configure_firewall defaults to site-to-site and stages firewall rules. apply defaults to true and refreshes the interface network after a change, including the Accept WireGuard rules the interface leaves staged. A stopped network is not refreshed. " + vpnDeleteOrder,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "WireGuard peer id.",
				Computed:            true,
				PlanModifiers:       stringState(),
			},
			"wireguard_id": schema.StringAttribute{
				MarkdownDescription: "WireGuard interface id, the same value as vergeio_network_wireguard.id. Changing it replaces this resource.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Peer name.",
				Required:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Peer description. Omit to leave the current description unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       stringState(),
			},
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the peer is enabled. Defaults to true.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
			"endpoint": schema.StringAttribute{
				MarkdownDescription: "Peer address or hostname. Leave empty for a roaming client.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       stringState(),
			},
			"port": schema.Int64Attribute{
				MarkdownDescription: "Peer UDP port. Omit to leave the current port unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       intState(),
			},
			"peer_ip": schema.StringAttribute{
				MarkdownDescription: "Address used to route traffic for this peer.",
				Required:            true,
			},
			"public_key": schema.StringAttribute{
				MarkdownDescription: "Peer public key.",
				Required:            true,
			},
			"preshared_key": schema.StringAttribute{
				MarkdownDescription: "Optional preshared key. A value the API hides is kept from the configuration.",
				Optional:            true,
				Sensitive:           true,
			},
			"allowed_ips": schema.StringAttribute{
				MarkdownDescription: "Comma-separated addresses this peer may send and receive, for example 10.0.0.0/24,192.168.255.10/32.",
				Required:            true,
			},
			"configure_firewall": schema.StringAttribute{
				MarkdownDescription: "Firewall rules to stage for this peer: site-to-site, remote-user, or none. Defaults to site-to-site.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(vergeos.WireGuardPeerFirewallSiteToSite),
				Validators: []validator.String{
					stringvalidator.OneOf(
						vergeos.WireGuardPeerFirewallSiteToSite,
						vergeos.WireGuardPeerFirewallRemoteUser,
						vergeos.WireGuardPeerFirewallNone,
					),
				},
			},
			"keepalive": schema.Int64Attribute{
				MarkdownDescription: "Persistent keepalive interval in seconds. 0 disables it. Omit to leave the current value unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       intState(),
			},
			"autogenerate_peer": schema.BoolAttribute{
				MarkdownDescription: "Generate a downloadable peer configuration. peer_config is set when this is true.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       boolState(),
			},
			"apply": schema.BoolAttribute{
				MarkdownDescription: "Refresh the interface network after a change so staged firewall rules take effect. Defaults to true.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
			"peer_config": schema.StringAttribute{
				MarkdownDescription: "Generated peer configuration when autogenerate_peer is true. It can contain a private key.",
				Computed:            true,
				Sensitive:           true,
				PlanModifiers:       stringState(),
			},
		},
	}
}

func (r *NetworkWireGuardPeerResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	api, diags := configureVPN(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	r.api = api
}

func (r *NetworkWireGuardPeerResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data wireGuardPeerModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	notice, err := r.api.createWireGuardPeer(ctx, &data)
	if err != nil {
		resp.Diagnostics.AddError("Error Creating Network WireGuard Peer", err.Error())
		return
	}
	addFirewallNotice(&resp.Diagnostics, notice)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, data.ID)
}

func (r *NetworkWireGuardPeerResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data wireGuardPeerModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.readWireGuardPeer(ctx, &data); err != nil {
		if vergeos.IsNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error Reading Network WireGuard Peer", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, data.ID)
}

func (r *NetworkWireGuardPeerResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data wireGuardPeerModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	notice, err := r.api.updateWireGuardPeer(ctx, &data)
	if err != nil {
		resp.Diagnostics.AddError("Error Updating Network WireGuard Peer", err.Error())
		return
	}
	addFirewallNotice(&resp.Diagnostics, notice)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, data.ID)
}

func (r *NetworkWireGuardPeerResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data wireGuardPeerModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	notice, err := r.api.deleteWireGuardPeer(ctx, &data)
	if err != nil {
		resp.Diagnostics.AddError("Error Deleting Network WireGuard Peer", err.Error())
		return
	}
	addFirewallNotice(&resp.Diagnostics, notice)
}

func (r *NetworkWireGuardPeerResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = shared.KeyIdentitySchema("WireGuard peer id. Import vergeio_network_wireguard_peer with this value.")
}

func (r *NetworkWireGuardPeerResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importVPN(ctx, req, resp, "Invalid Network WireGuard Peer Import ID", "Import vergeio_network_wireguard_peer with the peer id.")
}
