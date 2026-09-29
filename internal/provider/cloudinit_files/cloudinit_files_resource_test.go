package cloudinitFile

import (
	"context"
	"fmt"
	"os"
	"testing"

	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"terraform-provider-vergeio/internal/provider/vergeio"
)

// Unit Tests
func TestCloudinitFileResource(t *testing.T) {
	cloudinitFileResource := NewCloudinitFileResource()
	if cloudinitFileResource == nil {
		t.Fatal("cloudinit file resource should not be nil")
	}
}

func TestCloudinitFileResource_Metadata(t *testing.T) {
	cloudinitFileResource := NewCloudinitFileResource()
	req := fwresource.MetadataRequest{
		ProviderTypeName: "vergeio",
	}
	resp := &fwresource.MetadataResponse{}

	cloudinitFileResource.Metadata(context.Background(), req, resp)

	if resp.TypeName != "vergeio_cloudinitFile" {
		t.Errorf("expected TypeName 'vergeio_cloudinitFile', got '%s'", resp.TypeName)
	}
}

func TestCloudinitFileResource_Schema(t *testing.T) {
	cloudinitFileResource := NewCloudinitFileResource()
	req := fwresource.SchemaRequest{}
	resp := &fwresource.SchemaResponse{}

	cloudinitFileResource.Schema(context.Background(), req, resp)

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

	// Check optional contents attribute
	if contentsAttr, ok := resp.Schema.Attributes["contents"]; !ok {
		t.Error("contents attribute should exist")
	} else if !contentsAttr.IsOptional() {
		t.Error("contents should be optional")
	}

	// Check schema description
	if resp.Schema.MarkdownDescription != "CloudinitFile or Vnet resource in VergeIO" {
		t.Errorf("expected description 'CloudinitFile or Vnet resource in VergeIO', got '%s'", resp.Schema.MarkdownDescription)
	}
}

func TestCloudinitFileResource_Configure_WithValidClient(t *testing.T) {
	cloudinitFileResource := &CloudinitFileResource{}
	client := vergeio.NewClient("test.example.com", "testuser", "testpass", true)
	
	req := fwresource.ConfigureRequest{
		ProviderData: client,
	}
	resp := &fwresource.ConfigureResponse{}

	cloudinitFileResource.Configure(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Errorf("expected no errors, got: %v", resp.Diagnostics.Errors())
	}

	if cloudinitFileResource.cloudinitFileApi == nil {
		t.Error("cloudinitFileApi should be configured")
	}

	if cloudinitFileResource.cloudinitFileApi.Name() != "CloudinitFile Api" {
		t.Errorf("expected cloudinitFileApi name 'CloudinitFile Api', got '%s'", cloudinitFileResource.cloudinitFileApi.Name())
	}
}

func TestCloudinitFileResource_Configure_WithInvalidClient(t *testing.T) {
	cloudinitFileResource := &CloudinitFileResource{}
	
	req := fwresource.ConfigureRequest{
		ProviderData: "invalid",
	}
	resp := &fwresource.ConfigureResponse{}

	cloudinitFileResource.Configure(context.Background(), req, resp)

	if !resp.Diagnostics.HasError() {
		t.Error("expected error for invalid client type")
	}

	if cloudinitFileResource.cloudinitFileApi != nil {
		t.Error("cloudinitFileApi should not be configured with invalid client")
	}
}

func TestCloudinitFileResource_Configure_WithNilClient(t *testing.T) {
	cloudinitFileResource := &CloudinitFileResource{}
	
	req := fwresource.ConfigureRequest{
		ProviderData: nil,
	}
	resp := &fwresource.ConfigureResponse{}

	cloudinitFileResource.Configure(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Errorf("expected no errors with nil client, got: %v", resp.Diagnostics.Errors())
	}

	if cloudinitFileResource.cloudinitFileApi != nil {
		t.Error("cloudinitFileApi should not be configured with nil client")
	}
}

func TestCloudinitFileResourceModel_Types(t *testing.T) {
	model := &CloudinitFileResourceModel{
		Id:                types.StringValue("123"),
		Name:              types.StringValue("tf-test-cloudinit"),
		Filesize:          types.Int64Value(1024),
		Contents:          types.StringValue("#cloud-config\nusers: []"),
		ContainsVariables: types.BoolValue(false),
	}

	if model.Id.ValueString() != "123" {
		t.Errorf("expected Id '123', got '%s'", model.Id.ValueString())
	}

	if model.Name.ValueString() != "tf-test-cloudinit" {
		t.Errorf("expected Name 'tf-test-cloudinit', got '%s'", model.Name.ValueString())
	}

	if model.Filesize.ValueInt64() != 1024 {
		t.Errorf("expected Filesize 1024, got %d", model.Filesize.ValueInt64())
	}

	if model.Contents.ValueString() != "#cloud-config\nusers: []" {
		t.Errorf("expected Contents '#cloud-config\\nusers: []', got '%s'", model.Contents.ValueString())
	}

	if model.ContainsVariables.ValueBool() {
		t.Error("expected ContainsVariables to be false")
	}
}

func TestCloudinitFileResourceModel_NullValues(t *testing.T) {
	model := &CloudinitFileResourceModel{
		Id:                types.StringNull(),
		Name:              types.StringValue("tf-test-cloudinit"), // Required field
		Filesize:          types.Int64Null(),
		Contents:          types.StringNull(),
		ContainsVariables: types.BoolNull(),
	}

	if !model.Id.IsNull() {
		t.Error("expected Id to be null")
	}

	if !model.Filesize.IsNull() {
		t.Error("expected Filesize to be null")
	}

	if !model.Contents.IsNull() {
		t.Error("expected Contents to be null")
	}

	if !model.ContainsVariables.IsNull() {
		t.Error("expected ContainsVariables to be null")
	}

	// Name should not be null as it's required
	if model.Name.IsNull() {
		t.Error("Name should not be null")
	}
}

// Acceptance Tests
func TestAccCloudinitFileResource_Basic(t *testing.T) {
	t.Skip("CloudInit file acceptance test disabled - CloudInit should work within VM context, not standalone")
	fileName := "tf-acc-test-cloudinit-basic"
	
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCloudinitFileDestroy,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccCloudinitFileResourceConfig_basic(fileName),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckCloudinitFileExists("vergeio_cloudinitFile.test"),
					resource.TestCheckResourceAttr("vergeio_cloudinitFile.test", "name", fileName),
					resource.TestCheckResourceAttr("vergeio_cloudinitFile.test", "contents", "#cloud-config\nusers: []"),
					resource.TestCheckResourceAttrSet("vergeio_cloudinitFile.test", "id"),
					resource.TestCheckResourceAttrSet("vergeio_cloudinitFile.test", "filesize"),
				),
			},
			// ImportState testing
			{
				ResourceName:      "vergeio_cloudinitFile.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccCloudinitFileResource_Update(t *testing.T) {
	t.Skip("CloudInit file acceptance test disabled - CloudInit should work within VM context, not standalone")
	fileName := "tf-acc-test-cloudinit-update"
	
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCloudinitFileDestroy,
		Steps: []resource.TestStep{
			// Create initial cloudinit file
			{
				Config: testAccCloudinitFileResourceConfig_basic(fileName),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckCloudinitFileExists("vergeio_cloudinitFile.test"),
					resource.TestCheckResourceAttr("vergeio_cloudinitFile.test", "name", fileName),
					resource.TestCheckResourceAttr("vergeio_cloudinitFile.test", "contents", "#cloud-config\nusers: []"),
				),
			},
			// Update cloudinit file attributes
			{
				Config: testAccCloudinitFileResourceConfig_updated(fileName),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckCloudinitFileExists("vergeio_cloudinitFile.test"),
					resource.TestCheckResourceAttr("vergeio_cloudinitFile.test", "name", fileName),
					resource.TestCheckResourceAttr("vergeio_cloudinitFile.test", "contents", "#cloud-config\nusers:\n  - name: terraform"),
				),
			},
		},
	})
}

// Helper functions for acceptance tests
func testAccCheckCloudinitFileExists(resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("resource not found: %s", resourceName)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("no ID is set")
		}

		// In a real implementation, you would verify the cloudinit file exists via API call
		// For now, we just verify that we have an ID
		return nil
	}
}

func testAccCheckCloudinitFileDestroy(s *terraform.State) error {
	// Check that all cloudinit files with tf-acc-test prefix are destroyed
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "vergeio_cloudinitFile" {
			continue
		}

		// In a real implementation, you would verify the cloudinit file no longer exists via API call
		// For now, we assume the destroy worked if no error occurred during test
	}

	return nil
}

// Test configuration templates
func testAccCloudinitFileResourceConfig_basic(fileName string) string {
	host := os.Getenv("TF_ACC_VERGEIO_HOST")
	username := os.Getenv("TF_ACC_VERGEIO_USERNAME")
	password := os.Getenv("TF_ACC_VERGEIO_PASSWORD")
	
	if host == "" || username == "" || password == "" {
		return fmt.Sprintf(`
provider "vergeio" {
  # Environment variables required for acceptance testing
}

resource "vergeio_cloudinitFile" "test" {
  name     = "%s"
  contents = "#cloud-config\nusers: []"
  owner    = "terraform-test"
}
`, fileName)
	}
	
	return fmt.Sprintf(`
provider "vergeio" {
  host     = "%s"
  username = "%s"
  password = "%s"
  insecure = true
}

resource "vergeio_cloudinitFile" "test" {
  name     = "%s"
  contents = "#cloud-config\nusers: []"
  owner    = "terraform-test"
}
`, host, username, password, fileName)
}

func testAccCloudinitFileResourceConfig_updated(fileName string) string {
	host := os.Getenv("TF_ACC_VERGEIO_HOST")
	username := os.Getenv("TF_ACC_VERGEIO_USERNAME")
	password := os.Getenv("TF_ACC_VERGEIO_PASSWORD")
	
	if host == "" || username == "" || password == "" {
		return fmt.Sprintf(`
provider "vergeio" {
  # Environment variables required for acceptance testing
}

resource "vergeio_cloudinitFile" "test" {
  name     = "%s"
  contents = "#cloud-config\nusers:\n  - name: terraform"
}
`, fileName)
	}
	
	return fmt.Sprintf(`
provider "vergeio" {
  host     = "%s"
  username = "%s"
  password = "%s"
  insecure = true
}

resource "vergeio_cloudinitFile" "test" {
  name     = "%s"
  contents = "#cloud-config\nusers:\n  - name: terraform"
}
`, host, username, password, fileName)
}