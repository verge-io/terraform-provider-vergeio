package network

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/defaults"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int32planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"terraform-provider-vergeio/internal/client"
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

	// powerstate is a bool, read back after import, including when the config omits it.
	powerAttr, ok := resp.Schema.Attributes["powerstate"].(schema.BoolAttribute)
	if !ok {
		t.Fatal("powerstate should be a bool attribute")
	}
	if !powerAttr.IsOptional() || !powerAttr.IsComputed() {
		t.Error("powerstate should be optional and computed")
	}
	if len(powerAttr.PlanModifiers) != 1 {
		t.Fatalf("powerstate should keep prior state when omitted, got %d plan modifiers", len(powerAttr.PlanModifiers))
	}
	if resp.Schema.Version != 1 {
		t.Fatalf("schema version = %d, want 1", resp.Schema.Version)
	}

	restartAttr, ok := resp.Schema.Attributes["restart_on_change"].(schema.BoolAttribute)
	if !ok {
		t.Fatal("restart_on_change should be a bool attribute")
	}
	if !restartAttr.IsOptional() || !restartAttr.IsComputed() {
		t.Error("restart_on_change should be optional and computed")
	}
	def := restartAttr.Default
	if def == nil {
		t.Fatal("restart_on_change should default to true")
	}
	defaultResp := &defaults.BoolResponse{}
	def.DefaultBool(context.Background(), defaults.BoolRequest{}, defaultResp)
	if defaultResp.Diagnostics.HasError() || defaultResp.PlanValue.IsNull() || !defaultResp.PlanValue.ValueBool() {
		t.Fatalf("restart_on_change default = %#v, diagnostics %v", defaultResp.PlanValue, defaultResp.Diagnostics)
	}

	for _, name := range []string{"description", "dnslist", "domain"} {
		attr, ok := resp.Schema.Attributes[name].(schema.StringAttribute)
		if !ok {
			t.Fatalf("%s should be a string attribute", name)
		}
		if !attr.IsOptional() || !attr.IsComputed() || attr.IsRequired() {
			t.Errorf("%s should be optional and computed", name)
		}
	}
	rateLimit, ok := resp.Schema.Attributes["rate_limit"].(schema.Int64Attribute)
	if !ok {
		t.Fatal("rate_limit should be an int64 attribute")
	}
	if !rateLimit.IsOptional() || !rateLimit.IsComputed() || rateLimit.IsRequired() {
		t.Error("rate_limit should be optional and computed")
	}

	needRestartAttr, ok := resp.Schema.Attributes["need_restart"].(schema.BoolAttribute)
	if !ok {
		t.Fatal("need_restart should be a bool attribute")
	}
	if !needRestartAttr.IsComputed() || needRestartAttr.IsOptional() || needRestartAttr.IsRequired() {
		t.Error("need_restart should be computed only")
	}

	// Check schema description
	if resp.Schema.MarkdownDescription != "Network or Vnet resource in VergeIO. Destroy is refused while IPsec or WireGuard rows remain, because VergeOS does not delete them with the network." {
		t.Errorf("expected description to refuse destroy while VPN rows remain, got '%s'", resp.Schema.MarkdownDescription)
	}

	// need_restart is set by a staged update, so the plan must stay unknown.
	if len(needRestartAttr.PlanModifiers) != 0 {
		t.Fatalf("need_restart has %d plan modifiers, want 0", len(needRestartAttr.PlanModifiers))
	}
}

func TestNetworkResource_StableComputedPlanModifiers(t *testing.T) {
	networkResource := NewNetworkResource()
	resp := &fwresource.SchemaResponse{}
	networkResource.Schema(context.Background(), fwresource.SchemaRequest{}, resp)

	keepState := stringplanmodifier.UseStateForUnknown().Description(context.Background())
	keepStateInt := int32planmodifier.UseStateForUnknown().Description(context.Background())

	networkType := networkStringAttr(t, resp.Schema, "type")
	assertNetworkStringModifiers(t, "type", networkType.PlanModifiers, keepState)
	assertNetworkStringKeepsState(t, "type", networkType.PlanModifiers, "external", "internal")

	layer2Type := networkStringAttr(t, resp.Schema, "layer2_type")
	assertNetworkStringModifiers(t, "layer2_type", layer2Type.PlanModifiers, keepState)
	assertNetworkStringKeepsState(t, "layer2_type", layer2Type.PlanModifiers, "vlan", "vxlan")

	ipType := networkStringAttr(t, resp.Schema, "ipaddress_type")
	assertNetworkStringModifiers(t, "ipaddress_type", ipType.PlanModifiers, keepState)
	assertNetworkStringKeepsState(t, "ipaddress_type", ipType.PlanModifiers, "none", "static")

	layer2ID := networkInt32Attr(t, resp.Schema, "layer2_id")
	assertNetworkInt32Modifiers(t, "layer2_id", layer2ID.PlanModifiers, keepStateInt)
	assertNetworkInt32KeepsState(t, "layer2_id", layer2ID.PlanModifiers, 1000, 1001)

	mtu := networkInt32Attr(t, resp.Schema, "mtu")
	assertNetworkInt32Modifiers(t, "mtu", mtu.PlanModifiers, keepStateInt)
	assertNetworkInt32KeepsState(t, "mtu", mtu.PlanModifiers, 1500, 9000)

	interfaceVnet := networkInt32Attr(t, resp.Schema, "interface_vnet")
	assertNetworkInt32Modifiers(t, "interface_vnet", interfaceVnet.PlanModifiers, keepStateInt)
	assertNetworkInt32KeepsState(t, "interface_vnet", interfaceVnet.PlanModifiers, 4, 5)
}

func networkStringAttr(t *testing.T, s schema.Schema, name string) schema.StringAttribute {
	t.Helper()
	attr, ok := s.Attributes[name].(schema.StringAttribute)
	if !ok {
		t.Fatalf("%s is not a string attribute", name)
	}
	return attr
}

func networkInt32Attr(t *testing.T, s schema.Schema, name string) schema.Int32Attribute {
	t.Helper()
	attr, ok := s.Attributes[name].(schema.Int32Attribute)
	if !ok {
		t.Fatalf("%s is not an int32 attribute", name)
	}
	return attr
}

func assertNetworkStringModifiers(t *testing.T, name string, got []planmodifier.String, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s has %d plan modifiers, want %d", name, len(got), len(want))
	}
	for i := range want {
		if desc := got[i].Description(context.Background()); desc != want[i] {
			t.Errorf("%s modifier %d = %q, want %q", name, i, desc, want[i])
		}
	}
}

func assertNetworkInt32Modifiers(t *testing.T, name string, got []planmodifier.Int32, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s has %d plan modifiers, want %d", name, len(got), len(want))
	}
	for i := range want {
		if desc := got[i].Description(context.Background()); desc != want[i] {
			t.Errorf("%s modifier %d = %q, want %q", name, i, desc, want[i])
		}
	}
}

func networkPriorState() tfsdk.State {
	return tfsdk.State{Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: map[string]tftypes.Type{}}, map[string]tftypes.Value{})}
}

func assertNetworkStringKeepsState(t *testing.T, name string, mods []planmodifier.String, prior, next string) {
	t.Helper()
	plan := types.StringUnknown()
	for _, mod := range mods {
		resp := &planmodifier.StringResponse{PlanValue: plan}
		mod.PlanModifyString(context.Background(), planmodifier.StringRequest{
			ConfigValue: types.StringNull(),
			PlanValue:   plan,
			StateValue:  types.StringValue(prior),
			State:       networkPriorState(),
		}, resp)
		plan = resp.PlanValue
	}
	if plan.IsUnknown() || plan.ValueString() != prior {
		t.Fatalf("%s omitted config plan = %s, want %q", name, plan, prior)
	}

	plan = types.StringValue(next)
	for _, mod := range mods {
		resp := &planmodifier.StringResponse{PlanValue: plan}
		mod.PlanModifyString(context.Background(), planmodifier.StringRequest{
			ConfigValue: types.StringValue(next),
			PlanValue:   plan,
			StateValue:  types.StringValue(prior),
			State:       networkPriorState(),
		}, resp)
		plan = resp.PlanValue
	}
	if plan.ValueString() != next {
		t.Fatalf("%s configured plan = %s, want %q", name, plan, next)
	}
}

func assertNetworkInt32KeepsState(t *testing.T, name string, mods []planmodifier.Int32, prior, next int32) {
	t.Helper()
	plan := types.Int32Unknown()
	for _, mod := range mods {
		resp := &planmodifier.Int32Response{PlanValue: plan}
		mod.PlanModifyInt32(context.Background(), planmodifier.Int32Request{
			ConfigValue: types.Int32Null(),
			PlanValue:   plan,
			StateValue:  types.Int32Value(prior),
			State:       networkPriorState(),
		}, resp)
		plan = resp.PlanValue
	}
	if plan.IsUnknown() || plan.ValueInt32() != prior {
		t.Fatalf("%s omitted config plan = %s, want %d", name, plan, prior)
	}

	plan = types.Int32Value(next)
	for _, mod := range mods {
		resp := &planmodifier.Int32Response{PlanValue: plan}
		mod.PlanModifyInt32(context.Background(), planmodifier.Int32Request{
			ConfigValue: types.Int32Value(next),
			PlanValue:   plan,
			StateValue:  types.Int32Value(prior),
			State:       networkPriorState(),
		}, resp)
		plan = resp.PlanValue
	}
	if plan.ValueInt32() != next {
		t.Fatalf("%s configured plan = %s, want %d", name, plan, next)
	}
}

func TestNetworkResource_Configure_WithValidClient(t *testing.T) {
	networkResource := &NetworkResource{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
	}))
	t.Cleanup(server.Close)
	client := vergeio.NewClient(server.URL, "testuser", "testpass", true)

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
		PowerState:           types.BoolValue(true),
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
		PowerState:           types.BoolNull(),
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
