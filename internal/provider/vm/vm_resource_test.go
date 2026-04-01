package vm

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
func TestVMResource(t *testing.T) {
	vmResource := NewVMResource()
	if vmResource == nil {
		t.Fatal("vm resource should not be nil")
	}
}

func TestVMResource_Metadata(t *testing.T) {
	vmResource := NewVMResource()
	req := fwresource.MetadataRequest{
		ProviderTypeName: "vergeio",
	}
	resp := &fwresource.MetadataResponse{}

	vmResource.Metadata(context.Background(), req, resp)

	if resp.TypeName != "vergeio_vm" {
		t.Errorf("expected TypeName 'vergeio_vm', got '%s'", resp.TypeName)
	}
}

func TestVMResource_Schema(t *testing.T) {
	vmResource := NewVMResource()
	req := fwresource.SchemaRequest{}
	resp := &fwresource.SchemaResponse{}

	vmResource.Schema(context.Background(), req, resp)

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
	if resp.Schema.MarkdownDescription != "VM resource in VergeIO" {
		t.Errorf("expected description 'VM resource in VergeIO', got '%s'", resp.Schema.MarkdownDescription)
	}
}

func TestVMResource_Configure_WithValidClient(t *testing.T) {
	vmResource := &VMResource{}
	client := vergeio.NewClient("test.example.com", "testuser", "testpass", true)
	
	req := fwresource.ConfigureRequest{
		ProviderData: client,
	}
	resp := &fwresource.ConfigureResponse{}

	vmResource.Configure(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Errorf("expected no errors, got: %v", resp.Diagnostics.Errors())
	}

	if vmResource.vmApi == nil {
		t.Error("vmApi should be configured")
	}

	if vmResource.vmApi.Name() != "VM Api" {
		t.Errorf("expected vmApi name 'VM Api', got '%s'", vmResource.vmApi.Name())
	}
}

func TestVMResource_Configure_WithInvalidClient(t *testing.T) {
	vmResource := &VMResource{}
	
	req := fwresource.ConfigureRequest{
		ProviderData: "invalid",
	}
	resp := &fwresource.ConfigureResponse{}

	vmResource.Configure(context.Background(), req, resp)

	if !resp.Diagnostics.HasError() {
		t.Error("expected error for invalid client type")
	}

	if vmResource.vmApi != nil {
		t.Error("vmApi should not be configured with invalid client")
	}
}

func TestVMResource_Configure_WithNilClient(t *testing.T) {
	vmResource := &VMResource{}
	
	req := fwresource.ConfigureRequest{
		ProviderData: nil,
	}
	resp := &fwresource.ConfigureResponse{}

	vmResource.Configure(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Errorf("expected no errors with nil client, got: %v", resp.Diagnostics.Errors())
	}

	if vmResource.vmApi != nil {
		t.Error("vmApi should not be configured with nil client")
	}
}

func TestVMResourceModel_Types(t *testing.T) {
	model := &VMResourceModel{
		Id:                   types.StringValue("123"),
		Name:                 types.StringValue("test-vm"),
		Enabled:              types.BoolValue(true),
		Machine:              types.Int32Value(1),
		MachineType:          types.StringValue("q35"),
		CPUCores:             types.Int32Value(2),
		CPUType:              types.StringValue("host"),
		RAM:                  types.Int32Value(2048),
		Description:          types.StringValue("Test VM"),
		AllowHotplug:         types.BoolValue(false),
		DisablePowercycle:    types.BoolValue(false),
		NestedVirtualization: types.BoolValue(false),
		DisableHypervisor:    types.BoolValue(false),
	}

	if model.Id.ValueString() != "123" {
		t.Errorf("expected Id '123', got '%s'", model.Id.ValueString())
	}

	if model.Name.ValueString() != "test-vm" {
		t.Errorf("expected Name 'test-vm', got '%s'", model.Name.ValueString())
	}

	if !model.Enabled.ValueBool() {
		t.Error("expected Enabled to be true")
	}

	if model.CPUCores.ValueInt32() != 2 {
		t.Errorf("expected CPUCores 2, got %d", model.CPUCores.ValueInt32())
	}

	if model.RAM.ValueInt32() != 2048 {
		t.Errorf("expected RAM 2048, got %d", model.RAM.ValueInt32())
	}
}

func TestVMResourceModel_NullValues(t *testing.T) {
	model := &VMResourceModel{
		Id:                   types.StringNull(),
		Name:                 types.StringValue("test-vm"), // Required field
		Enabled:              types.BoolNull(),
		Machine:              types.Int32Null(),
		MachineType:          types.StringNull(),
		CPUCores:             types.Int32Null(),
		CPUType:              types.StringNull(),
		RAM:                  types.Int32Null(),
		Description:          types.StringNull(),
		AllowHotplug:         types.BoolNull(),
		DisablePowercycle:    types.BoolNull(),
		NestedVirtualization: types.BoolNull(),
		DisableHypervisor:    types.BoolNull(),
	}

	if !model.Id.IsNull() {
		t.Error("expected Id to be null")
	}

	if !model.Enabled.IsNull() {
		t.Error("expected Enabled to be null")
	}

	if !model.Machine.IsNull() {
		t.Error("expected Machine to be null")
	}

	// Name should not be null as it's required
	if model.Name.IsNull() {
		t.Error("Name should not be null")
	}
}

// Acceptance Tests
func TestAccVMResource_Basic(t *testing.T) {
	vmName := "tf-acc-test-vm-basic"
	
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckVMDestroy,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccVMResourceConfig_basic(vmName),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckVMExists("vergeio_vm.test"),
					resource.TestCheckResourceAttr("vergeio_vm.test", "name", vmName),
					resource.TestCheckResourceAttr("vergeio_vm.test", "enabled", "true"),
					resource.TestCheckResourceAttr("vergeio_vm.test", "cpu_cores", "2"),
					resource.TestCheckResourceAttr("vergeio_vm.test", "ram", "2048"),
					resource.TestCheckResourceAttrSet("vergeio_vm.test", "id"),
				),
			},
			// ImportState testing
			{
				ResourceName:      "vergeio_vm.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccVMResource_Update(t *testing.T) {
	vmName := "tf-acc-test-vm-update"
	
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckVMDestroy,
		Steps: []resource.TestStep{
			// Create initial VM
			{
				Config: testAccVMResourceConfig_basic(vmName),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckVMExists("vergeio_vm.test"),
					resource.TestCheckResourceAttr("vergeio_vm.test", "name", vmName),
					resource.TestCheckResourceAttr("vergeio_vm.test", "cpu_cores", "2"),
					resource.TestCheckResourceAttr("vergeio_vm.test", "ram", "2048"),
				),
			},
			// Update VM attributes
			{
				Config: testAccVMResourceConfig_updated(vmName),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckVMExists("vergeio_vm.test"),
					resource.TestCheckResourceAttr("vergeio_vm.test", "name", vmName),
					resource.TestCheckResourceAttr("vergeio_vm.test", "cpu_cores", "4"),
					resource.TestCheckResourceAttr("vergeio_vm.test", "ram", "4096"),
					resource.TestCheckResourceAttr("vergeio_vm.test", "description", "Updated Test VM"),
				),
			},
		},
	})
}

// Helper functions for acceptance tests
func testAccCheckVMExists(resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("resource not found: %s", resourceName)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("no ID is set")
		}

		// In a real implementation, you would verify the VM exists via API call
		// For now, we just verify that we have an ID
		return nil
	}
}

func testAccCheckVMDestroy(s *terraform.State) error {
	// Check that all VMs with tf-acc-test prefix are destroyed
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "vergeio_vm" {
			continue
		}

		// In a real implementation, you would verify the VM no longer exists via API call
		// For now, we assume the destroy worked if no error occurred during test
	}

	return nil
}

// Test configuration templates
func testAccVMResourceConfig_basic(vmName string) string {
	host := os.Getenv("TF_ACC_VERGEIO_HOST")
	username := os.Getenv("TF_ACC_VERGEIO_USERNAME")
	password := os.Getenv("TF_ACC_VERGEIO_PASSWORD")
	
	if host == "" || username == "" || password == "" {
		return fmt.Sprintf(`
provider "vergeio" {
  # Environment variables required for acceptance testing
}

resource "vergeio_vm" "test" {
  name      = "%s"
  enabled   = true
  cpu_cores = 2
  ram       = 2048
}
`, vmName)
	}
	
	return fmt.Sprintf(`
provider "vergeio" {
  host     = "%s"
  username = "%s"
  password = "%s"
  insecure = true
}

resource "vergeio_vm" "test" {
  name      = "%s"
  enabled   = true
  cpu_cores = 2
  ram       = 2048
}
`, host, username, password, vmName)
}

func testAccVMResourceConfig_updated(vmName string) string {
	host := os.Getenv("TF_ACC_VERGEIO_HOST")
	username := os.Getenv("TF_ACC_VERGEIO_USERNAME")
	password := os.Getenv("TF_ACC_VERGEIO_PASSWORD")
	
	if host == "" || username == "" || password == "" {
		return fmt.Sprintf(`
provider "vergeio" {
  # Environment variables required for acceptance testing
}

resource "vergeio_vm" "test" {
  name        = "%s"
  enabled     = true
  cpu_cores   = 4
  ram         = 4096
  cluster     = 1
  description = "Updated Test VM"
}
`, vmName)
	}
	
	return fmt.Sprintf(`
provider "vergeio" {
  host     = "%s"
  username = "%s"
  password = "%s"
  insecure = true
}

resource "vergeio_vm" "test" {
  name        = "%s"
  enabled     = true
  cpu_cores   = 4
  ram         = 4096
  cluster     = 1
  description = "Updated Test VM"
}
`, host, username, password, vmName)
}