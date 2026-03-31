package node

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
func TestNodeDataSource(t *testing.T) {
	dataSource := NewNodeDataSource()
	if dataSource == nil {
		t.Fatal("data source should not be nil")
	}
}

func TestNodeDataSource_Metadata(t *testing.T) {
	dataSource := NewNodeDataSource()
	req := datasource.MetadataRequest{
		ProviderTypeName: "vergeio",
	}
	resp := &datasource.MetadataResponse{}

	dataSource.Metadata(context.Background(), req, resp)

	if resp.TypeName != "vergeio_nodes" {
		t.Errorf("expected TypeName 'vergeio_nodes', got '%s'", resp.TypeName)
	}
}

func TestNodeDataSource_Schema(t *testing.T) {
	dataSource := NewNodeDataSource()
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

	// Check nodes attribute
	if nodes, ok := resp.Schema.Attributes["nodes"]; !ok {
		t.Error("nodes attribute should exist")
	} else if !nodes.IsComputed() {
		t.Error("nodes should be computed")
	}

	// Check schema description
	if resp.Schema.MarkdownDescription != "Node data source schema" {
		t.Errorf("expected description 'Node data source schema', got '%s'", resp.Schema.MarkdownDescription)
	}
}

func TestNodeDataSource_Configure_WithValidClient(t *testing.T) {
	dataSource := &NodeDataSource{}
	client := vergeio.NewClient("test.example.com", "testuser", "testpass", true)
	
	req := datasource.ConfigureRequest{
		ProviderData: client,
	}
	resp := &datasource.ConfigureResponse{}

	dataSource.Configure(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Errorf("expected no errors, got: %v", resp.Diagnostics.Errors())
	}

	if dataSource.nodeApi == nil {
		t.Error("nodeApi should be configured")
	}

	if dataSource.nodeApi.Name() != "Node Api" {
		t.Errorf("expected nodeApi name 'Node Api', got '%s'", dataSource.nodeApi.Name())
	}
}

func TestNodeDataSource_Configure_WithInvalidClient(t *testing.T) {
	dataSource := &NodeDataSource{}
	
	req := datasource.ConfigureRequest{
		ProviderData: "invalid",
	}
	resp := &datasource.ConfigureResponse{}

	dataSource.Configure(context.Background(), req, resp)

	if !resp.Diagnostics.HasError() {
		t.Error("expected error for invalid client type")
	}

	if dataSource.nodeApi != nil {
		t.Error("nodeApi should not be configured with invalid client")
	}
}

func TestNodeDataSource_Configure_WithNilClient(t *testing.T) {
	dataSource := &NodeDataSource{}
	
	req := datasource.ConfigureRequest{
		ProviderData: nil,
	}
	resp := &datasource.ConfigureResponse{}

	dataSource.Configure(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Errorf("expected no errors with nil client, got: %v", resp.Diagnostics.Errors())
	}

	if dataSource.nodeApi != nil {
		t.Error("nodeApi should not be configured with nil client")
	}
}

func TestNodeModel_Types(t *testing.T) {
	model := &NodeModel{
		Id:          types.Int32Value(123),
		Name:        types.StringValue("test-node"),
		Description: types.StringValue("test description"),
	}

	if model.Id.ValueInt32() != 123 {
		t.Errorf("expected Id 123, got %d", model.Id.ValueInt32())
	}

	if model.Name.ValueString() != "test-node" {
		t.Errorf("expected Name 'test-node', got '%s'", model.Name.ValueString())
	}

	if model.Description.ValueString() != "test description" {
		t.Errorf("expected Description 'test description', got '%s'", model.Description.ValueString())
	}
}

func TestNodeDataSourceModel_FilterHandling(t *testing.T) {
	model := &NodeDataSourceModel{
		FilterName: types.StringValue("node-01"),
		Nodes:      []*NodeModel{},
	}

	if model.FilterName.ValueString() != "node-01" {
		t.Errorf("expected FilterName 'node-01', got '%s'", model.FilterName.ValueString())
	}

	if len(model.Nodes) != 0 {
		t.Errorf("expected empty Nodes slice, got %d items", len(model.Nodes))
	}

	// Test null values
	nullModel := &NodeDataSourceModel{
		FilterName: types.StringNull(),
		Nodes:      []*NodeModel{},
	}

	if !nullModel.FilterName.IsNull() {
		t.Error("expected FilterName to be null")
	}
}

// Acceptance Tests
func TestAccNodeDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccNodeDataSourceConfig(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.vergeio_nodes.test", "nodes.#"),
				),
			},
		},
	})
}

func TestAccNodeDataSource_WithFilter(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccNodeDataSourceConfigWithFilter(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.vergeio_nodes.test", "filter_name", "node-01"),
					// nodes.# might be 0 if no node-01 exists, so don't require it to be set
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
  TF_ACC=1 go test ./internal/provider/node -v -run=TestAccNodeDataSource
`)
	}

	t.Logf("Acceptance test will connect to VergeOS at: %s (user: %s)", host, username)
}

// testProvider creates a minimal provider just for testing node data source
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
		NewNodeDataSource,
	}
}

func (p *testProvider) Resources(ctx context.Context) []func() fwresource.Resource {
	return []func() fwresource.Resource{}
}

var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"vergeio": providerserver.NewProtocol6WithError(&testProvider{}),
}

func testAccNodeDataSourceConfig() string {
	host := os.Getenv("TF_ACC_VERGEIO_HOST")
	username := os.Getenv("TF_ACC_VERGEIO_USERNAME")
	password := os.Getenv("TF_ACC_VERGEIO_PASSWORD")
	
	// If no environment variables, return minimal config that will be caught by PreCheck
	if host == "" || username == "" || password == "" {
		return `
provider "vergeio" {
  # Environment variables required for acceptance testing
}
data "vergeio_nodes" "test" {}
`
	}
	
	return `
provider "vergeio" {
  host     = "` + host + `"
  username = "` + username + `"
  password = "` + password + `"
  insecure = true
}
data "vergeio_nodes" "test" {}
`
}

func testAccNodeDataSourceConfigWithFilter() string {
	host := os.Getenv("TF_ACC_VERGEIO_HOST")
	username := os.Getenv("TF_ACC_VERGEIO_USERNAME")
	password := os.Getenv("TF_ACC_VERGEIO_PASSWORD")
	
	// If no environment variables, return minimal config that will be caught by PreCheck
	if host == "" || username == "" || password == "" {
		return `
provider "vergeio" {
  # Environment variables required for acceptance testing
}
data "vergeio_nodes" "test" {
  filter_name = "node-01"
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
data "vergeio_nodes" "test" {
  filter_name = "node-01"
}
`
}