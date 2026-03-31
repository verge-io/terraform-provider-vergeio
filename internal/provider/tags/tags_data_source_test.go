package tags

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
func TestTagsDataSource(t *testing.T) {
	dataSource := NewTagsDataSource()
	if dataSource == nil {
		t.Fatal("data source should not be nil")
	}
}

func TestTagsDataSource_Metadata(t *testing.T) {
	dataSource := NewTagsDataSource()
	req := datasource.MetadataRequest{
		ProviderTypeName: "vergeio",
	}
	resp := &datasource.MetadataResponse{}

	dataSource.Metadata(context.Background(), req, resp)

	if resp.TypeName != "vergeio_tags" {
		t.Errorf("expected TypeName 'vergeio_tags', got '%s'", resp.TypeName)
	}
}

func TestTagsDataSource_Schema(t *testing.T) {
	dataSource := NewTagsDataSource()
	req := datasource.SchemaRequest{}
	resp := &datasource.SchemaResponse{}

	dataSource.Schema(context.Background(), req, resp)

	// Check that required attributes exist
	if resp.Schema.Attributes == nil {
		t.Fatal("schema attributes should not be nil")
	}

	// Check filter attribute
	if filter, ok := resp.Schema.Attributes["filter"]; !ok {
		t.Error("filter attribute should exist")
	} else if !filter.IsOptional() {
		t.Error("filter should be optional")
	}

	// Check category_filter attribute
	if categoryFilter, ok := resp.Schema.Attributes["category_filter"]; !ok {
		t.Error("category_filter attribute should exist")
	} else if !categoryFilter.IsOptional() {
		t.Error("category_filter should be optional")
	}

	// Check category_name attribute
	if categoryName, ok := resp.Schema.Attributes["category_name"]; !ok {
		t.Error("category_name attribute should exist")
	} else if !categoryName.IsOptional() {
		t.Error("category_name should be optional")
	}

	// Check tags attribute
	if tags, ok := resp.Schema.Attributes["tags"]; !ok {
		t.Error("tags attribute should exist")
	} else if !tags.IsComputed() {
		t.Error("tags should be computed")
	}

	// Check schema description
	expected := "Tags data source to retrieve tag information from VergeOS. Supports filtering by name and/or category to handle duplicate tag names across categories."
	if resp.Schema.MarkdownDescription != expected {
		t.Errorf("expected description '%s', got '%s'", expected, resp.Schema.MarkdownDescription)
	}
}

func TestTagsDataSource_Configure_WithValidClient(t *testing.T) {
	dataSource := &TagsDataSource{}
	client := vergeio.NewClient("test.example.com", "testuser", "testpass", true)
	
	req := datasource.ConfigureRequest{
		ProviderData: client,
	}
	resp := &datasource.ConfigureResponse{}

	dataSource.Configure(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Errorf("expected no errors, got: %v", resp.Diagnostics.Errors())
	}

	if dataSource.tagsApi == nil {
		t.Error("tagsApi should be configured")
	}

	if dataSource.tagsApi.Name() != "Tags Api" {
		t.Errorf("expected tagsApi name 'Tags Api', got '%s'", dataSource.tagsApi.Name())
	}
}

func TestTagsDataSource_Configure_WithInvalidClient(t *testing.T) {
	dataSource := &TagsDataSource{}
	
	req := datasource.ConfigureRequest{
		ProviderData: "invalid",
	}
	resp := &datasource.ConfigureResponse{}

	dataSource.Configure(context.Background(), req, resp)

	if !resp.Diagnostics.HasError() {
		t.Error("expected error for invalid client type")
	}

	if dataSource.tagsApi != nil {
		t.Error("tagsApi should not be configured with invalid client")
	}
}

func TestTagsDataSource_Configure_WithNilClient(t *testing.T) {
	dataSource := &TagsDataSource{}
	
	req := datasource.ConfigureRequest{
		ProviderData: nil,
	}
	resp := &datasource.ConfigureResponse{}

	dataSource.Configure(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Errorf("expected no errors with nil client, got: %v", resp.Diagnostics.Errors())
	}

	if dataSource.tagsApi != nil {
		t.Error("tagsApi should not be configured with nil client")
	}
}

func TestTagModel_Types(t *testing.T) {
	model := &TagModel{
		Key:          types.Int32Value(123),
		Name:         types.StringValue("test-tag"),
		Category:     types.Int32Value(456),
		CategoryName: types.StringValue("test-category"),
	}

	if model.Key.ValueInt32() != 123 {
		t.Errorf("expected Key 123, got %d", model.Key.ValueInt32())
	}

	if model.Name.ValueString() != "test-tag" {
		t.Errorf("expected Name 'test-tag', got '%s'", model.Name.ValueString())
	}

	if model.Category.ValueInt32() != 456 {
		t.Errorf("expected Category 456, got %d", model.Category.ValueInt32())
	}

	if model.CategoryName.ValueString() != "test-category" {
		t.Errorf("expected CategoryName 'test-category', got '%s'", model.CategoryName.ValueString())
	}
}

func TestTagsDataSourceModel_FilterHandling(t *testing.T) {
	model := &TagsDataSourceModel{
		Filter:         types.StringValue("production"),
		CategoryFilter: types.Int32Value(123),
		CategoryName:   types.StringValue("environment"),
		Tags:           []TagModel{},
	}

	if model.Filter.ValueString() != "production" {
		t.Errorf("expected Filter 'production', got '%s'", model.Filter.ValueString())
	}

	if model.CategoryFilter.ValueInt32() != 123 {
		t.Errorf("expected CategoryFilter 123, got %d", model.CategoryFilter.ValueInt32())
	}

	if model.CategoryName.ValueString() != "environment" {
		t.Errorf("expected CategoryName 'environment', got '%s'", model.CategoryName.ValueString())
	}

	if len(model.Tags) != 0 {
		t.Errorf("expected empty Tags slice, got %d items", len(model.Tags))
	}

	// Test null values
	nullModel := &TagsDataSourceModel{
		Filter:         types.StringNull(),
		CategoryFilter: types.Int32Null(),
		CategoryName:   types.StringNull(),
		Tags:           []TagModel{},
	}

	if !nullModel.Filter.IsNull() {
		t.Error("expected Filter to be null")
	}

	if !nullModel.CategoryFilter.IsNull() {
		t.Error("expected CategoryFilter to be null")
	}

	if !nullModel.CategoryName.IsNull() {
		t.Error("expected CategoryName to be null")
	}
}

// Acceptance Tests
func TestAccTagsDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccTagsDataSourceConfig(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.vergeio_tags.test", "tags.#"),
				),
			},
		},
	})
}

func TestAccTagsDataSource_WithFilter(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccTagsDataSourceConfigWithFilter(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.vergeio_tags.test", "filter", "production"),
					// tags.# might be 0 if no production tags exist, so don't require it to be set
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
  TF_ACC=1 go test ./internal/provider/tags -v -run=TestAccTagsDataSource
`)
	}

	t.Logf("Acceptance test will connect to VergeOS at: %s (user: %s)", host, username)
}

// testProvider creates a minimal provider just for testing tags data source
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
		NewTagsDataSource,
	}
}

func (p *testProvider) Resources(ctx context.Context) []func() fwresource.Resource {
	return []func() fwresource.Resource{}
}

var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"vergeio": providerserver.NewProtocol6WithError(&testProvider{}),
}

func testAccTagsDataSourceConfig() string {
	host := os.Getenv("TF_ACC_VERGEIO_HOST")
	username := os.Getenv("TF_ACC_VERGEIO_USERNAME")
	password := os.Getenv("TF_ACC_VERGEIO_PASSWORD")
	
	// If no environment variables, return minimal config that will be caught by PreCheck
	if host == "" || username == "" || password == "" {
		return `
provider "vergeio" {
  # Environment variables required for acceptance testing
}
data "vergeio_tags" "test" {}
`
	}
	
	return `
provider "vergeio" {
  host     = "` + host + `"
  username = "` + username + `"
  password = "` + password + `"
  insecure = true
}
data "vergeio_tags" "test" {}
`
}

func testAccTagsDataSourceConfigWithFilter() string {
	host := os.Getenv("TF_ACC_VERGEIO_HOST")
	username := os.Getenv("TF_ACC_VERGEIO_USERNAME")
	password := os.Getenv("TF_ACC_VERGEIO_PASSWORD")
	
	// If no environment variables, return minimal config that will be caught by PreCheck
	if host == "" || username == "" || password == "" {
		return `
provider "vergeio" {
  # Environment variables required for acceptance testing
}
data "vergeio_tags" "test" {
  filter = "production"
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
data "vergeio_tags" "test" {
  filter = "production"
}
`
}