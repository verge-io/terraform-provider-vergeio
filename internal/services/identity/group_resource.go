// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package identity

import (
	"context"
	"fmt"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

var (
	_ resource.Resource                = &GroupResource{}
	_ resource.ResourceWithImportState = &GroupResource{}
)

func NewGroupResource() resource.Resource {
	return &GroupResource{}
}

// GroupResource is vergeio_group.
type GroupResource struct {
	api *GroupApi
}

// GroupResourceModel is the Terraform model for vergeio_group.
type GroupResourceModel struct {
	Id          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Enabled     types.Bool   `tfsdk:"enabled"`
}

func (r *GroupResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group"
}

func (r *GroupResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "VergeOS group. A group collects users and other objects through vergeio_member and can be granted rights with vergeio_permission.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Group key assigned by VergeOS.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Group name. Must be unique.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Group description. Omit to leave an existing description unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the group is enabled. Omit to leave the current value unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *GroupResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
	r.api = NewGroupApi(client)
}

func (r *GroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data GroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.createGroup(ctx, &data); err != nil {
		if groupIDSet(&data) {
			r.rememberGroup(ctx, resp, &data)
		}
		resp.Diagnostics.AddError("Error creating group", err.Error())
		return
	}
	// The group row exists. Keep its id before the follow-up read so a later
	// error still leaves the group in state.
	r.rememberGroup(ctx, resp, &data)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.readGroup(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error reading group", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, groupForState(&data))...)
}

func (r *GroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data GroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.readGroup(ctx, &data); err != nil {
		if vergeos.IsNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading group", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, groupForState(&data))...)
}

func (r *GroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state GroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.updateGroup(ctx, &plan, &state); err != nil {
		resp.Diagnostics.AddError("Error updating group", err.Error())
		return
	}
	plan.Id = state.Id
	if err := r.api.readGroup(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Error reading group", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, groupForState(&plan))...)
}

func (r *GroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data GroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.deleteGroup(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error deleting group", err.Error())
		return
	}
	tflog.Debug(ctx, "group deleted")
}

func (r *GroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// rememberGroup writes the group id into the create response before the
// follow-up read. Terraform keeps that state when a later step returns an
// error, so the next apply updates the group instead of creating another
// one with the same name.
func (r *GroupResource) rememberGroup(ctx context.Context, resp *resource.CreateResponse, data *GroupResourceModel) {
	if !groupIDSet(data) {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, groupForState(data))...)
	if resp.Diagnostics.HasError() {
		return
	}
	tflog.Debug(ctx, fmt.Sprintf("stored group %s in state before read", data.Id.ValueString()))
}

func groupIDSet(data *GroupResourceModel) bool {
	return data != nil && !data.Id.IsNull() && !data.Id.IsUnknown() && data.Id.ValueString() != ""
}

func groupForState(data *GroupResourceModel) GroupResourceModel {
	stored := *data
	stored.Id = knownString(data.Id)
	stored.Name = knownString(data.Name)
	stored.Description = knownString(data.Description)
	stored.Enabled = knownBool(data.Enabled)
	return stored
}
