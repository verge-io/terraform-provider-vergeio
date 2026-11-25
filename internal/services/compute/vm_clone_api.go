// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package compute

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"terraform-provider-vergeio/internal/shared"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

// vmClonePoll is how often a clone name is read. vmCloneTimeout is the whole wait.
var (
	vmClonePoll    = 2 * time.Second
	vmCloneTimeout = 2 * time.Minute
)

// vmCloneCreatedSkew is how far behind this process a VergeOS clock can be
// and still count as the copy Clone just stored.
const vmCloneCreatedSkew = 2 * time.Minute

// vmCloneLookupTimeout bounds the read after a wait failure. The caller
// context may already be canceled.
const vmCloneLookupTimeout = 30 * time.Second

func (va *VMApi) createVMClone(ctx context.Context, data *VMCloneResourceModel) error {
	if va == nil || va.sdk == nil {
		return fmt.Errorf("VM client is not configured")
	}
	snapshotID, err := positiveID(data.SnapshotID, "snapshot_id")
	if err != nil {
		return err
	}
	networkID, err := positiveID(data.NetworkID, "network_id")
	if err != nil {
		return err
	}
	name := strings.TrimSpace(data.Name.ValueString())
	if name == "" {
		return fmt.Errorf("name is required")
	}
	snap, err := va.sdk.VMSnapshots.Get(ctx, snapshotID)
	if err != nil {
		return err
	}
	if snap.SnapMachine.Int() <= 0 {
		return fmt.Errorf("snapshot %d has no snap_machine", snapshotID)
	}
	snapVM, err := vmByMachine(ctx, va.sdk, snap.SnapMachine.Int(), true)
	if err != nil {
		return fmt.Errorf("snapshot %d: %w", snapshotID, err)
	}
	live, err := vmByMachine(ctx, va.sdk, snap.Machine.Int(), false)
	if err != nil && !vergeos.IsNotFoundError(err) {
		return err
	}
	if live != nil && live.Key.Int() == snapVM.Key.Int() {
		return fmt.Errorf("snapshot %d resolved to live VM %d", snapshotID, live.Key.Int())
	}
	existing, err := va.sdk.VMs.GetByName(ctx, name)
	if err != nil && !vergeos.IsNotFoundError(err) {
		return err
	}
	if existing != nil && existing.Key.Int() > 0 {
		return fmt.Errorf("VM %q already exists", name)
	}
	started := time.Now()
	if err := va.sdk.VMs.Clone(ctx, snapVM.Key.Int(), &vergeos.VMCloneOptions{
		Name:         name,
		PreserveMACs: false,
	}); err != nil {
		return err
	}
	clone, err := waitVMByName(ctx, va.sdk, name)
	if err != nil {
		found, findErr := va.lookupVMByName(ctx, name)
		if findErr != nil {
			return fmt.Errorf("%w; looking up VM %q: %v", err, name, findErr)
		}
		if !vmCloneIsNew(found, name, started, snapVM.Key.Int(), liveVMKey(live)) {
			return err
		}
		assignVMClone(data, found, live)
		return err
	}
	if !vmCloneIsCopy(clone, name, snapVM.Key.Int(), liveVMKey(live)) {
		return fmt.Errorf("clone %q resolved to VM %d, which is not a new copy", name, clone.Key.Int())
	}
	if err := va.attachCloneNetwork(ctx, clone.Key.Int(), networkID); err != nil {
		cleanup := *data
		assignVMClone(&cleanup, clone, live)
		cleanup.Name = types.StringValue(clone.Name)
		cleanupErr := va.deleteVMClone(ctx, &cleanup)
		if cleanupErr != nil {
			return fmt.Errorf("attach clone %d to network %d: %w; delete clone: %v", clone.Key.Int(), networkID, err, cleanupErr)
		}
		return fmt.Errorf("attach clone %d to network %d: %w", clone.Key.Int(), networkID, err)
	}
	assignVMClone(data, clone, live)
	tflog.Debug(ctx, fmt.Sprintf("cloned snapshot VM %d as VM %d on network %d", snapVM.Key.Int(), clone.Key.Int(), networkID))
	return nil
}

func assignVMClone(data *VMCloneResourceModel, clone *vergeos.VM, live *vergeos.VM) {
	data.Id = types.StringValue(strconv.Itoa(clone.Key.Int()))
	data.Created = vmCloneCreated(clone.Created)
	if liveVMKey(live) == 0 {
		data.SourceVMID = types.StringNull()
		return
	}
	data.SourceVMID = types.StringValue(strconv.Itoa(liveVMKey(live)))
}

func vmCloneCreated(v int64) types.Int64 {
	if v == 0 {
		return types.Int64Null()
	}
	return types.Int64Value(v)
}

// vmCloneIsCopy reports that vm is a new VM with the requested name.
// A snapshot, the snapshot VM, and the live VM are not.
func vmCloneIsCopy(vm *vergeos.VM, name string, deny ...int) bool {
	if vm == nil || vm.Key.Int() <= 0 || vm.IsSnapshot || vm.Name != name {
		return false
	}
	for _, id := range deny {
		if id > 0 && vm.Key.Int() == id {
			return false
		}
	}
	return true
}

// vmCloneIsNew is vmCloneIsCopy plus the created time. A row that VergeOS
// stored before this clone is not the copy the wait missed.
func vmCloneIsNew(vm *vergeos.VM, name string, started time.Time, deny ...int) bool {
	if !vmCloneIsCopy(vm, name, deny...) {
		return false
	}
	if vm.Created > 0 && vm.Created < started.Add(-vmCloneCreatedSkew).Unix() {
		return false
	}
	return true
}

func (va *VMApi) lookupVMByName(ctx context.Context, name string) (*vergeos.VM, error) {
	lookupCtx, cancel := vmCloneLookupContext(ctx)
	defer cancel()
	vm, err := va.sdk.VMs.GetByName(lookupCtx, name)
	if err != nil {
		if vergeos.IsNotFoundError(err) {
			return nil, nil
		}
		return nil, err
	}
	return vm, nil
}

func vmCloneLookupContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(context.WithoutCancel(ctx), vmCloneLookupTimeout)
}

func liveVMKey(vm *vergeos.VM) int {
	if vm == nil {
		return 0
	}
	return vm.Key.Int()
}

func (va *VMApi) attachCloneNetwork(ctx context.Context, cloneID, networkID int) error {
	nics, err := va.sdk.VMNICs.List(ctx, cloneID)
	if err != nil {
		return err
	}
	if len(nics) == 0 {
		enabled := true
		_, err := va.sdk.VMNICs.Create(ctx, cloneID, &vergeos.VMNICCreateRequest{
			Name:      "lan",
			Interface: "virtio",
			VNET:      networkID,
			Enabled:   &enabled,
		})
		return err
	}
	for _, nic := range nics {
		vnet := networkID
		if _, err := va.sdk.VMNICs.Update(ctx, nic.Key.Int(), &vergeos.VMNICUpdateRequest{VNET: &vnet}); err != nil {
			return fmt.Errorf("NIC %d: %w", nic.Key.Int(), err)
		}
	}
	checked, err := va.sdk.VMNICs.List(ctx, cloneID)
	if err != nil {
		return err
	}
	if len(checked) == 0 {
		return fmt.Errorf("clone %d has no NIC on network %d", cloneID, networkID)
	}
	for _, nic := range checked {
		if nic.VNET.Int() != networkID {
			return fmt.Errorf("NIC %d is on network %d, want %d", nic.Key.Int(), nic.VNET.Int(), networkID)
		}
	}
	return nil
}

func (va *VMApi) readVMClone(ctx context.Context, data *VMCloneResourceModel) error {
	if va == nil || va.sdk == nil {
		return fmt.Errorf("VM client is not configured")
	}
	id, err := positiveID(data.Id, "id")
	if err != nil {
		return err
	}
	vm, err := va.sdk.VMs.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := ensureVMCloneOwned(data, vm); err != nil {
		return err
	}
	sourceID, err := optionalPositiveID(data.SourceVMID)
	if err != nil {
		return err
	}
	if sourceID > 0 && sourceID == id {
		return fmt.Errorf("clone id %d is the source VM", id)
	}
	data.Id = types.StringValue(strconv.Itoa(vm.Key.Int()))
	data.Name = types.StringValue(vm.Name)
	data.Created = vmCloneCreated(vm.Created)
	return nil
}

// ensureVMCloneOwned returns NotFound when the stored key now points at
// another VM. Name and created are both checked when state has them.
// Created is skipped when it was not stored (import, or state from before
// that attribute existed). A snapshot row is not the copy.
func ensureVMCloneOwned(data *VMCloneResourceModel, vm *vergeos.VM) error {
	if data == nil || vm == nil {
		return fmt.Errorf("VM clone read returned no row")
	}
	if vm.IsSnapshot {
		return &vergeos.NotFoundError{Resource: "VM", ID: vm.Key.Int()}
	}
	if nameKnown(data.Name) && vm.Name != strings.TrimSpace(data.Name.ValueString()) {
		return &vergeos.NotFoundError{Resource: "VM", ID: vm.Key.Int()}
	}
	if createdKnown(data.Created) && vm.Created != data.Created.ValueInt64() {
		return &vergeos.NotFoundError{Resource: "VM", ID: vm.Key.Int()}
	}
	return nil
}

func nameKnown(value types.String) bool {
	return !value.IsNull() && !value.IsUnknown() && strings.TrimSpace(value.ValueString()) != ""
}

func createdKnown(value types.Int64) bool {
	return !value.IsNull() && !value.IsUnknown() && value.ValueInt64() != 0
}

func (va *VMApi) deleteVMClone(ctx context.Context, data *VMCloneResourceModel) error {
	if va == nil || va.sdk == nil {
		return fmt.Errorf("VM client is not configured")
	}
	if data == nil {
		return fmt.Errorf("clone id is required")
	}
	id, err := positiveID(data.Id, "id")
	if err != nil {
		return err
	}
	sourceID, err := optionalPositiveID(data.SourceVMID)
	if err != nil {
		return err
	}
	if sourceID > 0 && id == sourceID {
		return fmt.Errorf("refusing to delete source VM %d", sourceID)
	}
	vm, getErr := va.sdk.VMs.Get(ctx, id)
	if getErr != nil {
		if vergeos.IsNotFoundError(getErr) {
			return nil
		}
		return getErr
	}
	if vm.IsSnapshot {
		return fmt.Errorf("refusing to delete snapshot VM %d", id)
	}
	foreign, err := shared.ForeignRow(nil, ensureVMCloneOwned(data, vm))
	if err != nil {
		return err
	}
	if foreign {
		label := ""
		if nameKnown(data.Name) {
			label = strings.TrimSpace(data.Name.ValueString())
		}
		tflog.Debug(ctx, fmt.Sprintf("VM %d is not clone %q; leaving it in place", id, label))
		return nil
	}
	// Same stop as vergeio_vm destroy: one ACPI poweroff, then kill if the
	// guest is still running. VMService.PowerOff is not used here.
	if err := va.gracefulPowerOff(ctx, strconv.Itoa(id), vm.Name, gracefulShutdownTimeout, true); err != nil {
		return err
	}
	if err := va.sdk.VMs.Delete(ctx, id); err != nil && !vergeos.IsNotFoundError(err) {
		return err
	}
	tflog.Debug(ctx, fmt.Sprintf("deleted VM clone %d", id))
	return nil
}

func vmByMachine(ctx context.Context, sdk *vergeos.Client, machine int, snapshot bool) (*vergeos.VM, error) {
	if machine <= 0 {
		return nil, &vergeos.NotFoundError{Resource: "VM", ID: machine}
	}
	flag := "false"
	if snapshot {
		flag = "true"
	}
	vms, err := sdk.VMs.List(ctx, vergeos.WithFilter(fmt.Sprintf("machine eq %d and is_snapshot eq %s", machine, flag)))
	if err != nil {
		return nil, err
	}
	var match *vergeos.VM
	for i := range vms {
		vm := &vms[i]
		if vm.IsSnapshot != snapshot || vm.Key.Int() <= 0 {
			continue
		}
		if vm.Machine != 0 && vm.Machine != machine {
			continue
		}
		if match != nil {
			return nil, fmt.Errorf("machine %d matches VM %d and VM %d", machine, match.Key.Int(), vm.Key.Int())
		}
		match = vm
	}
	if match == nil {
		resource := "VM"
		if snapshot {
			resource = "snapshot VM"
		}
		return nil, &vergeos.NotFoundError{Resource: resource, ID: machine}
	}
	return match, nil
}

func waitVMByName(ctx context.Context, sdk *vergeos.Client, name string) (*vergeos.VM, error) {
	deadline := time.Now().Add(vmCloneTimeout)
	for {
		vm, err := sdk.VMs.GetByName(ctx, name)
		if err == nil && vm != nil && vm.Key.Int() > 0 && vm.Machine > 0 && !vm.IsSnapshot {
			return vm, nil
		}
		if err != nil && !vergeos.IsNotFoundError(err) {
			return nil, err
		}
		if !time.Now().Before(deadline) {
			return nil, fmt.Errorf("VM %q was not created", name)
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		timer := time.NewTimer(vmClonePoll)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

// optionalPositiveID reads source_vm_id. Null and unknown mean the live
// VM is gone or was not stored, which is valid. A known value must be a
// positive integer.
func optionalPositiveID(value types.String) (int, error) {
	if value.IsNull() || value.IsUnknown() {
		return 0, nil
	}
	return positiveID(value, "source_vm_id")
}

func positiveID(value types.String, what string) (int, error) {
	id, known, err := shared.KnownPositiveID(value, what)
	if err != nil {
		return 0, err
	}
	if !known {
		return 0, fmt.Errorf("%s is required", what)
	}
	return id, nil
}
