// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package vm

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
var _ datasource.DataSource = &VMDataSource{}

func NewVMDataSource() datasource.DataSource {
	return &VMDataSource{}
}

// VMDataSource defines the data source implementation.
type VMDataSource struct {
	vmApi *VMApi
}

// VMDataSourceModel describes the data source data model.
type VMModel struct {
	Id          types.Int32     `tfsdk:"id"`
	Name        types.String    `tfsdk:"name"`
	Key         types.Int32     `tfsdk:"key"`
	IsSnapshot  types.Bool      `tfsdk:"is_snapshot"`
	CPUType     types.String    `tfsdk:"cpu_type"`
	MachineType types.String    `tfsdk:"machine_type"`
	OSFamily    types.String    `tfsdk:"os_family"`
	UEFI        types.Bool      `tfsdk:"uefi"`
	Drives      []*VMDriveModel `tfsdk:"drives"`
	Nics        []*VMNicModel   `tfsdk:"nics"`
}

type VMDriveModel struct {
	Key           types.Int32  `tfsdk:"key"`
	Name          types.String `tfsdk:"name"`
	Interface     types.String `tfsdk:"interface"`
	Media         types.String `tfsdk:"media"`
	Description   types.String `tfsdk:"description"`
	PreferredTier types.String `tfsdk:"preferred_tier"`
}

type VMNicModel struct {
	Key       types.Int32  `tfsdk:"key"`
	Name      types.String `tfsdk:"name"`
	Interface types.String `tfsdk:"interface"`
	Vnet      types.String `tfsdk:"vnet"`
	Status    types.String `tfsdk:"status"`
	Ipaddress types.String `tfsdk:"ipaddress"`
}

type VMDataSourceModel struct {
	FilterName types.String `tfsdk:"filter_name"`
	IsSnapshot types.Bool   `tfsdk:"is_snapshot"`
	Vms        []*VMModel   `tfsdk:"vms"`
}

func (d *VMDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vms"
}

func (d *VMDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "VM data source",

		Attributes: map[string]schema.Attribute{
			"filter_name": schema.StringAttribute{
				MarkdownDescription: "Filter by name",
				Optional:            true,
			},
			"is_snapshot": schema.BoolAttribute{
				MarkdownDescription: "Filter by snapshot",
				Optional:            true,
			},
			"vms": schema.ListNestedAttribute{
				MarkdownDescription: "List of VMs",
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
						"key": schema.Int32Attribute{
							MarkdownDescription: "Key",
							Computed:            true,
						},
						"is_snapshot": schema.BoolAttribute{
							MarkdownDescription: "Is snapshot",
							Computed:            true,
						},
						"cpu_type": schema.StringAttribute{
							MarkdownDescription: "Cpu type",
							Computed:            true,
						},
						"machine_type": schema.StringAttribute{
							MarkdownDescription: "Machine type",
							Computed:            true,
						},
						"os_family": schema.StringAttribute{
							MarkdownDescription: "Os family",
							Computed:            true,
						},
						"uefi": schema.BoolAttribute{
							MarkdownDescription: "Uefi",
							Computed:            true,
						},
						"drives": schema.ListNestedAttribute{
							MarkdownDescription: "Drives",
							Computed:            true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"key": schema.Int32Attribute{
										MarkdownDescription: "Key",
										Computed:            true,
									},
									"name": schema.StringAttribute{
										MarkdownDescription: "Name",
										Computed:            true,
									},
									"interface": schema.StringAttribute{
										MarkdownDescription: "Interface",
										Computed:            true,
									},
									"media": schema.StringAttribute{
										MarkdownDescription: "Media",
										Computed:            true,
									},
									"description": schema.StringAttribute{
										MarkdownDescription: "Description",
										Computed:            true,
									},
									"preferred_tier": schema.StringAttribute{
										MarkdownDescription: "Preferred tier",
										Computed:            true,
									},
								},
							},
						},
						"nics": schema.ListNestedAttribute{
							MarkdownDescription: "Nics",
							Computed:            true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"key": schema.Int32Attribute{
										MarkdownDescription: "Key",
										Computed:            true,
									},
									"name": schema.StringAttribute{
										MarkdownDescription: "Name",
										Computed:            true,
									},
									"interface": schema.StringAttribute{
										MarkdownDescription: "Interface",
										Computed:            true,
									},
									"vnet": schema.StringAttribute{
										MarkdownDescription: "Vnet",
										Computed:            true,
									},
									"status": schema.StringAttribute{
										MarkdownDescription: "Status",
										Computed:            true,
									},
									"ipaddress": schema.StringAttribute{
										MarkdownDescription: "Ipaddress",
										Computed:            true,
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func (d *VMDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

	d.vmApi = NewVMApi(client)
}

func (d *VMDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	tflog.Trace(ctx, "Start reading vm data source")

	var data VMDataSourceModel

	// Read Terraform configuration data into the model
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Read data from Verge API
	if err := d.vmApi.readVMs(ctx, &data); err != nil {
		resp.Diagnostics.AddError(
			"Error Fetching Data",
			err.Error(),
		)
		return
	}

	// Write logs using the tflog package
	// Documentation: https://terraform.io/plugin/log
	tflog.Trace(ctx, "End reading vm data source")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
