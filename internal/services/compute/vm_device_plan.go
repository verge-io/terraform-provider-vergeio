// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package compute

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

var (
	_ planmodifier.String = deviceComputedModifier{}
	_ planmodifier.Int32  = deviceComputedModifier{}
)

// deviceComputedModifier plans computed ids on a vergeio_device.
//
// UseStateForUnknown copies the value at the same list index, including
// null. Adding a device to a VM that is already in state plans key and
// machine as null. Apply stores the ids VergeOS assigns and Terraform
// rejects the plan as inconsistent. Inserting or reordering copies a
// different device's id the same way.
//
// Name is the identity. key, machine, and the TPM ids are copied from the
// device with that name. A device that is not in state yet stays unknown.
type deviceComputedModifier struct{}

func (m deviceComputedModifier) Description(context.Context) string {
	return "Plans a device id from the vergeio_device with the same name. A new device stays unknown."
}

func (m deviceComputedModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (m deviceComputedModifier) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if kept, ok := keepKnownDeviceString(req.ConfigValue); ok {
		resp.PlanValue = kept
		return
	}
	resp.PlanValue = types.StringUnknown()
	if !req.PlanValue.IsUnknown() && !req.PlanValue.IsNull() {
		resp.PlanValue = req.PlanValue
		return
	}
	if nestedAttributePath(req.Path) != "key" {
		resp.Diagnostics.AddAttributeError(req.Path, "Unexpected device attribute", "The device key plan modifier was applied to "+nestedAttributePath(req.Path)+".")
		return
	}
	prior, diags := m.lookup(ctx, req.Config, req.State, req.Path)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || prior == nil {
		return
	}
	value, ok := prior.(types.String)
	if !ok || value.IsNull() || value.IsUnknown() {
		return
	}
	resp.PlanValue = value
}

func (m deviceComputedModifier) PlanModifyInt32(ctx context.Context, req planmodifier.Int32Request, resp *planmodifier.Int32Response) {
	if kept, ok := keepKnownDeviceInt32(req.ConfigValue); ok {
		resp.PlanValue = kept
		return
	}
	resp.PlanValue = types.Int32Unknown()
	if !req.PlanValue.IsUnknown() && !req.PlanValue.IsNull() {
		resp.PlanValue = req.PlanValue
		return
	}
	switch nestedAttributePath(req.Path) {
	case "machine", "tpm_settings.key", "tpm_settings.machine_device":
	default:
		resp.Diagnostics.AddAttributeError(req.Path, "Unexpected device attribute", "The device plan modifier was applied to "+nestedAttributePath(req.Path)+".")
		return
	}
	prior, diags := m.lookup(ctx, req.Config, req.State, req.Path)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || prior == nil {
		return
	}
	value, ok := prior.(types.Int32)
	if !ok || value.IsNull() || value.IsUnknown() {
		return
	}
	resp.PlanValue = value
}

func keepKnownDeviceString(config types.String) (types.String, bool) {
	if config.IsNull() {
		return types.String{}, false
	}
	if config.IsUnknown() {
		return types.StringUnknown(), true
	}
	return config, true
}

func keepKnownDeviceInt32(config types.Int32) (types.Int32, bool) {
	if config.IsNull() {
		return types.Int32{}, false
	}
	if config.IsUnknown() {
		return types.Int32Unknown(), true
	}
	return config, true
}

func (m deviceComputedModifier) lookup(ctx context.Context, config tfsdk.Config, state tfsdk.State, attrPath path.Path) (attr.Value, diag.Diagnostics) {
	var diags diag.Diagnostics
	if !deviceRawReady(state.Raw) || !deviceRawReady(config.Raw) {
		return nil, diags
	}
	index, ok := deviceListIndex(attrPath)
	if !ok {
		diags.AddAttributeError(attrPath, "Unexpected device path", "The device plan modifier expected a list index.")
		return nil, diags
	}
	name, nameDiags := configuredDeviceName(ctx, config, attrPath, index)
	diags.Append(nameDiags...)
	if diags.HasError() || name == "" {
		return nil, diags
	}
	devices, deviceDiags := devicesFromState(ctx, state)
	diags.Append(deviceDiags...)
	if diags.HasError() {
		return nil, diags
	}
	device := deviceByName(devices, name)
	if device == nil {
		return nil, diags
	}
	return deviceComputedValue(device, nestedAttributePath(attrPath)), diags
}

func deviceRawReady(raw tftypes.Value) bool {
	if raw.Type() == nil || raw.IsNull() || !raw.IsKnown() {
		return false
	}
	return true
}

func configuredDeviceName(ctx context.Context, config tfsdk.Config, attrPath path.Path, index int) (string, diag.Diagnostics) {
	var diags diag.Diagnostics
	var model VMResourceModel
	diags.Append(config.Get(ctx, &model)...)
	if diags.HasError() {
		return "", diags
	}
	if index < 0 || index >= len(model.Devices) || model.Devices[index] == nil {
		diags.AddAttributeError(attrPath, "Unexpected device index", "The device plan modifier index is outside the configured device list.")
		return "", diags
	}
	name := model.Devices[index].Name
	if name.IsNull() || name.IsUnknown() {
		return "", diags
	}
	return strings.TrimSpace(name.ValueString()), diags
}

func devicesFromState(ctx context.Context, state tfsdk.State) ([]*deviceResourceModel, diag.Diagnostics) {
	var model VMResourceModel
	diags := state.Get(ctx, &model)
	if diags.HasError() {
		return nil, diags
	}
	return model.Devices, diags
}

func deviceByName(devices []*deviceResourceModel, name string) *deviceResourceModel {
	for _, device := range devices {
		if device == nil || device.Name.IsNull() || device.Name.IsUnknown() {
			continue
		}
		if strings.TrimSpace(device.Name.ValueString()) == name {
			return device
		}
	}
	return nil
}

func deviceComputedValue(device *deviceResourceModel, suffix string) attr.Value {
	switch suffix {
	case "key":
		return device.Key
	case "machine":
		return device.Machine
	case "tpm_settings.key":
		if device.DeviceTPMSettingsModel == nil {
			return nil
		}
		return device.DeviceTPMSettingsModel.Key
	case "tpm_settings.machine_device":
		if device.DeviceTPMSettingsModel == nil {
			return nil
		}
		return device.DeviceTPMSettingsModel.MachineDevice
	default:
		return nil
	}
}

func deviceListIndex(attrPath path.Path) (int, bool) {
	for _, step := range attrPath.Steps() {
		index, ok := step.(path.PathStepElementKeyInt)
		if ok {
			return int(index), true
		}
	}
	return 0, false
}

// nestedAttributePath is the attribute path inside the device element.
// vergeio_device[0].key is "key". vergeio_device[0].tpm_settings.key is
// "tpm_settings.key".
func nestedAttributePath(attrPath path.Path) string {
	var names []string
	seenIndex := false
	for _, step := range attrPath.Steps() {
		if _, ok := step.(path.PathStepElementKeyInt); ok {
			seenIndex = true
			names = nil
			continue
		}
		if !seenIndex {
			continue
		}
		name, ok := step.(path.PathStepAttributeName)
		if !ok {
			continue
		}
		names = append(names, string(name))
	}
	return strings.Join(names, ".")
}
