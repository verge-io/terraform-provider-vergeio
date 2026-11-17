// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package site

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const incomingStatusDescription = "Reads one incoming site sync. Set id, or set both site_id and name. max_lag_seconds fails the plan when the sync has not run, or when the last sync is older than that many seconds."

const outgoingStatusDescription = "Reads one outgoing site sync. Set id, or set both site_id and name. max_lag_seconds fails the plan when the sync has not run, or when the last run is older than that many seconds."

var (
	_ datasource.DataSource              = &incomingStatusDataSource{}
	_ datasource.DataSourceWithConfigure = &incomingStatusDataSource{}
	_ datasource.DataSource              = &outgoingStatusDataSource{}
	_ datasource.DataSourceWithConfigure = &outgoingStatusDataSource{}
)

func NewSyncIncomingStatusDataSource() datasource.DataSource {
	return &incomingStatusDataSource{}
}

func NewSyncOutgoingStatusDataSource() datasource.DataSource {
	return &outgoingStatusDataSource{}
}

type statusModel struct {
	ID            types.String `tfsdk:"id"`
	SiteID        types.String `tfsdk:"site_id"`
	Name          types.String `tfsdk:"name"`
	MaxLagSeconds types.Int64  `tfsdk:"max_lag_seconds"`
	Enabled       types.Bool   `tfsdk:"enabled"`
	Status        types.String `tfsdk:"status"`
	StatusInfo    types.String `tfsdk:"status_info"`
	State         types.String `tfsdk:"state"`
	LastActivity  types.Int64  `tfsdk:"last_activity"`
	LagSeconds    types.Int64  `tfsdk:"lag_seconds"`
	NeverSynced   types.Bool   `tfsdk:"never_synced"`
}

type incomingStatusDataSource struct {
	api *API
}

type outgoingStatusDataSource struct {
	api *API
}

func (d *incomingStatusDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_site_sync_incoming_status"
}

func (d *outgoingStatusDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_site_sync_outgoing_status"
}

func (d *incomingStatusDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = statusSchema(incomingStatusDescription, "Time of the last incoming sync, as seconds since the epoch. 0 means the sync has not run.")
}

func (d *outgoingStatusDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = statusSchema(outgoingStatusDescription, "Time of the last outgoing run, as seconds since the epoch. 0 means the sync has not run.")
}

func statusSchema(description, lastActivity string) schema.Schema {
	return schema.Schema{
		MarkdownDescription: description,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Sync key. Set this, or set both site_id and name.",
				Optional:            true,
				Computed:            true,
			},
			"site_id": schema.StringAttribute{
				MarkdownDescription: "Site key. Set this with name when id is omitted.",
				Optional:            true,
				Computed:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Sync name. Set this with site_id when id is omitted.",
				Optional:            true,
				Computed:            true,
			},
			"max_lag_seconds": schema.Int64Attribute{
				MarkdownDescription: "Fail the plan when the sync has not run, or when last activity is older than this many seconds.",
				Optional:            true,
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
				},
			},
			"enabled":       schema.BoolAttribute{MarkdownDescription: "Whether the sync is enabled.", Computed: true},
			"status":        schema.StringAttribute{MarkdownDescription: "Current sync status.", Computed: true},
			"status_info":   schema.StringAttribute{MarkdownDescription: "Detail for status.", Computed: true},
			"state":         schema.StringAttribute{MarkdownDescription: "Connection state.", Computed: true},
			"last_activity": schema.Int64Attribute{MarkdownDescription: lastActivity, Computed: true},
			"lag_seconds": schema.Int64Attribute{
				MarkdownDescription: "Seconds since last activity. Null when the sync has not run.",
				Computed:            true,
			},
			"never_synced": schema.BoolAttribute{
				MarkdownDescription: "True when last activity is 0.",
				Computed:            true,
			},
		},
	}
}

func (d *incomingStatusDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	api, diags := configureAPI(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	d.api = api
}

func (d *outgoingStatusDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	api, diags := configureAPI(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	d.api = api
}

func (d *incomingStatusDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data statusModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.api == nil {
		resp.Diagnostics.AddError("Error reading incoming site sync status", "vergeos client is nil")
		return
	}
	if err := d.api.readIncomingStatus(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error reading incoming site sync status", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (d *outgoingStatusDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data statusModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.api == nil {
		resp.Diagnostics.AddError("Error reading outgoing site sync status", "vergeos client is nil")
		return
	}
	if err := d.api.readOutgoingStatus(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error reading outgoing site sync status", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
