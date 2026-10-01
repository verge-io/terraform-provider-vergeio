// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package network

import (
	"context"
	"fmt"
	"strconv"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/verge-io/govergeos"
)

var _ resource.Resource = &NetworkRuleResource{}
var _ resource.ResourceWithImportState = &NetworkRuleResource{}

func NewNetworkRuleResource() resource.Resource {
	return &NetworkRuleResource{}
}

// NetworkRuleResource is one firewall rule on a network.
type NetworkRuleResource struct {
	api *RuleApi
}

func (r *NetworkRuleResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network_rule"
}

func (r *NetworkRuleResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	attributes := firewallRuleAttributes(ruleSchemaSingle)
	attributes["vnet"] = schema.StringAttribute{
		MarkdownDescription: "Network id, the same value as vergeio_network.id. Changing it replaces the rule.",
		Required:            true,
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.RequiresReplace(),
		},
	}
	attributes["apply"] = schema.BoolAttribute{
		MarkdownDescription: "Refresh the network's firewall rules after this rule changes. Defaults to true. Set false to stage several vergeio_network_rule resources, then leave it true on the last one, which depends_on the others. Each refresh reloads the whole network, so prefer vergeio_network_rules when one configuration owns the policy. Do not use both resources on the same network. A stopped network is not refreshed; it loads staged rules when it starts.",
		Optional:            true,
		Computed:            true,
		Default:             booldefault.StaticBool(true),
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "One firewall rule on a VergeOS network. The rule is matched by name. apply defaults to true and refreshes the network once after a change. System rules cannot be changed or deleted. Do not use this resource on a network that also has vergeio_network_rules.",
		Attributes:          attributes,
	}
}

func (r *NetworkRuleResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
	api, err := NewRuleApi(client)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Create VergeOS API Client",
			err.Error(),
		)
		return
	}
	r.api = api
}

func (r *NetworkRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, config firewallRuleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	data, ok := r.save(ctx, plan, config, 0, &resp.Diagnostics)
	if !ok {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *NetworkRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data firewallRuleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	key := ruleKey(data.ID)
	if key <= 0 {
		resp.Diagnostics.AddError("Error Reading Network Rule", "rule id is missing")
		return
	}
	rule, err := r.api.sdk.VNetRules.Get(ctx, key)
	if err != nil {
		if vergeos.IsNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error Reading Network Rule", err.Error())
		return
	}
	if rule.SystemRule {
		resp.Diagnostics.AddError(
			"Error Reading Network Rule",
			(&systemRuleError{Name: rule.Name}).Error(),
		)
		return
	}
	data = fillRuleModel(ruleFromAPI(*rule), data.VNet, data.Apply)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *NetworkRuleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, config, state firewallRuleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	data, ok := r.save(ctx, plan, config, ruleKey(state.ID), &resp.Diagnostics)
	if !ok {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *NetworkRuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data firewallRuleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	vnetID, err := parsePositiveID(stringOrEmpty(data.VNet))
	if err != nil {
		resp.Diagnostics.AddError("Error Deleting Network Rule", "vnet must be the network id, a positive integer.")
		return
	}
	notice, err := r.api.deleteRule(ctx, vnetID, ruleKey(data.ID), applyEnabled(data.Apply))
	if err != nil {
		resp.Diagnostics.AddError("Error Deleting Network Rule", err.Error())
		return
	}
	if notice != nil {
		resp.Diagnostics.AddWarning(notice.Summary, notice.Detail)
	}
}

func (r *NetworkRuleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if _, err := parsePositiveID(req.ID); err != nil {
		resp.Diagnostics.AddError(
			"Invalid Network Rule Import ID",
			"Import vergeio_network_rule with the rule id.",
		)
		return
	}
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *NetworkRuleResource) save(ctx context.Context, plan, config firewallRuleModel, existingKey int, diags *diag.Diagnostics) (firewallRuleModel, bool) {
	vnetID, err := parsePositiveID(stringOrEmpty(plan.VNet))
	if err != nil {
		diags.AddError("Invalid Network ID", "vnet must be the network id, a positive integer.")
		return firewallRuleModel{}, false
	}
	// Config is the mask. Omitted optional attributes are null there even when
	// the plan preserved a previous value.
	saved, notice, err := r.api.syncRule(ctx, vnetID, existingKey, config.rule(true), applyEnabled(plan.Apply))
	if err != nil {
		diags.AddError("Error Saving Network Rule", err.Error())
		return firewallRuleModel{}, false
	}
	if notice != nil {
		diags.AddWarning(notice.Summary, notice.Detail)
	}
	return fillRuleModel(saved, plan.VNet, plan.Apply), true
}

func fillRuleModel(rule firewallRule, vnet types.String, apply types.Bool) firewallRuleModel {
	model := firewallRuleModel{nestedFirewallRuleModel: rule.model()}
	model.VNet = vnet
	if model.VNet.IsNull() || model.VNet.IsUnknown() || model.VNet.ValueString() == "" {
		if rule.VNet > 0 {
			model.VNet = types.StringValue(strconv.Itoa(rule.VNet))
		}
	}
	model.Apply = apply
	if model.Apply.IsNull() || model.Apply.IsUnknown() {
		model.Apply = types.BoolValue(true)
	}
	return model
}
