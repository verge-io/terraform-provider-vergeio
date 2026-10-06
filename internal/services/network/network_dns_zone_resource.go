// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package network

import (
	"context"

	"terraform-provider-vergeio/internal/shared"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/verge-io/govergeos"
)

const dnsZoneDescription = "One DNS zone in a vergeio_network_dns_view. Set view_id to that view id so Terraform destroys this zone before the view. VergeOS deletes the records in this zone when the zone is deleted. If that delete is refused, Terraform removes the records first so they cannot block a later network delete. apply defaults to true and refreshes DNS on the view network after a change. A stopped network is not refreshed. It loads staged DNS when it starts."

var (
	_ resource.Resource                = &NetworkDNSZoneResource{}
	_ resource.ResourceWithConfigure   = &NetworkDNSZoneResource{}
	_ resource.ResourceWithImportState = &NetworkDNSZoneResource{}
	_ resource.ResourceWithIdentity    = &NetworkDNSZoneResource{}

	dnsZoneTypes = []string{
		vergeos.DNSZoneTypeMaster,
		vergeos.DNSZoneTypeSlave,
		vergeos.DNSZoneTypeRedirect,
		vergeos.DNSZoneTypeForward,
		vergeos.DNSZoneTypeStaticStub,
		vergeos.DNSZoneTypeStub,
	}
	dnsNotifyModes = []string{"yes", "no", "explicit"}
)

func NewNetworkDNSZoneResource() resource.Resource {
	return &NetworkDNSZoneResource{}
}

// NetworkDNSZoneResource is one DNS zone in a view.
type NetworkDNSZoneResource struct {
	api *DNSApi
}

type dnsZoneModel struct {
	ID              types.String `tfsdk:"id"`
	ViewID          types.String `tfsdk:"view_id"`
	NetworkID       types.String `tfsdk:"network_id"`
	Domain          types.String `tfsdk:"domain"`
	Type            types.String `tfsdk:"type"`
	Nameserver      types.String `tfsdk:"nameserver"`
	Email           types.String `tfsdk:"email"`
	Notify          types.String `tfsdk:"notify"`
	AllowNotify     types.String `tfsdk:"allow_notify"`
	AlsoNotify      types.String `tfsdk:"also_notify"`
	Masters         types.String `tfsdk:"masters"`
	AllowTransfer   types.String `tfsdk:"allow_transfer"`
	SerialNumber    types.Int64  `tfsdk:"serial_number"`
	DefaultTTL      types.String `tfsdk:"default_ttl"`
	RefreshInterval types.String `tfsdk:"refresh_interval"`
	RetryInterval   types.String `tfsdk:"retry_interval"`
	ExpiryPeriod    types.String `tfsdk:"expiry_period"`
	NegativeTTL     types.String `tfsdk:"negative_ttl"`
	Forwarders      types.String `tfsdk:"forwarders"`
	Modified        types.Int64  `tfsdk:"modified"`
	Apply           types.Bool   `tfsdk:"apply"`
}

func (r *NetworkDNSZoneResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network_dns_zone"
}

func (r *NetworkDNSZoneResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: dnsZoneDescription,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "DNS zone id.",
				Computed:            true,
				PlanModifiers:       stringState(),
			},
			"view_id": schema.StringAttribute{
				MarkdownDescription: "DNS view id, the same value as vergeio_network_dns_view.id. Changing it replaces the zone.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"network_id": schema.StringAttribute{
				MarkdownDescription: "Network id of the parent view. The same value as vergeio_network.id.",
				Computed:            true,
				PlanModifiers:       stringState(),
			},
			"domain": schema.StringAttribute{
				MarkdownDescription: "Zone domain name, for example example.com.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"type": schema.StringAttribute{
				MarkdownDescription: "Zone type. master, slave, redirect, forward, stub, or `static-stub`. VergeOS uses master when this is omitted on create. Omit on update to leave the current type unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       stringState(),
				Validators: []validator.String{
					stringvalidator.OneOf(dnsZoneTypes...),
				},
			},
			"nameserver": schema.StringAttribute{
				MarkdownDescription: "Primary name server written in the SOA record. Omit to leave the current name unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       stringState(),
			},
			"email": schema.StringAttribute{
				MarkdownDescription: "Admin email written in the SOA record. Omit to leave the current email unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       stringState(),
			},
			"notify": schema.StringAttribute{
				MarkdownDescription: "Notify messages: yes, no, or explicit. Omit to leave the current value unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       stringState(),
				Validators: []validator.String{
					stringvalidator.OneOf(dnsNotifyModes...),
				},
			},
			"allow_notify": schema.StringAttribute{
				MarkdownDescription: "Servers allowed to send notify messages, in the form VergeOS stores. Omit to leave the current list unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       stringState(),
			},
			"also_notify": schema.StringAttribute{
				MarkdownDescription: "Extra servers that receive notify messages, in the form VergeOS stores. Omit to leave the current list unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       stringState(),
			},
			"masters": schema.StringAttribute{
				MarkdownDescription: "Master servers for a slave zone, in the form VergeOS stores. Omit to leave the current list unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       stringState(),
			},
			"allow_transfer": schema.StringAttribute{
				MarkdownDescription: "Servers allowed to transfer this zone, in the form VergeOS stores. Omit to leave the current list unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       stringState(),
			},
			"serial_number": schema.Int64Attribute{
				MarkdownDescription: "SOA serial number. VergeOS increments it. Terraform does not set it.",
				Computed:            true,
				PlanModifiers:       intState(),
			},
			"default_ttl": schema.StringAttribute{
				MarkdownDescription: "Default record time to live, for example 1h or 30m. Omit to leave the current value unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       stringState(),
			},
			"refresh_interval": schema.StringAttribute{
				MarkdownDescription: "SOA refresh interval, for example 1h. Omit to leave the current value unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       stringState(),
			},
			"retry_interval": schema.StringAttribute{
				MarkdownDescription: "SOA retry interval, for example 15m. Omit to leave the current value unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       stringState(),
			},
			"expiry_period": schema.StringAttribute{
				MarkdownDescription: "SOA expiry period, for example 1w. Omit to leave the current value unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       stringState(),
			},
			"negative_ttl": schema.StringAttribute{
				MarkdownDescription: "How long a failed lookup is cached, for example 5m. Omit to leave the current value unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       stringState(),
			},
			"forwarders": schema.StringAttribute{
				MarkdownDescription: "Forwarder addresses, in the form VergeOS stores, for example 8.8.8.8;8.8.4.4;. Omit to leave the current list unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       stringState(),
			},
			"modified": schema.Int64Attribute{
				MarkdownDescription: "Last modification time, as seconds since the epoch.",
				Computed:            true,
				PlanModifiers:       intState(),
			},
			"apply": dnsApplyAttribute(),
		},
	}
}

func (r *NetworkDNSZoneResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	api, diags := configureDNS(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	r.api = api
}

func (r *NetworkDNSZoneResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data dnsZoneModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	notice, err := r.api.createZone(ctx, &data)
	if err != nil {
		resp.Diagnostics.AddError("Error Creating Network DNS Zone", err.Error())
		return
	}
	addDNSNotice(&resp.Diagnostics, notice)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, data.ID)
}

func (r *NetworkDNSZoneResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data dnsZoneModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.readZone(ctx, &data); err != nil {
		if dnsMissing(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error Reading Network DNS Zone", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, data.ID)
}

func (r *NetworkDNSZoneResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state dnsZoneModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	notice, err := r.api.updateZone(ctx, &plan, &state)
	if err != nil {
		resp.Diagnostics.AddError("Error Updating Network DNS Zone", err.Error())
		return
	}
	addDNSNotice(&resp.Diagnostics, notice)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, plan.ID)
}

func (r *NetworkDNSZoneResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data dnsZoneModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	notice, err := r.api.deleteZone(ctx, &data)
	if err != nil {
		resp.Diagnostics.AddError("Error Deleting Network DNS Zone", err.Error())
		return
	}
	addDNSNotice(&resp.Diagnostics, notice)
}

func (r *NetworkDNSZoneResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = shared.KeyIdentitySchema("DNS zone id. Import vergeio_network_dns_zone with this value.")
}

func (r *NetworkDNSZoneResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByPositiveID(ctx, req, resp, "Invalid Network DNS Zone Import ID", "Import vergeio_network_dns_zone with the zone id.")
}
