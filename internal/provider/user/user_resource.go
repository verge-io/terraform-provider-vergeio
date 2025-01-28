// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package user

import (
	"context"
	"fmt"

	"terraform-provider-vergeio/internal/provider/vergeio"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &UserResource{}
var _ resource.ResourceWithImportState = &UserResource{}

func NewUserResource() resource.Resource {
	return &UserResource{}
}

// UserResource defines the resource implementation.
type UserResource struct {
	userApi *UserApi
}

// UserResourceModel describes the resource data model.
type UserResourceModel struct {
	Id             types.String `tfsdk:"id"`
	AuthSource     types.Int32  `tfsdk:"auth_source"`
	Name           types.String `tfsdk:"name"`
	RemoteName     types.String `tfsdk:"remote_name"`
	Enabled        types.Bool   `tfsdk:"enabled"`
	DisplayName    types.String `tfsdk:"displayname"`
	Email          types.String `tfsdk:"email"`
	Type           types.String `tfsdk:"type"`
	Password       types.String `tfsdk:"password"`
	ChangePassword types.Bool   `tfsdk:"change_password"`
}

// Metadata returns the resource type name.
func (r *UserResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

func (r *UserResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "User resource in VergeIO",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Id",
				Computed:            true,
			},
			"auth_source": schema.Int32Attribute{
				MarkdownDescription: "Authentication source",
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Unique user name",
				Required:            true,
			},
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "User state",
				Optional:            true,
				Computed:            true,
			},
			"remote_name": schema.StringAttribute{
				MarkdownDescription: "Remote name",
				Optional:            true,
				Computed:            true,
			},
			"displayname": schema.StringAttribute{
				MarkdownDescription: "Display name",
				Optional:            true,
				Computed:            true,
			},
			"email": schema.StringAttribute{
				MarkdownDescription: "Email address",
				Optional:            true,
				Computed:            true,
			},
			"type": schema.StringAttribute{
				MarkdownDescription: "User type",
				Optional:            true,
				Computed:            true,
				Validators: []validator.String{
					// Validate string value must be ""normal","api","vdi","
					stringvalidator.OneOf([]string{"normal",
						"api",
						"vdi"}...),
				},
			},
			"password": schema.StringAttribute{
				MarkdownDescription: "User password",
				Optional:            true,
				Computed:            true,
			},
			"change_password": schema.BoolAttribute{
				MarkdownDescription: "Change password",
				Optional:            true,
				Computed:            true,
			},
		},
	}
}

func (r *UserResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

	r.userApi = NewUserApi(client)
}

// Create a new user.
func (r *UserResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data UserResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.userApi.createUser(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error Creating user", err.Error())
		return
	}

	// Write logs using the tflog package
	tflog.Debug(ctx, fmt.Sprintf("created a resource %v", data))

	// Read data into the model to get all the attributes
	readDataError := r.userApi.readUser(ctx, &data)

	if readDataError != nil {
		resp.Diagnostics.AddError(
			"Error Fetching Data",
			readDataError.Error(),
		)
		return
	}

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Read a user.
func (r *UserResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data UserResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	if readDataError := r.userApi.readUser(ctx, &data); readDataError != nil {
		resp.Diagnostics.AddError(
			"Error Fetching Data",
			readDataError.Error(),
		)
		return
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Update a user.
func (r *UserResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var planData, stateData UserResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &planData)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &stateData)...)

	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.userApi.updateUser(ctx, &planData, &stateData); err != nil {
		resp.Diagnostics.AddError("Error Updating user", err.Error())
		return
	}

	// Read data into the model to get all the attributes
	if readDataError := r.userApi.readUser(ctx, &stateData); readDataError != nil {
		resp.Diagnostics.AddError("Error Fetching Data", readDataError.Error())
		return
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &stateData)...)
}

// Delete a user.
func (r *UserResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data UserResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Proceed with user deletion
	if err := r.userApi.deleteUser(ctx, &data); err != nil {
		resp.Diagnostics.AddError(
			"Error Deleting Data",
			err.Error(),
		)
		return
	}

	tflog.Debug(ctx, "User was successfully deleted")
}

func (r *UserResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
