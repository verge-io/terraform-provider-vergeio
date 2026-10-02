// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tags

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
	_ resource.Resource                = &TagCategoryResource{}
	_ resource.ResourceWithImportState = &TagCategoryResource{}
)

func NewTagCategoryResource() resource.Resource {
	return &TagCategoryResource{}
}

// TagCategoryResource is vergeio_tag_category.
type TagCategoryResource struct {
	tagsApi *TagsApi
}

// TagCategoryResourceModel is the Terraform model for vergeio_tag_category.
// Bool fields stay null when the configuration omits them. They are not
// given a false default: sending false turns tagging off for that object type.
type TagCategoryResourceModel struct {
	Id                       types.String `tfsdk:"id"`
	Name                     types.String `tfsdk:"name"`
	Description              types.String `tfsdk:"description"`
	SingleTagSelection       types.Bool   `tfsdk:"single_tag_selection"`
	TaggableVolumes          types.Bool   `tfsdk:"taggable_volumes"`
	TaggableVNets            types.Bool   `tfsdk:"taggable_vnets"`
	TaggableVNetRules        types.Bool   `tfsdk:"taggable_vnet_rules"`
	TaggableVMwareContainers types.Bool   `tfsdk:"taggable_vmware_containers"`
	TaggableVMs              types.Bool   `tfsdk:"taggable_vms"`
	TaggableUsers            types.Bool   `tfsdk:"taggable_users"`
	TaggableTenantNodes      types.Bool   `tfsdk:"taggable_tenant_nodes"`
	TaggableSites            types.Bool   `tfsdk:"taggable_sites"`
	TaggableNodes            types.Bool   `tfsdk:"taggable_nodes"`
	TaggableGroups           types.Bool   `tfsdk:"taggable_groups"`
	TaggableClusters         types.Bool   `tfsdk:"taggable_clusters"`
	TaggableTenants          types.Bool   `tfsdk:"taggable_tenants"`
}

func (r *TagCategoryResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tag_category"
}

func (r *TagCategoryResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "VergeOS tag category (requires VergeOS v26+). Deleting the category deletes every tag in it and every assignment of those tags, with no confirmation. Omit a taggable_* flag to leave that object type unchanged; an omitted flag is not sent as false.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Tag category key assigned by VergeOS.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Category name. Must be unique.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Category description. Omit to leave an existing description unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"single_tag_selection":       omitEmptyBool("When true, an object can have only one tag from this category. Omit to leave the current value unchanged. An omitted value is not sent."),
			"taggable_volumes":           omitEmptyBool("Whether volumes can use tags from this category. Omit to leave the current value unchanged. An omitted flag is not sent."),
			"taggable_vnets":             omitEmptyBool("Whether networks can use tags from this category. Omit to leave the current value unchanged. An omitted flag is not sent."),
			"taggable_vnet_rules":        omitEmptyBool("Whether network rules can use tags from this category. Omit to leave the current value unchanged. An omitted flag is not sent."),
			"taggable_vmware_containers": omitEmptyBool("Whether VMware containers can use tags from this category. Omit to leave the current value unchanged. An omitted flag is not sent."),
			"taggable_vms":               omitEmptyBool("Whether VMs can use tags from this category. Omit to leave the current value unchanged. An omitted flag is not sent."),
			"taggable_users":             omitEmptyBool("Whether users can use tags from this category. Omit to leave the current value unchanged. An omitted flag is not sent."),
			"taggable_tenant_nodes":      omitEmptyBool("Whether tenant nodes can use tags from this category. Omit to leave the current value unchanged. An omitted flag is not sent."),
			"taggable_sites":             omitEmptyBool("Whether sites can use tags from this category. Omit to leave the current value unchanged. An omitted flag is not sent."),
			"taggable_nodes":             omitEmptyBool("Whether nodes can use tags from this category. Omit to leave the current value unchanged. An omitted flag is not sent."),
			"taggable_groups":            omitEmptyBool("Whether groups can use tags from this category. Omit to leave the current value unchanged. An omitted flag is not sent."),
			"taggable_clusters":          omitEmptyBool("Whether clusters can use tags from this category. Omit to leave the current value unchanged. An omitted flag is not sent."),
			"taggable_tenants":           omitEmptyBool("Whether tenants can use tags from this category. Omit to leave the current value unchanged. An omitted flag is not sent."),
		},
	}
}

// omitEmptyBool is an optional flag with no schema default. An omitted value
// is not sent as false. An update sends the flag only when it changes.
func omitEmptyBool(description string) schema.BoolAttribute {
	return schema.BoolAttribute{
		MarkdownDescription: description,
		Optional:            true,
		Computed:            true,
		PlanModifiers: []planmodifier.Bool{
			boolplanmodifier.UseStateForUnknown(),
		},
	}
}

func (r *TagCategoryResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *TagCategoryResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data TagCategoryResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.tagsApi.createTagCategory(ctx, &data); err != nil {
		if categoryIDSet(&data) {
			r.rememberCategory(ctx, resp, &data)
		}
		resp.Diagnostics.AddError("Error creating tag category", err.Error())
		return
	}
	// The category row exists. Keep its id before the follow-up read so a
	// later error still leaves the category in state.
	r.rememberCategory(ctx, resp, &data)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.tagsApi.readTagCategory(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error reading tag category", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, categoryForState(&data))...)
}

func (r *TagCategoryResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data TagCategoryResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.tagsApi.readTagCategory(ctx, &data); err != nil {
		if vergeos.IsNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading tag category", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, categoryForState(&data))...)
}

func (r *TagCategoryResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state TagCategoryResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.tagsApi.updateTagCategory(ctx, &plan, &state); err != nil {
		resp.Diagnostics.AddError("Error updating tag category", err.Error())
		return
	}
	plan.Id = state.Id
	if err := r.tagsApi.readTagCategory(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Error reading tag category", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, categoryForState(&plan))...)
}

func (r *TagCategoryResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data TagCategoryResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Surface the cascade where the destroy is applied. Docs and the example
	// say the same thing; prevent_destroy is how a configuration opts out.
	resp.Diagnostics.AddWarning(
		"Deleting a tag category deletes its tags and assignments",
		"VergeOS deletes every tag in this category and every assignment of those tags. It does not ask for confirmation.",
	)
	if err := r.tagsApi.deleteTagCategory(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error deleting tag category", err.Error())
		return
	}
	tflog.Debug(ctx, "tag category deleted")
}

func (r *TagCategoryResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError(
			"Invalid Tag Category Import ID",
			"Import vergeio_tag_category with the category key.",
		)
		return
	}
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *TagCategoryResource) rememberCategory(ctx context.Context, resp *resource.CreateResponse, data *TagCategoryResourceModel) {
	if !categoryIDSet(data) {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, categoryForState(data))...)
	if resp.Diagnostics.HasError() {
		return
	}
	tflog.Debug(ctx, fmt.Sprintf("stored tag category %s in state before read", data.Id.ValueString()))
}

func categoryIDSet(data *TagCategoryResourceModel) bool {
	return data != nil && !data.Id.IsNull() && !data.Id.IsUnknown() && data.Id.ValueString() != ""
}

func categoryForState(data *TagCategoryResourceModel) TagCategoryResourceModel {
	stored := *data
	stored.Id = knownString(data.Id)
	stored.Name = knownString(data.Name)
	stored.Description = knownString(data.Description)
	stored.SingleTagSelection = knownBool(data.SingleTagSelection)
	stored.TaggableVolumes = knownBool(data.TaggableVolumes)
	stored.TaggableVNets = knownBool(data.TaggableVNets)
	stored.TaggableVNetRules = knownBool(data.TaggableVNetRules)
	stored.TaggableVMwareContainers = knownBool(data.TaggableVMwareContainers)
	stored.TaggableVMs = knownBool(data.TaggableVMs)
	stored.TaggableUsers = knownBool(data.TaggableUsers)
	stored.TaggableTenantNodes = knownBool(data.TaggableTenantNodes)
	stored.TaggableSites = knownBool(data.TaggableSites)
	stored.TaggableNodes = knownBool(data.TaggableNodes)
	stored.TaggableGroups = knownBool(data.TaggableGroups)
	stored.TaggableClusters = knownBool(data.TaggableClusters)
	stored.TaggableTenants = knownBool(data.TaggableTenants)
	return stored
}
