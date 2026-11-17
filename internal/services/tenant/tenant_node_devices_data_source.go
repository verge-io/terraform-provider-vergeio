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

var _ datasource.DataSource = &TenantNodeDevicesDataSource{}

func NewTenantNodeDevicesDataSource() datasource.DataSource {
	return &TenantNodeDevicesDataSource{}
}

// TenantNodeDevicesDataSource is vergeio_tenant_node_devices.
type TenantNodeDevicesDataSource struct {
	api *API
}

// TenantNodeDeviceListModel is one device returned by vergeio_tenant_node_devices.
type TenantNodeDeviceListModel struct {
	Id            types.String `tfsdk:"id"`
	Name          types.String `tfsdk:"name"`
	Type          types.String `tfsdk:"type"`
	Description   types.String `tfsdk:"description"`
	ResourceGroup types.String `tfsdk:"resource_group"`
	Enabled       types.Bool   `tfsdk:"enabled"`
	Optional      types.Bool   `tfsdk:"optional"`
	Machine       types.Int32  `tfsdk:"machine"`
	MachineType   types.String `tfsdk:"machine_type"`
	OrderID       types.Int32  `tfsdk:"orderid"`
	UUID          types.String `tfsdk:"uuid"`
	Created       types.Int64  `tfsdk:"created"`
	Modified      types.Int64  `tfsdk:"modified"`
}

// TenantNodeDevicesDataSourceModel is the Terraform model for vergeio_tenant_node_devices.
type TenantNodeDevicesDataSourceModel struct {
	TenantNodeID types.String                 `tfsdk:"tenant_node_id"`
	Devices      []*TenantNodeDeviceListModel `tfsdk:"devices"`
}

func (d *TenantNodeDevicesDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tenant_node_devices"
}

func (d *TenantNodeDevicesDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Devices attached to one VergeOS tenant node, including devices this configuration did not create. A device passed to the tenant is thick provisioned: the tenant owns it even when no VM inside the tenant is using it. The list is ordered by name.",
		Attributes: map[string]schema.Attribute{
			"tenant_node_id": schema.StringAttribute{
				MarkdownDescription: "Key of the parent vergeio_tenant_node, the same value as vergeio_tenant_node.id.",
				Required:            true,
			},
			"devices": schema.ListNestedAttribute{
				MarkdownDescription: "Devices VergeOS has attached to this tenant node.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":             schema.StringAttribute{MarkdownDescription: "machine_devices key. Import vergeio_tenant_node_device with this value.", Computed: true},
						"name":           schema.StringAttribute{MarkdownDescription: "Device name.", Computed: true},
						"type":           schema.StringAttribute{MarkdownDescription: "Device type.", Computed: true},
						"description":    schema.StringAttribute{MarkdownDescription: "Device description.", Computed: true},
						"resource_group": schema.StringAttribute{MarkdownDescription: "Resource group UUID stored on the device.", Computed: true},
						"enabled":        schema.BoolAttribute{MarkdownDescription: "Whether the device is enabled.", Computed: true},
						"optional":       schema.BoolAttribute{MarkdownDescription: "Whether the tenant node can start without this device.", Computed: true},
						"machine":        schema.Int32Attribute{MarkdownDescription: "Machine key the device is attached to.", Computed: true},
						"machine_type":   schema.StringAttribute{MarkdownDescription: "Machine kind stored on the device. A tenant node device is tenant.", Computed: true},
						"orderid":        schema.Int32Attribute{MarkdownDescription: "Device order, from 0 through 64.", Computed: true},
						"uuid":           schema.StringAttribute{MarkdownDescription: "Device UUID assigned by VergeOS.", Computed: true},
						"created":        schema.Int64Attribute{MarkdownDescription: "Creation time as a Unix timestamp.", Computed: true},
						"modified":       schema.Int64Attribute{MarkdownDescription: "Last modification time as a Unix timestamp.", Computed: true},
					},
				},
			},
		},
	}
}

func (d *TenantNodeDevicesDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *TenantNodeDevicesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	tflog.Trace(ctx, "reading tenant node devices data source")
	var data TenantNodeDevicesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := d.api.readTenantNodeDevices(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error reading tenant node devices", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
