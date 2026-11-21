// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package compute

import (
	"context"
	"fmt"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

var (
	_ datasource.DataSource                   = &vmImportDataSource{}
	_ datasource.DataSourceWithValidateConfig = &vmImportDataSource{}
)

func NewVMImportDataSource() datasource.DataSource {
	return &vmImportDataSource{}
}

type vmImportDataSource struct {
	api *importAPI
}

func (d *vmImportDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vm_import"
}

func (d *vmImportDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads one vm_imports row. Set id to the 40 character hex key, or name to the import name. A name that matches more than one row is an error. wait defaults to true and blocks until the import finishes or fails. A failed import returns as soon as VergeOS reports it. Use this to wait for an import started in the UI or by Ansible. It does not start an import.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "vm_imports key, a 40 character hex string. Set this or name.",
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Import name. Set this or id. VergeOS does not require the name to be unique.",
				Optional:            true,
				Computed:            true,
			},
			"wait": schema.BoolAttribute{
				MarkdownDescription: "Wait until the import is complete or warning, or return an error when it fails. Omit to wait. Set false to read the current row, including one that is still importing.",
				Optional:            true,
			},
			"timeout": schema.StringAttribute{
				MarkdownDescription: "How long to wait when wait is true. A duration such as \"10m\" or \"30s\". Defaults to 10m.",
				Optional:            true,
				Validators: []validator.String{
					durationValidator{},
				},
			},
			"status": schema.StringAttribute{
				MarkdownDescription: "Import status: initializing, importing, complete, aborted, error, or warning.",
				Computed:            true,
			},
			"status_info": schema.StringAttribute{
				MarkdownDescription: "Detail VergeOS reported for status.",
				Computed:            true,
			},
			"vm_id": schema.Int64Attribute{
				MarkdownDescription: "Key of the VM created by the import. Null until VergeOS assigns one.",
				Computed:            true,
			},
			"file_id": schema.Int64Attribute{
				MarkdownDescription: "Media-catalog file id when the source is a file.",
				Computed:            true,
			},
			"volume": schema.StringAttribute{
				MarkdownDescription: "NAS volume key when the source is a volume.",
				Computed:            true,
			},
			"volume_path": schema.StringAttribute{
				MarkdownDescription: "OVA or OVF path on volume.",
				Computed:            true,
			},
			"shared_object": schema.Int64Attribute{
				MarkdownDescription: "Shared-object id when the source is a shared object.",
				Computed:            true,
			},
			"uuid": schema.StringAttribute{
				MarkdownDescription: "Source VM UUID.",
				Computed:            true,
			},
			"preferred_tier": schema.StringAttribute{
				MarkdownDescription: "Storage tier recorded on the import, from 1 through 5.",
				Computed:            true,
			},
			"importing": schema.BoolAttribute{
				MarkdownDescription: "True while VergeOS is still importing.",
				Computed:            true,
			},
			"aborted": schema.BoolAttribute{
				MarkdownDescription: "True when the import was aborted.",
				Computed:            true,
			},
			"failed_drive_count": schema.Int64Attribute{
				MarkdownDescription: "How many drives VergeOS has already failed.",
				Computed:            true,
			},
			"preserve_macs": schema.BoolAttribute{
				MarkdownDescription: "Whether source MAC addresses were kept.",
				Computed:            true,
			},
			"preserve_drive_format": schema.BoolAttribute{
				MarkdownDescription: "Whether the source disk format was kept.",
				Computed:            true,
			},
			"preserve_bios_serial": schema.BoolAttribute{
				MarkdownDescription: "Whether the source SMBIOS serial was kept. Null when the server is older than VergeOS 26.2.",
				Computed:            true,
			},
			"guest_agent_preinstalled": schema.BoolAttribute{
				MarkdownDescription: "Whether the guest agent was marked as already installed. Null when the server is older than VergeOS 26.2.",
				Computed:            true,
			},
			"replicate_tags": schema.BoolAttribute{
				MarkdownDescription: "Whether VMware tags were copied onto the VM. Null when the server is older than VergeOS 26.2.",
				Computed:            true,
			},
			"no_optical_drives": schema.BoolAttribute{
				MarkdownDescription: "Whether optical drives were skipped.",
				Computed:            true,
			},
			"override_drive_interface": schema.StringAttribute{
				MarkdownDescription: "Drive interface that replaced the source interface.",
				Computed:            true,
			},
			"override_nic_interface": schema.StringAttribute{
				MarkdownDescription: "NIC interface that replaced the source interface.",
				Computed:            true,
			},
		},
	}
}

func (d *vmImportDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	conn, ok := req.ProviderData.(*vergeio.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *vergeio.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}
	api, err := newImportAPI(conn)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Create VergeOS API Client", err.Error())
		return
	}
	d.api = api
}

func (d *vmImportDataSource) ValidateConfig(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var data importDataModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(validateImportData(&data)...)
}

func (d *vmImportDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data importDataModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := d.api.readData(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error reading VM import", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
