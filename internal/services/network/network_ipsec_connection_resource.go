// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package network

import (
	"context"

	"terraform-provider-vergeio/internal/shared"

	"github.com/hashicorp/terraform-plugin-framework-validators/objectvalidator"
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
	_ resource.Resource                = &NetworkIPSecConnectionResource{}
	_ resource.ResourceWithConfigure   = &NetworkIPSecConnectionResource{}
	_ resource.ResourceWithImportState = &NetworkIPSecConnectionResource{}
	_ resource.ResourceWithIdentity    = &NetworkIPSecConnectionResource{}
)

func NewNetworkIPSecConnectionResource() resource.Resource {
	return &NetworkIPSecConnectionResource{}
}

// NetworkIPSecConnectionResource is one IPsec tunnel: phase 1 and phase 2.
type NetworkIPSecConnectionResource struct {
	api *vpnAPI
}

func (r *NetworkIPSecConnectionResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network_ipsec_connection"
}

func (r *NetworkIPSecConnectionResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "One IPsec tunnel: a phase 1 (IKE) and one phase 2 (IPsec SA). Set ipsec_id to vergeio_network_ipsec.id. Destroy deletes every phase 2 under that phase 1, then the phase 1. Phase 2 goes first because a phase 1 delete while a phase 2 row remains fails and then keeps failing. Live security-association status is read from vnet_ipsec_connections and is not stored, because it changes while the tunnel is up. " + vpnDeleteOrder,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Phase 1 id. Import vergeio_network_ipsec_connection with this value.",
				Computed:            true,
				PlanModifiers:       stringState(),
			},
			"ipsec_id": schema.StringAttribute{
				MarkdownDescription: "IPsec configuration id, the same value as vergeio_network_ipsec.id. Changing it replaces this resource.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Phase 1 name.",
				Required:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Phase 1 description. Omit to leave the current description unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       stringState(),
			},
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the phase 1 is enabled. Defaults to true.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
			"remote_gateway": schema.StringAttribute{
				MarkdownDescription: "Remote peer address or hostname.",
				Required:            true,
			},
			"keyexchange": schema.StringAttribute{
				MarkdownDescription: "IKE version: ikev1, ikev2, or ike. ike initiates IKEv2 and accepts either. Omit to leave the current value unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       stringState(),
				Validators: []validator.String{
					stringvalidator.OneOf(vergeos.IPSecKeyExchangeIKEv1, vergeos.IPSecKeyExchangeIKEv2, vergeos.IPSecKeyExchangeAuto),
				},
			},
			"auth": schema.StringAttribute{
				MarkdownDescription: "Authentication: psk or pubkey. Omit to leave the current value unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       stringState(),
				Validators: []validator.String{
					stringvalidator.OneOf(vergeos.IPSecAuthPSK, vergeos.IPSecAuthPubkey),
				},
			},
			"negotiation": schema.StringAttribute{
				MarkdownDescription: "IKEv1 negotiation mode: main or aggressive. Omit to leave the current value unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       stringState(),
				Validators: []validator.String{
					stringvalidator.OneOf(vergeos.IPSecNegotiationMain, vergeos.IPSecNegotiationAggressive),
				},
			},
			"identifier": schema.StringAttribute{
				MarkdownDescription: "Local IKE identity. Blank uses the current address. Omit to leave the current value unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       stringState(),
			},
			"peer_identifier": schema.StringAttribute{
				MarkdownDescription: "Remote IKE identity. Blank uses remote_gateway. Omit to leave the current value unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       stringState(),
			},
			"psk": schema.StringAttribute{
				MarkdownDescription: "Pre-shared key. The API read used here does not return it, so Terraform keeps the configured value. Required by VergeOS when auth is psk.",
				Optional:            true,
				Sensitive:           true,
			},
			"ike": schema.StringAttribute{
				MarkdownDescription: "IKE cipher proposal. Omit to leave the current value unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       stringState(),
			},
			"ikelifetime": schema.Int64Attribute{
				MarkdownDescription: "IKE SA lifetime in seconds. Omit to leave the current value unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       intState(),
			},
			"auto": schema.StringAttribute{
				MarkdownDescription: "Startup behavior: add, route, or start. Omit to leave the current value unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       stringState(),
				Validators: []validator.String{
					stringvalidator.OneOf(vergeos.IPSecAutoAdd, vergeos.IPSecAutoRoute, vergeos.IPSecAutoStart),
				},
			},
			"mobike": schema.BoolAttribute{
				MarkdownDescription: "Enable IKEv2 MOBIKE. Omit to leave the current value unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       boolState(),
			},
			"split_connections": schema.BoolAttribute{
				MarkdownDescription: "Create a separate connection for each phase 2. Omit to leave the current value unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       boolState(),
			},
			"forceencaps": schema.BoolAttribute{
				MarkdownDescription: "Force UDP encapsulation. Omit to leave the current value unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       boolState(),
			},
			"keyingtries": schema.Int64Attribute{
				MarkdownDescription: "Negotiation attempts. 0 never gives up. Omit to leave the current value unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       intState(),
			},
			"rekey": schema.BoolAttribute{
				MarkdownDescription: "Renegotiate before the IKE SA expires. Omit to leave the current value unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       boolState(),
			},
			"reauth": schema.BoolAttribute{
				MarkdownDescription: "Reauthenticate during IKEv2 rekey. Omit to leave the current value unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       boolState(),
			},
			"margintime": schema.Int64Attribute{
				MarkdownDescription: "Seconds before expiry to start rekeying. Omit to leave the current value unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       intState(),
			},
			"dpdaction": schema.StringAttribute{
				MarkdownDescription: "Dead peer detection action: none, clear, hold, or restart. Omit to leave the current value unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       stringState(),
				Validators: []validator.String{
					stringvalidator.OneOf(vergeos.IPSecDPDNone, vergeos.IPSecDPDClear, vergeos.IPSecDPDHold, vergeos.IPSecDPDRestart),
				},
			},
			"dpddelay": schema.Int64Attribute{
				MarkdownDescription: "Seconds between dead peer detection checks. Omit to leave the current value unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       intState(),
			},
			"dpdfailures": schema.Int64Attribute{
				MarkdownDescription: "IKEv1 dead peer detection failures before disconnect. Omit to leave the current value unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       intState(),
			},
		},
		// phase 2 is its own vnet_ipsec_phase2s row, not a field on the phase 1
		// payload. A block matches that child row. An attribute would reject
		// the phase2 { } configuration this resource documents.
		Blocks: map[string]schema.Block{
			"phase2": schema.SingleNestedBlock{
				MarkdownDescription: "Phase 2 selector for this tunnel. Required. Destroy removes every phase 2 under the phase 1, including a row this resource does not list, and then removes the phase 1.",
				Validators: []validator.Object{
					objectvalidator.IsRequired(),
				},
				Attributes: map[string]schema.Attribute{
					"id": schema.StringAttribute{
						MarkdownDescription: "Phase 2 id.",
						Computed:            true,
						PlanModifiers:       stringState(),
					},
					"name": schema.StringAttribute{
						MarkdownDescription: "Phase 2 name.",
						Required:            true,
					},
					"description": schema.StringAttribute{
						MarkdownDescription: "Phase 2 description. Omit to leave the current description unchanged.",
						Optional:            true,
						Computed:            true,
						PlanModifiers:       stringState(),
					},
					"enabled": schema.BoolAttribute{
						MarkdownDescription: "Whether the phase 2 is enabled. Defaults to true.",
						Optional:            true,
						Computed:            true,
						Default:             booldefault.StaticBool(true),
					},
					"mode": schema.StringAttribute{
						MarkdownDescription: "tunnel or transport. Defaults to tunnel.",
						Optional:            true,
						Computed:            true,
						Default:             stringdefault.StaticString(vergeos.IPSecModeTunnel),
						Validators: []validator.String{
							stringvalidator.OneOf(vergeos.IPSecModeTunnel, vergeos.IPSecModeTransport),
						},
					},
					"local": schema.StringAttribute{
						MarkdownDescription: "Local network or address.",
						Required:            true,
					},
					"remote": schema.StringAttribute{
						MarkdownDescription: "Remote network or address. Omit to leave the current value unchanged.",
						Optional:            true,
						Computed:            true,
						PlanModifiers:       stringState(),
					},
					"lifetime": schema.Int64Attribute{
						MarkdownDescription: "IPsec SA lifetime in seconds. Omit to leave the current value unchanged.",
						Optional:            true,
						Computed:            true,
						PlanModifiers:       intState(),
					},
					"protocol": schema.StringAttribute{
						MarkdownDescription: "esp or ah. Defaults to esp.",
						Optional:            true,
						Computed:            true,
						Default:             stringdefault.StaticString(vergeos.IPSecProtocolESP),
						Validators: []validator.String{
							stringvalidator.OneOf(vergeos.IPSecProtocolESP, vergeos.IPSecProtocolAH),
						},
					},
					"ciphers": schema.StringAttribute{
						MarkdownDescription: "Phase 2 cipher proposal. Omit to leave the current value unchanged.",
						Optional:            true,
						Computed:            true,
						PlanModifiers:       stringState(),
					},
				},
			},
		},
	}
}

func (r *NetworkIPSecConnectionResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	api, diags := configureVPN(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	r.api = api
}

func (r *NetworkIPSecConnectionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ipsecConnectionModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.createIPSecConnection(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error Creating Network IPsec Connection", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, data.ID)
}

func (r *NetworkIPSecConnectionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ipsecConnectionModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.readIPSecConnection(ctx, &data); err != nil {
		if vergeos.IsNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error Reading Network IPsec Connection", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, data.ID)
}

func (r *NetworkIPSecConnectionResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data ipsecConnectionModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.updateIPSecConnection(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error Updating Network IPsec Connection", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, data.ID)
}

func (r *NetworkIPSecConnectionResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data ipsecConnectionModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.deleteIPSecConnection(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error Deleting Network IPsec Connection", err.Error())
	}
}

func (r *NetworkIPSecConnectionResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = shared.KeyIdentitySchema("IPsec phase 1 id. Import vergeio_network_ipsec_connection with this value.")
}

func (r *NetworkIPSecConnectionResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importVPN(ctx, req, resp, "Invalid Network IPsec Connection Import ID", "Import vergeio_network_ipsec_connection with the phase 1 id.")
}
