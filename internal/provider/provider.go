// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package provider

import (
	"context"

	cloudinitFile "terraform-provider-vergeio/internal/provider/cloudinit_files"
	"terraform-provider-vergeio/internal/provider/cluster"
	"terraform-provider-vergeio/internal/provider/groups"
	"terraform-provider-vergeio/internal/provider/mediasource"
	"terraform-provider-vergeio/internal/provider/member"
	"terraform-provider-vergeio/internal/provider/network"
	"terraform-provider-vergeio/internal/provider/node"
	resourseGroups "terraform-provider-vergeio/internal/provider/resource_groups"
	"terraform-provider-vergeio/internal/provider/user"
	"terraform-provider-vergeio/internal/provider/vergeio"
	"terraform-provider-vergeio/internal/provider/version"
	"terraform-provider-vergeio/internal/provider/vm"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/function"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure Provider satisfies various provider interfaces.
var _ provider.Provider = &vergeioProvider{}
var _ provider.ProviderWithFunctions = &vergeioProvider{}

// VergeioProvider defines the provider implementation.
type vergeioProvider struct {
	// version is set to the provider version on release, "dev" when the
	// provider is built and ran locally, and "test" when running acceptance
	// testing.
	version string
}

// VergeioProviderModel describes the provider data model.
type vergeioProviderModel struct {
	Host     types.String `tfsdk:"host"`
	Username types.String `tfsdk:"username"`
	Password types.String `tfsdk:"password"`
	Insecure types.Bool   `tfsdk:"insecure"`
}

func (p *vergeioProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "vergeio"
	resp.Version = p.version
}

func (p *vergeioProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"host": schema.StringAttribute{
				MarkdownDescription: "Host Address of VergeOS",
				Required:            true,
			},
			"username": schema.StringAttribute{
				MarkdownDescription: "Username",
				Required:            true,
			},
			"password": schema.StringAttribute{
				MarkdownDescription: "Password",
				Required:            true,
			},
			"insecure": schema.BoolAttribute{
				MarkdownDescription: "Allow insecure connections",
				Optional:            true,
			},
		},
	}
}

func (p *vergeioProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data vergeioProviderModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Configuration values are now available.
	// if data.Endpoint.IsNull() { /* ... */ }

	// Example client configuration for data sources and resources
	client := &vergeio.Client{
		Username: data.Username.ValueString(),
		Password: data.Password.ValueString(),
		Host:     data.Host.ValueString(),
		Insecure: data.Insecure.ValueBool(),
	}

	resp.DataSourceData = client
	resp.ResourceData = client
}

func (p *vergeioProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		network.NewNetworkResource,
		vm.NewVMResource,
		user.NewUserResource,
		member.NewMemberResource,
	}
}

func (p *vergeioProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		version.NewVersionDataSource,
		network.NewNetworkDataSource,
		vm.NewVMDataSource,
		cluster.NewClusterDataSource,
		groups.NewGroupsDataSource,
		mediasource.NewMediasourceDataSource,
		node.NewNodeDataSource,
		cloudinitFile.NewCloudinitFileDataSource,
		resourseGroups.NewResourceGroupsDataSource,
	}
}

func (p *vergeioProvider) Functions(ctx context.Context) []func() function.Function {
	return []func() function.Function{}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &vergeioProvider{
			version: version,
		}
	}
}
