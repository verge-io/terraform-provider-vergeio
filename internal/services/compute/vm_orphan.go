// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package compute

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"terraform-provider-vergeio/internal/client"
	"terraform-provider-vergeio/internal/shared"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

// vmNameInUse reports a create that the API rejected because the VM name
// is taken. VergeOS has answered that collision as 422 and, on current
// releases, as 409. Other 409 and 422 responses are left alone.
func vmNameInUse(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *vergeos.APIError
	if errors.As(err, &apiErr) {
		return nameConflictStatus(apiErr.StatusCode) && nameAlreadyInUse(apiErr.Message)
	}
	var clientErr vergeio.Error
	if errors.As(err, &clientErr) {
		return nameConflictStatus(clientErr.StatusCode) && nameAlreadyInUse(clientErr.VergeError)
	}
	msg := err.Error()
	if !nameAlreadyInUse(msg) {
		return false
	}
	return strings.Contains(msg, "409") || strings.Contains(msg, "422")
}

func nameConflictStatus(code int) bool {
	return code == 409 || code == 422
}

func nameAlreadyInUse(msg string) bool {
	return strings.Contains(strings.ToLower(msg), "already in use")
}

func vmNameFilter(name string) string {
	return fmt.Sprintf("name eq '%s'", vergeio.EscapeFilterValue(name))
}

// findVMByName returns the live VM with this exact name.
// A nil VM and a nil error means nothing matched. Snapshots are skipped.
// More than one live match is an error so create does not guess.
func (va *VMApi) findVMByName(ctx context.Context, name string) (*vergeos.VM, error) {
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("VM name is empty")
	}

	vms, err := va.sdk.VMs.List(ctx, vergeos.WithFilter(vmNameFilter(name)))
	if err != nil {
		return nil, err
	}
	vms = vergeio.KeepExact(vms, name, func(vm vergeos.VM) string { return vm.Name })

	var found []vergeos.VM
	for _, vm := range vms {
		if vm.IsSnapshot {
			continue
		}
		found = append(found, vm)
	}
	switch len(found) {
	case 0:
		return nil, nil
	case 1:
		full, err := va.sdk.VMs.Get(ctx, found[0].Key.Int())
		if err != nil {
			return nil, fmt.Errorf("reading VM %q (id %d): %w", name, found[0].Key.Int(), err)
		}
		return full, nil
	default:
		ids := make([]string, 0, len(found))
		for _, vm := range found {
			ids = append(ids, strconv.Itoa(vm.Key.Int()))
		}
		return nil, fmt.Errorf("%d VMs are named %q (ids %s)", len(found), name, strings.Join(ids, ", "))
	}
}

// adoptOrphanVM loads the existing VM when a create hit a name collision
// and the VM matches the planned configuration. The id is set before the
// read so a failed read can still be stored in state.
func (r *VMResource) adoptOrphanVM(ctx context.Context, data *VMResourceModel) error {
	name := data.Name.ValueString()
	existing, err := r.vmApi.findVMByName(ctx, name)
	if err != nil {
		return fmt.Errorf("VM %q is already in use, and looking it up failed: %w. Delete it in VergeOS, or import it with `terraform import vergeio_vm.<name> <id>`", name, err)
	}
	if existing == nil {
		return fmt.Errorf("VM %q is already in use, but no VM with that name was found. Delete the conflicting VM in VergeOS, or import it with `terraform import vergeio_vm.<name> <id>`", name)
	}

	id := strconv.Itoa(existing.Key.Int())
	if ok, reason := vmConfigMatches(data, existing); !ok {
		return fmt.Errorf("VM %q already exists with id %s and was not adopted because %s. Import it with `terraform import vergeio_vm.<name> %s`, or delete the VM in VergeOS and apply again", name, id, reason, id)
	}

	data.Id = types.StringValue(id)
	tflog.Info(ctx, fmt.Sprintf("Adopting existing VM %s (id %s) after a name conflict", name, id))
	if _, err := r.vmApi.readVM(ctx, data, false); err != nil {
		return fmt.Errorf("VM %q (id %s) matches this configuration but reading it failed: %w", name, id, err)
	}
	return nil
}

// vmConfigMatches reports whether an existing VM is the one this create
// would have made. Only attributes the plan sets are compared, so API
// defaults do not block adoption. The console password is skipped because
// the API does not return it. Cloud-init files are skipped because this VM
// record does not include their bodies. Drives and
// NICs are skipped because a partial create may not have finished them.
// A stopped VM is still a match when the plan powers it on; create
// turns it on after the devices exist.
func vmConfigMatches(plan *VMResourceModel, existing *vergeos.VM) (bool, string) {
	if plan == nil || existing == nil {
		return false, "the VM was not found"
	}
	if existing.IsSnapshot {
		return false, "it is a snapshot"
	}

	var diffs []string
	diffString("name", plan.Name, existing.Name, &diffs)
	diffString("description", plan.Description, existing.Description, &diffs)
	diffBool("enabled", plan.Enabled, existing.Enabled, &diffs)
	diffMachineType(plan.MachineType, existing.MachineType, &diffs)
	diffBool("allow_hotplug", plan.AllowHotplug, existing.AllowHotplug, &diffs)
	diffBool("disable_powercycle", plan.DisablePowercycle, existing.DisablePowercycle, &diffs)
	diffString("on_power_loss", plan.OnPowerLoss, existing.OnPowerLoss, &diffs)
	diffInt("cpu_cores", plan.CPUCores, existing.CPUCores, &diffs)
	diffString("cpu_type", plan.CPUType, existing.CPUType, &diffs)
	diffInt("ram", plan.RAM, existing.RAM, &diffs)
	diffString("console", plan.Console, existing.Console, &diffs)
	diffString("display", plan.Display, existing.Display, &diffs)
	diffString("video", plan.Video, existing.Video, &diffs)
	diffString("sound", plan.Sound, existing.Sound, &diffs)
	diffString("os_family", plan.OSFamily, existing.OSFamily, &diffs)
	diffString("os_description", plan.OSDescription, existing.OSDescription, &diffs)
	diffString("rtc_base", plan.RTCBase, existing.RTCBase, &diffs)
	diffString("boot_order", plan.BootOrder, existing.BootOrder, &diffs)
	diffBool("console_pass_enabled", plan.ConsolePassEnabled, existing.ConsolePassEnabled, &diffs)
	diffBool("usb_tablet", plan.USBTablet, existing.USBTablet, &diffs)
	diffBool("uefi", plan.UEFI, existing.UEFI, &diffs)
	diffBool("secure_boot", plan.SecureBoot, existing.SecureBoot, &diffs)
	diffBool("serial_port", plan.SerialPort, existing.SerialPort, &diffs)
	diffInt("boot_delay", plan.BootDelay, existing.BootDelay, &diffs)
	diffInt("preferred_node", plan.PreferredNode, existing.PreferredNode.Int(), &diffs)
	diffInt("snapshot_profile", plan.SnapshotProfile, existing.SnapshotProfile.Int(), &diffs)
	diffInt("cluster", plan.Cluster, existing.Cluster.Int(), &diffs)
	diffString("cloudinit_datasource", plan.CloudInitDataSource, existing.CloudInitDataSource, &diffs)
	diffBool("guest_agent", plan.GuestAgent, existing.GuestAgent, &diffs)
	diffString("ha_group", plan.HAGroup, existing.HAGroup, &diffs)
	diffString("advanced", plan.Advanced, existing.Advanced, &diffs)
	diffBool("nested_virtualization", plan.NestedVirtualization, existing.NestedVirtualization, &diffs)
	diffBool("disable_hypervisor", plan.DisableHypervisor, existing.DisableHypervisor, &diffs)
	// A VM the configuration leaves off must already be off. Create powers a
	// VM on when asked, but it does not power one off, so a running orphan
	// would not match a plan that says powerstate = false.
	if !plan.PowerState.IsNull() && !plan.PowerState.IsUnknown() && !plan.PowerState.ValueBool() && existing.PowerState {
		diffs = append(diffs, "powerstate is true, configuration is false")
	}

	if len(diffs) > 0 {
		return false, strings.Join(diffs, "; ")
	}
	return true, ""
}

func diffString(attr string, planned types.String, got string, diffs *[]string) {
	if planned.IsNull() || planned.IsUnknown() {
		return
	}
	if planned.ValueString() == got {
		return
	}
	*diffs = append(*diffs, fmt.Sprintf("%s is %q, configuration is %q", attr, got, planned.ValueString()))
}

func diffMachineType(planned types.String, got string, diffs *[]string) {
	if planned.IsNull() || planned.IsUnknown() {
		return
	}
	if machineTypesMatch(planned.ValueString(), got) {
		return
	}
	*diffs = append(*diffs, fmt.Sprintf("machine_type is %q, configuration is %q", got, planned.ValueString()))
}

func machineTypesMatch(a, b string) bool {
	return shared.MachineTypesAreEquivalent(a, b) || shared.MachineTypesAreEquivalent(b, a)
}

func diffBool(attr string, planned types.Bool, got bool, diffs *[]string) {
	if planned.IsNull() || planned.IsUnknown() {
		return
	}
	if planned.ValueBool() == got {
		return
	}
	*diffs = append(*diffs, fmt.Sprintf("%s is %t, configuration is %t", attr, got, planned.ValueBool()))
}

func diffInt(attr string, planned types.Int32, got int, diffs *[]string) {
	if planned.IsNull() || planned.IsUnknown() {
		return
	}
	if int(planned.ValueInt32()) == got {
		return
	}
	*diffs = append(*diffs, fmt.Sprintf("%s is %d, configuration is %d", attr, got, planned.ValueInt32()))
}

func vmIDSet(data *VMResourceModel) bool {
	return data != nil && !data.Id.IsNull() && !data.Id.IsUnknown() && strings.TrimSpace(data.Id.ValueString()) != ""
}

// rememberVM writes the VM into the create response. Terraform keeps that
// state when a later step returns an error, so the next plan updates or
// replaces this VM instead of creating a second one with the same name.
// A boot disk or device is included once its key is known.
func (r *VMResource) rememberVM(ctx context.Context, resp *resource.CreateResponse, data *VMResourceModel) {
	if !vmIDSet(data) {
		return
	}
	partial := partialVMForState(data)
	resp.Diagnostics.Append(resp.State.Set(ctx, &partial)...)
	if resp.Diagnostics.HasError() {
		return
	}
	tflog.Debug(ctx, fmt.Sprintf("Stored VM %s in state", data.Id.ValueString()))
}

// createError stores whatever create has already made, then records why it
// stopped. rememberVM does nothing until the VM id is known, so a failure
// before the VM row exists still leaves state empty. When the id is known
// but state could not be written, the diagnostic says how to import it.
func (r *VMResource) createError(ctx context.Context, resp *resource.CreateResponse, data *VMResourceModel, summary, detail string) {
	r.rememberVM(ctx, resp, data)
	if vmIDSet(data) && responseVMID(ctx, resp) == "" {
		resp.Diagnostics.AddError("Error saving VM state", vmNotInStateDetail(data))
	}
	resp.Diagnostics.AddError(summary, detail)
}

func vmNotInStateDetail(data *VMResourceModel) string {
	name := ""
	id := ""
	if data != nil {
		name = data.Name.ValueString()
		id = data.Id.ValueString()
	}
	return fmt.Sprintf("VM %q (id %s) exists in VergeOS but could not be stored in Terraform state. Import it with `terraform import vergeio_vm.<name> %s`, or delete it in VergeOS and apply again.", name, id, id)
}

// responseVMID is the VM id currently stored on the create response.
// An empty string means state was never written or could not be read.
func responseVMID(ctx context.Context, resp *resource.CreateResponse) string {
	if resp == nil || resp.State.Raw.Type() == nil || resp.State.Raw.IsNull() {
		return ""
	}
	var got VMResourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		return ""
	}
	if !vmIDSet(&got) {
		return ""
	}
	return got.Id.ValueString()
}

// partialVMForState is the model stored as soon as the VM row exists, and
// again after each later object is created. Nested blocks without a key are
// omitted because state cannot hold unknown values. The original model is
// left intact so create can still finish those drives and devices.
func partialVMForState(data *VMResourceModel) VMResourceModel {
	partial := *data
	partial.Machine = knownInt32(data.Machine)
	partial.Name = knownString(data.Name)
	partial.Cluster = knownInt32(data.Cluster)
	partial.Description = knownString(data.Description)
	partial.Enabled = knownBool(data.Enabled)
	partial.MachineType = knownString(data.MachineType)
	partial.AllowHotplug = knownBool(data.AllowHotplug)
	partial.DisablePowercycle = knownBool(data.DisablePowercycle)
	partial.OnPowerLoss = knownString(data.OnPowerLoss)
	partial.CPUCores = knownInt32(data.CPUCores)
	partial.CPUType = knownString(data.CPUType)
	partial.RAM = knownInt32(data.RAM)
	partial.Console = knownString(data.Console)
	partial.Display = knownString(data.Display)
	partial.Video = knownString(data.Video)
	partial.Sound = knownString(data.Sound)
	partial.OSFamily = knownString(data.OSFamily)
	partial.OSDescription = knownString(data.OSDescription)
	partial.RTCBase = knownString(data.RTCBase)
	partial.BootOrder = knownString(data.BootOrder)
	partial.ConsolePassEnabled = knownBool(data.ConsolePassEnabled)
	partial.ConsolePass = knownString(data.ConsolePass)
	partial.USBTablet = knownBool(data.USBTablet)
	partial.UEFI = knownBool(data.UEFI)
	partial.SecureBoot = knownBool(data.SecureBoot)
	partial.SerialPort = knownBool(data.SerialPort)
	partial.BootDelay = knownInt32(data.BootDelay)
	partial.PreferredNode = knownInt32(data.PreferredNode)
	partial.SnapshotProfile = knownInt32(data.SnapshotProfile)
	partial.CloudInitDataSource = knownString(data.CloudInitDataSource)
	partial.HAGroup = knownString(data.HAGroup)
	partial.CloudInitFiles = knownCloudInitFiles(data.CloudInitFiles)
	partial.PowerState = knownBool(data.PowerState)
	partial.ForcePowerOff = knownBool(data.ForcePowerOff)
	partial.ShutdownOnDestroy = knownString(data.ShutdownOnDestroy)
	partial.Timeouts = knownTimeouts(data.Timeouts)
	partial.GuestAgent = knownBool(data.GuestAgent)
	partial.Advanced = knownString(data.Advanced)
	partial.WaitForGuestAgentInfo = knownInt32(data.WaitForGuestAgentInfo)
	partial.BootDisk = knownBootDisk(data.BootDisk)
	partial.Devices = knownDevices(data.Devices)
	partial.GuestAgentIPs = knownStringList(data.GuestAgentIPs)
	partial.NestedVirtualization = knownBool(data.NestedVirtualization)
	partial.DisableHypervisor = knownBool(data.DisableHypervisor)
	partial.WaitForGuestIPTimeout = knownInt32(data.WaitForGuestIPTimeout)
	partial.IgnoredGuestIPs = knownString(data.IgnoredGuestIPs)
	return partial
}

func knownString(v types.String) types.String {
	if v.IsNull() || v.IsUnknown() {
		return types.StringNull()
	}
	return v
}

func knownBool(v types.Bool) types.Bool {
	if v.IsNull() || v.IsUnknown() {
		return types.BoolNull()
	}
	return v
}

func knownInt32(v types.Int32) types.Int32 {
	if v.IsNull() || v.IsUnknown() {
		return types.Int32Null()
	}
	return v
}

func knownFloat64(v types.Float64) types.Float64 {
	if v.IsNull() || v.IsUnknown() {
		return types.Float64Null()
	}
	return v
}

func knownTimeouts(timeouts *vmTimeoutsModel) *vmTimeoutsModel {
	if timeouts == nil {
		return nil
	}
	out := *timeouts
	out.Update = knownString(timeouts.Update)
	out.Delete = knownString(timeouts.Delete)
	return &out
}

// knownBootDisk keeps a boot disk whose key VergeOS has already assigned.
// A planned disk whose key is still unknown is left out of state.
func knownBootDisk(disk *bootDiskModel) *bootDiskModel {
	if !bootDiskKeySet(disk) {
		return nil
	}
	out := *disk
	out.Key = knownString(disk.Key)
	out.Name = knownString(disk.Name)
	out.Size = knownFloat64(disk.Size)
	out.Source = knownInt32(disk.Source)
	out.Media = knownString(disk.Media)
	return &out
}

// knownDevices keeps devices that already have a key, in plan order.
// A device whose create failed before VergeOS returned a key is omitted
// so state does not store an unknown value.
func knownDevices(devices []*deviceResourceModel) []*deviceResourceModel {
	if len(devices) == 0 {
		return nil
	}
	out := make([]*deviceResourceModel, 0, len(devices))
	for _, device := range devices {
		stored := knownDevice(device)
		if stored == nil {
			continue
		}
		out = append(out, stored)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func knownDevice(device *deviceResourceModel) *deviceResourceModel {
	if device == nil || device.Key.IsNull() || device.Key.IsUnknown() || strings.TrimSpace(device.Key.ValueString()) == "" {
		return nil
	}
	out := *device
	out.Key = knownString(device.Key)
	out.Machine = knownInt32(device.Machine)
	out.Type = knownString(device.Type)
	out.Name = knownString(device.Name)
	out.Description = knownString(device.Description)
	out.ResourceGroup = knownString(device.ResourceGroup)
	out.Enabled = knownBool(device.Enabled)
	out.Status = knownInt32(device.Status)
	out.DeviceUSBSettingsModel = knownUSBSettings(device.DeviceUSBSettingsModel)
	out.DeviceTPMSettingsModel = knownTPMSettings(device.DeviceTPMSettingsModel)
	out.DeviceNvidiaVGPUSettingsModel = knownVGPUSettings(device.DeviceNvidiaVGPUSettingsModel)
	return &out
}

func knownUSBSettings(in *DeviceUSBSettingsModel) *DeviceUSBSettingsModel {
	if in == nil {
		return nil
	}
	out := *in
	out.Key = knownInt32(in.Key)
	out.MachineDevice = knownInt32(in.MachineDevice)
	out.GuestReset = knownBool(in.GuestReset)
	out.GuestResetsAll = knownBool(in.GuestResetsAll)
	return &out
}

func knownTPMSettings(in *DeviceTPMSettingsModel) *DeviceTPMSettingsModel {
	if in == nil {
		return nil
	}
	out := *in
	out.Key = knownInt32(in.Key)
	out.MachineDevice = knownInt32(in.MachineDevice)
	out.Model = knownString(in.Model)
	out.Version = knownTPMVersion(in.Version)
	return &out
}

func knownTPMVersion(v TPMVersion) TPMVersion {
	if v.IsNull() || v.IsUnknown() {
		return NewTPMVersionNull()
	}
	return v
}

func knownVGPUSettings(in *DeviceNvidiaVGPUSettingsModel) *DeviceNvidiaVGPUSettingsModel {
	if in == nil {
		return nil
	}
	out := *in
	out.Key = knownInt32(in.Key)
	out.MachineDevice = knownInt32(in.MachineDevice)
	out.ProfileType = knownString(in.ProfileType)
	out.FrameRateLimiter = knownInt32(in.FrameRateLimiter)
	out.DisableVNC = knownBool(in.DisableVNC)
	out.EnableUVM = knownBool(in.EnableUVM)
	out.EnableDebugging = knownBool(in.EnableDebugging)
	out.EnableProfiling = knownBool(in.EnableProfiling)
	return &out
}

func knownStringList(v types.List) types.List {
	if v.IsNull() || v.IsUnknown() {
		return types.ListNull(types.StringType)
	}
	return v
}

func knownCloudInitFiles(files []CloudInitFile) []CloudInitFile {
	if len(files) == 0 {
		return nil
	}
	for _, file := range files {
		if file.Name.IsUnknown() || file.Contents.IsUnknown() {
			return nil
		}
	}
	return files
}
