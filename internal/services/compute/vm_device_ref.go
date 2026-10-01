package compute

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/verge-io/govergeos"
)

// replaceWhenVMChanges replaces the drive or NIC when vm_id changes to a
// different VM. A null state value is the first plan after an import that
// only had the device key, so filling vm_id in from configuration is not a
// replace.
func replaceWhenVMChanges() planmodifier.String {
	return stringplanmodifier.RequiresReplaceIf(
		func(_ context.Context, req planmodifier.StringRequest, resp *stringplanmodifier.RequiresReplaceIfFuncResponse) {
			if req.StateValue.IsNull() || req.StateValue.IsUnknown() {
				return
			}
			if strings.TrimSpace(req.StateValue.ValueString()) == "" {
				return
			}
			if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
				return
			}
			if req.ConfigValue.ValueString() != req.StateValue.ValueString() {
				resp.RequiresReplace = true
			}
		},
		"Changing vm_id replaces the drive or NIC.",
		"Changing vm_id replaces the drive or NIC.",
	)
}

// parseVMScopedImportID accepts a device key or vm_id/name.
// Name is matched on that VM. A slash in the name is kept: only the first
// slash separates the VM id.
func parseVMScopedImportID(id string) (vmID, name string, byName bool, err error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", "", false, errors.New("import id is empty")
	}
	vmID, name, ok := strings.Cut(id, "/")
	if !ok {
		return id, "", false, nil
	}
	vmID = strings.TrimSpace(vmID)
	name = strings.TrimSpace(name)
	if vmID == "" || name == "" {
		return "", "", false, fmt.Errorf("import id %q must be a device key or vm_id/name", id)
	}
	return vmID, name, true, nil
}

func notFound(err error) bool {
	if err == nil {
		return false
	}
	var clientErr vergeio.Error
	if errors.As(err, &clientErr) && clientErr.StatusCode == 404 {
		return true
	}
	if vergeos.IsNotFoundError(err) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not found") || strings.Contains(msg, "api error 404")
}

// findVMByMachine returns the live VM whose machine id matches.
// Snapshots are skipped. More than one live match is an error.
func (va *VMApi) findVMByMachine(ctx context.Context, machineID int32) (*vergeos.VM, error) {
	if va == nil || va.sdk == nil {
		return nil, errors.New("missing VM API client")
	}
	vms, err := va.sdk.VMs.List(ctx, vergeos.WithFilter(fmt.Sprintf("machine eq %d", machineID)))
	if err != nil {
		return nil, err
	}
	var found []vergeos.VM
	for _, vm := range vms {
		if vm.IsSnapshot || vm.Machine != int(machineID) {
			continue
		}
		found = append(found, vm)
	}
	switch len(found) {
	case 0:
		return nil, nil
	case 1:
		return va.sdk.VMs.Get(ctx, found[0].Key.Int())
	default:
		ids := make([]string, 0, len(found))
		for _, vm := range found {
			ids = append(ids, strconv.Itoa(vm.Key.Int()))
		}
		return nil, fmt.Errorf("%d VMs use machine %d (ids %s)", len(found), machineID, strings.Join(ids, ", "))
	}
}

func (va *VMApi) machineIDForVM(ctx context.Context, vmID string) (types.Int32, error) {
	vmID = strings.TrimSpace(vmID)
	if vmID == "" {
		return types.Int32Null(), errors.New("vm_id is empty")
	}
	data := &VMResourceModel{Id: types.StringValue(vmID)}
	if err := va.readVM(ctx, data); err != nil {
		return types.Int32Null(), err
	}
	if data.Machine.IsNull() || data.Machine.IsUnknown() {
		return types.Int32Null(), fmt.Errorf("VM %s has no machine id", vmID)
	}
	return data.Machine, nil
}

// vmScopedImportError is the diagnostic detail when an import id cannot be resolved.
func vmScopedImportError(kind, id string, err error) (string, string) {
	return fmt.Sprintf("Error importing %s", kind), fmt.Sprintf("Import id %q: %s", id, err.Error())
}
