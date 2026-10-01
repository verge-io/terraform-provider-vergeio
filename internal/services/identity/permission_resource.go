// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package identity

import (
	"context"
	"fmt"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

const permissionMarkdown = "Grant a user or a group rights on a VergeOS table or on one object in that table. " +
	"Rights are the booleans list, read, create, modify, and delete. They are not a bit mask. " +
	"Omit object_id to grant the rights on every row of the table. " +
	"Set one of user_id or group_id. Changing the grantee, table, or object replaces the permission."

var (
	_ resource.Resource                   = &PermissionResource{}
	_ resource.ResourceWithImportState    = &PermissionResource{}
	_ resource.ResourceWithValidateConfig = &PermissionResource{}
)

func NewPermissionResource() resource.Resource {
	return &PermissionResource{}
}

// PermissionResource is vergeio_permission.
type PermissionResource struct {
	api *PermissionApi
}

// PermissionResourceModel is the Terraform model for vergeio_permission.
type PermissionResourceModel struct {
	Id       types.String `tfsdk:"id"`
	UserID   types.String `tfsdk:"user_id"`
	GroupID  types.String `tfsdk:"group_id"`
	Table    types.String `tfsdk:"table"`
	ObjectID types.Int64  `tfsdk:"object_id"`
	List     types.Bool   `tfsdk:"list"`
	Read     types.Bool   `tfsdk:"read"`
	Create   types.Bool   `tfsdk:"create"`
	Modify   types.Bool   `tfsdk:"modify"`
	Delete   types.Bool   `tfsdk:"delete"`
}

func (r *PermissionResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_permission"
}

func (r *PermissionResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: permissionMarkdown,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Permission key assigned by VergeOS.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"user_id": schema.StringAttribute{
				MarkdownDescription: "Key of the vergeio_user that receives the grant. Set user_id or group_id, not both. Changing it replaces the permission.",
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.ExactlyOneOf(
						path.MatchRoot("user_id"),
						path.MatchRoot("group_id"),
					),
					stringvalidator.LengthAtLeast(1),
				},
			},
			"group_id": schema.StringAttribute{
				MarkdownDescription: "Key of the vergeio_group that receives the grant. Set user_id or group_id, not both. Changing it replaces the permission.",
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.ExactlyOneOf(
						path.MatchRoot("user_id"),
						path.MatchRoot("group_id"),
					),
					stringvalidator.LengthAtLeast(1),
				},
			},
			"table": schema.StringAttribute{
				MarkdownDescription: "VergeOS table the grant applies to, such as vms, vnets, volumes, tenants, users, or groups. Changing it replaces the permission.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"object_id": schema.Int64Attribute{
				MarkdownDescription: "Row key of one object in table. Omit to grant the rights on the whole table. VergeOS stores that grant with row 0. Changing it replaces the permission.",
				Optional:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
				},
			},
			"list":   rightAttribute("See the table or object in listings. VergeOS turns list on when read is on. Omit to keep the current value."),
			"read":   rightAttribute("View the table or object. When read is true, list must be true or omitted."),
			"create": rightAttribute("Create rows in the table, or child objects of this row. Omit to keep the current value."),
			"modify": rightAttribute("Update the table or object. Omit to keep the current value."),
			"delete": rightAttribute("Delete the table rows or the object. Omit to keep the current value."),
		},
	}
}

func rightAttribute(description string) schema.BoolAttribute {
	return schema.BoolAttribute{
		MarkdownDescription: description,
		Optional:            true,
		Computed:            true,
		PlanModifiers: []planmodifier.Bool{
			boolplanmodifier.UseStateForUnknown(),
		},
	}
}

func (r *PermissionResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
	api, err := NewPermissionApi(client)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Create VergeOS API Client",
			err.Error(),
		)
		return
	}
	r.api = api
}

func (r *PermissionResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data PermissionResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if data.Read.IsNull() || data.Read.IsUnknown() || !data.Read.ValueBool() {
		return
	}
	if data.List.IsNull() || data.List.IsUnknown() {
		return
	}
	if !data.List.ValueBool() {
		resp.Diagnostics.AddAttributeError(
			path.Root("list"),
			"Invalid permission",
			"VergeOS sets list when read is true. Set list to true, or omit list, when read is true.",
		)
	}
}

func (r *PermissionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data PermissionResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.createPermission(ctx, &data); err != nil {
		if permissionIDSet(&data) {
			r.rememberPermission(ctx, resp, &data)
		}
		resp.Diagnostics.AddError("Error creating permission", err.Error())
		return
	}
	r.rememberPermission(ctx, resp, &data)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.readPermission(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error reading permission", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, permissionForState(&data))...)
}

func (r *PermissionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data PermissionResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.readPermission(ctx, &data); err != nil {
		if vergeos.IsNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading permission", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, permissionForState(&data))...)
}

func (r *PermissionResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state PermissionResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.updatePermission(ctx, &plan, &state); err != nil {
		resp.Diagnostics.AddError("Error updating permission", err.Error())
		return
	}
	plan.Id = state.Id
	if err := r.api.readPermission(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Error reading permission", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, permissionForState(&plan))...)
}

func (r *PermissionResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data PermissionResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.deletePermission(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error deleting permission", err.Error())
		return
	}
	tflog.Debug(ctx, "permission deleted")
}

func (r *PermissionResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// rememberPermission writes the permission id into the create response before
// the follow-up read. Terraform keeps that state when a later step returns
// an error, so the next apply updates the grant instead of creating a second one.
func (r *PermissionResource) rememberPermission(ctx context.Context, resp *resource.CreateResponse, data *PermissionResourceModel) {
	if !permissionIDSet(data) {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, permissionForState(data))...)
	if resp.Diagnostics.HasError() {
		return
	}
	tflog.Debug(ctx, fmt.Sprintf("stored permission %s in state before read", data.Id.ValueString()))
}

func permissionIDSet(data *PermissionResourceModel) bool {
	return data != nil && !data.Id.IsNull() && !data.Id.IsUnknown() && data.Id.ValueString() != ""
}

func permissionForState(data *PermissionResourceModel) PermissionResourceModel {
	stored := *data
	stored.Id = knownString(data.Id)
	stored.UserID = knownString(data.UserID)
	stored.GroupID = knownString(data.GroupID)
	stored.Table = knownString(data.Table)
	stored.ObjectID = knownInt64(data.ObjectID)
	stored.List = knownBool(data.List)
	stored.Read = knownBool(data.Read)
	stored.Create = knownBool(data.Create)
	stored.Modify = knownBool(data.Modify)
	stored.Delete = knownBool(data.Delete)
	return stored
}
