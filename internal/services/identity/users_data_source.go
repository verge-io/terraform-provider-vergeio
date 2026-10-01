// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package identity

import (
	"context"
	"fmt"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ datasource.DataSource = &UsersDataSource{}

func NewUsersDataSource() datasource.DataSource {
	return &UsersDataSource{}
}

// UsersDataSource lists VergeOS users.
type UsersDataSource struct {
	api *UsersApi
}

// UserModel is one user returned by vergeio_users.
type UserModel struct {
	Id          types.Int32  `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	DisplayName types.String `tfsdk:"displayname"`
	Email       types.String `tfsdk:"email"`
	Type        types.String `tfsdk:"type"`
	Enabled     types.Bool   `tfsdk:"enabled"`
}

// UsersDataSourceModel is the Terraform model for vergeio_users.
type UsersDataSourceModel struct {
	FilterName types.String `tfsdk:"filter_name"`
	Users      []*UserModel `tfsdk:"users"`
}

func (d *UsersDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_users"
}

func (d *UsersDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Users data source. Lists VergeOS users, optionally filtered to one name.",
		Attributes: map[string]schema.Attribute{
			"filter_name": schema.StringAttribute{
				MarkdownDescription: "If specified, results are filtered to this exact name.",
				Optional:            true,
			},
			"users": schema.ListNestedAttribute{
				MarkdownDescription: "List of users",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.Int32Attribute{
							MarkdownDescription: "User key",
							Computed:            true,
						},
						"name": schema.StringAttribute{
							MarkdownDescription: "User name",
							Computed:            true,
						},
						"displayname": schema.StringAttribute{
							MarkdownDescription: "Display name",
							Computed:            true,
						},
						"email": schema.StringAttribute{
							MarkdownDescription: "Email address",
							Computed:            true,
						},
						"type": schema.StringAttribute{
							MarkdownDescription: "User type, such as normal, api, or vdi",
							Computed:            true,
						},
						"enabled": schema.BoolAttribute{
							MarkdownDescription: "Whether the user is enabled",
							Computed:            true,
						},
					},
				},
			},
		},
	}
}

func (d *UsersDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
	api, err := NewUsersApi(client)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Create VergeOS API Client",
			err.Error(),
		)
		return
	}
	d.api = api
}

func (d *UsersDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	tflog.Trace(ctx, "start reading users data source")

	var data UsersDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := d.api.readUsers(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error Fetching Data", err.Error())
		return
	}
	tflog.Trace(ctx, "end reading users data source")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
