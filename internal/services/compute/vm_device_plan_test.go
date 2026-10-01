// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package compute

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// TestDevicePlanFollowsName is the unit stand-in for adding a vergeio_device
// to a VM that is already in state. The planned key and machine were null,
// apply stored the ids VergeOS assigned, and Terraform rejected the result.
func TestDevicePlanFollowsName(t *testing.T) {
	ctx := context.Background()
	vmSchema := vmSchema(t)
	gpu := storedPlainDevice("gpu", "12", 70)
	tpm := storedTPM("tpm", "3", 70, 1, 1)

	t.Run("adding the first device stays unknown", func(t *testing.T) {
		state := vmValue(t, vmSchema, nil)
		config := vmValue(t, vmSchema, []*deviceResourceModel{configuredTPM("tpm")})
		assertDeviceUnknown(t, ctx, vmSchema, config, state, 0, "key")
		assertDeviceUnknown(t, ctx, vmSchema, config, state, 0, "machine")
		assertDeviceUnknown(t, ctx, vmSchema, config, state, 0, "tpm_settings.key")
		assertDeviceUnknown(t, ctx, vmSchema, config, state, 0, "tpm_settings.machine_device")
	})

	t.Run("existing device keeps its ids", func(t *testing.T) {
		state := vmValue(t, vmSchema, []*deviceResourceModel{tpm})
		edited := configuredTPM("tpm")
		edited.Description = types.StringValue("changed")
		config := vmValue(t, vmSchema, []*deviceResourceModel{edited})
		assertDeviceString(t, ctx, vmSchema, config, state, 0, "key", "3")
		assertDeviceInt32(t, ctx, vmSchema, config, state, 0, "machine", 70)
		assertDeviceInt32(t, ctx, vmSchema, config, state, 0, "tpm_settings.key", 1)
		assertDeviceInt32(t, ctx, vmSchema, config, state, 0, "tpm_settings.machine_device", 1)
	})

	t.Run("insert does not copy the device at that index", func(t *testing.T) {
		state := vmValue(t, vmSchema, []*deviceResourceModel{gpu})
		config := vmValue(t, vmSchema, []*deviceResourceModel{configuredTPM("tpm"), configuredPlain("gpu")})
		assertDeviceUnknown(t, ctx, vmSchema, config, state, 0, "key")
		assertDeviceUnknown(t, ctx, vmSchema, config, state, 0, "machine")
		assertDeviceUnknown(t, ctx, vmSchema, config, state, 0, "tpm_settings.key")
		assertDeviceUnknown(t, ctx, vmSchema, config, state, 0, "tpm_settings.machine_device")
		assertDeviceString(t, ctx, vmSchema, config, state, 1, "key", "12")
		assertDeviceInt32(t, ctx, vmSchema, config, state, 1, "machine", 70)
	})

	t.Run("reorder keeps each device id", func(t *testing.T) {
		state := vmValue(t, vmSchema, []*deviceResourceModel{gpu, tpm})
		config := vmValue(t, vmSchema, []*deviceResourceModel{configuredTPM("tpm"), configuredPlain("gpu")})
		assertDeviceString(t, ctx, vmSchema, config, state, 0, "key", "3")
		assertDeviceInt32(t, ctx, vmSchema, config, state, 0, "machine", 70)
		assertDeviceInt32(t, ctx, vmSchema, config, state, 0, "tpm_settings.key", 1)
		assertDeviceInt32(t, ctx, vmSchema, config, state, 0, "tpm_settings.machine_device", 1)
		assertDeviceString(t, ctx, vmSchema, config, state, 1, "key", "12")
	})

	t.Run("rename stays unknown", func(t *testing.T) {
		state := vmValue(t, vmSchema, []*deviceResourceModel{tpm})
		config := vmValue(t, vmSchema, []*deviceResourceModel{configuredTPM("other")})
		assertDeviceUnknown(t, ctx, vmSchema, config, state, 0, "key")
		assertDeviceUnknown(t, ctx, vmSchema, config, state, 0, "machine")
		assertDeviceUnknown(t, ctx, vmSchema, config, state, 0, "tpm_settings.key")
	})

	t.Run("configured key is kept", func(t *testing.T) {
		state := vmValue(t, vmSchema, []*deviceResourceModel{tpm})
		device := configuredTPM("tpm")
		device.Key = types.StringValue("99")
		config := vmValue(t, vmSchema, []*deviceResourceModel{device})
		assertDeviceString(t, ctx, vmSchema, config, state, 0, "key", "99")
	})
}

func TestDevicePlanUnknownOnCreate(t *testing.T) {
	ctx := context.Background()
	vmSchema := vmSchema(t)
	config := vmValue(t, vmSchema, []*deviceResourceModel{configuredTPM("tpm")})
	state := tfsdk.State{
		Schema: vmSchema,
		Raw:    tftypes.NewValue(vmSchema.Type().TerraformType(ctx), nil),
	}
	assertDeviceUnknown(t, ctx, vmSchema, config, state, 0, "key")
	assertDeviceUnknown(t, ctx, vmSchema, config, state, 0, "machine")
	assertDeviceUnknown(t, ctx, vmSchema, config, state, 0, "tpm_settings.key")
	assertDeviceUnknown(t, ctx, vmSchema, config, state, 0, "tpm_settings.machine_device")
}

func vmSchema(t *testing.T) schema.Schema {
	t.Helper()
	resp := &fwresource.SchemaResponse{}
	NewVMResource().Schema(context.Background(), fwresource.SchemaRequest{}, resp)
	return resp.Schema
}

func vmValue(t *testing.T, vmSchema schema.Schema, devices []*deviceResourceModel) tfsdk.State {
	t.Helper()
	state := tfsdk.State{Schema: vmSchema}
	diags := state.Set(context.Background(), &VMResourceModel{
		Id:            types.StringValue("8"),
		Name:          types.StringValue("vm"),
		Machine:       types.Int32Value(70),
		Devices:       devices,
		GuestAgentIPs: types.ListNull(types.StringType),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	return state
}

func configuredTPM(name string) *deviceResourceModel {
	return &deviceResourceModel{
		Name: types.StringValue(name),
		Type: types.StringValue("tpm"),
		DeviceTPMSettingsModel: &DeviceTPMSettingsModel{
			Model:   types.StringValue("crb"),
			Version: types.StringValue("2.0"),
		},
	}
}

func configuredPlain(name string) *deviceResourceModel {
	return &deviceResourceModel{
		Name: types.StringValue(name),
		Type: types.StringValue("node_pci_devices"),
	}
}

func storedTPM(name, key string, machine, tpmKey, machineDevice int32) *deviceResourceModel {
	device := configuredTPM(name)
	device.Key = types.StringValue(key)
	device.Machine = types.Int32Value(machine)
	device.DeviceTPMSettingsModel.Key = types.Int32Value(tpmKey)
	device.DeviceTPMSettingsModel.MachineDevice = types.Int32Value(machineDevice)
	return device
}

func storedPlainDevice(name, key string, machine int32) *deviceResourceModel {
	device := configuredPlain(name)
	device.Key = types.StringValue(key)
	device.Machine = types.Int32Value(machine)
	return device
}

func assertDeviceUnknown(t *testing.T, ctx context.Context, vmSchema schema.Schema, config, state tfsdk.State, index int, name string) {
	t.Helper()
	got := planDeviceAttribute(t, ctx, vmSchema, config, state, index, name)
	if got == nil || got.IsNull() || !got.IsUnknown() {
		t.Fatalf("vergeio_device[%d].%s plan = %s, want unknown", index, name, got)
	}
}

func assertDeviceString(t *testing.T, ctx context.Context, vmSchema schema.Schema, config, state tfsdk.State, index int, name, want string) {
	t.Helper()
	got := planDeviceAttribute(t, ctx, vmSchema, config, state, index, name)
	value, ok := got.(types.String)
	if !ok || value.IsNull() || value.IsUnknown() || value.ValueString() != want {
		t.Fatalf("vergeio_device[%d].%s plan = %s, want %q", index, name, got, want)
	}
}

func assertDeviceInt32(t *testing.T, ctx context.Context, vmSchema schema.Schema, config, state tfsdk.State, index int, name string, want int32) {
	t.Helper()
	got := planDeviceAttribute(t, ctx, vmSchema, config, state, index, name)
	value, ok := got.(types.Int32)
	if !ok || value.IsNull() || value.IsUnknown() || value.ValueInt32() != want {
		t.Fatalf("vergeio_device[%d].%s plan = %s, want %d", index, name, got, want)
	}
}

func planDeviceAttribute(t *testing.T, ctx context.Context, vmSchema schema.Schema, config, state tfsdk.State, index int, name string) attr.Value {
	t.Helper()
	block, ok := vmSchema.Blocks["vergeio_device"].(schema.ListNestedBlock)
	if !ok {
		t.Fatal("vergeio_device should be a list nested block")
	}
	attrPath := path.Root("vergeio_device").AtListIndex(index)
	attribute := block.NestedObject.Attributes
	parts := splitDeviceAttr(name)
	var schemaAttr schema.Attribute
	for i, part := range parts {
		attrPath = attrPath.AtName(part)
		next, ok := attribute[part]
		if !ok {
			t.Fatalf("vergeio_device has no %s", name)
		}
		if i == len(parts)-1 {
			schemaAttr = next
			break
		}
		nested, ok := next.(schema.SingleNestedAttribute)
		if !ok {
			t.Fatalf("%s is not a single nested attribute", part)
		}
		attribute = nested.Attributes
	}
	configData := tfsdk.Config{Schema: vmSchema, Raw: config.Raw}
	planData := tfsdk.Plan{Schema: vmSchema, Raw: config.Raw}
	switch typed := schemaAttr.(type) {
	case schema.StringAttribute:
		var configValue types.String
		if diags := configData.GetAttribute(ctx, attrPath, &configValue); diags.HasError() {
			t.Fatal(diags)
		}
		got := configValue
		if configValue.IsNull() {
			got = types.StringUnknown()
		}
		for _, mod := range typed.PlanModifiers {
			resp := &planmodifier.StringResponse{PlanValue: got}
			mod.PlanModifyString(ctx, planmodifier.StringRequest{
				Config:      configData,
				ConfigValue: configValue,
				Plan:        planData,
				PlanValue:   got,
				State:       state,
				StateValue:  types.StringNull(),
				Path:        attrPath,
			}, resp)
			if resp.Diagnostics.HasError() {
				t.Fatal(resp.Diagnostics)
			}
			got = resp.PlanValue
		}
		return got
	case schema.Int32Attribute:
		var configValue types.Int32
		if diags := configData.GetAttribute(ctx, attrPath, &configValue); diags.HasError() {
			t.Fatal(diags)
		}
		got := configValue
		if configValue.IsNull() {
			got = types.Int32Unknown()
		}
		for _, mod := range typed.PlanModifiers {
			resp := &planmodifier.Int32Response{PlanValue: got}
			mod.PlanModifyInt32(ctx, planmodifier.Int32Request{
				Config:      configData,
				ConfigValue: configValue,
				Plan:        planData,
				PlanValue:   got,
				State:       state,
				StateValue:  types.Int32Null(),
				Path:        attrPath,
			}, resp)
			if resp.Diagnostics.HasError() {
				t.Fatal(resp.Diagnostics)
			}
			got = resp.PlanValue
		}
		return got
	default:
		t.Fatalf("vergeio_device.%s has unexpected type %T", name, schemaAttr)
		return nil
	}
}

func splitDeviceAttr(name string) []string {
	switch name {
	case "tpm_settings.key":
		return []string{"tpm_settings", "key"}
	case "tpm_settings.machine_device":
		return []string{"tpm_settings", "machine_device"}
	case "tpm_settings.version":
		return []string{"tpm_settings", "version"}
	default:
		return []string{name}
	}
}
