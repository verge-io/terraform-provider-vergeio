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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/verge-io/govergeos"
)

const (
	dnsApplyDescription = "Refresh DNS on this network after the change. Defaults to true. Set false to stage several DNS resources, then leave it true on the last one, which depends_on the others. Each refresh reloads DNS for the whole network. A stopped network is not refreshed. It loads staged DNS when it starts. With apply set to false, Terraform leaves need_dns_apply set and warns. VergeOS may act on a stale pending flag on its own."

	dnsViewDescription = "One DNS view on a VergeOS network. A view selects which clients receive which zones. Set network_id to vergeio_network.id so Terraform destroys this view before the network. apply defaults to true and refreshes DNS on a running network after a change. Set apply to false to stage several DNS resources, then leave it true on the last one, which depends_on the others. A stopped network is not refreshed. It loads staged DNS when it starts. VergeOS deletes the zones and records in this view when the view is deleted. If that delete is refused, Terraform removes the records and zones first so a partial create cannot leave a row that blocks network delete."
)

var (
	_ resource.Resource                = &NetworkDNSViewResource{}
	_ resource.ResourceWithConfigure   = &NetworkDNSViewResource{}
	_ resource.ResourceWithImportState = &NetworkDNSViewResource{}
	_ resource.ResourceWithIdentity    = &NetworkDNSViewResource{}
)

func NewNetworkDNSViewResource() resource.Resource {
	return &NetworkDNSViewResource{}
}

// NetworkDNSViewResource is one DNS view on a network.
type NetworkDNSViewResource struct {
	api *DNSApi
}

type dnsViewModel struct {
	ID                types.String `tfsdk:"id"`
	NetworkID         types.String `tfsdk:"network_id"`
	Name              types.String `tfsdk:"name"`
	Recursion         types.Bool   `tfsdk:"recursion"`
	MatchClients      types.String `tfsdk:"match_clients"`
	MatchDestinations types.String `tfsdk:"match_destinations"`
	MaxCacheSize      types.Int64  `tfsdk:"max_cache_size"`
	OrderID           types.Int64  `tfsdk:"orderid"`
	QuerySource       types.Int64  `tfsdk:"query_source"`
	Modified          types.Int64  `tfsdk:"modified"`
	Apply             types.Bool   `tfsdk:"apply"`
}

func dnsApplyAttribute() schema.BoolAttribute {
	return schema.BoolAttribute{
		MarkdownDescription: dnsApplyDescription,
		Optional:            true,
		Computed:            true,
		Default:             booldefault.StaticBool(true),
	}
}

func dnsMissing(err error) bool {
	return vergeos.IsNotFoundError(err)
}

func (r *NetworkDNSViewResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network_dns_view"
}

func (r *NetworkDNSViewResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: dnsViewDescription,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "DNS view id.",
				Computed:            true,
				PlanModifiers:       stringState(),
			},
			"network_id": schema.StringAttribute{
				MarkdownDescription: "Network id, the same value as vergeio_network.id. Changing it replaces the view.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "View name, unique on the network.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"recursion": schema.BoolAttribute{
				MarkdownDescription: "Allow this view to resolve names it does not serve itself. Omit to leave the current setting unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       boolState(),
			},
			"match_clients": schema.StringAttribute{
				MarkdownDescription: "Clients that use this view, in the form VergeOS stores, for example 10.0.0.0/8;192.168.0.0/16;. Omit to leave the current list unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       stringState(),
			},
			"match_destinations": schema.StringAttribute{
				MarkdownDescription: "Destinations that use this view, in the same form as match_clients. Omit to leave the current list unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       stringState(),
			},
			"max_cache_size": schema.Int64Attribute{
				MarkdownDescription: "Maximum cache size in bytes. 0 means no limit. Omit to leave the current size unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       intState(),
			},
			"orderid": schema.Int64Attribute{
				MarkdownDescription: "Processing order. Lower numbers are checked first. Omit to leave the current order unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       intState(),
			},
			"query_source": schema.Int64Attribute{
				MarkdownDescription: "Address id used as the source of outgoing queries. Omit to leave the current value unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       intState(),
			},
			// modified changes on every write. UseStateForUnknown would keep the
			// prior time in the plan, and the refreshed value would fail apply
			// as an inconsistent result. Leave it unknown so the read can store it.
			"modified": schema.Int64Attribute{
				MarkdownDescription: "Last modification time, as seconds since the epoch.",
				Computed:            true,
			},
			"apply": dnsApplyAttribute(),
		},
	}
}

func (r *NetworkDNSViewResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	api, diags := configureDNS(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	r.api = api
}

func (r *NetworkDNSViewResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data dnsViewModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	notice, err := r.api.createView(ctx, &data)
	if err != nil {
		resp.Diagnostics.AddError("Error Creating Network DNS View", err.Error())
		return
	}
	addDNSNotice(&resp.Diagnostics, notice)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, data.ID)
}

func (r *NetworkDNSViewResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data dnsViewModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.readView(ctx, &data); err != nil {
		if dnsMissing(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error Reading Network DNS View", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, data.ID)
}

func (r *NetworkDNSViewResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state dnsViewModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	notice, err := r.api.updateView(ctx, &plan, &state)
	if err != nil {
		resp.Diagnostics.AddError("Error Updating Network DNS View", err.Error())
		return
	}
	addDNSNotice(&resp.Diagnostics, notice)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, plan.ID)
}

func (r *NetworkDNSViewResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data dnsViewModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	notice, err := r.api.deleteView(ctx, &data)
	if err != nil {
		resp.Diagnostics.AddError("Error Deleting Network DNS View", err.Error())
		return
	}
	addDNSNotice(&resp.Diagnostics, notice)
}

func (r *NetworkDNSViewResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = shared.KeyIdentitySchema("DNS view id. Import vergeio_network_dns_view with this value.")
}

func (r *NetworkDNSViewResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByPositiveID(ctx, req, resp, "Invalid Network DNS View Import ID", "Import vergeio_network_dns_view with the view id.")
}
