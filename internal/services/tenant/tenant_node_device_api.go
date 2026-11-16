// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tenant

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

// tenantNodeDeviceTypes are machine_devices.type values from govergeos.
// The node_* names are the ones pyVergeOS creates. gpu and nvidia_vgpu are the older names.
func tenantNodeDeviceTypes() []string {
	return []string{
		vergeos.DeviceTypeUSB,
		vergeos.DeviceTypeTPM,
		vergeos.DeviceTypePCI,
		vergeos.DeviceTypeVGPU,
		vergeos.DeviceTypeHostGPU,
		vergeos.DeviceTypeSRIOVNIC,
		vergeos.DeviceTypeGPULegacy,
		vergeos.DeviceTypeVGPULegacy,
	}
}

func tenantNodeDeviceCreateRequest(data *TenantNodeDeviceResourceModel) (*vergeos.VMDeviceCreateRequest, error) {
	name := strings.TrimSpace(data.Name.ValueString())
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	deviceType := strings.TrimSpace(data.Type.ValueString())
	if deviceType == "" {
		return nil, fmt.Errorf("type is required")
	}
	req := &vergeos.VMDeviceCreateRequest{
		Name: name,
		Type: deviceType,
	}
	if desc := vergeio.KnownString(data.Description); desc != nil {
		req.Description = *desc
	}
	if group := knownResourceGroup(data.ResourceGroup); group != "" {
		req.ResourceGroup = group
	}
	req.Enabled = vergeio.KnownBool(data.Enabled)
	req.Optional = vergeio.KnownBool(data.Optional)
	if count := vergeio.KnownInt32(data.Count); count != nil {
		n := int(*count)
		if n < 1 || n > 16 {
			return nil, fmt.Errorf("device_count must be from 1 through 16")
		}
		req.Count = &n
	}
	return req, nil
}

func tenantNodeDeviceUpdateRequest(plan, state *TenantNodeDeviceResourceModel) *vergeos.VMDeviceUpdateRequest {
	req := &vergeos.VMDeviceUpdateRequest{}
	changed := false
	if name := vergeio.ChangedString(plan.Name, state.Name); name != nil {
		req.Name = name
		changed = true
	}
	if desc := vergeio.ChangedString(plan.Description, state.Description); desc != nil {
		req.Description = desc
		changed = true
	}
	if group := vergeio.ChangedString(plan.ResourceGroup, state.ResourceGroup); group != nil {
		value := vergeos.DeviceResourceGroup(strings.TrimSpace(*group))
		req.ResourceGroup = &value
		changed = true
	}
	if enabled := vergeio.ChangedBool(plan.Enabled, state.Enabled); enabled != nil {
		req.Enabled = enabled
		changed = true
	}
	if optional := vergeio.ChangedBool(plan.Optional, state.Optional); optional != nil {
		req.Optional = optional
		changed = true
	}
	if !changed {
		return nil
	}
	return req
}

// knownResourceGroup returns the resource group UUID when it is set.
// The JSON name on the device is resource_group. An empty value is omitted.
func knownResourceGroup(v types.String) vergeos.DeviceResourceGroup {
	text := vergeio.KnownString(v)
	if text == nil {
		return ""
	}
	return vergeos.DeviceResourceGroup(strings.TrimSpace(*text))
}

// assignTenantNodeDevice copies a machine_devices row onto the Terraform model.
// device_count is not a column, so a value already in the model is kept.
// A read that omits resource_group keeps a UUID already stored. VergeOS
// sometimes leaves that field blank on the row it just wrote.
func assignTenantNodeDevice(data *TenantNodeDeviceResourceModel, device *vergeos.VMDevice) {
	count := data.Count
	group := data.ResourceGroup
	data.Id = idString(device.Key.Int())
	data.Name = types.StringValue(device.Name)
	data.Type = types.StringValue(device.Type)
	data.Description = types.StringValue(device.Description)
	if device.ResourceGroup.String() != "" {
		data.ResourceGroup = types.StringValue(device.ResourceGroup.String())
	} else if group.IsNull() || group.IsUnknown() || strings.TrimSpace(group.ValueString()) == "" {
		data.ResourceGroup = types.StringNull()
	} else {
		data.ResourceGroup = group
	}
	data.Enabled = types.BoolValue(device.Enabled)
	data.Optional = types.BoolValue(device.Optional)
	if device.Machine > 0 {
		data.Machine = types.Int32Value(int32(device.Machine))
	} else {
		data.Machine = types.Int32Null()
	}
	data.MachineType = enumString(device.MachineType)
	data.OrderID = types.Int32Value(int32(device.OrderID))
	data.UUID = enumString(device.UUID)
	data.Created = timestamp(device.Created)
	data.Modified = timestamp(device.Modified)
	if count.IsNull() || count.IsUnknown() {
		data.Count = types.Int32Value(1)
		return
	}
	data.Count = count
}

// ensureTenantNodeDeviceOwned checks that the device is still the one on this tenant node.
// VergeOS reuses machine_devices keys. A device whose machine is not this node's
// machine is treated as gone so the next apply creates instead of deleting the other row.
// Import leaves tenant_node_id empty. The node is found by its machine key.
func (a *API) ensureTenantNodeDeviceOwned(ctx context.Context, data *TenantNodeDeviceResourceModel, device *vergeos.VMDevice) error {
	if device.MachineType != "" && device.MachineType != vergeos.DeviceMachineTypeTenant {
		return &vergeos.NotFoundError{Resource: "TenantNodeDevice", ID: device.Key.Int()}
	}
	nodeID, ok, err := configuredPositiveID(data.TenantNodeID)
	if err != nil {
		return err
	}
	if ok {
		node, err := a.sdk.TenantNodes.Get(ctx, nodeID)
		if err != nil {
			if vergeos.IsNotFoundError(err) {
				return fmt.Errorf("tenant node %d was not found while reading device %d", nodeID, device.Key.Int())
			}
			return err
		}
		if node.IsSnapshot || node.Machine.Int() != device.Machine {
			return &vergeos.NotFoundError{Resource: "TenantNodeDevice", ID: device.Key.Int()}
		}
		return nil
	}
	nodes, err := a.sdk.TenantNodes.List(ctx)
	if err != nil {
		return err
	}
	var match *vergeos.TenantNode
	for i := range nodes {
		node := &nodes[i]
		if node.IsSnapshot || node.Machine.Int() != device.Machine || node.Key.Int() <= 0 {
			continue
		}
		if match != nil {
			return fmt.Errorf("device %d machine %d matches more than one tenant node", device.Key.Int(), device.Machine)
		}
		match = node
	}
	if match == nil {
		return fmt.Errorf("device %d is attached to machine %d, which is not a tenant node", device.Key.Int(), device.Machine)
	}
	data.TenantNodeID = idString(match.Key.Int())
	return nil
}

func (a *API) createTenantNodeDevice(ctx context.Context, data *TenantNodeDeviceResourceModel) error {
	nodeID, err := parseID(data.TenantNodeID, "tenant node")
	if err != nil {
		return err
	}
	req, err := tenantNodeDeviceCreateRequest(data)
	if err != nil {
		return err
	}
	created, err := a.sdk.TenantNodeDevices.Create(ctx, nodeID, req)
	if created != nil && created.Key.Int() > 0 {
		assignTenantNodeDevice(data, created)
		tflog.Debug(ctx, fmt.Sprintf("created tenant node device %d on node %d", created.Key.Int(), nodeID))
	}
	if err != nil {
		return err
	}
	if created == nil || created.Key.Int() <= 0 {
		return fmt.Errorf("VergeOS did not return an id for the tenant node device")
	}
	if err := a.ensureTenantNodeDeviceOwned(ctx, data, created); err != nil {
		if delErr := a.sdk.TenantNodeDevices.Delete(ctx, created.Key.Int()); delErr != nil && !vergeos.IsNotFoundError(delErr) {
			return fmt.Errorf("%w (also failed to delete device %d: %v)", err, created.Key.Int(), delErr)
		}
		data.Id = types.StringNull()
		return err
	}
	return nil
}

func (a *API) readTenantNodeDevice(ctx context.Context, data *TenantNodeDeviceResourceModel) error {
	id, err := parseID(data.Id, "tenant node device")
	if err != nil {
		return err
	}
	device, err := a.sdk.TenantNodeDevices.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := a.ensureTenantNodeDeviceOwned(ctx, data, device); err != nil {
		return err
	}
	assignTenantNodeDevice(data, device)
	return nil
}

func (a *API) updateTenantNodeDevice(ctx context.Context, plan, state *TenantNodeDeviceResourceModel) error {
	id, err := parseID(state.Id, "tenant node device")
	if err != nil {
		return err
	}
	plan.Id = state.Id
	req := tenantNodeDeviceUpdateRequest(plan, state)
	if req == nil {
		return nil
	}
	updated, err := a.sdk.TenantNodeDevices.Update(ctx, id, req)
	if err != nil {
		return err
	}
	if err := a.ensureTenantNodeDeviceOwned(ctx, plan, updated); err != nil {
		return err
	}
	assignTenantNodeDevice(plan, updated)
	tflog.Debug(ctx, fmt.Sprintf("updated tenant node device %d", id))
	return nil
}

func (a *API) deleteTenantNodeDevice(ctx context.Context, data *TenantNodeDeviceResourceModel) error {
	id, err := parseID(data.Id, "tenant node device")
	if err != nil {
		return err
	}
	if err := a.sdk.TenantNodeDevices.Delete(ctx, id); err != nil && !vergeos.IsNotFoundError(err) {
		return err
	}
	tflog.Debug(ctx, fmt.Sprintf("deleted tenant node device %d", id))
	return nil
}

func (a *API) readTenantNodeDevices(ctx context.Context, data *TenantNodeDevicesDataSourceModel) error {
	nodeID, err := parseID(data.TenantNodeID, "tenant node")
	if err != nil {
		return err
	}
	devices, err := a.sdk.TenantNodeDevices.List(ctx, nodeID)
	if err != nil {
		return err
	}
	sort.Slice(devices, func(i, j int) bool {
		if devices[i].Name != devices[j].Name {
			return devices[i].Name < devices[j].Name
		}
		return devices[i].Key.Int() < devices[j].Key.Int()
	})
	models := make([]*TenantNodeDeviceListModel, 0, len(devices))
	for i := range devices {
		models = append(models, tenantNodeDeviceListModel(&devices[i]))
	}
	data.Devices = models
	tflog.Debug(ctx, fmt.Sprintf("read %d devices on tenant node %d", len(models), nodeID))
	return nil
}

func tenantNodeDeviceListModel(device *vergeos.VMDevice) *TenantNodeDeviceListModel {
	group := types.StringNull()
	if device.ResourceGroup.String() != "" {
		group = types.StringValue(device.ResourceGroup.String())
	}
	machine := types.Int32Null()
	if device.Machine > 0 {
		machine = types.Int32Value(int32(device.Machine))
	}
	return &TenantNodeDeviceListModel{
		Id:            idString(device.Key.Int()),
		Name:          types.StringValue(device.Name),
		Type:          enumString(device.Type),
		Description:   types.StringValue(device.Description),
		ResourceGroup: group,
		Enabled:       types.BoolValue(device.Enabled),
		Optional:      types.BoolValue(device.Optional),
		Machine:       machine,
		MachineType:   enumString(device.MachineType),
		OrderID:       types.Int32Value(int32(device.OrderID)),
		UUID:          enumString(device.UUID),
		Created:       timestamp(device.Created),
		Modified:      timestamp(device.Modified),
	}
}
