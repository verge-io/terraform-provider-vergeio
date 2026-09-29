package compute

import (
	"context"
	"testing"

	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int32planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-vergeio/internal/client"
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

	// powerstate is optional and computed. When configuration omits it, the
	// plan must keep the prior state instead of becoming unknown.
	if powerAttr, ok := resp.Schema.Attributes["powerstate"]; !ok {
		t.Error("powerstate attribute should exist")
	} else {
		boolAttr, ok := powerAttr.(schema.BoolAttribute)
		if !ok {
			t.Fatal("powerstate should be a bool attribute")
		}
		if !boolAttr.Optional || !boolAttr.Computed {
			t.Error("powerstate should be optional and computed")
		}
		if len(boolAttr.PlanModifiers) != 1 {
			t.Fatalf("powerstate should have one plan modifier, got %d", len(boolAttr.PlanModifiers))
		}
		got := boolAttr.PlanModifiers[0].Description(context.Background())
		want := boolplanmodifier.UseStateForUnknown().Description(context.Background())
		if got != want {
			t.Errorf("powerstate plan modifier description %q, want UseStateForUnknown %q", got, want)
		}
	}

	// Check schema description
	if resp.Schema.MarkdownDescription != "VM resource in VergeIO" {
		t.Errorf("expected description 'VM resource in VergeIO', got '%s'", resp.Schema.MarkdownDescription)
	}
}

func TestVMResource_DriveAndNICPlanModifiers(t *testing.T) {
	vmResource := NewVMResource()
	resp := &fwresource.SchemaResponse{}
	vmResource.Schema(context.Background(), fwresource.SchemaRequest{}, resp)

	driveKey := nestedStringAttr(t, resp.Schema.Blocks, "vergeio_drive", "key")
	assertPlanModifiers(t, "vergeio_drive.key", driveKey.PlanModifiers,
		stringplanmodifier.UseStateForUnknown().Description(context.Background()),
		stringplanmodifier.RequiresReplaceIfConfigured().Description(context.Background()),
	)

	driveMedia := nestedStringAttr(t, resp.Schema.Blocks, "vergeio_drive", "media")
	assertPlanModifiers(t, "vergeio_drive.media", driveMedia.PlanModifiers,
		stringplanmodifier.RequiresReplace().Description(context.Background()),
	)

	driveSource := nestedInt32Attr(t, resp.Schema.Blocks, "vergeio_drive", "media_source")
	if len(driveSource.PlanModifiers) != 1 {
		t.Fatalf("vergeio_drive.media_source modifiers = %d, want 1", len(driveSource.PlanModifiers))
	}
	got := driveSource.PlanModifiers[0].Description(context.Background())
	want := int32planmodifier.RequiresReplace().Description(context.Background())
	if got != want {
		t.Errorf("vergeio_drive.media_source modifier %q, want %q", got, want)
	}

	nicID := nestedStringAttr(t, resp.Schema.Blocks, "vergeio_nic", "id")
	assertPlanModifiers(t, "vergeio_nic.id", nicID.PlanModifiers,
		stringplanmodifier.UseStateForUnknown().Description(context.Background()),
		stringplanmodifier.RequiresReplaceIfConfigured().Description(context.Background()),
	)

	nicMAC := nestedStringAttr(t, resp.Schema.Blocks, "vergeio_nic", "macaddress")
	assertPlanModifiers(t, "vergeio_nic.macaddress", nicMAC.PlanModifiers,
		stringplanmodifier.UseStateForUnknown().Description(context.Background()),
	)
}

func nestedStringAttr(t *testing.T, blocks map[string]schema.Block, blockName, attrName string) schema.StringAttribute {
	t.Helper()
	block, ok := blocks[blockName].(schema.ListNestedBlock)
	if !ok {
		t.Fatalf("%s is not a list nested block", blockName)
	}
	attr, ok := block.NestedObject.Attributes[attrName].(schema.StringAttribute)
	if !ok {
		t.Fatalf("%s.%s is not a string attribute", blockName, attrName)
	}
	return attr
}

func nestedInt32Attr(t *testing.T, blocks map[string]schema.Block, blockName, attrName string) schema.Int32Attribute {
	t.Helper()
	block, ok := blocks[blockName].(schema.ListNestedBlock)
	if !ok {
		t.Fatalf("%s is not a list nested block", blockName)
	}
	attr, ok := block.NestedObject.Attributes[attrName].(schema.Int32Attribute)
	if !ok {
		t.Fatalf("%s.%s is not an int32 attribute", blockName, attrName)
	}
	return attr
}

func assertPlanModifiers(t *testing.T, name string, got []planmodifier.String, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s has %d plan modifiers, want %d", name, len(got), len(want))
	}
	for i := range want {
		desc := got[i].Description(context.Background())
		if desc != want[i] {
			t.Errorf("%s modifier %d = %q, want %q", name, i, desc, want[i])
		}
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
