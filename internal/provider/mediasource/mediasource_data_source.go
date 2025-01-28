// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package mediasource

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
var _ datasource.DataSource = &MediasourceDataSource{}

func NewMediasourceDataSource() datasource.DataSource {
	return &MediasourceDataSource{}
}

// MediasourceDataSource defines the data source implementation.
type MediasourceDataSource struct {
	mediasourceApi *MediasourceApi
}

// MediasourceDataSourceModel describes the data source data model.
type MediasourceModel struct {
	Id          types.Int32  `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Filesize    types.Int64  `tfsdk:"filesize"`
}

type MediasourceDataSourceModel struct {
	FilterName   types.String        `tfsdk:"filter_name"`
	Mediasources []*MediasourceModel `tfsdk:"mediasources"`
}

func (d *MediasourceDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_mediasources"
}

func (d *MediasourceDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Mediasource data source schema",

		Attributes: map[string]schema.Attribute{
			"filter_name": schema.StringAttribute{
				MarkdownDescription: "Filter by name",
				Optional:            true,
			},
			"mediasources": schema.ListNestedAttribute{
				MarkdownDescription: "List of Mediasources",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.Int32Attribute{
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
						"filesize": schema.Int64Attribute{
							MarkdownDescription: "Filesize",
							Computed:            true,
						},
					},
				},
			},
		},
	}
}

func (d *MediasourceDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

	d.mediasourceApi = NewMediasourceApi(client)
}

// Read refreshes the Terraform state with the latest data.
func (d *MediasourceDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	tflog.Trace(ctx, "Start reading mediasource data source")

	var data MediasourceDataSourceModel

	// Read Terraform configuration data into the model
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Read data from Verge API
	if err := d.mediasourceApi.readMediasources(ctx, &data); err != nil {
		resp.Diagnostics.AddError(
			"Error Fetching Data",
			err.Error(),
		)
		return
	}

	// Write logs using the tflog package
	// Documentation: https://terraform.io/plugin/log
	tflog.Trace(ctx, "End reading mediasource data source")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
