// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package resourseGroups

import (
	"context"
	"fmt"

	"terraform-provider-vergeio/internal/provider/vergeio"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &ResourceGroupsDataSource{}

func NewResourceGroupsDataSource() datasource.DataSource {
	return &ResourceGroupsDataSource{}
}

// ResourceGroupDataSource defines the data source implementation.
type ResourceGroupsDataSource struct {
	resourceGroupsApi *ResourceGroupsApi
}

// GroupDataSourceModel describes the data source data model.
type ResourceGroupModel struct {
	Id          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Enabled     types.Bool   `tfsdk:"enabled"`
	Type        types.String `tfsdk:"type"`
	Class       types.String `tfsdk:"class"`
}

type ResourceGroupDataSourceModel struct {
	FilterName     types.String          `tfsdk:"filter_name"`
	ResourceGroups []*ResourceGroupModel `tfsdk:"resource_groups"`
}

func (d *ResourceGroupsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_resource_groups"
}

// Schema defines the schema for the data source.
func (d *ResourceGroupsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Resource Group data source schema",

		Attributes: map[string]schema.Attribute{
			"filter_name": schema.StringAttribute{
				MarkdownDescription: "Filter by name",
				Optional:            true,
			},
			"resource_groups": schema.ListNestedAttribute{
				MarkdownDescription: "List of Resource Groups",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							MarkdownDescription: "Id",
							Computed:            true,
						},
						"name": schema.StringAttribute{
							MarkdownDescription: "Name",
							Computed:            true,
						},
						"description": schema.StringAttribute{
							MarkdownDescription: "Description",
							Computed:            true,
						},
						"enabled": schema.BoolAttribute{
							MarkdownDescription: "Enabled",
							Computed:            true,
						},
						"type": schema.StringAttribute{
							MarkdownDescription: "Type",
							Computed:            true,
						},
						"class": schema.StringAttribute{
							MarkdownDescription: "Class",
							Computed:            true,
						},
					},
				},
			},
		},
	}
}

func (d *ResourceGroupsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*vergeio.Client)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *http.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	d.resourceGroupsApi = NewResourceGroupsApi(client)
}

// Read refreshes the Terraform state with the latest data.
func (d *ResourceGroupsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	tflog.Trace(ctx, "Start reading resource groups data source")

	var data ResourceGroupDataSourceModel

	// Read Terraform configuration data into the model
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Read data from Verge API
	if err := d.resourceGroupsApi.readResourceGroups(ctx, &data); err != nil {
		resp.Diagnostics.AddError(
			"Error Fetching Data",
			err.Error(),
		)
		return
	}

	// Write logs using the tflog package
	tflog.Trace(ctx, "End reading resource groups data source")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
