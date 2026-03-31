package vm

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
func TestVMDataSource(t *testing.T) {
	dataSource := NewVMDataSource()
	if dataSource == nil {
		t.Fatal("data source should not be nil")
	}
}

func TestVMDataSource_Metadata(t *testing.T) {
	dataSource := NewVMDataSource()
	req := datasource.MetadataRequest{
		ProviderTypeName: "vergeio",
	}
	resp := &datasource.MetadataResponse{}

	dataSource.Metadata(context.Background(), req, resp)

	if resp.TypeName != "vergeio_vms" {
		t.Errorf("expected TypeName 'vergeio_vms', got '%s'", resp.TypeName)
	}
}

func TestVMDataSource_Schema(t *testing.T) {
	dataSource := NewVMDataSource()
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

	// Check is_snapshot attribute
	if isSnapshot, ok := resp.Schema.Attributes["is_snapshot"]; !ok {
		t.Error("is_snapshot attribute should exist")
	} else if !isSnapshot.IsOptional() {
		t.Error("is_snapshot should be optional")
	}

	// Check vms attribute
	if vms, ok := resp.Schema.Attributes["vms"]; !ok {
		t.Error("vms attribute should exist")
	} else if !vms.IsComputed() {
		t.Error("vms should be computed")
	}

	// Check schema description
	if resp.Schema.MarkdownDescription != "VM data source" {
		t.Errorf("expected description 'VM data source', got '%s'", resp.Schema.MarkdownDescription)
	}
}

func TestVMDataSource_Configure_WithValidClient(t *testing.T) {
	dataSource := &VMDataSource{}
	client := vergeio.NewClient("test.example.com", "testuser", "testpass", true)
	
	req := datasource.ConfigureRequest{
		ProviderData: client,
	}
	resp := &datasource.ConfigureResponse{}

	dataSource.Configure(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Errorf("expected no errors, got: %v", resp.Diagnostics.Errors())
	}

	if dataSource.vmApi == nil {
		t.Error("vmApi should be configured")
	}

	if dataSource.vmApi.Name() != "VM Api" {
		t.Errorf("expected vmApi name 'VM Api', got '%s'", dataSource.vmApi.Name())
	}
}

func TestVMDataSource_Configure_WithInvalidClient(t *testing.T) {
	dataSource := &VMDataSource{}
	
	req := datasource.ConfigureRequest{
		ProviderData: "invalid",
	}
	resp := &datasource.ConfigureResponse{}

	dataSource.Configure(context.Background(), req, resp)

	if !resp.Diagnostics.HasError() {
		t.Error("expected error for invalid client type")
	}

	if dataSource.vmApi != nil {
		t.Error("vmApi should not be configured with invalid client")
	}
}

func TestVMDataSource_Configure_WithNilClient(t *testing.T) {
	dataSource := &VMDataSource{}
	
	req := datasource.ConfigureRequest{
		ProviderData: nil,
	}
	resp := &datasource.ConfigureResponse{}

	dataSource.Configure(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Errorf("expected no errors with nil client, got: %v", resp.Diagnostics.Errors())
	}

	if dataSource.vmApi != nil {
		t.Error("vmApi should not be configured with nil client")
	}
}

func TestVMModel_Types(t *testing.T) {
	model := &VMModel{
		Id:          types.Int32Value(123),
		Name:        types.StringValue("test-vm"),
		Key:         types.Int32Value(456),
		IsSnapshot:  types.BoolValue(false),
		CPUType:     types.StringValue("x86_64"),
		MachineType: types.StringValue("pc"),
		OSFamily:    types.StringValue("Linux"),
		UEFI:        types.BoolValue(true),
		Drives:      []*VMDriveModel{},
		Nics:        []*VMNicModel{},
	}

	if model.Id.ValueInt32() != 123 {
		t.Errorf("expected Id 123, got %d", model.Id.ValueInt32())
	}

	if model.Name.ValueString() != "test-vm" {
		t.Errorf("expected Name 'test-vm', got '%s'", model.Name.ValueString())
	}

	if model.Key.ValueInt32() != 456 {
		t.Errorf("expected Key 456, got %d", model.Key.ValueInt32())
	}

	if model.IsSnapshot.ValueBool() {
		t.Error("expected IsSnapshot to be false")
	}

	if model.CPUType.ValueString() != "x86_64" {
		t.Errorf("expected CPUType 'x86_64', got '%s'", model.CPUType.ValueString())
	}

	if !model.UEFI.ValueBool() {
		t.Error("expected UEFI to be true")
	}
}

func TestVMDataSourceModel_FilterHandling(t *testing.T) {
	model := &VMDataSourceModel{
		FilterName: types.StringValue("test-server"),
		IsSnapshot: types.BoolValue(false),
		Vms:        []*VMModel{},
	}

	if model.FilterName.ValueString() != "test-server" {
		t.Errorf("expected FilterName 'test-server', got '%s'", model.FilterName.ValueString())
	}

	if model.IsSnapshot.ValueBool() {
		t.Error("expected IsSnapshot to be false")
	}

	if len(model.Vms) != 0 {
		t.Errorf("expected empty Vms slice, got %d items", len(model.Vms))
	}

	// Test null values
	nullModel := &VMDataSourceModel{
		FilterName: types.StringNull(),
		IsSnapshot: types.BoolNull(),
		Vms:        []*VMModel{},
	}

	if !nullModel.FilterName.IsNull() {
		t.Error("expected FilterName to be null")
	}

	if !nullModel.IsSnapshot.IsNull() {
		t.Error("expected IsSnapshot to be null")
	}
}

// Acceptance Tests
func TestAccVMDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccVMDataSourceConfig(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.vergeio_vms.test", "vms.#"),
				),
			},
		},
	})
}

func TestAccVMDataSource_WithFilter(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccVMDataSourceConfigWithFilter(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.vergeio_vms.test", "filter_name", "test-vm"),
					// vms.# might be 0 if no test-vm exists, so don't require it to be set
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
  TF_ACC=1 go test ./internal/provider/vm -v -run=TestAccVMDataSource
`)
	}

	t.Logf("Acceptance test will connect to VergeOS at: %s (user: %s)", host, username)
}

// testProvider creates a minimal provider just for testing vm data source
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
		NewVMDataSource,
	}
}

func (p *testProvider) Resources(ctx context.Context) []func() fwresource.Resource {
	return []func() fwresource.Resource{}
}

var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"vergeio": providerserver.NewProtocol6WithError(&testProvider{}),
}

func testAccVMDataSourceConfig() string {
	host := os.Getenv("TF_ACC_VERGEIO_HOST")
	username := os.Getenv("TF_ACC_VERGEIO_USERNAME")
	password := os.Getenv("TF_ACC_VERGEIO_PASSWORD")
	
	// If no environment variables, return minimal config that will be caught by PreCheck
	if host == "" || username == "" || password == "" {
		return `
provider "vergeio" {
  # Environment variables required for acceptance testing
}
data "vergeio_vms" "test" {}
`
	}
	
	return `
provider "vergeio" {
  host     = "` + host + `"
  username = "` + username + `"
  password = "` + password + `"
  insecure = true
}
data "vergeio_vms" "test" {}
`
}

func testAccVMDataSourceConfigWithFilter() string {
	host := os.Getenv("TF_ACC_VERGEIO_HOST")
	username := os.Getenv("TF_ACC_VERGEIO_USERNAME")
	password := os.Getenv("TF_ACC_VERGEIO_PASSWORD")
	
	// If no environment variables, return minimal config that will be caught by PreCheck
	if host == "" || username == "" || password == "" {
		return `
provider "vergeio" {
  # Environment variables required for acceptance testing
}
data "vergeio_vms" "test" {
  filter_name = "test-vm"
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
data "vergeio_vms" "test" {
  filter_name = "test-vm"
}
`
}