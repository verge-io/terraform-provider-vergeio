// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package network

import (
	"context"
	"fmt"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

var _ resource.Resource = &NetworkRulesResource{}
var _ resource.ResourceWithImportState = &NetworkRulesResource{}

func NewNetworkRulesResource() resource.Resource {
	return &NetworkRulesResource{}
}

// NetworkRulesResource owns every non-system firewall rule on one network.
type NetworkRulesResource struct {
	api *RuleApi
}

type networkRulesModel struct {
	ID   types.String `tfsdk:"id"`
	VNet types.String `tfsdk:"vnet"`
	Rule types.List   `tfsdk:"rule"`
}

func (r *NetworkRulesResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network_rules"
}

func (r *NetworkRulesResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Authoritative firewall rules for one VergeOS network. `rule` is every non-system rule, in order. Create, update, and delete are written together and the network is refreshed once. System rules are never modified or deleted. Do not combine this resource with vergeio_network_rule on the same network; rules missing from the list are deleted. A stopped network is not refreshed, because it loads the staged rules when it starts.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Network id. One rule list per network.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"vnet": schema.StringAttribute{
				MarkdownDescription: "Network id, the same value as vergeio_network.id. Changing it replaces the rule list.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"rule": schema.ListNestedAttribute{
				MarkdownDescription: "Non-system firewall rules in processing order. An empty list deletes every non-system rule.",
				Required:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: firewallRuleAttributes(ruleSchemaList),
				},
			},
		},
	}
}

func (r *NetworkRulesResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
	r.api = NewRuleApi(client)
}

func (r *NetworkRulesResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data networkRulesModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if ok := r.sync(ctx, &data, &resp.Diagnostics); !ok {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *NetworkRulesResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data networkRulesModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	vnetID, err := r.vnetID(data)
	if err != nil {
		resp.Diagnostics.AddError("Error Reading Network Rules", err.Error())
		return
	}
	if _, err := r.api.sdk.Networks.Get(ctx, vnetID); err != nil {
		if vergeos.IsNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error Reading Network Rules", err.Error())
		return
	}
	rules, err := r.api.managedRules(ctx, vnetID)
	if err != nil {
		resp.Diagnostics.AddError("Error Reading Network Rules", err.Error())
		return
	}
	if err := r.store(ctx, &data, vnetID, rules); err != nil {
		resp.Diagnostics.AddError("Error Reading Network Rules", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *NetworkRulesResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data networkRulesModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if ok := r.sync(ctx, &data, &resp.Diagnostics); !ok {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *NetworkRulesResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data networkRulesModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	vnetID, err := r.vnetID(data)
	if err != nil {
		resp.Diagnostics.AddError("Error Deleting Network Rules", err.Error())
		return
	}
	_, notice, err := r.api.syncRuleSet(ctx, vnetID, nil, true)
	if err != nil {
		resp.Diagnostics.AddError("Error Deleting Network Rules", err.Error())
		return
	}
	if notice != nil {
		resp.Diagnostics.AddWarning(notice.Summary, notice.Detail)
	}
	tflog.Debug(ctx, fmt.Sprintf("Deleted non-system firewall rules on network %d", vnetID))
}

func (r *NetworkRulesResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if _, err := parsePositiveID(req.ID); err != nil {
		resp.Diagnostics.AddError(
			"Invalid Network Rules Import ID",
			"Import vergeio_network_rules with the network id.",
		)
		return
	}
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("vnet"), req.ID)...)
}

func (r *NetworkRulesResource) sync(ctx context.Context, data *networkRulesModel, diags *diag.Diagnostics) bool {
	vnetID, err := parsePositiveID(data.VNet.ValueString())
	if err != nil {
		diags.AddError("Invalid Network ID", "vnet must be the network id, a positive integer.")
		return false
	}
	desired, listDiags := rulesFromList(ctx, data.Rule, false)
	diags.Append(listDiags...)
	if diags.HasError() {
		return false
	}
	rules, notice, err := r.api.syncRuleSet(ctx, vnetID, desired, true)
	if err != nil {
		diags.AddError("Error Saving Network Rules", err.Error())
		return false
	}
	if notice != nil {
		diags.AddWarning(notice.Summary, notice.Detail)
	}
	if err := r.store(ctx, data, vnetID, rules); err != nil {
		diags.AddError("Error Saving Network Rules", err.Error())
		return false
	}
	return true
}

func (r *NetworkRulesResource) store(ctx context.Context, data *networkRulesModel, vnetID int, rules []firewallRule) error {
	list, listDiags := rulesToList(ctx, rules)
	if listDiags.HasError() {
		return fmt.Errorf("%s", listDiags.Errors()[0].Detail())
	}
	id := strconvID(vnetID)
	data.ID = id
	data.VNet = id
	data.Rule = list
	return nil
}

func (r *NetworkRulesResource) vnetID(data networkRulesModel) (int, error) {
	raw := stringOrEmpty(data.VNet)
	if raw == "" {
		raw = stringOrEmpty(data.ID)
	}
	return parsePositiveID(raw)
}

func strconvID(id int) types.String {
	return types.StringValue(fmt.Sprintf("%d", id))
}
