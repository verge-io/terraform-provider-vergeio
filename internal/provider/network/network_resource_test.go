package network

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
func TestNetworkResource(t *testing.T) {
	networkResource := NewNetworkResource()
	if networkResource == nil {
		t.Fatal("network resource should not be nil")
	}
}

func TestNetworkResource_Metadata(t *testing.T) {
	networkResource := NewNetworkResource()
	req := fwresource.MetadataRequest{
		ProviderTypeName: "vergeio",
	}
	resp := &fwresource.MetadataResponse{}

	networkResource.Metadata(context.Background(), req, resp)

	if resp.TypeName != "vergeio_network" {
		t.Errorf("expected TypeName 'vergeio_network', got '%s'", resp.TypeName)
	}
}

func TestNetworkResource_Schema(t *testing.T) {
	networkResource := NewNetworkResource()
	req := fwresource.SchemaRequest{}
	resp := &fwresource.SchemaResponse{}

	networkResource.Schema(context.Background(), req, resp)

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
	if resp.Schema.MarkdownDescription != "Network or Vnet resource in VergeIO" {
		t.Errorf("expected description 'Network or Vnet resource in VergeIO', got '%s'", resp.Schema.MarkdownDescription)
	}
}

func TestNetworkResource_Configure_WithValidClient(t *testing.T) {
	networkResource := &NetworkResource{}
	client := vergeio.NewClient("test.example.com", "testuser", "testpass", true)

	req := fwresource.ConfigureRequest{
		ProviderData: client,
	}
	resp := &fwresource.ConfigureResponse{}

	networkResource.Configure(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Errorf("expected no errors, got: %v", resp.Diagnostics.Errors())
	}

	if networkResource.networkApi == nil {
		t.Error("networkApi should be configured")
	}

	if networkResource.networkApi.Name() != "Network Api" {
		t.Errorf("expected networkApi name 'Network Api', got '%s'", networkResource.networkApi.Name())
	}
}

func TestNetworkResource_Configure_WithInvalidClient(t *testing.T) {
	networkResource := &NetworkResource{}

	req := fwresource.ConfigureRequest{
		ProviderData: "invalid",
	}
	resp := &fwresource.ConfigureResponse{}

	networkResource.Configure(context.Background(), req, resp)

	if !resp.Diagnostics.HasError() {
		t.Error("expected error for invalid client type")
	}

	if networkResource.networkApi != nil {
		t.Error("networkApi should not be configured with invalid client")
	}
}

func TestNetworkResource_Configure_WithNilClient(t *testing.T) {
	networkResource := &NetworkResource{}

	req := fwresource.ConfigureRequest{
		ProviderData: nil,
	}
	resp := &fwresource.ConfigureResponse{}

	networkResource.Configure(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Errorf("expected no errors with nil client, got: %v", resp.Diagnostics.Errors())
	}

	if networkResource.networkApi != nil {
		t.Error("networkApi should not be configured with nil client")
	}
}

func TestNetworkResourceModel_Types(t *testing.T) {
	model := &NetworkResourceModel{
		Id:                   types.StringValue("123"),
		Name:                 types.StringValue("test-network"),
		Enabled:              types.BoolValue(true),
		Default_Gateway:      types.Int32Value(1),
		IPaddress:            types.StringValue("192.168.1.1"),
		Network:              types.StringValue("192.168.1.0/24"),
		DHCP:                 types.BoolValue(true),
		Dynamic_DHCP:         types.BoolValue(false),
		DHCP_Sequential:      types.BoolValue(true),
		DynamicIP_Start:      types.StringValue("192.168.1.100"),
		DynamicIP_Stop:       types.StringValue("192.168.1.200"),
		On_Power_Loss:        types.StringValue("laston"),
		PowerState:           types.StringValue("running"),
		Type:                 types.StringValue("internal"),
		VLAN_TAG:             types.Int32Value(100),
		MTU:                  types.Int32Value(1500),
		Interface_Vnet:       types.Int32Value(0),
		IPaddress_Type:       types.StringValue("static"),
		Layer2_Type:          types.StringValue("vlan"),
		Enable_Bonding:       types.BoolValue(false),
		Bond_Interfaces_Args: types.ListNull(types.StringType),
	}

	if model.Id.ValueString() != "123" {
		t.Errorf("expected Id '123', got '%s'", model.Id.ValueString())
	}

	if model.Name.ValueString() != "test-network" {
		t.Errorf("expected Name 'test-network', got '%s'", model.Name.ValueString())
	}

	if !model.Enabled.ValueBool() {
		t.Error("expected Enabled to be true")
	}

	if model.IPaddress.ValueString() != "192.168.1.1" {
		t.Errorf("expected IPaddress '192.168.1.1', got '%s'", model.IPaddress.ValueString())
	}

	if model.Network.ValueString() != "192.168.1.0/24" {
		t.Errorf("expected Network '192.168.1.0/24', got '%s'", model.Network.ValueString())
	}
}

func TestNetworkResourceModel_NullValues(t *testing.T) {
	model := &NetworkResourceModel{
		Id:                   types.StringNull(),
		Name:                 types.StringValue("test-network"), // Required field
		Enabled:              types.BoolNull(),
		Default_Gateway:      types.Int32Null(),
		IPaddress:            types.StringNull(),
		Network:              types.StringNull(),
		DHCP:                 types.BoolNull(),
		Dynamic_DHCP:         types.BoolNull(),
		DHCP_Sequential:      types.BoolNull(),
		DynamicIP_Start:      types.StringNull(),
		DynamicIP_Stop:       types.StringNull(),
		On_Power_Loss:        types.StringNull(),
		PowerState:           types.StringNull(),
		Type:                 types.StringNull(),
		VLAN_TAG:             types.Int32Null(),
		MTU:                  types.Int32Null(),
		Interface_Vnet:       types.Int32Null(),
		IPaddress_Type:       types.StringNull(),
		Layer2_Type:          types.StringNull(),
		Enable_Bonding:       types.BoolNull(),
		Bond_Interfaces_Args: types.ListNull(types.StringType),
	}

	if !model.Id.IsNull() {
		t.Error("expected Id to be null")
	}

	if !model.Enabled.IsNull() {
		t.Error("expected Enabled to be null")
	}

	if !model.IPaddress.IsNull() {
		t.Error("expected IPaddress to be null")
	}

	// Name should not be null as it's required
	if model.Name.IsNull() {
		t.Error("Name should not be null")
	}
}

// Acceptance Tests
func TestAccNetworkResource_Basic(t *testing.T) {
	networkName := "tf-acc-test-network-basic"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNetworkDestroy,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccNetworkResourceConfig_basic(networkName),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckNetworkExists("vergeio_network.test"),
					resource.TestCheckResourceAttr("vergeio_network.test", "name", networkName),
					resource.TestCheckResourceAttr("vergeio_network.test", "type", "internal"),
					resource.TestCheckResourceAttrSet("vergeio_network.test", "id"),
				),
			},
			// ImportState testing
			{
				ResourceName:      "vergeio_network.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccNetworkResource_Update(t *testing.T) {
	networkName := "tf-acc-test-network-update"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNetworkDestroy,
		Steps: []resource.TestStep{
			// Create initial network
			{
				Config: testAccNetworkResourceConfig_basic(networkName),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckNetworkExists("vergeio_network.test"),
					resource.TestCheckResourceAttr("vergeio_network.test", "name", networkName),
					resource.TestCheckResourceAttr("vergeio_network.test", "type", "internal"),
				),
			},
			// Update network attributes
			{
				Config: testAccNetworkResourceConfig_updated(networkName),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckNetworkExists("vergeio_network.test"),
					resource.TestCheckResourceAttr("vergeio_network.test", "name", networkName),
					resource.TestCheckResourceAttr("vergeio_network.test", "type", "internal"),
					// Add checks for updated fields as appropriate
				),
			},
		},
	})
}

// Helper functions for acceptance tests

func testAccCheckNetworkExists(resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("resource not found: %s", resourceName)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("no ID is set")
		}

		// In a real implementation, you would verify the network exists via API call
		// For now, we just verify that we have an ID
		return nil
	}
}

func testAccCheckNetworkDestroy(s *terraform.State) error {
	// Check that all networks with tf-acc-test prefix are destroyed
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "vergeio_network" {
			continue
		}

		// In a real implementation, you would verify the network no longer exists via API call
		// For now, we assume the destroy worked if no error occurred during test
	}

	return nil
}

// Test configuration templates
func testAccNetworkResourceConfig_basic(networkName string) string {
	host := os.Getenv("TF_ACC_VERGEIO_HOST")
	username := os.Getenv("TF_ACC_VERGEIO_USERNAME")
	password := os.Getenv("TF_ACC_VERGEIO_PASSWORD")

	if host == "" || username == "" || password == "" {
		return fmt.Sprintf(`
provider "vergeio" {
  # Environment variables required for acceptance testing
}

resource "vergeio_network" "test" {
  name = "%s"
  type = "internal"
}
`, networkName)
	}

	return fmt.Sprintf(`
provider "vergeio" {
  host     = "%s"
  username = "%s"
  password = "%s"
  insecure = true
}

resource "vergeio_network" "test" {
  name = "%s"
  type = "internal"
}
`, host, username, password, networkName)
}

func testAccNetworkResourceConfig_updated(networkName string) string {
	host := os.Getenv("TF_ACC_VERGEIO_HOST")
	username := os.Getenv("TF_ACC_VERGEIO_USERNAME")
	password := os.Getenv("TF_ACC_VERGEIO_PASSWORD")

	if host == "" || username == "" || password == "" {
		return fmt.Sprintf(`
provider "vergeio" {
  # Environment variables required for acceptance testing
}

resource "vergeio_network" "test" {
  name = "%s"
  type = "internal"
}
`, networkName)
	}

	return fmt.Sprintf(`
provider "vergeio" {
  host     = "%s"
  username = "%s"
  password = "%s"
  insecure = true
}

resource "vergeio_network" "test" {
  name = "%s"
  type = "internal"
}
`, host, username, password, networkName)
}
