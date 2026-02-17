package vergeos

import (
	"context"
	"fmt"
	"net/url"
	"time"
)

const (
	// Power action constants
	vmActionPowerOn     = "poweron"
	vmActionPowerOff    = "poweroff"
	vmActionReset       = "reset"
	vmActionKill        = "kill"
	vmActionClone       = "clone"
	vmActionSnapshot    = "quiesce_snapshot"
	vmActionGuestReboot = "guestreboot"

	// Polling configuration
	powerStateMaxRetries   = 30
	powerStatePollInterval = 5 * time.Second
)

// VMService handles VM operations.
type VMService struct {
	client *Client
}

// List returns all VMs, with optional filtering and pagination.
func (s *VMService) List(ctx context.Context, opts ...ListOption) ([]VM, error) {
	options := applyListOptions(opts)

	// Use VM-specific fields if not specified
	if options.Fields == "most" {
		options.Fields = vmListFields
	}

	params := options.toQueryParams()

	var vms []VM
	if err := s.client.get(ctx, "/vms", params, &vms); err != nil {
		return nil, err
	}

	return vms, nil
}

// Get returns a single VM by ID.
func (s *VMService) Get(ctx context.Context, id int) (*VM, error) {
	params := url.Values{}
	params.Set("fields", vmGetFields)

	var vm VM
	endpoint := fmt.Sprintf("/vms/%d", id)
	if err := s.client.get(ctx, endpoint, params, &vm); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "VM", ID: id}
		}
		return nil, err
	}

	return &vm, nil
}

// Create creates a new VM and returns the created VM.
func (s *VMService) Create(ctx context.Context, req *VMCreateRequest) (*VM, error) {
	if req == nil {
		return nil, &ValidationError{Message: "create request is required"}
	}
	if req.Name == "" {
		return nil, &ValidationError{Field: "name", Message: "name is required"}
	}
	if req.CPUCores <= 0 {
		return nil, &ValidationError{Field: "cpu_cores", Message: "cpu_cores must be positive"}
	}
	if req.RAM <= 0 {
		return nil, &ValidationError{Field: "ram", Message: "ram must be positive"}
	}

	// Set defaults
	if req.Enabled == nil {
		enabled := true
		req.Enabled = &enabled
	}

	var resp apiResponse
	if err := s.client.post(ctx, "/vms", req, &resp); err != nil {
		return nil, err
	}

	// Extract the created VM's ID
	id, err := getKey(resp)
	if err != nil {
		return nil, err
	}

	// Read back the created VM
	return s.Get(ctx, id)
}

// Update updates a VM and returns the updated VM.
func (s *VMService) Update(ctx context.Context, id int, req *VMUpdateRequest) (*VM, error) {
	if req == nil {
		return nil, &ValidationError{Message: "update request is required"}
	}

	endpoint := fmt.Sprintf("/vms/%d", id)
	if err := s.client.put(ctx, endpoint, req, nil); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "VM", ID: id}
		}
		return nil, err
	}

	// Read back the updated VM
	return s.Get(ctx, id)
}

// Delete deletes a VM.
func (s *VMService) Delete(ctx context.Context, id int) error {
	endpoint := fmt.Sprintf("/vms/%d", id)
	if err := s.client.delete(ctx, endpoint); err != nil {
		if IsNotFoundError(err) {
			return &NotFoundError{Resource: "VM", ID: id}
		}
		return err
	}
	return nil
}

// PowerOn powers on a VM and waits for it to start.
func (s *VMService) PowerOn(ctx context.Context, id int) error {
	// Get current state
	vm, err := s.Get(ctx, id)
	if err != nil {
		return err
	}

	// Already running
	if vm.PowerState {
		return nil
	}

	// Send power on action
	action := vmAction{
		VM:     id,
		Action: vmActionPowerOn,
		Params: vmActionParams{},
	}

	if err := s.client.post(ctx, "/vm_actions", action, nil); err != nil {
		return fmt.Errorf("vergeos: failed to power on VM %d: %w", id, err)
	}

	// Wait for VM to start
	return s.waitForPowerState(ctx, id, true)
}

// PowerOff powers off a VM and waits for it to stop.
func (s *VMService) PowerOff(ctx context.Context, id int) error {
	// Get current state
	vm, err := s.Get(ctx, id)
	if err != nil {
		return err
	}

	// Already stopped
	if !vm.PowerState {
		return nil
	}

	// Send kill action
	action := vmAction{
		VM:     id,
		Action: vmActionKill,
		Params: vmActionParams{},
	}

	if err := s.client.post(ctx, "/vm_actions", action, nil); err != nil {
		return fmt.Errorf("vergeos: failed to power off VM %d: %w", id, err)
	}

	// Wait for VM to stop
	return s.waitForPowerState(ctx, id, false)
}

// waitForPowerState waits for a VM to reach the desired power state.
func (s *VMService) waitForPowerState(ctx context.Context, id int, desiredState bool) error {
	stateStr := "stopped"
	if desiredState {
		stateStr = "running"
	}

	for i := 0; i < powerStateMaxRetries; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(powerStatePollInterval):
		}

		vm, err := s.Get(ctx, id)
		if err != nil {
			return err
		}

		if vm.PowerState == desiredState {
			return nil
		}
	}

	return fmt.Errorf("vergeos: timeout waiting for VM %d to become %s", id, stateStr)
}

// Reset sends a reset signal to a running VM (equivalent to pressing the reset button).
func (s *VMService) Reset(ctx context.Context, id int) error {
	action := vmAction{
		VM:     id,
		Action: vmActionReset,
		Params: vmActionParams{},
	}

	if err := s.client.post(ctx, "/vm_actions", action, nil); err != nil {
		return fmt.Errorf("vergeos: failed to reset VM %d: %w", id, err)
	}
	return nil
}

// GuestReboot sends a reboot request to the guest OS via ACPI.
func (s *VMService) GuestReboot(ctx context.Context, id int) error {
	action := vmAction{
		VM:     id,
		Action: vmActionGuestReboot,
		Params: vmActionParams{},
	}

	if err := s.client.post(ctx, "/vm_actions", action, nil); err != nil {
		return fmt.Errorf("vergeos: failed to guest reboot VM %d: %w", id, err)
	}
	return nil
}

// GuestShutdown sends a graceful shutdown request to the guest OS via ACPI (poweroff action).
func (s *VMService) GuestShutdown(ctx context.Context, id int) error {
	action := vmAction{
		VM:     id,
		Action: vmActionPowerOff,
		Params: vmActionParams{},
	}

	if err := s.client.post(ctx, "/vm_actions", action, nil); err != nil {
		return fmt.Errorf("vergeos: failed to guest shutdown VM %d: %w", id, err)
	}
	return nil
}

// VMCloneOptions contains options for cloning a VM.
type VMCloneOptions struct {
	// Name is the name for the cloned VM. Defaults to "${NAME}_${TIMESTAMP}".
	Name string
	// PreserveMACs indicates whether to preserve MAC addresses from the source VM.
	PreserveMACs bool
}

// Clone creates a copy of a VM.
func (s *VMService) Clone(ctx context.Context, id int, opts *VMCloneOptions) error {
	params := map[string]interface{}{}
	if opts != nil {
		if opts.Name != "" {
			params["name"] = opts.Name
		}
		params["preserve_macs"] = opts.PreserveMACs
	}

	action := struct {
		VM     int                    `json:"vm"`
		Action string                 `json:"action"`
		Params map[string]interface{} `json:"params"`
	}{
		VM:     id,
		Action: vmActionClone,
		Params: params,
	}

	if err := s.client.post(ctx, "/vm_actions", action, nil); err != nil {
		return fmt.Errorf("vergeos: failed to clone VM %d: %w", id, err)
	}
	return nil
}

// VMSnapshotOptions contains options for taking a VM snapshot.
type VMSnapshotOptions struct {
	// Retention is the snapshot retention duration in seconds. Defaults to 86400 (24 hours).
	Retention int
	// Quiesce indicates whether to quiesce the filesystem before snapshot (requires guest agent).
	Quiesce bool
}

// Snapshot takes a snapshot of a VM.
func (s *VMService) Snapshot(ctx context.Context, id int, opts *VMSnapshotOptions) error {
	params := map[string]interface{}{}
	if opts != nil {
		if opts.Retention > 0 {
			params["retention"] = opts.Retention
		}
		params["quiesce"] = opts.Quiesce
	}

	action := struct {
		VM     int                    `json:"vm"`
		Action string                 `json:"action"`
		Params map[string]interface{} `json:"params"`
	}{
		VM:     id,
		Action: vmActionSnapshot,
		Params: params,
	}

	if err := s.client.post(ctx, "/vm_actions", action, nil); err != nil {
		return fmt.Errorf("vergeos: failed to snapshot VM %d: %w", id, err)
	}
	return nil
}

// VMMigrateOptions contains options for migrating a VM to another node.
type VMMigrateOptions struct {
	// TargetNode is the destination node ID (required).
	TargetNode int
	// Live indicates whether to perform a live migration (default: true).
	Live *bool
}

// Migrate migrates a VM to another node.
// Live migration moves the VM without downtime if Live is true (default).
func (s *VMService) Migrate(ctx context.Context, id int, opts *VMMigrateOptions) error {
	if opts == nil {
		return &ValidationError{Message: "migrate options are required"}
	}
	if opts.TargetNode <= 0 {
		return &ValidationError{Field: "target_node", Message: "target_node is required"}
	}

	params := map[string]interface{}{
		"node": opts.TargetNode,
	}

	// Default to live migration
	if opts.Live == nil || *opts.Live {
		params["method"] = "live"
	} else {
		params["method"] = "auto"
	}

	action := struct {
		VM     int                    `json:"vm"`
		Action string                 `json:"action"`
		Params map[string]interface{} `json:"params"`
	}{
		VM:     id,
		Action: "migrate",
		Params: params,
	}

	if err := s.client.post(ctx, "/vm_actions", action, nil); err != nil {
		return fmt.Errorf("vergeos: failed to migrate VM %d: %w", id, err)
	}
	return nil
}

// GetConsoleURL returns the console URL for a VM.
func (s *VMService) GetConsoleURL(ctx context.Context, id int) (string, error) {
	vm, err := s.Get(ctx, id)
	if err != nil {
		return "", err
	}

	if !vm.PowerState {
		return "", fmt.Errorf("vergeos: VM %d is not running", id)
	}

	// Console URL format depends on the console type
	consoleURL := fmt.Sprintf("%s/ui/#/main/vms/%d/console", s.client.baseURL, id)
	return consoleURL, nil
}
