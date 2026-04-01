package cluster

import (
	"context"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/types"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"terraform-provider-vergeio/internal/provider/vergeio"
)

// Unit Tests
func TestClusterDataSource(t *testing.T) {
	dataSource := NewClusterDataSource()
	if dataSource == nil {
		t.Fatal("data source should not be nil")
	}
}

func TestClusterDataSource_Metadata(t *testing.T) {
	dataSource := NewClusterDataSource()
	req := datasource.MetadataRequest{
		ProviderTypeName: "vergeio",
	}
	resp := &datasource.MetadataResponse{}

	dataSource.Metadata(context.Background(), req, resp)

	if resp.TypeName != "vergeio_clusters" {
		t.Errorf("expected TypeName 'vergeio_clusters', got '%s'", resp.TypeName)
	}
}

func TestClusterDataSource_Schema(t *testing.T) {
	dataSource := NewClusterDataSource()
	req := datasource.SchemaRequest{}
	resp := &datasource.SchemaResponse{}

	dataSource.Schema(context.Background(), req, resp)

	// Check that required attributes exist
	if resp.Schema.Attributes == nil {
		t.Fatal("schema attributes should not be nil")
	}

	// Check filter_name attribute
	if filterName, ok := resp.Schema.Attributes["filter_name"]; !ok {
		t.Error("filter_name attribute should exist")
	} else if !filterName.IsOptional() {
		t.Error("filter_name should be optional")
	}

	// Check clusters attribute
	if clusters, ok := resp.Schema.Attributes["clusters"]; !ok {
		t.Error("clusters attribute should exist")
	} else if !clusters.IsComputed() {
		t.Error("clusters should be computed")
	}

	// Check schema description
	if resp.Schema.MarkdownDescription != "Cluster data source schema" {
		t.Errorf("expected description 'Cluster data source schema', got '%s'", resp.Schema.MarkdownDescription)
	}
}

func TestClusterDataSource_Configure_WithValidClient(t *testing.T) {
	dataSource := &ClusterDataSource{}
	client := vergeio.NewClient("test.example.com", "testuser", "testpass", true)
	
	req := datasource.ConfigureRequest{
		ProviderData: client,
	}
	resp := &datasource.ConfigureResponse{}

	dataSource.Configure(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Errorf("expected no errors, got: %v", resp.Diagnostics.Errors())
	}

	if dataSource.clusterApi == nil {
		t.Error("clusterApi should be configured")
	}

	if dataSource.clusterApi.Name() != "Cluster Api" {
		t.Errorf("expected clusterApi name 'Cluster Api', got '%s'", dataSource.clusterApi.Name())
	}
}

func TestClusterDataSource_Configure_WithInvalidClient(t *testing.T) {
	dataSource := &ClusterDataSource{}
	
	req := datasource.ConfigureRequest{
		ProviderData: "invalid",
	}
	resp := &datasource.ConfigureResponse{}

	dataSource.Configure(context.Background(), req, resp)

	if !resp.Diagnostics.HasError() {
		t.Error("expected error for invalid client type")
	}

	if dataSource.clusterApi != nil {
		t.Error("clusterApi should not be configured with invalid client")
	}
}

func TestClusterDataSource_Configure_WithNilClient(t *testing.T) {
	dataSource := &ClusterDataSource{}
	
	req := datasource.ConfigureRequest{
		ProviderData: nil,
	}
	resp := &datasource.ConfigureResponse{}

	dataSource.Configure(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Errorf("expected no errors with nil client, got: %v", resp.Diagnostics.Errors())
	}

	if dataSource.clusterApi != nil {
		t.Error("clusterApi should not be configured with nil client")
	}
}

func TestClusterModel_Types(t *testing.T) {
	model := &ClusterModel{
		Id:          types.Int32Value(123),
		Name:        types.StringValue("test-cluster"),
		Description: types.StringValue("test description"),
	}

	if model.Id.ValueInt32() != 123 {
		t.Errorf("expected Id 123, got %d", model.Id.ValueInt32())
	}

	if model.Name.ValueString() != "test-cluster" {
		t.Errorf("expected Name 'test-cluster', got '%s'", model.Name.ValueString())
	}

	if model.Description.ValueString() != "test description" {
		t.Errorf("expected Description 'test description', got '%s'", model.Description.ValueString())
	}
}

func TestClusterDataSourceModel_FilterHandling(t *testing.T) {
	model := &ClusterDataSourceModel{
		FilterName: types.StringValue("Production"),
		Clusters:   []*ClusterModel{},
	}

	if model.FilterName.ValueString() != "Production" {
		t.Errorf("expected FilterName 'Production', got '%s'", model.FilterName.ValueString())
	}

	if len(model.Clusters) != 0 {
		t.Errorf("expected empty Clusters slice, got %d items", len(model.Clusters))
	}

	// Test null values
	nullModel := &ClusterDataSourceModel{
		FilterName: types.StringNull(),
		Clusters:   []*ClusterModel{},
	}

	if !nullModel.FilterName.IsNull() {
		t.Error("expected FilterName to be null")
	}
}

// Acceptance Tests
func TestAccClusterDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccClusterDataSourceConfig(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.vergeio_clusters.test", "clusters.#"),
				),
			},
		},
	})
}

func TestAccClusterDataSource_WithNameFilter(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccClusterDataSourceConfigWithFilter(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.vergeio_clusters.test", "filter_name", "Production"),
					// clusters.# might be 0 if no Production clusters exist, so don't require it to be set
				),
			},
		},
	})
}

func testAccPreCheck(t *testing.T) {
	host := os.Getenv("TF_ACC_VERGEIO_HOST")
	username := os.Getenv("TF_ACC_VERGEIO_USERNAME") 
	password := os.Getenv("TF_ACC_VERGEIO_PASSWORD")

	if host == "" || username == "" || password == "" {
		t.Skip(`
Acceptance test skipped: VergeOS environment not configured.

To run acceptance tests, set these environment variables:
  export TF_ACC_VERGEIO_HOST="your-verge-host.com"
  export TF_ACC_VERGEIO_USERNAME="your-username"  
  export TF_ACC_VERGEIO_PASSWORD="your-password"

Then run:
  TF_ACC=1 go test ./internal/provider/cluster -v -run=TestAccClusterDataSource
`)
	}

	t.Logf("Acceptance test will connect to VergeOS at: %s (user: %s)", host, username)
}

// testProvider creates a minimal provider just for testing cluster data source
type testProvider struct{}

func (p *testProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "vergeio"
}

func (p *testProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"host":     schema.StringAttribute{Optional: true},
			"username": schema.StringAttribute{Optional: true},
			"password": schema.StringAttribute{Optional: true, Sensitive: true},
			"insecure": schema.BoolAttribute{Optional: true},
		},
	}
}

func (p *testProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data struct {
		Host     types.String `tfsdk:"host"`
		Username types.String `tfsdk:"username"`
		Password types.String `tfsdk:"password"`
		Insecure types.Bool   `tfsdk:"insecure"`
	}

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Create client with configuration
	client := vergeio.NewClient(
		data.Host.ValueString(),
		data.Username.ValueString(),
		data.Password.ValueString(),
		data.Insecure.ValueBool(),
	)

	resp.DataSourceData = client
	resp.ResourceData = client
}

func (p *testProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewClusterDataSource,
	}
}

func (p *testProvider) Resources(ctx context.Context) []func() fwresource.Resource {
	return []func() fwresource.Resource{}
}

var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"vergeio": providerserver.NewProtocol6WithError(&testProvider{}),
}

func testAccClusterDataSourceConfig() string {
	host := os.Getenv("TF_ACC_VERGEIO_HOST")
	username := os.Getenv("TF_ACC_VERGEIO_USERNAME")
	password := os.Getenv("TF_ACC_VERGEIO_PASSWORD")
	
	// If no environment variables, return minimal config that will be caught by PreCheck
	if host == "" || username == "" || password == "" {
		return `
provider "vergeio" {
  # Environment variables required for acceptance testing
}
data "vergeio_clusters" "test" {}
`
	}
	
	return `
provider "vergeio" {
  host     = "` + host + `"
  username = "` + username + `"
  password = "` + password + `"
  insecure = true
}
data "vergeio_clusters" "test" {}
`
}

func testAccClusterDataSourceConfigWithFilter() string {
	host := os.Getenv("TF_ACC_VERGEIO_HOST")
	username := os.Getenv("TF_ACC_VERGEIO_USERNAME")
	password := os.Getenv("TF_ACC_VERGEIO_PASSWORD")
	
	// If no environment variables, return minimal config that will be caught by PreCheck
	if host == "" || username == "" || password == "" {
		return `
provider "vergeio" {
  # Environment variables required for acceptance testing
}
data "vergeio_clusters" "test" {
  filter_name = "Production"
}
`
	}
	
	return `
provider "vergeio" {
  host     = "` + host + `"
  username = "` + username + `"
  password = "` + password + `"
  insecure = true
}
data "vergeio_clusters" "test" {
  filter_name = "Production"
}
`
}