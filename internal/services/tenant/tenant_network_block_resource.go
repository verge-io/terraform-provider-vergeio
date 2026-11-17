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
	_ resource.Resource                = &TenantNetworkBlockResource{}
	_ resource.ResourceWithImportState = &TenantNetworkBlockResource{}
)

func NewTenantNetworkBlockResource() resource.Resource {
	return &TenantNetworkBlockResource{}
}

// TenantNetworkBlockResource is vergeio_tenant_network_block.
type TenantNetworkBlockResource struct {
	api *API
}

// TenantNetworkBlockResourceModel is the Terraform model for vergeio_tenant_network_block.
type TenantNetworkBlockResourceModel struct {
	Id                    types.String `tfsdk:"id"`
	TenantID              types.String `tfsdk:"tenant_id"`
	NetworkID             types.String `tfsdk:"network_id"`
	CIDR                  types.String `tfsdk:"cidr"`
	Description           types.String `tfsdk:"description"`
	ApplyParentFirewall   types.Bool   `tfsdk:"apply_parent_firewall"`
	ParentFirewallPending types.Bool   `tfsdk:"parent_firewall_pending"`
	ParentFirewallApplied types.Bool   `tfsdk:"parent_firewall_applied"`
}

func (r *TenantNetworkBlockResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tenant_network_block"
}

func (r *TenantNetworkBlockResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "One routed CIDR on a parent network, owned by a VergeOS tenant. The tenant can build a network on this range. Changing tenant_id, network_id, cidr, or description replaces the block. apply_parent_firewall passes WithApplyParentFirewall on create, update, and delete. parent_firewall_pending is the parent network need_fw_apply flag. VergeOS refuses a delete while a tenant network still uses the block, and that error is returned.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "vnet_cidrs key assigned by VergeOS.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"tenant_id": schema.StringAttribute{
				MarkdownDescription: "Key of the parent vergeio_tenant. Changing it replaces the block.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"network_id": schema.StringAttribute{
				MarkdownDescription: "Key of the parent network the block is routed from, the same value as vergeio_network.id. Changing it replaces the block.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"cidr": schema.StringAttribute{
				MarkdownDescription: "Routed block in CIDR notation, such as 192.168.100.0/24. The address must be the network address. Changing it replaces the block.",
				Required:            true,
				Validators: []validator.String{
					cidrValidator{},
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Optional description. Changing it replaces the block.",
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"apply_parent_firewall": schema.BoolAttribute{
				MarkdownDescription: "Apply the parent network's firewall rules after this block is created, updated, or deleted. Defaults to false. When false, VergeOS can leave need_fw_apply set. When true, Terraform waits until that flag clears. parent_firewall_pending reports the flag.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"parent_firewall_pending": schema.BoolAttribute{
				MarkdownDescription: "True when the parent network still has need_fw_apply set. After a successful apply, Terraform reads the flag until it clears.",
				Computed:            true,
			},
			"parent_firewall_applied": schema.BoolAttribute{
				MarkdownDescription: "True when the last create or update applied the parent network's firewall rules.",
				Computed:            true,
			},
		},
	}
}

func (r *TenantNetworkBlockResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *TenantNetworkBlockResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data TenantNetworkBlockResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.api.createTenantNetworkBlock(ctx, &data)
	if data.Id.ValueString() == "" {
		if err == nil {
			err = fmt.Errorf("VergeOS did not return an id for the tenant network block")
		}
		resp.Diagnostics.AddError("Error creating tenant network block", err.Error())
		return
	}
	if err != nil {
		resp.Diagnostics.AddWarning("Parent firewall follow-up failed", networkBlockFollowUpDetail("created", err))
	} else if summary, detail := networkBlockFirewallPendingWarning(applyParentFirewall(data.ApplyParentFirewall), data); summary != "" {
		resp.Diagnostics.AddWarning(summary, detail)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *TenantNetworkBlockResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data TenantNetworkBlockResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.readTenantNetworkBlock(ctx, &data); err != nil {
		if vergeos.IsNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading tenant network block", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *TenantNetworkBlockResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan TenantNetworkBlockResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.updateTenantNetworkBlock(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Error updating tenant network block", err.Error())
		return
	}
	if err := r.api.readTenantNetworkBlock(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Error reading tenant network block", err.Error())
		return
	}
	if summary, detail := networkBlockFirewallPendingWarning(applyParentFirewall(plan.ApplyParentFirewall), plan); summary != "" {
		resp.Diagnostics.AddWarning(summary, detail)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *TenantNetworkBlockResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data TenantNetworkBlockResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	status, err := r.api.deleteTenantNetworkBlock(ctx, &data)
	// A block that still backs a tenant network fails in VergeOS before the
	// row is removed. Return that error so the block stays in state.
	if err != nil && status == nil {
		resp.Diagnostics.AddError("Error deleting tenant network block", err.Error())
		return
	}
	if err != nil {
		resp.Diagnostics.AddWarning("Parent firewall follow-up failed", networkBlockFollowUpDetail("deleted", err))
		return
	}
	if status != nil && status.Pending && !applyParentFirewall(data.ApplyParentFirewall) {
		resp.Diagnostics.AddWarning(
			"Parent firewall rules were not applied",
			fmt.Sprintf("Parent network %d still has need_fw_apply set after the network block was deleted. apply_parent_firewall is false, so Terraform did not apply the rules. VergeOS may act on a stale pending flag on its own.", status.NetworkID),
		)
	}
}

func (r *TenantNetworkBlockResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError(
			"Invalid Tenant Network Block Import ID",
			"Import vergeio_tenant_network_block with the vnet cidr key.",
		)
		return
	}
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

type cidrValidator struct{}

func (v cidrValidator) Description(context.Context) string {
	return "value must be a network address in CIDR notation"
}

func (v cidrValidator) MarkdownDescription(context.Context) string {
	return "value must be a network address in CIDR notation"
}

func (v cidrValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if err := validateNetworkCIDR(req.ConfigValue.ValueString()); err != nil {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid CIDR block",
			"cidr must be a network address in CIDR notation, such as 192.168.100.0/24.",
		)
	}
}

func networkBlockFollowUpDetail(action string, err error) string {
	return fmt.Sprintf("The network block was %s, but the parent network firewall follow-up failed: %s. parent_firewall_pending reports whether need_fw_apply is still set.", action, err.Error())
}

func networkBlockFirewallPendingWarning(apply bool, data TenantNetworkBlockResourceModel) (string, string) {
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

// validateNetworkCIDR accepts a CIDR whose address is the network address
// and whose text matches the canonical form net.ParseCIDR produces.
func validateNetworkCIDR(cidr string) error {
	cidr = strings.TrimSpace(cidr)
	ip, network, err := net.ParseCIDR(cidr)
	if err != nil || network == nil || !ip.Equal(network.IP) || network.String() != cidr {
		return fmt.Errorf("cidr must be a network address in CIDR notation, such as 192.168.100.0/24")
	}
	return nil
}
