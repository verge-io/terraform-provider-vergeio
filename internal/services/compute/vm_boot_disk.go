package compute

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// bootDiskModel is the one drive vergeio_vm owns.
// Other drives are vergeio_vm_drive resources. Name is how an existing
// drive is adopted so a config change does not create a second disk.
type bootDiskModel struct {
	Key    types.String  `tfsdk:"key"`
	Name   types.String  `tfsdk:"name"`
	Size   types.Float64 `tfsdk:"size"`
	Source types.Int32   `tfsdk:"source"`
	Media  types.String  `tfsdk:"media"`
}

func (b *bootDiskModel) toDisk(machine types.Int32) *diskResourceModel {
	if b == nil {
		return nil
	}
	return &diskResourceModel{
		Key:         b.Key,
		Machine:     machine,
		Name:        b.Name,
		DiskSize:    b.Size,
		MediaSource: b.Source,
		Media:       b.Media,
	}
}

func bootDiskFromDisk(disk *diskResourceModel, prior *bootDiskModel) *bootDiskModel {
	if disk == nil {
		return nil
	}
	boot := &bootDiskModel{
		Key:    disk.Key,
		Name:   disk.Name,
		Size:   disk.DiskSize,
		Source: disk.MediaSource,
		Media:  disk.Media,
	}
	if prior == nil {
		return boot
	}
	// readDisk does not return media or media_source. Keep the configured values.
	if boot.Media.IsNull() || boot.Media.IsUnknown() {
		boot.Media = prior.Media
	}
	if boot.Source.IsNull() || boot.Source.IsUnknown() {
		boot.Source = prior.Source
	}
	if boot.Name.IsNull() || boot.Name.IsUnknown() || boot.Name.ValueString() == "" {
		boot.Name = prior.Name
	}
	if boot.Size.IsNull() || boot.Size.IsUnknown() {
		boot.Size = prior.Size
	}
	return boot
}

// syncBootDisk creates, adopts, updates, or deletes the one drive this VM owns.
// Drives that belong to vergeio_vm_drive are left alone. Adopting matches
// the configured name to a drive that is already on the machine.
func (r *VMResource) syncBootDisk(ctx context.Context, plan, state *bootDiskModel, machine types.Int32, vmID types.String) (*bootDiskModel, error) {
	if plan == nil && state == nil {
		return nil, nil
	}
	if plan == nil {
		return nil, r.deleteBootDisk(ctx, state, vmID)
	}
	if machine.IsNull() || machine.IsUnknown() {
		return nil, fmt.Errorf("boot disk %q: VM has no machine id", plan.Name.ValueString())
	}

	ownedKey := ""
	if state != nil && !state.Key.IsNull() && !state.Key.IsUnknown() {
		ownedKey = state.Key.ValueString()
	}
	if ownedKey != "" {
		return r.updateBootDisk(ctx, plan, state, machine)
	}
	return r.adoptOrCreateBootDisk(ctx, plan, machine, vmID)
}

func (r *VMResource) deleteBootDisk(ctx context.Context, state *bootDiskModel, vmID types.String) error {
	if state == nil || state.Key.IsNull() || state.Key.IsUnknown() || state.Key.ValueString() == "" {
		return nil
	}
	disk := state.toDisk(types.Int32Null())
	if err := r.diskApi.deleteDisk(ctx, disk, vmID); err != nil {
		if notFound(err) {
			tflog.Debug(ctx, fmt.Sprintf("Boot disk %s is already gone", state.Key.ValueString()))
			return nil
		}
		return fmt.Errorf("failed to delete boot disk: %w", err)
	}
	return nil
}

func (r *VMResource) updateBootDisk(ctx context.Context, plan, state *bootDiskModel, machine types.Int32) (*bootDiskModel, error) {
	planned := plan.toDisk(machine)
	planned.Key = state.Key
	current := state.toDisk(machine)
	if knownMediaChanged(planned, current) {
		return nil, fmt.Errorf("boot disk %q: changing media or source replaces the VM", plan.Name.ValueString())
	}
	if !diskNeedsUpdate(planned, current) {
		return bootDiskFromDisk(current, plan), nil
	}
	if err := r.diskApi.updateDisk(ctx, planned, current); err != nil {
		return nil, fmt.Errorf("failed to update boot disk: %w", err)
	}
	return bootDiskFromDisk(current, plan), nil
}

func (r *VMResource) adoptOrCreateBootDisk(ctx context.Context, plan *bootDiskModel, machine types.Int32, vmID types.String) (*bootDiskModel, error) {
	name := plan.Name.ValueString()
	existing, err := r.diskApi.findDiskByName(ctx, machine.ValueInt32(), name)
	if err != nil {
		return nil, err
	}
	planned := plan.toDisk(machine)
	if existing != nil {
		tflog.Info(ctx, fmt.Sprintf("Adopting existing drive %q (key %s) as the boot disk of VM %s", name, existing.Key.ValueString(), vmID.ValueString()))
		planned.Key = existing.Key
		if knownMediaChanged(planned, existing) {
			return nil, fmt.Errorf("boot disk %q matches drive %s, but media or source differs; refusing to recreate it", name, existing.Key.ValueString())
		}
		if diskNeedsUpdate(planned, existing) {
			if err := r.diskApi.updateDisk(ctx, planned, existing); err != nil {
				return nil, fmt.Errorf("failed to update adopted boot disk: %w", err)
			}
			return bootDiskFromDisk(existing, plan), nil
		}
		return bootDiskFromDisk(existing, plan), nil
	}

	if err := r.diskApi.createDisk(ctx, planned); err != nil {
		return nil, fmt.Errorf("failed to create boot disk: %w", err)
	}
	return bootDiskFromDisk(planned, plan), nil
}

// knownMediaChanged reports a media or source change where both sides are known.
// A refresh often leaves media null because the drive GET omits it. That is
// not a change, and it must not replace the disk.
func knownMediaChanged(plan, state *diskResourceModel) bool {
	if plan == nil || state == nil {
		return false
	}
	if !state.Media.IsNull() && !state.Media.IsUnknown() && !plan.Media.IsNull() && !plan.Media.IsUnknown() && !plan.Media.Equal(state.Media) {
		return true
	}
	if !state.MediaSource.IsNull() && !state.MediaSource.IsUnknown() && !plan.MediaSource.IsNull() && !plan.MediaSource.IsUnknown() && !plan.MediaSource.Equal(state.MediaSource) {
		return true
	}
	return false
}

// refreshBootDisk re-reads the drive this VM owns.
// A VM with no boot_disk in state is left alone, so import and refresh do
// not adopt every drive on the machine or delete drives this resource does
// not own.
func (r *VMResource) refreshBootDisk(ctx context.Context, data *VMResourceModel) error {
	if data == nil || data.BootDisk == nil {
		return nil
	}
	if data.Machine.IsNull() || data.Machine.IsUnknown() {
		return nil
	}
	prior := *data.BootDisk
	key := ""
	if !prior.Key.IsNull() && !prior.Key.IsUnknown() {
		key = prior.Key.ValueString()
	}
	if key == "" && !prior.Name.IsNull() && !prior.Name.IsUnknown() {
		found, err := r.diskApi.findDiskByName(ctx, data.Machine.ValueInt32(), prior.Name.ValueString())
		if err != nil {
			return err
		}
		if found == nil {
			data.BootDisk = nil
			return nil
		}
		key = found.Key.ValueString()
	}
	if key == "" {
		return nil
	}
	disk := &diskResourceModel{Key: types.StringValue(key)}
	if err := r.diskApi.readDisk(ctx, disk); err != nil {
		if notFound(err) {
			data.BootDisk = nil
			return nil
		}
		return err
	}
	data.BootDisk = bootDiskFromDisk(disk, &prior)
	return nil
}
