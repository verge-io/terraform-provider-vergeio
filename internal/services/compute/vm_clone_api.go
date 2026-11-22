// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package compute

import (
	"context"
	"fmt"
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
	if err := va.sdk.VMs.Clone(ctx, snapVM.Key.Int(), &vergeos.VMCloneOptions{
		Name:         name,
		PreserveMACs: false,
	}); err != nil {
		return err
	}
	clone, err := waitVMByName(ctx, va.sdk, name)
	if err != nil {
		return err
	}
	if clone.Key.Int() == snapVM.Key.Int() || (live != nil && clone.Key.Int() == live.Key.Int()) || clone.IsSnapshot {
		return fmt.Errorf("clone %q resolved to VM %d, which is not a new copy", name, clone.Key.Int())
	}
	if err := va.attachCloneNetwork(ctx, clone.Key.Int(), networkID); err != nil {
		cleanupErr := va.deleteVMClone(ctx, clone.Key.Int(), liveVMKey(live))
		if cleanupErr != nil {
			return fmt.Errorf("attach clone %d to network %d: %w; delete clone: %v", clone.Key.Int(), networkID, err, cleanupErr)
		}
		return fmt.Errorf("attach clone %d to network %d: %w", clone.Key.Int(), networkID, err)
	}
	data.Id = types.StringValue(fmt.Sprintf("%d", clone.Key.Int()))
	data.SourceVMID = types.StringValue(fmt.Sprintf("%d", liveVMKey(live)))
	if liveVMKey(live) == 0 {
		data.SourceVMID = types.StringNull()
	}
	tflog.Debug(ctx, fmt.Sprintf("cloned snapshot VM %d as VM %d on network %d", snapVM.Key.Int(), clone.Key.Int(), networkID))
	return nil
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
	if vm.IsSnapshot || vm.Name != strings.TrimSpace(data.Name.ValueString()) {
		return &vergeos.NotFoundError{Resource: "VM", ID: id}
	}
	sourceID, sourceKnown, err := shared.KnownPositiveID(data.SourceVMID, "source_vm_id")
	if err != nil {
		return err
	}
	if sourceKnown && sourceID == id {
		return fmt.Errorf("clone id %d is the source VM", id)
	}
	data.Id = types.StringValue(fmt.Sprintf("%d", vm.Key.Int()))
	data.Name = types.StringValue(vm.Name)
	return nil
}

func (va *VMApi) deleteVMClone(ctx context.Context, id, sourceID int) error {
	if va == nil || va.sdk == nil {
		return fmt.Errorf("VM client is not configured")
	}
	if id <= 0 {
		return fmt.Errorf("clone id is required")
	}
	if sourceID > 0 && id == sourceID {
		return fmt.Errorf("refusing to delete source VM %d", sourceID)
	}
	vm, err := va.sdk.VMs.Get(ctx, id)
	if err != nil {
		if vergeos.IsNotFoundError(err) {
			return nil
		}
		return err
	}
	if vm.IsSnapshot {
		return fmt.Errorf("refusing to delete snapshot VM %d", id)
	}
	if err := va.sdk.VMs.PowerOff(ctx, id); err != nil && !vergeos.IsNotFoundError(err) {
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
