package mediasource

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
func TestMediasourceDataSource(t *testing.T) {
	dataSource := NewMediasourceDataSource()
	if dataSource == nil {
		t.Fatal("data source should not be nil")
	}
}

func TestMediasourceDataSource_Metadata(t *testing.T) {
	dataSource := NewMediasourceDataSource()
	req := datasource.MetadataRequest{
		ProviderTypeName: "vergeio",
	}
	resp := &datasource.MetadataResponse{}

	dataSource.Metadata(context.Background(), req, resp)

	if resp.TypeName != "vergeio_mediasources" {
		t.Errorf("expected TypeName 'vergeio_mediasources', got '%s'", resp.TypeName)
	}
}

func TestMediasourceDataSource_Schema(t *testing.T) {
	dataSource := NewMediasourceDataSource()
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

	// Check mediasources attribute
	if mediasources, ok := resp.Schema.Attributes["mediasources"]; !ok {
		t.Error("mediasources attribute should exist")
	} else if !mediasources.IsComputed() {
		t.Error("mediasources should be computed")
	}

	// Check schema description
	if resp.Schema.MarkdownDescription != "Mediasource data source schema" {
		t.Errorf("expected description 'Mediasource data source schema', got '%s'", resp.Schema.MarkdownDescription)
	}
}

func TestMediasourceDataSource_Configure_WithValidClient(t *testing.T) {
	dataSource := &MediasourceDataSource{}
	client := vergeio.NewClient("test.example.com", "testuser", "testpass", true)
	
	req := datasource.ConfigureRequest{
		ProviderData: client,
	}
	resp := &datasource.ConfigureResponse{}

	dataSource.Configure(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Errorf("expected no errors, got: %v", resp.Diagnostics.Errors())
	}

	if dataSource.mediasourceApi == nil {
		t.Error("mediasourceApi should be configured")
	}

	if dataSource.mediasourceApi.Name() != "Mediasource Api" {
		t.Errorf("expected mediasourceApi name 'Mediasource Api', got '%s'", dataSource.mediasourceApi.Name())
	}
}

func TestMediasourceDataSource_Configure_WithInvalidClient(t *testing.T) {
	dataSource := &MediasourceDataSource{}
	
	req := datasource.ConfigureRequest{
		ProviderData: "invalid",
	}
	resp := &datasource.ConfigureResponse{}

	dataSource.Configure(context.Background(), req, resp)

	if !resp.Diagnostics.HasError() {
		t.Error("expected error for invalid client type")
	}

	if dataSource.mediasourceApi != nil {
		t.Error("mediasourceApi should not be configured with invalid client")
	}
}

func TestMediasourceDataSource_Configure_WithNilClient(t *testing.T) {
	dataSource := &MediasourceDataSource{}
	
	req := datasource.ConfigureRequest{
		ProviderData: nil,
	}
	resp := &datasource.ConfigureResponse{}

	dataSource.Configure(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Errorf("expected no errors with nil client, got: %v", resp.Diagnostics.Errors())
	}

	if dataSource.mediasourceApi != nil {
		t.Error("mediasourceApi should not be configured with nil client")
	}
}

func TestMediasourceModel_Types(t *testing.T) {
	model := &MediasourceModel{
		Id:          types.Int32Value(123),
		Name:        types.StringValue("test-media"),
		Description: types.StringValue("test description"),
		Filesize:    types.Int64Value(1048576),
	}

	if model.Id.ValueInt32() != 123 {
		t.Errorf("expected Id 123, got %d", model.Id.ValueInt32())
	}

	if model.Name.ValueString() != "test-media" {
		t.Errorf("expected Name 'test-media', got '%s'", model.Name.ValueString())
	}

	if model.Description.ValueString() != "test description" {
		t.Errorf("expected Description 'test description', got '%s'", model.Description.ValueString())
	}

	if model.Filesize.ValueInt64() != 1048576 {
		t.Errorf("expected Filesize 1048576, got %d", model.Filesize.ValueInt64())
	}
}

func TestMediasourceDataSourceModel_FilterHandling(t *testing.T) {
	model := &MediasourceDataSourceModel{
		FilterName:   types.StringValue("ubuntu-20.04"),
		Mediasources: []*MediasourceModel{},
	}

	if model.FilterName.ValueString() != "ubuntu-20.04" {
		t.Errorf("expected FilterName 'ubuntu-20.04', got '%s'", model.FilterName.ValueString())
	}

	if len(model.Mediasources) != 0 {
		t.Errorf("expected empty Mediasources slice, got %d items", len(model.Mediasources))
	}

	// Test null values
	nullModel := &MediasourceDataSourceModel{
		FilterName:   types.StringNull(),
		Mediasources: []*MediasourceModel{},
	}

	if !nullModel.FilterName.IsNull() {
		t.Error("expected FilterName to be null")
	}
}

// Acceptance Tests
func TestAccMediasourceDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccMediasourceDataSourceConfig(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.vergeio_mediasources.test", "mediasources.#"),
				),
			},
		},
	})
}

func TestAccMediasourceDataSource_WithFilter(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccMediasourceDataSourceConfigWithFilter(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.vergeio_mediasources.test", "filter_name", "ubuntu"),
					// mediasources.# might be 0 if no ubuntu mediasources exist, so don't require it to be set
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
  TF_ACC=1 go test ./internal/provider/mediasource -v -run=TestAccMediasourceDataSource
`)
	}

	t.Logf("Acceptance test will connect to VergeOS at: %s (user: %s)", host, username)
}

// testProvider creates a minimal provider just for testing mediasource data source
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
		NewMediasourceDataSource,
	}
}

func (p *testProvider) Resources(ctx context.Context) []func() fwresource.Resource {
	return []func() fwresource.Resource{}
}

var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"vergeio": providerserver.NewProtocol6WithError(&testProvider{}),
}

func testAccMediasourceDataSourceConfig() string {
	host := os.Getenv("TF_ACC_VERGEIO_HOST")
	username := os.Getenv("TF_ACC_VERGEIO_USERNAME")
	password := os.Getenv("TF_ACC_VERGEIO_PASSWORD")
	
	// If no environment variables, return minimal config that will be caught by PreCheck
	if host == "" || username == "" || password == "" {
		return `
provider "vergeio" {
  # Environment variables required for acceptance testing
}
data "vergeio_mediasources" "test" {}
`
	}
	
	return `
provider "vergeio" {
  host     = "` + host + `"
  username = "` + username + `"
  password = "` + password + `"
  insecure = true
}
data "vergeio_mediasources" "test" {}
`
}

func testAccMediasourceDataSourceConfigWithFilter() string {
	host := os.Getenv("TF_ACC_VERGEIO_HOST")
	username := os.Getenv("TF_ACC_VERGEIO_USERNAME")
	password := os.Getenv("TF_ACC_VERGEIO_PASSWORD")
	
	// If no environment variables, return minimal config that will be caught by PreCheck
	if host == "" || username == "" || password == "" {
		return `
provider "vergeio" {
  # Environment variables required for acceptance testing
}
data "vergeio_mediasources" "test" {
  filter_name = "ubuntu"
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
data "vergeio_mediasources" "test" {
  filter_name = "ubuntu"
}
`
}