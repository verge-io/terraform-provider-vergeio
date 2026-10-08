// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package compute

import (
	"context"
	"fmt"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ datasource.DataSource = &CatalogsDataSource{}

func NewCatalogsDataSource() datasource.DataSource {
	return &CatalogsDataSource{}
}

// CatalogsDataSource is vergeio_catalogs.
type CatalogsDataSource struct {
	api *RecipeCatalogApi
}

// CatalogModel is one catalog returned by vergeio_catalogs.
type CatalogModel struct {
	Id              types.String `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	Description     types.String `tfsdk:"description"`
	PublishingScope types.String `tfsdk:"publishing_scope"`
	Enabled         types.Bool   `tfsdk:"enabled"`
	Repository      types.Int64  `tfsdk:"repository"`
	RepositoryName  types.String `tfsdk:"repository_name"`
	Created         types.Int64  `tfsdk:"created"`
}

// CatalogsDataSourceModel is the Terraform model for vergeio_catalogs.
type CatalogsDataSourceModel struct {
	FilterName types.String   `tfsdk:"filter_name"`
	Catalogs   []CatalogModel `tfsdk:"catalogs"`
}

func (d *CatalogsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_catalogs"
}

func (d *CatalogsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists VergeOS recipe catalogs. filter_name keeps an exact name match. The same name in two repositories is returned as two catalogs. Use id with vergeio_vm_recipes.catalog_id or vergeio_tenant_recipes.catalog_id.",
		Attributes: map[string]schema.Attribute{
			"filter_name": schema.StringAttribute{
				MarkdownDescription: "Exact catalog name. Omit to list every catalog.",
				Optional:            true,
			},
			"catalogs": schema.ListNestedAttribute{
				MarkdownDescription: "Catalogs visible to this provider configuration.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":               schema.StringAttribute{MarkdownDescription: "Catalog key, 40 hexadecimal characters.", Computed: true},
						"name":             schema.StringAttribute{MarkdownDescription: "Catalog name.", Computed: true},
						"description":      schema.StringAttribute{MarkdownDescription: "Catalog description.", Computed: true},
						"publishing_scope": schema.StringAttribute{MarkdownDescription: "Who can see the catalog: private, global, tenant, or none.", Computed: true},
						"enabled":          schema.BoolAttribute{MarkdownDescription: "Whether the catalog is enabled.", Computed: true},
						"repository":       schema.Int64Attribute{MarkdownDescription: "Parent catalog repository key. Null when VergeOS did not report one.", Computed: true},
						"repository_name":  schema.StringAttribute{MarkdownDescription: "Parent catalog repository name.", Computed: true},
						"created":          schema.Int64Attribute{MarkdownDescription: "Creation time in microseconds, as VergeOS stores it.", Computed: true},
					},
				},
			},
		},
	}
}

func (d *CatalogsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
	api, err := NewRecipeCatalogApi(client)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Create VergeOS API Client",
			err.Error(),
		)
		return
	}
	d.api = api
}

func (d *CatalogsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	tflog.Trace(ctx, "Start reading catalogs data source")
	var data CatalogsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := d.api.readCatalogs(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error Fetching Catalogs", err.Error())
		return
	}
	tflog.Trace(ctx, "End reading catalogs data source")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
