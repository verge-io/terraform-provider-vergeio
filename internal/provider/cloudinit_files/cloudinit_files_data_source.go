// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package cloudinitFile

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
var _ datasource.DataSource = &CloudinitFileDataSource{}

func NewCloudinitFileDataSource() datasource.DataSource {
	return &CloudinitFileDataSource{}
}

// CloudinitFileDataSource defines the data source implementation.
type CloudinitFileDataSource struct {
	cloudinitFileApi *CloudinitFileApi
}

// CloudinitFileDataSourceModel describes the data source data model.
type CloudinitFileModel struct {
	Id                types.String `tfsdk:"id"`
	Name              types.String `tfsdk:"name"`
	Filesize          types.Int64  `tfsdk:"filesize"`
	Contents          types.String `tfsdk:"contents"`
	ContainsVariables types.Bool   `tfsdk:"contains_variables"`
}

type CloudinitFileDataSourceModel struct {
	FilterName     types.String          `tfsdk:"filter_name"`
	CloudinitFiles []*CloudinitFileModel `tfsdk:"cloudinit_files"`
}

func (d *CloudinitFileDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cloudinitfiles"
}

// Schema defines the schema for the data source.
func (d *CloudinitFileDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "CloudinitFile data source schema",

		Attributes: map[string]schema.Attribute{
			"filter_name": schema.StringAttribute{
				MarkdownDescription: "Filter by name",
				Optional:            true,
			},
			"cloudinit_files": schema.ListNestedAttribute{
				MarkdownDescription: "List of CloudinitFiles",
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
						"filesize": schema.Int64Attribute{
							MarkdownDescription: "Filesize",
							Computed:            true,
						},
						"contents": schema.StringAttribute{
							MarkdownDescription: "Contents",
							Computed:            true,
						},
						"contains_variables": schema.BoolAttribute{
							MarkdownDescription: "Contains variables",
							Computed:            true,
						},
					},
				},
			},
		},
	}
}

func (d *CloudinitFileDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

	d.cloudinitFileApi = NewCloudinitFileApi(client)
}

// Read refreshes the Terraform state with the latest data.
func (d *CloudinitFileDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	tflog.Trace(ctx, "Start reading cloudinitFile data source")

	var data CloudinitFileDataSourceModel

	// Read Terraform configuration data into the model
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Read data from Verge API
	if err := d.cloudinitFileApi.readCloudinitFiles(ctx, &data); err != nil {
		resp.Diagnostics.AddError(
			"Error Fetching Data",
			err.Error(),
		)
		return
	}

	// Write logs using the tflog package
	// Documentation: https://terraform.io/plugin/log
	tflog.Trace(ctx, "End reading cloudinitFile data source")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
