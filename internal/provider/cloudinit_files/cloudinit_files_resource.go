// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package cloudinitFile

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
var _ resource.Resource = &CloudinitFileResource{}
var _ resource.ResourceWithImportState = &CloudinitFileResource{}

func NewCloudinitFileResource() resource.Resource {
	return &CloudinitFileResource{}
}

// CloudinitFileResource defines the resource implementation.
type CloudinitFileResource struct {
	cloudinitFileApi *CloudinitFileApi
}

// CloudinitFileResourceModel describes the resource data model.
type CloudinitFileResourceModel struct {
	Id                types.String `tfsdk:"id"`
	Name              types.String `tfsdk:"name"`
	Filesize          types.Int64  `tfsdk:"filesize"`
	Contents          types.String `tfsdk:"contents"`
	ContainsVariables types.Bool   `tfsdk:"contains_variables"`
}

// Metadata returns the resource type name.
func (r *CloudinitFileResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cloudinitFile"
}

func (r *CloudinitFileResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "CloudinitFile or Vnet resource in VergeIO",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "CloudinitFile id (returned as the key) in VergeIO",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Unique cloudinitFile name",
				Required:            true,
			},
			"filesize": schema.Int64Attribute{
				MarkdownDescription: "Filesize",
				Computed:            true,
				Optional:            true,
			},
			"contents": schema.StringAttribute{
				MarkdownDescription: "Contents",
				Optional:            true,
				Computed:            true,
			},
			"contains_variables": schema.BoolAttribute{
				MarkdownDescription: "Contains variables",
				Optional:            true,
				Computed:            true,
			},
		},
	}
}

func (r *CloudinitFileResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

	r.cloudinitFileApi = NewCloudinitFileApi(client)
}

func (r *CloudinitFileResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data CloudinitFileResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Call the API to create the cloudinitFile
	if err := r.cloudinitFileApi.createCloudinitFile(ctx, &data); err != nil {
		resp.Diagnostics.AddError(
			"Error Creating CloudinitFile",
			err.Error(),
		)
		return
	}

	// Write logs using the tflog package
	tflog.Debug(ctx, fmt.Sprintf("created a resource %v", data))

	// Read data into the model to get all the attributes
	readDataError := r.cloudinitFileApi.readCloudinitFile(ctx, &data)

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

func (r *CloudinitFileResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data CloudinitFileResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	readDataError := r.cloudinitFileApi.readCloudinitFile(ctx, &data)

	if readDataError != nil {
		// if the resource was not found, likely deleted outside of terraform
		// remove the resource from the state
		// and return
		if strings.Contains(readDataError.Error(), "not found") {
			resp.State.RemoveResource(ctx)
			return
		}

		resp.Diagnostics.AddError(
			"Error Fetching Data",
			readDataError.Error(),
		)
		return
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *CloudinitFileResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var planData, stateData CloudinitFileResourceModel

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

	// Call the API to update the cloudinitFiler
	if err := r.cloudinitFileApi.updateCloudinitFile(ctx, &planData, &stateData); err != nil {
		resp.Diagnostics.AddError(
			"Error Updating CloudinitFile",
			err.Error(),
		)
	}

	// Read data into the model to get all the attributes
	readDataError := r.cloudinitFileApi.readCloudinitFile(ctx, &planData)
	if readDataError != nil {
		resp.Diagnostics.AddError(
			"Error Fetching Data",
			readDataError.Error(),
		)
		return
	}
	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &planData)...)
}

func (r *CloudinitFileResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data CloudinitFileResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Proceed with cloudinitFile deletion
	if err := r.cloudinitFileApi.deleteCloudinitFile(ctx, &data); err != nil {
		resp.Diagnostics.AddError(
			"Error Deleting CloudinitFile",
			err.Error(),
		)
		return
	}

	tflog.Debug(ctx, "CloudinitFile was successfully deleted")
}

func (r *CloudinitFileResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
