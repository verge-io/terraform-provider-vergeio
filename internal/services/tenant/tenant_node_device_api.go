// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tenant

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
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
	copies := 1
	if req.Count != nil && *req.Count > 1 {
		copies = *req.Count
	}
	var before []vergeos.VMDevice
	if copies > 1 {
		// The list is the only way to see the extra rows count creates.
		// Fail before posting when that list is unavailable, so a create
		// does not leave copies that cannot be tracked or removed.
		before, err = a.sdk.TenantNodeDevices.List(ctx, nodeID)
		if err != nil {
			return fmt.Errorf("list devices on tenant node %d before create: %w", nodeID, err)
		}
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
	ids := []int{created.Key.Int()}
	if copies > 1 {
		after, listErr := a.sdk.TenantNodeDevices.List(ctx, nodeID)
		if listErr != nil {
			// The returned row stays tracked. A failed list is not proof
			// the copies are someone else's, so they are not deleted.
			if setErr := setDeviceIDs(ctx, data, ids); setErr != nil {
				return setErr
			}
			return fmt.Errorf("created tenant node device %d, but could not list the other copies: %w", created.Key.Int(), listErr)
		}
		matched, matchErr := matchingCopyIDs(before, after, created, copies)
		if setErr := setDeviceIDs(ctx, data, matched); setErr != nil {
			return setErr
		}
		if matchErr != nil {
			// Too many or too few name matches. Keep only the row VergeOS
			// returned. Do not adopt or delete the others.
			return matchErr
		}
		ids = matched
	}
	if err := setDeviceIDs(ctx, data, ids); err != nil {
		return err
	}
	if err := a.ensureTenantNodeDeviceOwned(ctx, data, created); err != nil {
		if !vergeos.IsNotFoundError(err) {
			// A timeout or a 500 is not an ownership mismatch. Leave every
			// row in place and keep the ids in state.
			return err
		}
		left, delErr := a.deleteDeviceIDs(ctx, ids)
		if len(left) == 0 {
			data.Id = types.StringNull()
			data.DeviceIDs = types.ListNull(types.StringType)
		} else if setErr := setDeviceIDs(ctx, data, left); setErr != nil {
			delErr = errors.Join(delErr, setErr)
			data.Id = idString(left[0])
		} else {
			data.Id = idString(left[0])
		}
		if delErr != nil {
			return fmt.Errorf("device %d is not owned by tenant node %d, and deleting the new rows failed: %w", created.Key.Int(), nodeID, delErr)
		}
		return fmt.Errorf("device %d is not owned by tenant node %d", created.Key.Int(), nodeID)
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
	ids, err := trackedDeviceIDs(ctx, data)
	if err != nil {
		return err
	}
	kept := []int{device.Key.Int()}
	for _, id := range ids {
		if id == device.Key.Int() {
			continue
		}
		sibling, err := a.sdk.TenantNodeDevices.Get(ctx, id)
		if err != nil {
			if vergeos.IsNotFoundError(err) {
				continue
			}
			return err
		}
		if sibling.Machine != device.Machine || (sibling.MachineType != "" && sibling.MachineType != vergeos.DeviceMachineTypeTenant) {
			continue
		}
		kept = append(kept, id)
	}
	return setDeviceIDs(ctx, data, orderedDeviceIDs(kept, device.Key.Int()))
}

func (a *API) updateTenantNodeDevice(ctx context.Context, plan, state *TenantNodeDeviceResourceModel) error {
	id, err := parseID(state.Id, "tenant node device")
	if err != nil {
		return err
	}
	plan.Id = state.Id
	plan.DeviceIDs = state.DeviceIDs
	req := tenantNodeDeviceUpdateRequest(plan, state)
	if req == nil {
		return nil
	}
	ids, err := trackedDeviceIDs(ctx, state)
	if err != nil {
		return err
	}
	var updated *vergeos.VMDevice
	for _, deviceID := range ids {
		row, err := a.sdk.TenantNodeDevices.Update(ctx, deviceID, req)
		if err != nil {
			return err
		}
		if deviceID == id {
			updated = row
		}
	}
	if updated == nil {
		updated, err = a.sdk.TenantNodeDevices.Get(ctx, id)
		if err != nil {
			return err
		}
	}
	if err := a.ensureTenantNodeDeviceOwned(ctx, plan, updated); err != nil {
		return err
	}
	assignTenantNodeDevice(plan, updated)
	plan.DeviceIDs = state.DeviceIDs
	tflog.Debug(ctx, fmt.Sprintf("updated tenant node device %d", id))
	return nil
}

func (a *API) deleteTenantNodeDevice(ctx context.Context, data *TenantNodeDeviceResourceModel) error {
	ids, err := trackedDeviceIDs(ctx, data)
	if err != nil {
		return err
	}
	left, delErr := a.deleteDeviceIDs(ctx, ids)
	if len(left) == 0 {
		tflog.Debug(ctx, fmt.Sprintf("deleted tenant node device %s", data.Id.ValueString()))
		return delErr
	}
	if setErr := setDeviceIDs(ctx, data, left); setErr != nil {
		delErr = errors.Join(delErr, setErr)
	}
	data.Id = idString(left[0])
	if delErr != nil {
		return delErr
	}
	return fmt.Errorf("tenant node devices %v are still present", left)
}

// matchingCopyIDs returns the new rows that belong to this create.
// A copy has the created device's type and a name of that device or
// name-N. Any other new row on the node is left alone. The count has to
// match, so a second resource created in the same apply is not claimed.
func matchingCopyIDs(before, after []vergeos.VMDevice, created *vergeos.VMDevice, copies int) ([]int, error) {
	prior := make(map[int]struct{}, len(before))
	for i := range before {
		if id := before[i].Key.Int(); id > 0 {
			prior[id] = struct{}{}
		}
	}
	matched := make([]int, 0, copies)
	for i := range after {
		id := after[i].Key.Int()
		if id <= 0 {
			continue
		}
		if _, ok := prior[id]; ok {
			continue
		}
		if !deviceIsCopy(after[i], created) {
			continue
		}
		matched = append(matched, id)
	}
	ids := orderedDeviceIDs(matched, created.Key.Int())
	if len(ids) != copies {
		return []int{created.Key.Int()}, fmt.Errorf("tenant node device %d matched %d of %d copies named %q type %q", created.Key.Int(), len(ids), copies, created.Name, created.Type)
	}
	return ids, nil
}

func deviceIsCopy(device vergeos.VMDevice, created *vergeos.VMDevice) bool {
	if device.Key.Int() == created.Key.Int() {
		return true
	}
	if device.Type != created.Type || device.Machine != created.Machine {
		return false
	}
	if device.MachineType != "" && created.MachineType != "" && device.MachineType != created.MachineType {
		return false
	}
	return copyName(device.Name, created.Name)
}

func copyName(name, base string) bool {
	if name == base {
		return true
	}
	rest, ok := strings.CutPrefix(name, base+"-")
	if !ok || rest == "" {
		return false
	}
	n, err := strconv.Atoi(rest)
	return err == nil && n > 1
}

func orderedDeviceIDs(ids []int, primary int) []int {
	seen := map[int]struct{}{}
	rest := make([]int, 0, len(ids))
	for _, id := range ids {
		if id <= 0 || id == primary {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		rest = append(rest, id)
	}
	sort.Ints(rest)
	if primary <= 0 {
		return rest
	}
	return append([]int{primary}, rest...)
}

func (a *API) deleteDeviceIDs(ctx context.Context, ids []int) (left []int, err error) {
	var errs []error
	for _, id := range ids {
		delErr := a.sdk.TenantNodeDevices.Delete(ctx, id)
		if delErr == nil || vergeos.IsNotFoundError(delErr) {
			continue
		}
		errs = append(errs, fmt.Errorf("delete device %d: %w", id, delErr))
		left = append(left, id)
	}
	return left, errors.Join(errs...)
}

func trackedDeviceIDs(ctx context.Context, data *TenantNodeDeviceResourceModel) ([]int, error) {
	primary, err := parseID(data.Id, "tenant node device")
	if err != nil && (data.DeviceIDs.IsNull() || data.DeviceIDs.IsUnknown()) {
		return nil, err
	}
	if data.DeviceIDs.IsNull() || data.DeviceIDs.IsUnknown() {
		return []int{primary}, nil
	}
	var texts []string
	if diags := data.DeviceIDs.ElementsAs(ctx, &texts, false); diags.HasError() {
		return nil, fmt.Errorf("device_ids: %s", diags)
	}
	ids := make([]int, 0, len(texts)+1)
	if primary > 0 {
		ids = append(ids, primary)
	}
	for _, text := range texts {
		id, convErr := strconv.Atoi(strings.TrimSpace(text))
		if convErr != nil || id <= 0 {
			return nil, fmt.Errorf("device_ids value %q is not a positive integer", text)
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("tenant node device id is empty")
	}
	return orderedDeviceIDs(ids, primary), nil
}

func setDeviceIDs(ctx context.Context, data *TenantNodeDeviceResourceModel, ids []int) error {
	texts := make([]string, len(ids))
	for i, id := range ids {
		texts[i] = strconv.Itoa(id)
	}
	list, diags := types.ListValueFrom(ctx, types.StringType, texts)
	if diags.HasError() {
		return fmt.Errorf("device_ids: %s", diags)
	}
	data.DeviceIDs = list
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
