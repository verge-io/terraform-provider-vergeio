package compute

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int32planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

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

	snapshotProfile, ok := resp.Schema.Attributes["snapshot_profile"].(schema.Int32Attribute)
	if !ok {
		t.Fatal("snapshot_profile should be an int32 attribute")
	}
	if !snapshotProfile.Optional {
		t.Error("snapshot_profile should be optional")
	}
	if !strings.Contains(snapshotProfile.MarkdownDescription, "tonumber(vergeio_snapshot_profile.example.id)") {
		t.Errorf("snapshot_profile should document the profile resource id, got %q", snapshotProfile.MarkdownDescription)
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

	boot, ok := resp.Schema.Blocks["boot_disk"].(schema.SingleNestedBlock)
	if !ok {
		t.Fatal("boot_disk should be a single nested block")
	}
	bootKey, ok := boot.Attributes["key"].(schema.StringAttribute)
	if !ok {
		t.Fatal("boot_disk.key should be a string attribute")
	}
	assertPlanModifiers(t, "boot_disk.key", bootKey.PlanModifiers,
		stringplanmodifier.UseNonNullStateForUnknown().Description(context.Background()),
	)
	assertStringKeepsPriorState(t, "boot_disk.key", bootKey.PlanModifiers, "45", "46")
	// Adding boot_disk to an existing VM has a null prior key. That plan must
	// stay unknown so apply can store the key VergeOS assigns. Create already
	// plans the key unknown and must stay that way.
	assertStringStaysUnknownWhenPriorNull(t, "boot_disk.key", bootKey.PlanModifiers)

	keepStateInt := int32planmodifier.UseStateForUnknown().Description(context.Background())

	devicePlan := deviceComputedModifier{}.Description(context.Background())
	deviceKey := nestedStringAttr(t, resp.Schema.Blocks, "vergeio_device", "key")
	assertPlanModifiers(t, "vergeio_device.key", deviceKey.PlanModifiers, devicePlan)

	deviceMachine := nestedInt32Attr(t, resp.Schema.Blocks, "vergeio_device", "machine")
	assertInt32PlanModifiers(t, "vergeio_device.machine", deviceMachine.PlanModifiers, devicePlan)

	// key and machine follow the device name. TPM ids do too. USB and vGPU
	// settings are unchanged and still copy the value at the same index.
	tpmKey := deviceSettingInt32(t, resp.Schema.Blocks, "tpm_settings", "key")
	assertInt32PlanModifiers(t, "vergeio_device.tpm_settings.key", tpmKey.PlanModifiers, devicePlan)
	tpmMachine := deviceSettingInt32(t, resp.Schema.Blocks, "tpm_settings", "machine_device")
	assertInt32PlanModifiers(t, "vergeio_device.tpm_settings.machine_device", tpmMachine.PlanModifiers, devicePlan)
	for _, setting := range []string{"usb_settings", "nvidia_vgpu_settings"} {
		settingKey := deviceSettingInt32(t, resp.Schema.Blocks, setting, "key")
		assertInt32PlanModifiers(t, "vergeio_device."+setting+".key", settingKey.PlanModifiers, keepStateInt)
		settingMachine := deviceSettingInt32(t, resp.Schema.Blocks, setting, "machine_device")
		assertInt32PlanModifiers(t, "vergeio_device."+setting+".machine_device", settingMachine.PlanModifiers, keepStateInt)
	}

	machineAttr, ok := resp.Schema.Attributes["machine"].(schema.Int32Attribute)
	if !ok {
		t.Fatal("machine should be an int32 attribute")
	}
	assertInt32PlanModifiers(t, "machine", machineAttr.PlanModifiers, keepStateInt)
	assertInt32KeepsPriorState(t, "machine", machineAttr.PlanModifiers, 66, 7)
	assertInt32StaysUnknownOnCreate(t, "machine", machineAttr.PlanModifiers)

	// These can change on update, so an unrelated plan must keep them unknown.
	if mods := nestedInt32Attr(t, resp.Schema.Blocks, "vergeio_device", "status").PlanModifiers; len(mods) != 0 {
		t.Fatalf("vergeio_device.status has %d plan modifiers, want 0", len(mods))
	}
	ips, ok := resp.Schema.Attributes["guest_agent_ips"].(schema.ListAttribute)
	if !ok {
		t.Fatal("guest_agent_ips should be a list attribute")
	}
	if len(ips.PlanModifiers) != 0 {
		t.Fatalf("guest_agent_ips has %d plan modifiers, want 0", len(ips.PlanModifiers))
	}
}

func TestVMDriveAndNICResourcePlanModifiers(t *testing.T) {
	ctx := context.Background()
	driveResp := &fwresource.SchemaResponse{}
	NewVMDriveResource().Schema(ctx, fwresource.SchemaRequest{}, driveResp)
	nicResp := &fwresource.SchemaResponse{}
	NewVMNICResource().Schema(ctx, fwresource.SchemaRequest{}, nicResp)

	keepState := stringplanmodifier.UseStateForUnknown().Description(ctx)
	keepStateInt := int32planmodifier.UseStateForUnknown().Description(ctx)
	replace := stringplanmodifier.RequiresReplace().Description(ctx)

	driveID := resourceStringAttr(t, driveResp.Schema.Attributes, "id")
	assertPlanModifiers(t, "vergeio_vm_drive.id", driveID.PlanModifiers, keepState)
	assertStringKeepsPriorState(t, "vergeio_vm_drive.id", driveID.PlanModifiers, "45", "46")

	driveMedia := resourceStringAttr(t, driveResp.Schema.Attributes, "media")
	assertPlanModifiers(t, "vergeio_vm_drive.media", driveMedia.PlanModifiers, replace)

	driveSource, ok := driveResp.Schema.Attributes["media_source"].(schema.Int32Attribute)
	if !ok {
		t.Fatal("vergeio_vm_drive.media_source should be an int32 attribute")
	}
	if len(driveSource.PlanModifiers) != 1 || driveSource.PlanModifiers[0].Description(ctx) != int32planmodifier.RequiresReplace().Description(ctx) {
		t.Fatal("vergeio_vm_drive.media_source should require replace")
	}

	driveTier, ok := driveResp.Schema.Attributes["preferred_tier"].(schema.Int32Attribute)
	if !ok {
		t.Fatal("vergeio_vm_drive.preferred_tier should be an int32 attribute")
	}
	assertInt32PlanModifiers(t, "vergeio_vm_drive.preferred_tier", driveTier.PlanModifiers, keepStateInt)
	assertInt32KeepsPriorState(t, "vergeio_vm_drive.preferred_tier", driveTier.PlanModifiers, 4, 1)

	driveAsset := resourceStringAttr(t, driveResp.Schema.Attributes, "asset")
	assertPlanModifiers(t, "vergeio_vm_drive.asset", driveAsset.PlanModifiers, keepState)
	assertStringKeepsPriorState(t, "vergeio_vm_drive.asset", driveAsset.PlanModifiers, "48", "49")

	nicMAC := resourceStringAttr(t, nicResp.Schema.Attributes, "macaddress")
	assertPlanModifiers(t, "vergeio_vm_nic.macaddress", nicMAC.PlanModifiers, keepState)
	// An omitted MAC keeps the server-assigned address. A configured address,
	// including one that is still unknown, is not replaced with state.
	assertStringKeepsPriorState(t, "vergeio_vm_nic.macaddress", nicMAC.PlanModifiers, "f0:db:30:26:43:0c", "52:54:00:11:22:33")
	assertStringLeavesUnknownConfig(t, "vergeio_vm_nic.macaddress", nicMAC.PlanModifiers, "f0:db:30:26:43:0c")

	nicAsset := resourceStringAttr(t, nicResp.Schema.Attributes, "asset")
	assertPlanModifiers(t, "vergeio_vm_nic.asset", nicAsset.PlanModifiers, keepState)

	ip := resourceStringAttr(t, nicResp.Schema.Attributes, "ipaddress")
	if len(ip.PlanModifiers) != 0 {
		t.Fatalf("vergeio_vm_nic.ipaddress has %d plan modifiers, want 0", len(ip.PlanModifiers))
	}
}

func resourceStringAttr(t *testing.T, attrs map[string]schema.Attribute, name string) schema.StringAttribute {
	t.Helper()
	attr, ok := attrs[name].(schema.StringAttribute)
	if !ok {
		t.Fatalf("%s is not a string attribute", name)
	}
	return attr
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

func assertInt32PlanModifiers(t *testing.T, name string, got []planmodifier.Int32, want ...string) {
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

func deviceSettingInt32(t *testing.T, blocks map[string]schema.Block, setting, name string) schema.Int32Attribute {
	t.Helper()
	block, ok := blocks["vergeio_device"].(schema.ListNestedBlock)
	if !ok {
		t.Fatal("vergeio_device is not a list nested block")
	}
	parent, ok := block.NestedObject.Attributes[setting].(schema.SingleNestedAttribute)
	if !ok {
		t.Fatalf("vergeio_device.%s is not a single nested attribute", setting)
	}
	attr, ok := parent.Attributes[name].(schema.Int32Attribute)
	if !ok {
		t.Fatalf("vergeio_device.%s.%s is not an int32 attribute", setting, name)
	}
	return attr
}

// priorResourceState is a non-null resource state. UseStateForUnknown does
// nothing while state is null, which is create.
func priorResourceState() tfsdk.State {
	return tfsdk.State{Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: map[string]tftypes.Type{}}, map[string]tftypes.Value{})}
}

func assertStringKeepsPriorState(t *testing.T, name string, mods []planmodifier.String, prior, next string) {
	t.Helper()
	plan := types.StringUnknown()
	var replace bool
	for _, mod := range mods {
		resp := &planmodifier.StringResponse{PlanValue: plan}
		mod.PlanModifyString(context.Background(), planmodifier.StringRequest{
			ConfigValue: types.StringNull(),
			PlanValue:   plan,
			StateValue:  types.StringValue(prior),
			State:       priorResourceState(),
		}, resp)
		plan = resp.PlanValue
		replace = replace || resp.RequiresReplace
	}
	if plan.IsUnknown() || plan.ValueString() != prior {
		t.Fatalf("%s omitted config plan = %s, want %q", name, plan, prior)
	}
	if replace {
		t.Fatalf("%s omitted config planned a replacement", name)
	}

	plan = types.StringValue(next)
	for _, mod := range mods {
		resp := &planmodifier.StringResponse{PlanValue: plan}
		mod.PlanModifyString(context.Background(), planmodifier.StringRequest{
			ConfigValue: types.StringValue(next),
			PlanValue:   plan,
			StateValue:  types.StringValue(prior),
			State:       priorResourceState(),
		}, resp)
		plan = resp.PlanValue
	}
	if plan.ValueString() != next {
		t.Fatalf("%s configured plan = %s, want %q", name, plan, next)
	}
}

// assertStringStaysUnknownWhenPriorNull covers a nested computed attribute
// whose block is new. The resource is already in state, so the prior value
// is null, and the framework has marked the plan unknown. Copying that null
// makes apply fail with an inconsistent result. Create, where the whole
// resource state is null, must also stay unknown.
func assertStringStaysUnknownWhenPriorNull(t *testing.T, name string, mods []planmodifier.String) {
	t.Helper()
	plan := types.StringUnknown()
	for _, mod := range mods {
		resp := &planmodifier.StringResponse{PlanValue: plan}
		mod.PlanModifyString(context.Background(), planmodifier.StringRequest{
			ConfigValue: types.StringNull(),
			PlanValue:   plan,
			StateValue:  types.StringNull(),
			State:       priorResourceState(),
		}, resp)
		plan = resp.PlanValue
	}
	if !plan.IsUnknown() {
		t.Fatalf("%s null prior plan = %s, want unknown", name, plan)
	}

	plan = types.StringUnknown()
	for _, mod := range mods {
		resp := &planmodifier.StringResponse{PlanValue: plan}
		mod.PlanModifyString(context.Background(), planmodifier.StringRequest{
			ConfigValue: types.StringNull(),
			PlanValue:   plan,
			StateValue:  types.StringNull(),
			State: tfsdk.State{
				Raw: tftypes.NewValue(tftypes.Object{AttributeTypes: map[string]tftypes.Type{}}, nil),
			},
		}, resp)
		plan = resp.PlanValue
	}
	if !plan.IsUnknown() {
		t.Fatalf("%s create plan = %s, want unknown", name, plan)
	}
}

func assertStringLeavesUnknownConfig(t *testing.T, name string, mods []planmodifier.String, prior string) {
	t.Helper()
	plan := types.StringUnknown()
	for _, mod := range mods {
		resp := &planmodifier.StringResponse{PlanValue: plan}
		mod.PlanModifyString(context.Background(), planmodifier.StringRequest{
			ConfigValue: types.StringUnknown(),
			PlanValue:   plan,
			StateValue:  types.StringValue(prior),
			State:       priorResourceState(),
		}, resp)
		plan = resp.PlanValue
	}
	if !plan.IsUnknown() {
		t.Fatalf("%s unknown config plan = %s, want unknown", name, plan)
	}
}

func assertInt32KeepsPriorState(t *testing.T, name string, mods []planmodifier.Int32, prior, next int32) {
	t.Helper()
	plan := types.Int32Unknown()
	for _, mod := range mods {
		resp := &planmodifier.Int32Response{PlanValue: plan}
		mod.PlanModifyInt32(context.Background(), planmodifier.Int32Request{
			ConfigValue: types.Int32Null(),
			PlanValue:   plan,
			StateValue:  types.Int32Value(prior),
			State:       priorResourceState(),
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
			State:       priorResourceState(),
		}, resp)
		plan = resp.PlanValue
	}
	if plan.ValueInt32() != next {
		t.Fatalf("%s configured plan = %s, want %d", name, plan, next)
	}
}

func assertInt32StaysUnknownOnCreate(t *testing.T, name string, mods []planmodifier.Int32) {
	t.Helper()
	plan := types.Int32Unknown()
	for _, mod := range mods {
		resp := &planmodifier.Int32Response{PlanValue: plan}
		mod.PlanModifyInt32(context.Background(), planmodifier.Int32Request{
			ConfigValue: types.Int32Null(),
			PlanValue:   plan,
			StateValue:  types.Int32Null(),
		}, resp)
		plan = resp.PlanValue
	}
	if !plan.IsUnknown() {
		t.Fatalf("%s create plan = %s, want unknown", name, plan)
	}
}

func TestVMResource_Configure_WithValidClient(t *testing.T) {
	vmResource := &VMResource{}
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
