// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package provider

import (
	"context"

	"terraform-provider-vergeio/internal/client"
	"terraform-provider-vergeio/internal/services/compute"
	"terraform-provider-vergeio/internal/services/identity"
	"terraform-provider-vergeio/internal/services/nas"
	"terraform-provider-vergeio/internal/services/network"
	"terraform-provider-vergeio/internal/services/platform"
	"terraform-provider-vergeio/internal/services/site"
	"terraform-provider-vergeio/internal/services/snapshot"
	"terraform-provider-vergeio/internal/services/storage"
	"terraform-provider-vergeio/internal/services/system"
	"terraform-provider-vergeio/internal/services/tags"
	"terraform-provider-vergeio/internal/services/tenant"

	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral"
	"github.com/hashicorp/terraform-plugin-framework/function"
	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure Provider satisfies various provider interfaces.
var _ provider.Provider = &vergeioProvider{}
var _ provider.ProviderWithFunctions = &vergeioProvider{}
var _ provider.ProviderWithEphemeralResources = &vergeioProvider{}
var _ provider.ProviderWithListResources = &vergeioProvider{}
var _ provider.ProviderWithActions = &vergeioProvider{}

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
	APIKey   types.String `tfsdk:"api_key"`
	Insecure types.Bool   `tfsdk:"insecure"`
	Timeout  types.Int64  `tfsdk:"timeout"`
}

func (p *vergeioProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "vergeio"
	resp.Version = p.version
}

func (p *vergeioProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Connects to a VergeOS system. Every argument is optional and falls back to a VERGEOS_* environment variable. A value in the provider block takes precedence over the environment.",
		Attributes: map[string]schema.Attribute{
			"host": schema.StringAttribute{
				MarkdownDescription: "VergeOS hostname or IP address, with or without an https:// prefix. If omitted, the provider uses VERGEOS_HOST.",
				Optional:            true,
			},
			"username": schema.StringAttribute{
				MarkdownDescription: "Username for password authentication. If omitted, the provider uses VERGEOS_USERNAME. Required unless api_key is set.",
				Optional:            true,
			},
			"password": schema.StringAttribute{
				MarkdownDescription: "Password for password authentication. If omitted, the provider uses VERGEOS_PASSWORD. Required unless api_key is set.",
				Optional:            true,
				Sensitive:           true,
			},
			"api_key": schema.StringAttribute{
				MarkdownDescription: "API key sent as a bearer token. If omitted, the provider uses VERGEOS_API_KEY. When set, it is used instead of username and password.",
				Optional:            true,
				Sensitive:           true,
			},
			"insecure": schema.BoolAttribute{
				MarkdownDescription: "Skip TLS certificate verification. If omitted, the provider uses VERGEOS_INSECURE, or VERGEOS_VERIFY_SSL when that is false. Defaults to false.",
				Optional:            true,
			},
			"timeout": schema.Int64Attribute{
				MarkdownDescription: "HTTP request timeout in seconds. If omitted, the provider uses VERGEOS_TIMEOUT, or 60 when that is unset.",
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

	// Provider values win over the environment. Missing host or credentials
	// fail here, before the first API request.
	cfg, diags := resolveProviderConfig(data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Build the HTTP client once here. NewClientWithConfig applies the timeout
	// and connection settings. Do does not create a client on first use.
	client := vergeio.NewClientWithConfig(cfg)

	// Initialize field cache for session-based caching
	client.FieldCache = vergeio.NewFieldCache(client)

	resp.DataSourceData = client
	resp.ResourceData = client
	resp.EphemeralResourceData = client
	resp.ListResourceData = client
	resp.ActionData = client
}

func (p *vergeioProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		network.NewNetworkResource,
		network.NewNetworkIPSecResource,
		network.NewNetworkIPSecConnectionResource,
		network.NewNetworkWireGuardResource,
		network.NewNetworkWireGuardPeerResource,
		network.NewNetworkRuleResource,
		network.NewNetworkRulesResource,
		network.NewNetworkRuleAliasResource,
		network.NewNetworkDNSViewResource,
		network.NewNetworkDNSZoneResource,
		network.NewNetworkDNSRecordResource,
		nas.NewServiceResource,
		nas.NewVolumeResource,
		nas.NewCIFSShareResource,
		nas.NewNFSShareResource,
		compute.NewVMResource,
		compute.NewVMDriveResource,
		compute.NewVMNICResource,
		compute.NewVMRecipeInstanceResource,
		identity.NewUserResource,
		identity.NewGroupResource,
		identity.NewMemberResource,
		identity.NewPermissionResource,
		identity.NewAPIKeyResource,
		identity.NewAuthSourceResource,
		snapshot.NewSnapshotProfileResource,
		site.NewSiteResource,
		platform.NewCertificateResource,
		platform.NewWebhookURLResource,
		platform.NewWebhookResource,
		platform.NewSettingResource,
		site.NewSyncIncomingResource,
		site.NewSyncOutgoingResource,
		tags.NewTagCategoryResource,
		tags.NewTagResource,
		tags.NewTagMemberResource,
		tenant.NewTenantResource,
		tenant.NewTenantNodeResource,
		tenant.NewTenantStorageResource,
		tenant.NewTenantExternalIPResource,
		tenant.NewTenantLayer2NetworkResource,
		tenant.NewTenantNetworkBlockResource,
	}
}

func (p *vergeioProvider) ListResources(ctx context.Context) []func() list.ListResource {
	return []func() list.ListResource{
		compute.NewVMListResource,
		network.NewNetworkListResource,
		tenant.NewTenantListResource,
		identity.NewUserListResource,
		identity.NewGroupListResource,
		tags.NewTagListResource,
		snapshot.NewSnapshotProfileListResource,
	}
}

func (p *vergeioProvider) EphemeralResources(ctx context.Context) []func() ephemeral.EphemeralResource {
	return []func() ephemeral.EphemeralResource{
		identity.NewAPIKeyEphemeralResource,
	}
}

func (p *vergeioProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		system.NewVersionDataSource,
		network.NewNetworkDataSource,
		compute.NewVMDataSource,
		compute.NewCatalogsDataSource,
		compute.NewVMRecipesDataSource,
		system.NewClusterDataSource,
		identity.NewGroupsDataSource,
		identity.NewUsersDataSource,
		storage.NewMediasourceDataSource,
		system.NewNodeDataSource,
		compute.NewCloudinitFileDataSource,
		system.NewResourceGroupsDataSource,
		tags.NewTagsDataSource,
		site.NewSyncIncomingStatusDataSource,
		site.NewSyncOutgoingStatusDataSource,
		tenant.NewTenantsDataSource,
	}
}

func (p *vergeioProvider) Actions(ctx context.Context) []func() action.Action {
	return []func() action.Action{
		compute.NewVMSnapshotAction,
		network.NewNetworkApplyAction,
		compute.NewVMPowerAction,
		tenant.NewTenantSnapshotAction,
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
