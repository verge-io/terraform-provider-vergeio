// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package network

import (
	"context"
	"strings"

	"terraform-provider-vergeio/internal/shared"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/verge-io/govergeos"
)

const dnsRecordDescription = "One DNS record in a vergeio_network_dns_zone. Set zone_id to that zone id so Terraform destroys this record before the zone. value is the record data. For an A or AAAA record, set value to a vergeio_vm_nic ipaddress so the VM and the record live in one module, or set vm_nic_id to that NIC id and Terraform reads the address. apply defaults to true and refreshes DNS on the zone network after a change. A stopped network is not refreshed. It loads staged DNS when it starts."

var (
	_ resource.Resource                   = &NetworkDNSRecordResource{}
	_ resource.ResourceWithConfigure      = &NetworkDNSRecordResource{}
	_ resource.ResourceWithImportState    = &NetworkDNSRecordResource{}
	_ resource.ResourceWithIdentity       = &NetworkDNSRecordResource{}
	_ resource.ResourceWithModifyPlan     = &NetworkDNSRecordResource{}
	_ resource.ResourceWithValidateConfig = &NetworkDNSRecordResource{}

	dnsRecordTypes = []string{
		vergeos.DNSRecordTypeA,
		vergeos.DNSRecordTypeAAAA,
		vergeos.DNSRecordTypeCNAME,
		vergeos.DNSRecordTypeMX,
		vergeos.DNSRecordTypeNS,
		vergeos.DNSRecordTypePTR,
		vergeos.DNSRecordTypeSRV,
		vergeos.DNSRecordTypeTXT,
		vergeos.DNSRecordTypeCAA,
	}
)

func NewNetworkDNSRecordResource() resource.Resource {
	return &NetworkDNSRecordResource{}
}

// NetworkDNSRecordResource is one DNS record in a zone.
type NetworkDNSRecordResource struct {
	api *DNSApi
}

type dnsRecordModel struct {
	ID            types.String `tfsdk:"id"`
	ZoneID        types.String `tfsdk:"zone_id"`
	NetworkID     types.String `tfsdk:"network_id"`
	Host          types.String `tfsdk:"host"`
	TTL           types.String `tfsdk:"ttl"`
	Type          types.String `tfsdk:"type"`
	Value         types.String `tfsdk:"value"`
	VMNICID       types.String `tfsdk:"vm_nic_id"`
	Description   types.String `tfsdk:"description"`
	MXPreference  types.Int64  `tfsdk:"mx_preference"`
	Weight        types.Int64  `tfsdk:"weight"`
	Port          types.Int64  `tfsdk:"port"`
	IssueWildcard types.Bool   `tfsdk:"issue_wildcard"`
	OrderID       types.Int64  `tfsdk:"orderid"`
	Modified      types.Int64  `tfsdk:"modified"`
	Apply         types.Bool   `tfsdk:"apply"`
}

func (r *NetworkDNSRecordResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network_dns_record"
}

func (r *NetworkDNSRecordResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: dnsRecordDescription,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "DNS record id.",
				Computed:            true,
				PlanModifiers:       stringState(),
			},
			"zone_id": schema.StringAttribute{
				MarkdownDescription: "DNS zone id, the same value as vergeio_network_dns_zone.id. Changing it replaces the record.",
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
			"host": schema.StringAttribute{
				MarkdownDescription: "Record name relative to the zone. An empty name is the zone itself. Omit to leave the current name unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       stringState(),
			},
			"ttl": schema.StringAttribute{
				MarkdownDescription: "Record time to live, for example 1h or 30m. Omit to leave the current value unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       stringState(),
			},
			"type": schema.StringAttribute{
				MarkdownDescription: "Record type: A, AAAA, CNAME, MX, NS, PTR, SRV, TXT, or CAA.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.OneOf(dnsRecordTypes...),
				},
			},
			"value": schema.StringAttribute{
				MarkdownDescription: "Record data. An A record is an IPv4 address and an AAAA record is an IPv6 address. Set this to vergeio_vm_nic.example.ipaddress so a VM and its A record live in one module. Leave it unset when vm_nic_id is set.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       stringState(),
			},
			"vm_nic_id": schema.StringAttribute{
				MarkdownDescription: "NIC id, the same value as vergeio_vm_nic.id. Terraform reads that NIC address and writes it as value. Only an A or AAAA record can set this. Do not set value in the same resource. A later plan updates this record when the NIC address changes.",
				Optional:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Record description. Omit to leave the current description unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       stringState(),
			},
			"mx_preference": schema.Int64Attribute{
				MarkdownDescription: "MX preference from 0 through 65535. Omit to leave the current preference unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       intState(),
				Validators: []validator.Int64{
					int64validator.Between(0, 65535),
				},
			},
			"weight": schema.Int64Attribute{
				MarkdownDescription: "SRV weight from 0 through 65535. Omit to leave the current weight unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       intState(),
				Validators: []validator.Int64{
					int64validator.Between(0, 65535),
				},
			},
			"port": schema.Int64Attribute{
				MarkdownDescription: "SRV port from 0 through 65535. Omit to leave the current port unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       intState(),
				Validators: []validator.Int64{
					int64validator.Between(0, 65535),
				},
			},
			"issue_wildcard": schema.BoolAttribute{
				MarkdownDescription: "CAA flag. True limits issuance to wildcard certificates. Omit to leave the current flag unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       boolState(),
			},
			"orderid": schema.Int64Attribute{
				MarkdownDescription: "Display order. Omit to leave the current order unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers:       intState(),
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

func (r *NetworkDNSRecordResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	api, diags := configureDNS(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	r.api = api
}

func (r *NetworkDNSRecordResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data dnsRecordModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	valueKnown := !data.Value.IsUnknown()
	nicKnown := !data.VMNICID.IsUnknown()
	valueSet := valueKnown && !data.Value.IsNull() && strings.TrimSpace(data.Value.ValueString()) != ""
	nicSet := nicKnown && !data.VMNICID.IsNull() && strings.TrimSpace(data.VMNICID.ValueString()) != ""
	if valueKnown && nicKnown {
		if valueSet && nicSet {
			resp.Diagnostics.AddAttributeError(path.Root("vm_nic_id"), "Conflicting DNS Record Value", "Set value or vm_nic_id, not both.")
		}
		if !valueSet && !nicSet {
			resp.Diagnostics.AddAttributeError(path.Root("value"), "Missing DNS Record Value", "Set value, or set vm_nic_id to a vergeio_vm_nic id.")
		}
	}
	if nicSet && !data.Type.IsUnknown() && !data.Type.IsNull() {
		recType := data.Type.ValueString()
		if recType != vergeos.DNSRecordTypeA && recType != vergeos.DNSRecordTypeAAAA {
			resp.Diagnostics.AddAttributeError(path.Root("vm_nic_id"), "Invalid DNS Record Type", "vm_nic_id is only valid for an A or AAAA record.")
		}
	}
}

func (r *NetworkDNSRecordResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if r.api == nil || req.Plan.Raw.IsNull() {
		return
	}
	var plan dnsRecordModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	nicID, ok, err := optionalPositiveID(plan.VMNICID)
	if err != nil || !ok {
		return
	}
	ip, err := r.api.nicAddress(ctx, nicID)
	if err != nil {
		if dnsMissing(err) || strings.Contains(err.Error(), "no IP address") {
			resp.Diagnostics.AddAttributeError(path.Root("vm_nic_id"), "NIC Address Unavailable", err.Error())
		}
		return
	}
	if plan.Value.IsUnknown() || plan.Value.IsNull() || strings.TrimSpace(plan.Value.ValueString()) != ip {
		plan.Value = types.StringValue(ip)
		resp.Diagnostics.Append(resp.Plan.Set(ctx, &plan)...)
	}
}

func (r *NetworkDNSRecordResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data dnsRecordModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	notice, err := r.api.createRecord(ctx, &data)
	if err != nil {
		resp.Diagnostics.AddError("Error Creating Network DNS Record", err.Error())
		return
	}
	addDNSNotice(&resp.Diagnostics, notice)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, data.ID)
}

func (r *NetworkDNSRecordResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data dnsRecordModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.readRecord(ctx, &data); err != nil {
		if dnsMissing(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error Reading Network DNS Record", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, data.ID)
}

func (r *NetworkDNSRecordResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state dnsRecordModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	notice, err := r.api.updateRecord(ctx, &plan, &state)
	if err != nil {
		resp.Diagnostics.AddError("Error Updating Network DNS Record", err.Error())
		return
	}
	addDNSNotice(&resp.Diagnostics, notice)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, plan.ID)
}

func (r *NetworkDNSRecordResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data dnsRecordModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	notice, err := r.api.deleteRecord(ctx, &data)
	if err != nil {
		resp.Diagnostics.AddError("Error Deleting Network DNS Record", err.Error())
		return
	}
	addDNSNotice(&resp.Diagnostics, notice)
}

func (r *NetworkDNSRecordResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = shared.KeyIdentitySchema("DNS record id. Import vergeio_network_dns_record with this value.")
}

func (r *NetworkDNSRecordResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importByPositiveID(ctx, req, resp, "Invalid Network DNS Record Import ID", "Import vergeio_network_dns_record with the record id.")
}
