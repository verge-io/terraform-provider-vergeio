// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tenant

import (
	"context"
	"fmt"
	"net"
	"strings"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/verge-io/govergeos"
)

var (
	_ resource.Resource                = &TenantExternalIPResource{}
	_ resource.ResourceWithImportState = &TenantExternalIPResource{}
)

func NewTenantExternalIPResource() resource.Resource {
	return &TenantExternalIPResource{}
}

// TenantExternalIPResource is vergeio_tenant_external_ip.
type TenantExternalIPResource struct {
	api *API
}

// TenantExternalIPResourceModel is the Terraform model for vergeio_tenant_external_ip.
type TenantExternalIPResourceModel struct {
	Id                    types.String `tfsdk:"id"`
	TenantID              types.String `tfsdk:"tenant_id"`
	NetworkID             types.String `tfsdk:"network_id"`
	IP                    types.String `tfsdk:"ip"`
	Hostname              types.String `tfsdk:"hostname"`
	Description           types.String `tfsdk:"description"`
	ApplyParentFirewall   types.Bool   `tfsdk:"apply_parent_firewall"`
	ParentFirewallPending types.Bool   `tfsdk:"parent_firewall_pending"`
	ParentFirewallApplied types.Bool   `tfsdk:"parent_firewall_applied"`
}

func (r *TenantExternalIPResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tenant_external_ip"
}

func (r *TenantExternalIPResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "One virtual IP on a parent network, owned by a VergeOS tenant. VergeOS uses the first assigned IP as the tenant UI address. Changing tenant_id, network_id, ip, hostname, or description replaces the address. apply_parent_firewall passes WithApplyParentFirewall on create, update, and delete. parent_firewall_pending is the parent network need_fw_apply flag.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "vnet_addresses key assigned by VergeOS.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"tenant_id": schema.StringAttribute{
				MarkdownDescription: "Key of the parent vergeio_tenant. Changing it replaces the address.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"network_id": schema.StringAttribute{
				MarkdownDescription: "Key of the parent network the address is taken from, the same value as vergeio_network.id. Changing it replaces the address.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"ip": schema.StringAttribute{
				MarkdownDescription: "IP address to assign. Changing it replaces the address.",
				Required:            true,
				Validators: []validator.String{
					ipAddressValidator{},
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"hostname": schema.StringAttribute{
				MarkdownDescription: "Optional hostname stored on the address. Changing it replaces the address.",
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Optional description. Changing it replaces the address.",
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"apply_parent_firewall": schema.BoolAttribute{
				MarkdownDescription: "Apply the parent network's firewall rules after this address is created, updated, or deleted. Defaults to false. When false, VergeOS can leave need_fw_apply set. parent_firewall_pending reports that flag.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"parent_firewall_pending": schema.BoolAttribute{
				MarkdownDescription: "True when the parent network still has need_fw_apply set.",
				Computed:            true,
			},
			"parent_firewall_applied": schema.BoolAttribute{
				MarkdownDescription: "True when the last create or update applied the parent network's firewall rules.",
				Computed:            true,
			},
		},
	}
}

func (r *TenantExternalIPResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*vergeio.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *vergeio.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}
	api, err := NewAPI(client)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Create VergeOS API Client",
			err.Error(),
		)
		return
	}
	r.api = api
}

func (r *TenantExternalIPResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data TenantExternalIPResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.api.createTenantExternalIP(ctx, &data)
	if data.Id.ValueString() == "" {
		if err == nil {
			err = fmt.Errorf("VergeOS did not return an id for the tenant external IP")
		}
		resp.Diagnostics.AddError("Error creating tenant external IP", err.Error())
		return
	}
	if err != nil {
		resp.Diagnostics.AddWarning("Parent firewall follow-up failed", parentFirewallFollowUpDetail("created", err))
	} else if summary, detail := parentFirewallPendingWarning(applyParentFirewall(data.ApplyParentFirewall), data); summary != "" {
		resp.Diagnostics.AddWarning(summary, detail)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *TenantExternalIPResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data TenantExternalIPResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.readTenantExternalIP(ctx, &data); err != nil {
		if vergeos.IsNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading tenant external IP", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *TenantExternalIPResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan TenantExternalIPResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.updateTenantExternalIP(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Error updating tenant external IP", err.Error())
		return
	}
	if err := r.api.readTenantExternalIP(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Error reading tenant external IP", err.Error())
		return
	}
	if summary, detail := parentFirewallPendingWarning(applyParentFirewall(plan.ApplyParentFirewall), plan); summary != "" {
		resp.Diagnostics.AddWarning(summary, detail)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *TenantExternalIPResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data TenantExternalIPResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	status, err := r.api.deleteTenantExternalIP(ctx, &data)
	if err != nil && status == nil {
		resp.Diagnostics.AddError("Error deleting tenant external IP", err.Error())
		return
	}
	if err != nil {
		resp.Diagnostics.AddWarning("Parent firewall follow-up failed", parentFirewallFollowUpDetail("deleted", err))
		return
	}
	if status != nil && status.Pending && !applyParentFirewall(data.ApplyParentFirewall) {
		resp.Diagnostics.AddWarning(
			"Parent firewall rules were not applied",
			fmt.Sprintf("Parent network %d still has need_fw_apply set after the external IP was deleted. apply_parent_firewall is false, so Terraform did not apply the rules. VergeOS may act on a stale pending flag on its own.", status.NetworkID),
		)
	}
}

func (r *TenantExternalIPResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError(
			"Invalid Tenant External IP Import ID",
			"Import vergeio_tenant_external_ip with the vnet address key.",
		)
		return
	}
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

type ipAddressValidator struct{}

func (v ipAddressValidator) Description(context.Context) string {
	return "value must be an IP address"
}

func (v ipAddressValidator) MarkdownDescription(context.Context) string {
	return "value must be an IP address"
}

func (v ipAddressValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if net.ParseIP(strings.TrimSpace(req.ConfigValue.ValueString())) == nil {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid IP address",
			"ip must be a valid IP address.",
		)
	}
}

func parentFirewallFollowUpDetail(action string, err error) string {
	return fmt.Sprintf("The external IP was %s, but the parent network firewall follow-up failed: %s. parent_firewall_pending reports whether need_fw_apply is still set.", action, err.Error())
}

func parentFirewallPendingWarning(apply bool, data TenantExternalIPResourceModel) (string, string) {
	if data.ParentFirewallPending.IsNull() || data.ParentFirewallPending.IsUnknown() || !data.ParentFirewallPending.ValueBool() {
		return "", ""
	}
	networkID := data.NetworkID.ValueString()
	if apply {
		return "Parent firewall rules are still pending", fmt.Sprintf(
			"Terraform applied the firewall rules on parent network %s, and need_fw_apply is still set. parent_firewall_pending stays true.",
			networkID,
		)
	}
	return "Parent firewall rules were not applied", fmt.Sprintf(
		"Parent network %s still has need_fw_apply set. apply_parent_firewall is false, so Terraform did not apply the rules. parent_firewall_pending stays true until the rules are applied. VergeOS may act on a stale pending flag on its own.",
		networkID,
	)
}
