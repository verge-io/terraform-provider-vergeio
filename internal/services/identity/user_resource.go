// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package identity

import (
	"context"
	"fmt"
	"strings"

	"terraform-provider-vergeio/internal/client"
	"terraform-provider-vergeio/internal/shared"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

const userPasswordDeprecation = "Deprecated. Terraform stores this value in state through v3.x. It will be removed in v4. Use password_wo and password_wo_version. Increment password_wo_version to change the password. password_wo requires Terraform 1.11 or OpenTofu 1.11."

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &UserResource{}
var _ resource.ResourceWithImportState = &UserResource{}
var _ resource.ResourceWithIdentity = &UserResource{}

func NewUserResource() resource.Resource {
	return &UserResource{}
}

// UserResource defines the resource implementation.
type UserResource struct {
	userApi *UserApi
}

// UserResourceModel describes the resource data model.
type UserResourceModel struct {
	Id                types.String `tfsdk:"id"`
	AuthSource        types.Int32  `tfsdk:"auth_source"`
	Name              types.String `tfsdk:"name"`
	RemoteName        types.String `tfsdk:"remote_name"`
	Enabled           types.Bool   `tfsdk:"enabled"`
	DisplayName       types.String `tfsdk:"displayname"`
	Email             types.String `tfsdk:"email"`
	Type              types.String `tfsdk:"type"`
	Password          types.String `tfsdk:"password"`
	PasswordWO        types.String `tfsdk:"password_wo"`
	PasswordWOVersion types.Int64  `tfsdk:"password_wo_version"`
	ChangePassword    types.Bool   `tfsdk:"change_password"`
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
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
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
				MarkdownDescription: "User password. Stored in state. Deprecated: use password_wo so the password is not stored.",
				Optional:            true,
				Computed:            true,
				Sensitive:           true,
				DeprecationMessage:  userPasswordDeprecation,
				Validators: []validator.String{
					stringvalidator.ConflictsWith(path.MatchRoot("password_wo")),
				},
			},
			"password_wo": schema.StringAttribute{
				MarkdownDescription: "User password sent on create, and again when password_wo_version changes. Terraform does not store it. Requires Terraform 1.11 or OpenTofu 1.11. Do not set password as well.",
				Optional:            true,
				WriteOnly:           true,
				Sensitive:           true,
				Validators: []validator.String{
					stringvalidator.ConflictsWith(path.MatchRoot("password")),
					stringvalidator.AlsoRequires(path.MatchRoot("password_wo_version")),
				},
			},
			"password_wo_version": schema.Int64Attribute{
				MarkdownDescription: "Version of password_wo. Increment it to set a new password. Terraform stores this number, not the password. Changing password_wo without changing this version does not update the user.",
				Optional:            true,
				Validators: []validator.Int64{
					int64validator.AlsoRequires(path.MatchRoot("password_wo")),
				},
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

	userApi, err := NewUserApi(client)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Create VergeOS API Client",
			err.Error(),
		)
		return
	}
	r.userApi = userApi
}

// Create a new user.
func (r *UserResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data, config UserResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// password_wo is null in the plan. Copy it onto the request body only.
	apiData := data
	shared.ApplyWriteOnlyString(&apiData.Password, apiData.PasswordWOVersion, types.Int64Null(), config.PasswordWO, false)

	if err := r.userApi.createUser(ctx, &apiData); err != nil {
		resp.Diagnostics.AddError("Error Creating user", err.Error())
		return
	}
	data.Id = apiData.Id

	// Log the id only. The model includes the password.
	tflog.Debug(ctx, fmt.Sprintf("created a user resource with id %s", data.Id.ValueString()))

	// Read data into the model to get all the attributes
	readDataError := r.userApi.readUser(ctx, &data)

	if readDataError != nil {
		resp.Diagnostics.AddError(
			"Error Fetching Data",
			readDataError.Error(),
		)
		return
	}

	scrubUserPassword(&data)

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, data.Id)
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
		// if the resource was not found, likely deleted outside of terraform
		// remove the resource from the state
		// and return
		if strings.Contains(readDataError.Error(), "not found") {
			shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, data.Id)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}

		resp.Diagnostics.AddError(
			"Error Fetching Data",
			readDataError.Error(),
		)
		return
	}

	scrubUserPassword(&data)

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, data.Id)
}

// Update a user.
func (r *UserResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var planData, stateData, config UserResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &planData)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &stateData)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Send password_wo only when its version changed. The plan value is null.
	apiPlan := planData
	shared.ApplyWriteOnlyString(&apiPlan.Password, apiPlan.PasswordWOVersion, stateData.PasswordWOVersion, config.PasswordWO, true)

	if err := r.userApi.updateUser(ctx, &apiPlan, &stateData); err != nil {
		resp.Diagnostics.AddError("Error Updating user", err.Error())
		return
	}

	// Copy the ID from state to plan since ID is computed and not in plan
	planData.Id = stateData.Id

	// Read data into the model to get all the attributes
	if readDataError := r.userApi.readUser(ctx, &planData); readDataError != nil {
		resp.Diagnostics.AddError("Error Fetching Data", readDataError.Error())
		return
	}

	scrubUserPassword(&planData)

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &planData)...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, planData.Id)
}

// scrubUserPassword drops a write-only password before state is saved.
// password_wo is null in the plan already. When password_wo_version is set,
// password must stay null so the secret is not written to the stored attribute.
func scrubUserPassword(data *UserResourceModel) {
	if data == nil {
		return
	}
	data.PasswordWO = types.StringNull()
	if shared.WriteOnlyVersionSet(data.PasswordWOVersion) {
		data.Password = types.StringNull()
	}
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

func (r *UserResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = shared.KeyIdentitySchema("User id. Import vergeio_user with this value.")
}

func (r *UserResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	shared.ImportByID(ctx, req, resp, "Invalid User Import ID", "Import vergeio_user with the user id.")
}
