// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tags

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
var _ datasource.DataSource = &TagsDataSource{}

func NewTagsDataSource() datasource.DataSource {
	return &TagsDataSource{}
}

// TagsDataSource defines the data source implementation.
type TagsDataSource struct {
	tagsApi *TagsApi
}

// TagModel describes individual tag data model.
type TagModel struct {
	Key  types.Int32  `tfsdk:"key"`
	Name types.String `tfsdk:"name"`
}

// TagsDataSourceModel describes the data source data model.
type TagsDataSourceModel struct {
	Filter types.String `tfsdk:"filter"`
	Tags   []TagModel   `tfsdk:"tags"`
}

func (d *TagsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tags"
}

func (d *TagsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Tags data source to retrieve tag information from VergeOS",

		Attributes: map[string]schema.Attribute{
			"filter": schema.StringAttribute{
				MarkdownDescription: "Filter tags by name (optional). If provided, only tags with matching names will be returned.",
				Optional:            true,
			},
			"tags": schema.ListNestedAttribute{
				MarkdownDescription: "List of tags",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"key": schema.Int32Attribute{
							MarkdownDescription: "Tag key/ID",
							Computed:            true,
						},
						"name": schema.StringAttribute{
							MarkdownDescription: "Tag name",
							Computed:            true,
						},
					},
				},
			},
		},
	}
}

func (d *TagsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*vergeio.Client)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *vergeio.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	d.tagsApi = NewTagsApi(client)
}

func (d *TagsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	tflog.Trace(ctx, "Start reading tags data source")

	var data TagsDataSourceModel

	// Read Terraform configuration data into the model
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Read data from VergeOS API
	if err := d.tagsApi.readTags(ctx, &data); err != nil {
		resp.Diagnostics.AddError(
			"Error Fetching Tags Data",
			fmt.Sprintf("Unable to read tags, got error: %s", err.Error()),
		)
		return
	}

	// Write logs using the tflog package
	tflog.Trace(ctx, "End reading tags data source")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}