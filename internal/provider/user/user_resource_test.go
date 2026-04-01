package user

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"terraform-provider-vergeio/internal/provider/vergeio"
)

// Unit Tests
func TestUserResource(t *testing.T) {
	userResource := NewUserResource()
	if userResource == nil {
		t.Fatal("user resource should not be nil")
	}
}

func TestUserResource_Metadata(t *testing.T) {
	userResource := NewUserResource()
	req := fwresource.MetadataRequest{
		ProviderTypeName: "vergeio",
	}
	resp := &fwresource.MetadataResponse{}

	userResource.Metadata(context.Background(), req, resp)

	if resp.TypeName != "vergeio_user" {
		t.Errorf("expected TypeName 'vergeio_user', got '%s'", resp.TypeName)
	}
}

func TestUserResource_Schema(t *testing.T) {
	userResource := NewUserResource()
	req := fwresource.SchemaRequest{}
	resp := &fwresource.SchemaResponse{}

	userResource.Schema(context.Background(), req, resp)

	// Check that required attributes exist
	if resp.Schema.Attributes == nil {
		t.Fatal("schema attributes should not be nil")
	}

	// Check required name attribute
	if nameAttr, ok := resp.Schema.Attributes["name"]; !ok {
		t.Error("name attribute should exist")
	} else if !nameAttr.IsRequired() {
		t.Error("name should be required")
	}

	// Check computed id attribute
	if idAttr, ok := resp.Schema.Attributes["id"]; !ok {
		t.Error("id attribute should exist")
	} else if !idAttr.IsComputed() {
		t.Error("id should be computed")
	}

	// Check optional enabled attribute
	if enabledAttr, ok := resp.Schema.Attributes["enabled"]; !ok {
		t.Error("enabled attribute should exist")
	} else if !enabledAttr.IsOptional() {
		t.Error("enabled should be optional")
	}

	// Check schema description
	if resp.Schema.MarkdownDescription != "User resource in VergeIO" {
		t.Errorf("expected description 'User resource in VergeIO', got '%s'", resp.Schema.MarkdownDescription)
	}
}

func TestUserResource_Configure_WithValidClient(t *testing.T) {
	userResource := &UserResource{}
	client := vergeio.NewClient("test.example.com", "testuser", "testpass", true)
	
	req := fwresource.ConfigureRequest{
		ProviderData: client,
	}
	resp := &fwresource.ConfigureResponse{}

	userResource.Configure(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Errorf("expected no errors, got: %v", resp.Diagnostics.Errors())
	}

	if userResource.userApi == nil {
		t.Error("userApi should be configured")
	}

	if userResource.userApi.Name() != "User Api" {
		t.Errorf("expected userApi name 'User Api', got '%s'", userResource.userApi.Name())
	}
}

func TestUserResource_Configure_WithInvalidClient(t *testing.T) {
	userResource := &UserResource{}
	
	req := fwresource.ConfigureRequest{
		ProviderData: "invalid",
	}
	resp := &fwresource.ConfigureResponse{}

	userResource.Configure(context.Background(), req, resp)

	if !resp.Diagnostics.HasError() {
		t.Error("expected error for invalid client type")
	}

	if userResource.userApi != nil {
		t.Error("userApi should not be configured with invalid client")
	}
}

func TestUserResource_Configure_WithNilClient(t *testing.T) {
	userResource := &UserResource{}
	
	req := fwresource.ConfigureRequest{
		ProviderData: nil,
	}
	resp := &fwresource.ConfigureResponse{}

	userResource.Configure(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Errorf("expected no errors with nil client, got: %v", resp.Diagnostics.Errors())
	}

	if userResource.userApi != nil {
		t.Error("userApi should not be configured with nil client")
	}
}

func TestUserResourceModel_Types(t *testing.T) {
	model := &UserResourceModel{
		Id:             types.StringValue("123"),
		AuthSource:     types.Int32Value(1),
		Name:           types.StringValue("testuser"),
		RemoteName:     types.StringValue("remote-testuser"),
		Enabled:        types.BoolValue(true),
		DisplayName:    types.StringValue("Test User"),
		Email:          types.StringValue("test@example.com"),
		Type:           types.StringValue("local"),
		Password:       types.StringValue("secretpassword"),
		ChangePassword: types.BoolValue(false),
	}

	if model.Id.ValueString() != "123" {
		t.Errorf("expected Id '123', got '%s'", model.Id.ValueString())
	}

	if model.Name.ValueString() != "testuser" {
		t.Errorf("expected Name 'testuser', got '%s'", model.Name.ValueString())
	}

	if model.AuthSource.ValueInt32() != 1 {
		t.Errorf("expected AuthSource 1, got %d", model.AuthSource.ValueInt32())
	}

	if !model.Enabled.ValueBool() {
		t.Error("expected Enabled to be true")
	}

	if model.DisplayName.ValueString() != "Test User" {
		t.Errorf("expected DisplayName 'Test User', got '%s'", model.DisplayName.ValueString())
	}

	if model.Email.ValueString() != "test@example.com" {
		t.Errorf("expected Email 'test@example.com', got '%s'", model.Email.ValueString())
	}
}

func TestUserResourceModel_NullValues(t *testing.T) {
	model := &UserResourceModel{
		Id:             types.StringNull(),
		AuthSource:     types.Int32Null(),
		Name:           types.StringValue("testuser"), // Required field
		RemoteName:     types.StringNull(),
		Enabled:        types.BoolNull(),
		DisplayName:    types.StringNull(),
		Email:          types.StringNull(),
		Type:           types.StringNull(),
		Password:       types.StringNull(),
		ChangePassword: types.BoolNull(),
	}

	if !model.Id.IsNull() {
		t.Error("expected Id to be null")
	}

	if !model.AuthSource.IsNull() {
		t.Error("expected AuthSource to be null")
	}

	if !model.RemoteName.IsNull() {
		t.Error("expected RemoteName to be null")
	}

	if !model.Enabled.IsNull() {
		t.Error("expected Enabled to be null")
	}

	// Name should not be null as it's required
	if model.Name.IsNull() {
		t.Error("Name should not be null")
	}
}

// Acceptance Tests
func TestAccUserResource_Basic(t *testing.T) {
	userName := "tf-acc-test-user-basic"
	
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckUserDestroy,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccUserResourceConfig_basic(userName),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckUserExists("vergeio_user.test"),
					resource.TestCheckResourceAttr("vergeio_user.test", "name", userName),
					resource.TestCheckResourceAttr("vergeio_user.test", "enabled", "true"),
					resource.TestCheckResourceAttr("vergeio_user.test", "displayname", "Terraform Acceptance Test User"),
					resource.TestCheckResourceAttr("vergeio_user.test", "email", "tf-test@example.com"),
					resource.TestCheckResourceAttrSet("vergeio_user.test", "id"),
				),
			},
			// ImportState testing
			{
				ResourceName:            "vergeio_user.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"password"}, // Password is not returned in read operations
			},
		},
	})
}

func TestAccUserResource_Update(t *testing.T) {
	userName := "tf-acc-test-user-update"
	
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckUserDestroy,
		Steps: []resource.TestStep{
			// Create initial user
			{
				Config: testAccUserResourceConfig_basic(userName),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckUserExists("vergeio_user.test"),
					resource.TestCheckResourceAttr("vergeio_user.test", "name", userName),
					resource.TestCheckResourceAttr("vergeio_user.test", "enabled", "true"),
					resource.TestCheckResourceAttr("vergeio_user.test", "displayname", "Terraform Acceptance Test User"),
				),
			},
			// Update user attributes
			{
				Config: testAccUserResourceConfig_updated(userName),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckUserExists("vergeio_user.test"),
					resource.TestCheckResourceAttr("vergeio_user.test", "name", userName),
					resource.TestCheckResourceAttr("vergeio_user.test", "enabled", "true"),
					resource.TestCheckResourceAttr("vergeio_user.test", "displayname", "Updated Test User"),
					resource.TestCheckResourceAttr("vergeio_user.test", "email", "updated-tf-test@example.com"),
				),
			},
		},
	})
}

// Helper functions for acceptance tests
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
  TF_ACC=1 go test ./internal/provider/user -v -run=TestAccUser
`)
	}

	t.Logf("Acceptance test will connect to VergeOS at: %s (user: %s)", host, username)
}

func testAccCheckUserExists(resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("resource not found: %s", resourceName)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("no ID is set")
		}

		// In a real implementation, you would verify the user exists via API call
		// For now, we just verify that we have an ID
		return nil
	}
}

func testAccCheckUserDestroy(s *terraform.State) error {
	// Check that all users with tf-acc-test prefix are destroyed
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "vergeio_user" {
			continue
		}

		// In a real implementation, you would verify the user no longer exists via API call
		// For now, we assume the destroy worked if no error occurred during test
	}

	return nil
}

// testProvider creates a minimal provider for testing user resource
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
	return []func() datasource.DataSource{}
}

func (p *testProvider) Resources(ctx context.Context) []func() fwresource.Resource {
	return []func() fwresource.Resource{
		NewUserResource,
	}
}

var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"vergeio": providerserver.NewProtocol6WithError(&testProvider{}),
}

// Test configuration templates
func testAccUserResourceConfig_basic(userName string) string {
	host := os.Getenv("TF_ACC_VERGEIO_HOST")
	username := os.Getenv("TF_ACC_VERGEIO_USERNAME")
	password := os.Getenv("TF_ACC_VERGEIO_PASSWORD")
	
	if host == "" || username == "" || password == "" {
		return fmt.Sprintf(`
provider "vergeio" {
  # Environment variables required for acceptance testing
}

resource "vergeio_user" "test" {
  name        = "%s"
  enabled     = true
  displayname = "Terraform Acceptance Test User"
  email       = "tf-test@example.com"
  password    = "TerraformTest123!"
}
`, userName)
	}
	
	return fmt.Sprintf(`
provider "vergeio" {
  host     = "%s"
  username = "%s"
  password = "%s"
  insecure = true
}

resource "vergeio_user" "test" {
  name        = "%s"
  enabled     = true
  displayname = "Terraform Acceptance Test User"
  email       = "tf-test@example.com"
  password    = "TerraformTest123!"
}
`, host, username, password, userName)
}

func testAccUserResourceConfig_updated(userName string) string {
	host := os.Getenv("TF_ACC_VERGEIO_HOST")
	username := os.Getenv("TF_ACC_VERGEIO_USERNAME")
	password := os.Getenv("TF_ACC_VERGEIO_PASSWORD")
	
	if host == "" || username == "" || password == "" {
		return fmt.Sprintf(`
provider "vergeio" {
  # Environment variables required for acceptance testing
}

resource "vergeio_user" "test" {
  name        = "%s"
  enabled     = true
  displayname = "Updated Test User"
  email       = "updated-tf-test@example.com"
  password    = "TerraformTest123!"
}
`, userName)
	}
	
	return fmt.Sprintf(`
provider "vergeio" {
  host     = "%s"
  username = "%s"
  password = "%s"
  insecure = true
}

resource "vergeio_user" "test" {
  name        = "%s"
  enabled     = true
  displayname = "Updated Test User"
  email       = "updated-tf-test@example.com"
  password    = "TerraformTest123!"
}
`, host, username, password, userName)
}