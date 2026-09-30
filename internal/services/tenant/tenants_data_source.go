// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tenant

import (
	"context"
	"fmt"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ datasource.DataSource = &TenantsDataSource{}

func NewTenantsDataSource() datasource.DataSource {
	return &TenantsDataSource{}
}

// TenantsDataSource is vergeio_tenants.
type TenantsDataSource struct {
	api *API
}

// TenantModel is one tenant returned by vergeio_tenants.
type TenantModel struct {
	Id                   types.Int32  `tfsdk:"id"`
	Name                 types.String `tfsdk:"name"`
	Description          types.String `tfsdk:"description"`
	URL                  types.String `tfsdk:"url"`
	UUID                 types.String `tfsdk:"uuid"`
	VNet                 types.Int32  `tfsdk:"vnet"`
	UIAddressID          types.Int32  `tfsdk:"ui_address_id"`
	UIAddress            types.String `tfsdk:"ui_address"`
	Isolate              types.Bool   `tfsdk:"isolate"`
	IsSnapshot           types.Bool   `tfsdk:"is_snapshot"`
	ExposeCloudSnapshots types.Bool   `tfsdk:"expose_cloud_snapshots"`
	AllowBranding        types.Bool   `tfsdk:"allow_branding"`
	ThemeAccess          types.String `tfsdk:"theme_access"`
	PowerState           types.Bool   `tfsdk:"powerstate"`
	Status               types.String `tfsdk:"status"`
	State                types.String `tfsdk:"state"`
	Creator              types.String `tfsdk:"creator"`
	Created              types.Int64  `tfsdk:"created"`
}

// TenantsDataSourceModel is the Terraform model for vergeio_tenants.
type TenantsDataSourceModel struct {
	FilterName types.String   `tfsdk:"filter_name"`
	Tenants    []*TenantModel `tfsdk:"tenants"`
}

func (d *TenantsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tenants"
}

func (d *TenantsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists VergeOS tenants on the parent system. filter_name keeps an exact name match. Each tenant includes its power state and, when VergeOS has assigned one, the UI address.",
		Attributes: map[string]schema.Attribute{
			"filter_name": schema.StringAttribute{
				MarkdownDescription: "Exact tenant name. Omit to list every tenant.",
				Optional:            true,
			},
			"tenants": schema.ListNestedAttribute{
				MarkdownDescription: "Tenants visible to this provider configuration.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":                     schema.Int32Attribute{MarkdownDescription: "Tenant key.", Computed: true},
						"name":                   schema.StringAttribute{MarkdownDescription: "Tenant name.", Computed: true},
						"description":            schema.StringAttribute{MarkdownDescription: "Tenant description.", Computed: true},
						"url":                    schema.StringAttribute{MarkdownDescription: "URL recorded on the tenant.", Computed: true},
						"uuid":                   schema.StringAttribute{MarkdownDescription: "Tenant UUID.", Computed: true},
						"vnet":                   schema.Int32Attribute{MarkdownDescription: "Key of the network created for the tenant.", Computed: true},
						"ui_address_id":          schema.Int32Attribute{MarkdownDescription: "Key of the vnet address row for the tenant UI.", Computed: true},
						"ui_address":             schema.StringAttribute{MarkdownDescription: "IP address of the tenant UI.", Computed: true},
						"isolate":                schema.BoolAttribute{MarkdownDescription: "Whether network isolation is enabled.", Computed: true},
						"is_snapshot":            schema.BoolAttribute{MarkdownDescription: "Whether this record is a snapshot.", Computed: true},
						"expose_cloud_snapshots": schema.BoolAttribute{MarkdownDescription: "Whether the tenant may request system cloud snapshots.", Computed: true},
						"allow_branding":         schema.BoolAttribute{MarkdownDescription: "Whether the tenant may customize branding.", Computed: true},
						"theme_access":           schema.StringAttribute{MarkdownDescription: "Theme visibility.", Computed: true},
						"powerstate":             schema.BoolAttribute{MarkdownDescription: "Whether the tenant is powered on.", Computed: true},
						"status":                 schema.StringAttribute{MarkdownDescription: "Tenant status string.", Computed: true},
						"state":                  schema.StringAttribute{MarkdownDescription: "Simplified tenant state.", Computed: true},
						"creator":                schema.StringAttribute{MarkdownDescription: "Username that created the tenant.", Computed: true},
						"created":                schema.Int64Attribute{MarkdownDescription: "Creation time as a Unix timestamp.", Computed: true},
					},
				},
			},
		},
	}
}

func (d *TenantsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
	d.api = NewAPI(client)
}

func (d *TenantsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	tflog.Trace(ctx, "reading tenants data source")
	var data TenantsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := d.api.readTenants(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error reading tenants", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
