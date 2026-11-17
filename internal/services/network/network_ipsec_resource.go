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
	_ resource.Resource                = &NetworkIPSecResource{}
	_ resource.ResourceWithConfigure   = &NetworkIPSecResource{}
	_ resource.ResourceWithImportState = &NetworkIPSecResource{}
	_ resource.ResourceWithIdentity    = &NetworkIPSecResource{}
)

func NewNetworkIPSecResource() resource.Resource {
	return &NetworkIPSecResource{}
}

// NetworkIPSecResource is the IPsec configuration for one network.
type NetworkIPSecResource struct {
	api *vpnAPI
}

func (r *NetworkIPSecResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network_ipsec"
}

func (r *NetworkIPSecResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "IPsec settings for one VergeOS network. A network has one of these. Set network_id to vergeio_network.id so Terraform destroys this resource before the network. Destroy is refused while a phase 1 row still exists. Delete vergeio_network_ipsec_connection first. That resource deletes phase 2, then phase 1. Deleting phase 1 while a phase 2 row remains makes later phase 1 deletes fail permanently, and deleting the network does not remove these rows. " + vpnDeleteOrder,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "IPsec configuration id.",
				Computed:            true,
				PlanModifiers:       stringState(),
			},
			"network_id": schema.StringAttribute{
				MarkdownDescription: "Network id, the same value as vergeio_network.id. Changing it replaces this resource. A reference creates the destroy edge the platform does not.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether IPsec is enabled on the network. Defaults to true.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
			"mode": schema.StringAttribute{
				MarkdownDescription: "normal uses the phase 1 and phase 2 resources. advanced uses strongswan_conf, ipsec_conf, and ipsec_secrets. Defaults to normal.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(vergeos.IPSecModeNormal),
				Validators: []validator.String{
					stringvalidator.OneOf(vergeos.IPSecModeNormal, vergeos.IPSecModeAdvanced),
				},
			},
			"uniqueids": schema.StringAttribute{
				MarkdownDescription: "How duplicate IKE identities are handled: yes, no, never, replace, or keep. Omit to leave the current value unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       stringState(),
				Validators: []validator.String{
					stringvalidator.OneOf(
						vergeos.IPSecUniqueIDsYes,
						vergeos.IPSecUniqueIDsNo,
						vergeos.IPSecUniqueIDsNever,
						vergeos.IPSecUniqueIDsReplace,
						vergeos.IPSecUniqueIDsKeep,
					),
				},
			},
			"compress": schema.BoolAttribute{
				MarkdownDescription: "Propose IPComp compression. Omit to leave the current value unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       boolState(),
			},
			"exclude_network": schema.BoolAttribute{
				MarkdownDescription: "Exclude the local subnet from IPsec. Omit to leave the current value unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       boolState(),
			},
			"strongswan_conf": schema.StringAttribute{
				MarkdownDescription: "Raw strongswan.conf content for advanced mode. The API read used here does not return it, so Terraform keeps the configured value.",
				Optional:            true,
				Sensitive:           true,
			},
			"ipsec_conf": schema.StringAttribute{
				MarkdownDescription: "Raw ipsec.conf content for advanced mode. The API read used here does not return it, so Terraform keeps the configured value.",
				Optional:            true,
				Sensitive:           true,
			},
			"ipsec_secrets": schema.StringAttribute{
				MarkdownDescription: "Raw ipsec.secrets content for advanced mode. The API read used here does not return it, so Terraform keeps the configured value.",
				Optional:            true,
				Sensitive:           true,
			},
			"cisco_unity": schema.BoolAttribute{
				MarkdownDescription: "Send the Cisco Unity vendor ID payload. IKEv1 only. The API read used here does not return it, so Terraform keeps the configured value.",
				Optional:            true,
			},
			"accept_unencrypted_mainmode": schema.BoolAttribute{
				MarkdownDescription: "Accept unencrypted ID and HASH payloads in IKEv1 main mode. The API read used here does not return it, so Terraform keeps the configured value.",
				Optional:            true,
			},
			"mss_clamp": schema.Int64Attribute{
				MarkdownDescription: "MSS clamp on installed routes. 0 disables it. The API read used here does not return it, so Terraform keeps the configured value.",
				Optional:            true,
			},
			"strictcrlpolicy": schema.StringAttribute{
				MarkdownDescription: "CRL validation: yes, ifuri, or no. Omit to leave the current value unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       stringState(),
				Validators: []validator.String{
					stringvalidator.OneOf("yes", "ifuri", "no"),
				},
			},
			"make_before_break": schema.BoolAttribute{
				MarkdownDescription: "Use make-before-break reauthentication. IKEv2 only. The API read used here does not return it, so Terraform keeps the configured value.",
				Optional:            true,
			},
		},
	}
}

func (r *NetworkIPSecResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	api, diags := configureVPN(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	r.api = api
}

func (r *NetworkIPSecResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ipsecModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.createIPSec(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error Creating Network IPsec", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, data.ID)
}

func (r *NetworkIPSecResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ipsecModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.readIPSec(ctx, &data); err != nil {
		if vergeos.IsNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error Reading Network IPsec", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, data.ID)
}

func (r *NetworkIPSecResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data ipsecModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.updateIPSec(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error Updating Network IPsec", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, data.ID)
}

func (r *NetworkIPSecResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data ipsecModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.deleteIPSec(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error Deleting Network IPsec", err.Error())
	}
}

func (r *NetworkIPSecResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = shared.KeyIdentitySchema("IPsec configuration id. Import vergeio_network_ipsec with this value.")
}

func (r *NetworkIPSecResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importVPN(ctx, req, resp, "Invalid Network IPsec Import ID", "Import vergeio_network_ipsec with the IPsec configuration id.")
}
