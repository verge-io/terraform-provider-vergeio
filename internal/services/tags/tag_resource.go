// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tags

import (
	"context"
	"fmt"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework-validators/int32validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int32planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

var (
	_ resource.Resource                = &TagResource{}
	_ resource.ResourceWithImportState = &TagResource{}
)

func NewTagResource() resource.Resource {
	return &TagResource{}
}

// TagResource is vergeio_tag.
type TagResource struct {
	tagsApi *TagsApi
}

// TagResourceModel is the Terraform model for vergeio_tag.
type TagResourceModel struct {
	Id           types.String `tfsdk:"id"`
	Category     types.Int32  `tfsdk:"category"`
	Name         types.String `tfsdk:"name"`
	Description  types.String `tfsdk:"description"`
	CategoryName types.String `tfsdk:"category_name"`
}

func (r *TagResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tag"
}

func (r *TagResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "VergeOS tag in a tag category (requires VergeOS v26+). Deleting a tag removes every assignment of that tag. Changing category replaces the tag.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Tag key assigned by VergeOS.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"category": schema.Int32Attribute{
				MarkdownDescription: "Key of the vergeio_tag_category this tag belongs to. Use tonumber(vergeio_tag_category.example.id). Changing category replaces the tag, because VergeOS does not move a tag between categories.",
				Required:            true,
				Validators: []validator.Int32{
					int32validator.AtLeast(1),
				},
				PlanModifiers: []planmodifier.Int32{
					int32planmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Tag name.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Tag description. Omit to leave an existing description unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			// category_name is the category's display name. Renaming the category
			// leaves this tag's category id unchanged, but VergeOS still returns
			// the new name on the next read. Keeping the prior name with
			// UseStateForUnknown makes that apply fail as an inconsistent
			// result. Leave it computed with no plan modifier so an update
			// plans it unknown and the read can store the refreshed name.
			"category_name": schema.StringAttribute{
				MarkdownDescription: "Name of the tag category, read from VergeOS. Refreshed when this tag is applied, including after the category is renamed.",
				Computed:            true,
			},
		},
	}
}

func (r *TagResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
	tagsApi, err := NewTagsApi(client)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Create VergeOS API Client",
			err.Error(),
		)
		return
	}
	r.tagsApi = tagsApi
}

func (r *TagResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data TagResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.tagsApi.createTag(ctx, &data); err != nil {
		if tagIDSet(&data) {
			r.rememberTag(ctx, resp, &data)
		}
		resp.Diagnostics.AddError("Error creating tag", err.Error())
		return
	}
	r.rememberTag(ctx, resp, &data)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.tagsApi.readTag(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error reading tag", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, tagForState(&data))...)
}

func (r *TagResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data TagResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.tagsApi.readTag(ctx, &data); err != nil {
		if vergeos.IsNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading tag", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, tagForState(&data))...)
}

func (r *TagResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state TagResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.tagsApi.updateTag(ctx, &plan, &state); err != nil {
		resp.Diagnostics.AddError("Error updating tag", err.Error())
		return
	}
	plan.Id = state.Id
	if err := r.tagsApi.readTag(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Error reading tag", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, tagForState(&plan))...)
}

func (r *TagResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data TagResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.AddWarning(
		"Deleting a tag removes its assignments",
		"VergeOS removes every assignment of this tag. It does not ask for confirmation.",
	)
	if err := r.tagsApi.deleteTag(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error deleting tag", err.Error())
		return
	}
	tflog.Debug(ctx, "tag deleted")
}

func (r *TagResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError(
			"Invalid Tag Import ID",
			"Import vergeio_tag with the tag key.",
		)
		return
	}
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *TagResource) rememberTag(ctx context.Context, resp *resource.CreateResponse, data *TagResourceModel) {
	if !tagIDSet(data) {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, tagForState(data))...)
	if resp.Diagnostics.HasError() {
		return
	}
	tflog.Debug(ctx, fmt.Sprintf("stored tag %s in state before read", data.Id.ValueString()))
}

func tagIDSet(data *TagResourceModel) bool {
	return data != nil && !data.Id.IsNull() && !data.Id.IsUnknown() && data.Id.ValueString() != ""
}

func tagForState(data *TagResourceModel) TagResourceModel {
	stored := *data
	stored.Id = knownString(data.Id)
	stored.Category = knownInt32(data.Category)
	stored.Name = knownString(data.Name)
	stored.Description = knownString(data.Description)
	stored.CategoryName = knownString(data.CategoryName)
	return stored
}
