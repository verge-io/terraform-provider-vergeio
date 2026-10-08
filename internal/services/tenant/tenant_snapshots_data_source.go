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

var _ datasource.DataSource = &TenantSnapshotsDataSource{}

func NewTenantSnapshotsDataSource() datasource.DataSource {
	return &TenantSnapshotsDataSource{}
}

// TenantSnapshotsDataSource is vergeio_tenant_snapshots.
type TenantSnapshotsDataSource struct {
	api *API
}

// TenantSnapshotListModel is one snapshot returned by vergeio_tenant_snapshots.
type TenantSnapshotListModel struct {
	Id      types.String `tfsdk:"id"`
	Name    types.String `tfsdk:"name"`
	Type    types.String `tfsdk:"type"`
	Created types.Int64  `tfsdk:"created"`
	Expires types.Int64  `tfsdk:"expires"`
}

// TenantSnapshotsDataSourceModel is the Terraform model for vergeio_tenant_snapshots.
type TenantSnapshotsDataSourceModel struct {
	TenantID  types.String               `tfsdk:"tenant_id"`
	Snapshots []*TenantSnapshotListModel `tfsdk:"snapshots"`
}

func (d *TenantSnapshotsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tenant_snapshots"
}

func (d *TenantSnapshotsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists snapshots of one VergeOS tenant, ordered by name. type is the coverage the API returns: full, partial_include, or partial_exclude. The tenant UI labels cloud snapshots Provider or Local. That label is not on these rows, so it is not in this list. expires is null when the snapshot does not expire.",
		Attributes: map[string]schema.Attribute{
			"tenant_id": schema.StringAttribute{
				MarkdownDescription: "Key of the parent vergeio_tenant, the same value as vergeio_tenant.id.",
				Required:            true,
			},
			"snapshots": schema.ListNestedAttribute{
				MarkdownDescription: "Snapshots VergeOS has for this tenant, including ones this configuration did not create.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":      schema.StringAttribute{MarkdownDescription: "Snapshot key. Import vergeio_tenant_snapshot with this value.", Computed: true},
						"name":    schema.StringAttribute{MarkdownDescription: "Snapshot name.", Computed: true},
						"type":    schema.StringAttribute{MarkdownDescription: "Snapshot coverage: full, partial_include, or partial_exclude.", Computed: true},
						"created": schema.Int64Attribute{MarkdownDescription: "Creation time as a Unix timestamp.", Computed: true},
						"expires": schema.Int64Attribute{MarkdownDescription: "Expiration time as a Unix timestamp. Null when the snapshot does not expire.", Computed: true},
					},
				},
			},
		},
	}
}

func (d *TenantSnapshotsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
	api, err := NewAPI(client)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Create VergeOS API Client",
			err.Error(),
		)
		return
	}
	d.api = api
}

func (d *TenantSnapshotsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	tflog.Trace(ctx, "reading tenant snapshots data source")
	var data TenantSnapshotsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := d.api.readTenantSnapshots(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error reading tenant snapshots", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
