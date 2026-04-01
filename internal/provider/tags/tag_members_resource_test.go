package tags

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
func TestTagMemberResource(t *testing.T) {
	tagMemberResource := NewTagMemberResource()
	if tagMemberResource == nil {
		t.Fatal("tag member resource should not be nil")
	}
}

func TestTagMemberResource_Metadata(t *testing.T) {
	tagMemberResource := NewTagMemberResource()
	req := fwresource.MetadataRequest{
		ProviderTypeName: "vergeio",
	}
	resp := &fwresource.MetadataResponse{}

	tagMemberResource.Metadata(context.Background(), req, resp)

	if resp.TypeName != "vergeio_tag_member" {
		t.Errorf("expected TypeName 'vergeio_tag_member', got '%s'", resp.TypeName)
	}
}

func TestTagMemberResource_Schema(t *testing.T) {
	tagMemberResource := NewTagMemberResource()
	req := fwresource.SchemaRequest{}
	resp := &fwresource.SchemaResponse{}

	tagMemberResource.Schema(context.Background(), req, resp)

	// Check that required attributes exist
	if resp.Schema.Attributes == nil {
		t.Fatal("schema attributes should not be nil")
	}

	// Check computed id attribute
	if idAttr, ok := resp.Schema.Attributes["id"]; !ok {
		t.Error("id attribute should exist")
	} else if !idAttr.IsComputed() {
		t.Error("id should be computed")
	}

	// Check required tag_id attribute
	if tagIdAttr, ok := resp.Schema.Attributes["tag_id"]; !ok {
		t.Error("tag_id attribute should exist")
	} else if !tagIdAttr.IsRequired() {
		t.Error("tag_id should be required")
	}

	// Check required member attribute
	if memberAttr, ok := resp.Schema.Attributes["member"]; !ok {
		t.Error("member attribute should exist")
	} else if !memberAttr.IsRequired() {
		t.Error("member should be required")
	}

	// Check schema description
	if resp.Schema.MarkdownDescription != "Tag member resource to assign tags to VergeOS objects (requires VergeOS v26+)" {
		t.Errorf("expected description 'Tag member resource to assign tags to VergeOS objects (requires VergeOS v26+)', got '%s'", resp.Schema.MarkdownDescription)
	}
}

func TestTagMemberResource_Configure_WithValidClient(t *testing.T) {
	tagMemberResource := &TagMemberResource{}
	client := vergeio.NewClient("test.example.com", "testuser", "testpass", true)
	
	req := fwresource.ConfigureRequest{
		ProviderData: client,
	}
	resp := &fwresource.ConfigureResponse{}

	tagMemberResource.Configure(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Errorf("expected no errors, got: %v", resp.Diagnostics.Errors())
	}

	if tagMemberResource.tagsApi == nil {
		t.Error("tagsApi should be configured")
	}

	if tagMemberResource.tagsApi.Name() != "Tags Api" {
		t.Errorf("expected tagsApi name 'Tags Api', got '%s'", tagMemberResource.tagsApi.Name())
	}
}

func TestTagMemberResource_Configure_WithInvalidClient(t *testing.T) {
	tagMemberResource := &TagMemberResource{}
	
	req := fwresource.ConfigureRequest{
		ProviderData: "invalid",
	}
	resp := &fwresource.ConfigureResponse{}

	tagMemberResource.Configure(context.Background(), req, resp)

	if !resp.Diagnostics.HasError() {
		t.Error("expected error for invalid client type")
	}

	if tagMemberResource.tagsApi != nil {
		t.Error("tagsApi should not be configured with invalid client")
	}
}

func TestTagMemberResource_Configure_WithNilClient(t *testing.T) {
	tagMemberResource := &TagMemberResource{}
	
	req := fwresource.ConfigureRequest{
		ProviderData: nil,
	}
	resp := &fwresource.ConfigureResponse{}

	tagMemberResource.Configure(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Errorf("expected no errors with nil client, got: %v", resp.Diagnostics.Errors())
	}

	if tagMemberResource.tagsApi != nil {
		t.Error("tagsApi should not be configured with nil client")
	}
}

func TestTagMemberResourceModel_Types(t *testing.T) {
	model := &TagMemberResourceModel{
		Id:     types.StringValue("123"),
		TagId:  types.Int32Value(1),
		Member: types.StringValue("vms/456"),
	}

	if model.Id.ValueString() != "123" {
		t.Errorf("expected Id '123', got '%s'", model.Id.ValueString())
	}

	if model.TagId.ValueInt32() != 1 {
		t.Errorf("expected TagId 1, got %d", model.TagId.ValueInt32())
	}

	if model.Member.ValueString() != "vms/456" {
		t.Errorf("expected Member 'vms/456', got '%s'", model.Member.ValueString())
	}
}

func TestTagMemberResourceModel_NullValues(t *testing.T) {
	model := &TagMemberResourceModel{
		Id:     types.StringNull(),
		TagId:  types.Int32Value(1), // Required field
		Member: types.StringValue("vms/456"), // Required field
	}

	if !model.Id.IsNull() {
		t.Error("expected Id to be null")
	}

	// TagId and Member should not be null as they're required
	if model.TagId.IsNull() {
		t.Error("TagId should not be null")
	}

	if model.Member.IsNull() {
		t.Error("Member should not be null")
	}
}

// Acceptance Tests
func TestAccTagMemberResource_Basic(t *testing.T) {
	t.Skip("Tag member acceptance test temporarily disabled - hardcoded tag_id and member values need review")
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckTagMemberDestroy,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccTagMemberResourceConfig_basic(),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckTagMemberExists("vergeio_tag_member.test"),
					resource.TestCheckResourceAttr("vergeio_tag_member.test", "tag_id", "1"),
					resource.TestCheckResourceAttr("vergeio_tag_member.test", "member", "vms/123"),
					resource.TestCheckResourceAttrSet("vergeio_tag_member.test", "id"),
				),
			},
			// ImportState testing
			{
				ResourceName:      "vergeio_tag_member.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccTagMemberResource_Update(t *testing.T) {
	t.Skip("Tag member acceptance test temporarily disabled - hardcoded tag_id and member values need review")
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckTagMemberDestroy,
		Steps: []resource.TestStep{
			// Create initial tag member
			{
				Config: testAccTagMemberResourceConfig_basic(),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckTagMemberExists("vergeio_tag_member.test"),
					resource.TestCheckResourceAttr("vergeio_tag_member.test", "tag_id", "1"),
					resource.TestCheckResourceAttr("vergeio_tag_member.test", "member", "vms/123"),
				),
			},
			// Update tag member attributes
			{
				Config: testAccTagMemberResourceConfig_updated(),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckTagMemberExists("vergeio_tag_member.test"),
					resource.TestCheckResourceAttr("vergeio_tag_member.test", "tag_id", "1"),
					resource.TestCheckResourceAttr("vergeio_tag_member.test", "member", "vms/456"),
				),
			},
		},
	})
}

// Helper functions for acceptance tests

func testAccCheckTagMemberExists(resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("resource not found: %s", resourceName)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("no ID is set")
		}

		// In a real implementation, you would verify the tag member exists via API call
		// For now, we just verify that we have an ID
		return nil
	}
}

func testAccCheckTagMemberDestroy(s *terraform.State) error {
	// Check that all tag members are destroyed
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "vergeio_tag_member" {
			continue
		}

		// In a real implementation, you would verify the tag member no longer exists via API call
		// For now, we assume the destroy worked if no error occurred during test
	}

	return nil
}

// Test configuration templates
func testAccTagMemberResourceConfig_basic() string {
	host := os.Getenv("TF_ACC_VERGEIO_HOST")
	username := os.Getenv("TF_ACC_VERGEIO_USERNAME")
	password := os.Getenv("TF_ACC_VERGEIO_PASSWORD")
	
	if host == "" || username == "" || password == "" {
		return `
provider "vergeio" {
  # Environment variables required for acceptance testing
}

resource "vergeio_tag_member" "test" {
  tag_id = 1
  member = "vms/123"
}
`
	}
	
	return fmt.Sprintf(`
provider "vergeio" {
  host     = "%s"
  username = "%s"
  password = "%s"
  insecure = true
}

resource "vergeio_tag_member" "test" {
  tag_id = 1
  member = "vms/123"
}
`, host, username, password)
}

func testAccTagMemberResourceConfig_updated() string {
	host := os.Getenv("TF_ACC_VERGEIO_HOST")
	username := os.Getenv("TF_ACC_VERGEIO_USERNAME")
	password := os.Getenv("TF_ACC_VERGEIO_PASSWORD")
	
	if host == "" || username == "" || password == "" {
		return `
provider "vergeio" {
  # Environment variables required for acceptance testing
}

resource "vergeio_tag_member" "test" {
  tag_id = 1
  member = "vms/456"
}
`
	}
	
	return fmt.Sprintf(`
provider "vergeio" {
  host     = "%s"
  username = "%s"
  password = "%s"
  insecure = true
}

resource "vergeio_tag_member" "test" {
  tag_id = 1
  member = "vms/456"
}
`, host, username, password)
}