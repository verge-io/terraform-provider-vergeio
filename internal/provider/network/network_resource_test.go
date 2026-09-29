package network

import (
	"context"
	"testing"

	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

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

	// powerstate is read back after import, including when the config omits it.
	if powerAttr, ok := resp.Schema.Attributes["powerstate"]; !ok {
		t.Error("powerstate attribute should exist")
	} else if !powerAttr.IsOptional() || !powerAttr.IsComputed() {
		t.Error("powerstate should be optional and computed")
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
