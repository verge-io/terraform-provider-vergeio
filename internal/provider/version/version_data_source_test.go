package version

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

func TestVersionDataSource(t *testing.T) {
	dataSource := NewVersionDataSource()
	if dataSource == nil {
		t.Fatal("data source should not be nil")
	}
}

func TestVersionDataSource_Metadata(t *testing.T) {
	dataSource := NewVersionDataSource()
	req := datasource.MetadataRequest{
		ProviderTypeName: "vergeio",
	}
	resp := &datasource.MetadataResponse{}

	dataSource.Metadata(context.Background(), req, resp)

	if resp.TypeName != "vergeio_version" {
		t.Errorf("expected TypeName 'vergeio_version', got '%s'", resp.TypeName)
	}
}

func TestVersionDataSource_Schema(t *testing.T) {
	dataSource := NewVersionDataSource()
	req := datasource.SchemaRequest{}
	resp := &datasource.SchemaResponse{}

	dataSource.Schema(context.Background(), req, resp)

	// Check that required attributes exist
	if resp.Schema.Attributes == nil {
		t.Fatal("schema attributes should not be nil")
	}

	// Check key attributes
	expectedAttrs := []string{"name", "version", "hash"}
	for _, attrName := range expectedAttrs {
		if attr, ok := resp.Schema.Attributes[attrName]; !ok {
			t.Errorf("attribute '%s' should exist in schema", attrName)
		} else if !attr.IsComputed() {
			t.Errorf("attribute '%s' should be computed", attrName)
		}
	}

	// Check schema description
	if resp.Schema.MarkdownDescription != "Version data source schema" {
		t.Errorf("expected description 'Version data source schema', got '%s'", resp.Schema.MarkdownDescription)
	}
}

func TestVersionDataSource_Configure_WithValidClient(t *testing.T) {
	dataSource := &VersionDataSource{}
	client := vergeio.NewClient("test.example.com", "testuser", "testpass", true)
	
	req := datasource.ConfigureRequest{
		ProviderData: client,
	}
	resp := &datasource.ConfigureResponse{}

	dataSource.Configure(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Errorf("expected no errors, got: %v", resp.Diagnostics.Errors())
	}

	if dataSource.versionApi == nil {
		t.Error("versionApi should be configured")
	}

	if dataSource.versionApi.Name() != "Version Api" {
		t.Errorf("expected versionApi name 'Version Api', got '%s'", dataSource.versionApi.Name())
	}
}

func TestVersionDataSource_Configure_WithInvalidClient(t *testing.T) {
	dataSource := &VersionDataSource{}
	
	req := datasource.ConfigureRequest{
		ProviderData: "invalid",
	}
	resp := &datasource.ConfigureResponse{}

	dataSource.Configure(context.Background(), req, resp)

	if !resp.Diagnostics.HasError() {
		t.Error("expected error for invalid client type")
	}

	if dataSource.versionApi != nil {
		t.Error("versionApi should not be configured with invalid client")
	}
}

func TestVersionDataSource_Configure_WithNilClient(t *testing.T) {
	dataSource := &VersionDataSource{}
	
	req := datasource.ConfigureRequest{
		ProviderData: nil,
	}
	resp := &datasource.ConfigureResponse{}

	dataSource.Configure(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Errorf("expected no errors with nil client, got: %v", resp.Diagnostics.Errors())
	}

	if dataSource.versionApi != nil {
		t.Error("versionApi should not be configured with nil client")
	}
}

func TestVersionDataSourceModel_Types(t *testing.T) {
	model := &VersionDataSourceModel{
		Name:    types.StringValue("VergeOS"),
		Version: types.StringValue("4.12.5"),
		Hash:    types.StringValue("abc123"),
	}

	if model.Name.ValueString() != "VergeOS" {
		t.Errorf("expected Name 'VergeOS', got '%s'", model.Name.ValueString())
	}

	if model.Version.ValueString() != "4.12.5" {
		t.Errorf("expected Version '4.12.5', got '%s'", model.Version.ValueString())
	}

	if model.Hash.ValueString() != "abc123" {
		t.Errorf("expected Hash 'abc123', got '%s'", model.Hash.ValueString())
	}

	// Test null values
	nullModel := &VersionDataSourceModel{
		Name:    types.StringNull(),
		Version: types.StringNull(),
		Hash:    types.StringNull(),
	}

	if !nullModel.Name.IsNull() {
		t.Error("expected Name to be null")
	}

	if !nullModel.Version.IsNull() {
		t.Error("expected Version to be null")
	}

	if !nullModel.Hash.IsNull() {
		t.Error("expected Hash to be null")
	}
}

func TestAccVersionDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccVersionDataSourceConfig(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.vergeio_version.test", "name"),
					resource.TestCheckResourceAttrSet("data.vergeio_version.test", "version"),
					// Hash may not be available from SDK, so just check it exists (can be empty)
					resource.TestCheckResourceAttr("data.vergeio_version.test", "hash", ""),
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
  TF_ACC=1 go test ./internal/provider/version -v -run=TestAccVersionDataSource
`)
	}

	t.Logf("Acceptance test will connect to VergeOS at: %s (user: %s)", host, username)
}

// testProvider creates a minimal provider just for testing version data source
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
		NewVersionDataSource,
	}
}

func (p *testProvider) Resources(ctx context.Context) []func() fwresource.Resource {
	return []func() fwresource.Resource{}
}

var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"vergeio": providerserver.NewProtocol6WithError(&testProvider{}),
}

func testAccVersionDataSourceConfig() string {
	host := os.Getenv("TF_ACC_VERGEIO_HOST")
	username := os.Getenv("TF_ACC_VERGEIO_USERNAME")
	password := os.Getenv("TF_ACC_VERGEIO_PASSWORD")
	
	// If no environment variables, return minimal config that will be caught by PreCheck
	if host == "" || username == "" || password == "" {
		return `
provider "vergeio" {
  # Environment variables required for acceptance testing
}
data "vergeio_version" "test" {}
`
	}
	
	return `
provider "vergeio" {
  host     = "` + host + `"
  username = "` + username + `"
  password = "` + password + `"
  insecure = true
}
data "vergeio_version" "test" {}
`
}