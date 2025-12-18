// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tags

import (
	"context"
	"fmt"
	"strings"

	"terraform-provider-vergeio/internal/provider/vergeio"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &TagMemberResource{}
var _ resource.ResourceWithImportState = &TagMemberResource{}

func NewTagMemberResource() resource.Resource {
	return &TagMemberResource{}
}

// TagMemberResource defines the resource implementation.
type TagMemberResource struct {
	tagsApi *TagsApi
}

// TagMemberResourceModel describes the resource data model.
type TagMemberResourceModel struct {
	Id     types.String `tfsdk:"id"`
	TagId  types.Int32  `tfsdk:"tag_id"`
	Member types.String `tfsdk:"member"`
}

// Metadata returns the resource type name.
func (r *TagMemberResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tag_member"
}

// Schema defines the schema for the resource.
func (r *TagMemberResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Tag member resource to assign tags to VergeOS objects (requires VergeOS v26+)",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Tag member ID (returned as the key) in VergeOS",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"tag_id": schema.Int32Attribute{
				MarkdownDescription: "ID of the tag to assign",
				Required:            true,
			},
			"member": schema.StringAttribute{
				MarkdownDescription: "Object to tag in format 'object_type/object_id' (e.g., 'vms/123', 'vnets/456')",
				Required:            true,
			},
		},
	}
}

// Configure the resource.
func (r *TagMemberResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
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

	r.tagsApi = NewTagsApi(client)
}

// Create a new tag member assignment.
func (r *TagMemberResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data TagMemberResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Validate member format (should contain '/')
	memberValue := data.Member.ValueString()
	if !strings.Contains(memberValue, "/") {
		resp.Diagnostics.AddAttributeError(
			path.Root("member"),
			"Invalid Member Format",
			"Member must be in format 'object_type/object_id' (e.g., 'vms/123', 'vnets/456')",
		)
		return
	}

	// Create the tag member assignment
	if err := r.tagsApi.createTagMember(ctx, &data); err != nil {
		resp.Diagnostics.AddError(
			"Error Creating Tag Member",
			fmt.Sprintf("Unable to create tag member, got error: %s", err.Error()),
		)
		return
	}

	// Write logs using the tflog package
	tflog.Debug(ctx, fmt.Sprintf("created tag member resource %v", data))

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Read tag member information.
func (r *TagMemberResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data TagMemberResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Read data from the API
	if err := r.tagsApi.readTagMember(ctx, &data); err != nil {
		// if the resource was not found, likely deleted outside of terraform
		// remove the resource from the state and return
		if strings.Contains(err.Error(), "tag member not found") {
			resp.State.RemoveResource(ctx)
			return
		}

		resp.Diagnostics.AddError(
			"Error Reading Tag Member",
			fmt.Sprintf("Unable to read tag member, got error: %s", err.Error()),
		)
		return
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Update a tag member assignment.
func (r *TagMemberResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data TagMemberResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Validate member format (should contain '/')
	memberValue := data.Member.ValueString()
	if !strings.Contains(memberValue, "/") {
		resp.Diagnostics.AddAttributeError(
			path.Root("member"),
			"Invalid Member Format",
			"Member must be in format 'object_type/object_id' (e.g., 'vms/123', 'vnets/456')",
		)
		return
	}

	// Update the tag member assignment
	if err := r.tagsApi.updateTagMember(ctx, &data); err != nil {
		resp.Diagnostics.AddError(
			"Error Updating Tag Member",
			fmt.Sprintf("Unable to update tag member, got error: %s", err.Error()),
		)
		return
	}

	tflog.Debug(ctx, fmt.Sprintf("updated tag member resource %v", data))

	// Read updated data from API to ensure consistency
	if err := r.tagsApi.readTagMember(ctx, &data); err != nil {
		resp.Diagnostics.AddError(
			"Error Reading Updated Tag Member",
			fmt.Sprintf("Unable to read updated tag member, got error: %s", err.Error()),
		)
		return
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Delete a tag member assignment.
func (r *TagMemberResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data TagMemberResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, fmt.Sprintf("Deleting tag member %v", data))

	// Delete the tag member assignment
	if err := r.tagsApi.deleteTagMember(ctx, &data); err != nil {
		resp.Diagnostics.AddError(
			"Error Deleting Tag Member",
			fmt.Sprintf("Unable to delete tag member, got error: %s", err.Error()),
		)
		return
	}

	tflog.Debug(ctx, "Tag member was successfully deleted")
}

// ImportState imports an existing tag member by ID.
func (r *TagMemberResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}