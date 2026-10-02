// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package network

import (
	"context"
	"fmt"
	"strings"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &NetworkRuleAliasResource{}
var _ resource.ResourceWithImportState = &NetworkRuleAliasResource{}

func NewNetworkRuleAliasResource() resource.Resource {
	return &NetworkRuleAliasResource{}
}

// NetworkRuleAliasResource is a named address or port group.
type NetworkRuleAliasResource struct {
	api *RuleApi
}

type networkRuleAliasModel struct {
	ID              types.String `tfsdk:"id"`
	AliasID         types.String `tfsdk:"alias_id"`
	Name            types.String `tfsdk:"name"`
	Description     types.String `tfsdk:"description"`
	Value           types.String `tfsdk:"value"`
	PublishingScope types.String `tfsdk:"publishing_scope"`
}

func (r *NetworkRuleAliasResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network_rule_alias"
}

func (r *NetworkRuleAliasResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A named address or port group. Firewall rules reference it as `alias:<id>` in source_ip, destination_ip, source_ports, or destination_ports, where `<id>` is this resource's id (the vnet_rule_aliases key), for example `alias:${vergeio_network_rule_alias.example.id}`. VergeOS rejects `alias:<name>`. Aliases are global on the system. Changing an alias does not refresh a network; the next firewall apply uses the new value.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Alias id, the vnet_rule_aliases key.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"alias_id": schema.StringAttribute{
				MarkdownDescription: "Readonly SHA1 hex id VergeOS assigns to the alias row. Distinct from id (the reusable table key). Used to detect key reuse after an outside delete.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Alias name, unique on the system. Rules reference the alias by id (`alias:${...id}`), not by this name.",
				Required:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Alias description. Omit to leave an existing description unchanged.",
				Optional:            true,
				Computed:            true,
			},
			"value": schema.StringAttribute{
				MarkdownDescription: "Comma-separated addresses or ports. An entry may include a label after a pipe, for example 192.0.2.10|web,192.0.2.11|db.",
				Required:            true,
			},
			"publishing_scope": schema.StringAttribute{
				MarkdownDescription: "Visibility of the alias: private, global, tenant, or none. Omit to leave the current scope unchanged.",
				Optional:            true,
				Computed:            true,
				Validators: []validator.String{
					stringvalidator.OneOf(aliasScopes...),
				},
			},
		},
	}
}

func (r *NetworkRuleAliasResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *NetworkRuleAliasResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data networkRuleAliasModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.createAlias(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error Creating Network Rule Alias", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *NetworkRuleAliasResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data networkRuleAliasModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.readAlias(ctx, &data); err != nil {
		if strings.Contains(err.Error(), "not found") {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error Reading Network Rule Alias", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *NetworkRuleAliasResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state networkRuleAliasModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.updateAlias(ctx, &plan, &state); err != nil {
		resp.Diagnostics.AddError("Error Updating Network Rule Alias", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *NetworkRuleAliasResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data networkRuleAliasModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.deleteAlias(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error Deleting Network Rule Alias", err.Error())
		return
	}
}

func (r *NetworkRuleAliasResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if _, err := parsePositiveID(req.ID); err != nil {
		resp.Diagnostics.AddError(
			"Invalid Network Rule Alias Import ID",
			"Import vergeio_network_rule_alias with the alias id.",
		)
		return
	}
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
