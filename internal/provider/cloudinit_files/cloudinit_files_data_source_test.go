package cloudinitFile

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
func TestCloudinitFileDataSource(t *testing.T) {
	dataSource := NewCloudinitFileDataSource()
	if dataSource == nil {
		t.Fatal("data source should not be nil")
	}
}

func TestCloudinitFileDataSource_Metadata(t *testing.T) {
	dataSource := NewCloudinitFileDataSource()
	req := datasource.MetadataRequest{
		ProviderTypeName: "vergeio",
	}
	resp := &datasource.MetadataResponse{}

	dataSource.Metadata(context.Background(), req, resp)

	if resp.TypeName != "vergeio_cloudinit_files" {
		t.Errorf("expected TypeName 'vergeio_cloudinit_files', got '%s'", resp.TypeName)
	}
}

func TestCloudinitFileDataSource_Schema(t *testing.T) {
	dataSource := NewCloudinitFileDataSource()
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

	// Check cloudinit_files attribute
	if cloudinitFiles, ok := resp.Schema.Attributes["cloudinit_files"]; !ok {
		t.Error("cloudinit_files attribute should exist")
	} else if !cloudinitFiles.IsComputed() {
		t.Error("cloudinit_files should be computed")
	}

	// Check schema description
	if resp.Schema.MarkdownDescription != "CloudinitFile data source schema" {
		t.Errorf("expected description 'CloudinitFile data source schema', got '%s'", resp.Schema.MarkdownDescription)
	}
}

func TestCloudinitFileDataSource_Configure_WithValidClient(t *testing.T) {
	dataSource := &CloudinitFileDataSource{}
	client := vergeio.NewClient("test.example.com", "testuser", "testpass", true)
	
	req := datasource.ConfigureRequest{
		ProviderData: client,
	}
	resp := &datasource.ConfigureResponse{}

	dataSource.Configure(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Errorf("expected no errors, got: %v", resp.Diagnostics.Errors())
	}

	if dataSource.cloudinitFileApi == nil {
		t.Error("cloudinitFileApi should be configured")
	}

	if dataSource.cloudinitFileApi.Name() != "CloudinitFile Api" {
		t.Errorf("expected cloudinitFileApi name 'CloudinitFile Api', got '%s'", dataSource.cloudinitFileApi.Name())
	}
}

func TestCloudinitFileDataSource_Configure_WithInvalidClient(t *testing.T) {
	dataSource := &CloudinitFileDataSource{}
	
	req := datasource.ConfigureRequest{
		ProviderData: "invalid",
	}
	resp := &datasource.ConfigureResponse{}

	dataSource.Configure(context.Background(), req, resp)

	if !resp.Diagnostics.HasError() {
		t.Error("expected error for invalid client type")
	}

	if dataSource.cloudinitFileApi != nil {
		t.Error("cloudinitFileApi should not be configured with invalid client")
	}
}

func TestCloudinitFileDataSource_Configure_WithNilClient(t *testing.T) {
	dataSource := &CloudinitFileDataSource{}
	
	req := datasource.ConfigureRequest{
		ProviderData: nil,
	}
	resp := &datasource.ConfigureResponse{}

	dataSource.Configure(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Errorf("expected no errors with nil client, got: %v", resp.Diagnostics.Errors())
	}

	if dataSource.cloudinitFileApi != nil {
		t.Error("cloudinitFileApi should not be configured with nil client")
	}
}

func TestCloudinitFileModel_Types(t *testing.T) {
	model := &CloudinitFileModel{
		Id:                types.StringValue("123"),
		Name:              types.StringValue("test-cloudinit"),
		Filesize:          types.Int64Value(1024),
		Contents:          types.StringValue("test contents"),
		ContainsVariables: types.BoolValue(true),
	}

	if model.Id.ValueString() != "123" {
		t.Errorf("expected Id '123', got '%s'", model.Id.ValueString())
	}

	if model.Name.ValueString() != "test-cloudinit" {
		t.Errorf("expected Name 'test-cloudinit', got '%s'", model.Name.ValueString())
	}

	if model.Filesize.ValueInt64() != 1024 {
		t.Errorf("expected Filesize 1024, got %d", model.Filesize.ValueInt64())
	}

	if !model.ContainsVariables.ValueBool() {
		t.Error("expected ContainsVariables to be true")
	}
}

func TestCloudinitFileDataSourceModel_FilterHandling(t *testing.T) {
	model := &CloudinitFileDataSourceModel{
		FilterName:     types.StringValue("ubuntu-setup"),
		CloudinitFiles: []*CloudinitFileModel{},
	}

	if model.FilterName.ValueString() != "ubuntu-setup" {
		t.Errorf("expected FilterName 'ubuntu-setup', got '%s'", model.FilterName.ValueString())
	}

	if len(model.CloudinitFiles) != 0 {
		t.Errorf("expected empty CloudinitFiles slice, got %d items", len(model.CloudinitFiles))
	}

	// Test null values
	nullModel := &CloudinitFileDataSourceModel{
		FilterName:     types.StringNull(),
		CloudinitFiles: []*CloudinitFileModel{},
	}

	if !nullModel.FilterName.IsNull() {
		t.Error("expected FilterName to be null")
	}
}

// Acceptance Tests
func TestAccCloudinitFilesDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccCloudinitFilesDataSourceConfig(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.vergeio_cloudinit_files.test", "cloudinit_files.#"),
				),
			},
		},
	})
}

func TestAccCloudinitFilesDataSource_WithFilter(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccCloudinitFilesDataSourceConfigWithFilter(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.vergeio_cloudinit_files.test", "filter_name", "ubuntu"),
					// cloudinit_files.# might be 0 if no ubuntu files exist, so don't require it to be set
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
  TF_ACC=1 go test ./internal/provider/cloudinit_files -v -run=TestAccCloudinitFilesDataSource
`)
	}

	t.Logf("Acceptance test will connect to VergeOS at: %s (user: %s)", host, username)
}

// testProvider creates a minimal provider just for testing cloudinit_files data source
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
		NewCloudinitFileDataSource,
	}
}

func (p *testProvider) Resources(ctx context.Context) []func() fwresource.Resource {
	return []func() fwresource.Resource{
		NewCloudinitFileResource,
	}
}

var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"vergeio": providerserver.NewProtocol6WithError(&testProvider{}),
}

func testAccCloudinitFilesDataSourceConfig() string {
	host := os.Getenv("TF_ACC_VERGEIO_HOST")
	username := os.Getenv("TF_ACC_VERGEIO_USERNAME")
	password := os.Getenv("TF_ACC_VERGEIO_PASSWORD")
	
	// If no environment variables, return minimal config that will be caught by PreCheck
	if host == "" || username == "" || password == "" {
		return `
provider "vergeio" {
  # Environment variables required for acceptance testing
}
data "vergeio_cloudinit_files" "test" {}
`
	}
	
	return `
provider "vergeio" {
  host     = "` + host + `"
  username = "` + username + `"
  password = "` + password + `"
  insecure = true
}
data "vergeio_cloudinit_files" "test" {}
`
}

func testAccCloudinitFilesDataSourceConfigWithFilter() string {
	host := os.Getenv("TF_ACC_VERGEIO_HOST")
	username := os.Getenv("TF_ACC_VERGEIO_USERNAME")
	password := os.Getenv("TF_ACC_VERGEIO_PASSWORD")
	
	// If no environment variables, return minimal config that will be caught by PreCheck
	if host == "" || username == "" || password == "" {
		return `
provider "vergeio" {
  # Environment variables required for acceptance testing
}
data "vergeio_cloudinit_files" "test" {
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
data "vergeio_cloudinit_files" "test" {
  filter_name = "ubuntu"
}
`
}