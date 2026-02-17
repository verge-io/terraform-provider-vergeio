package vergeos

import (
	"context"
	"fmt"
	"net/url"
	"time"
)

const (
	// NIC hotplug action
	vmActionHotplugNIC = "hotplugnic"

	// NIC power state polling
	nicUnplugMaxRetries   = 5
	nicUnplugPollInterval = 5 * time.Second
)

// VMNICService handles VM NIC operations.
type VMNICService struct {
	client *Client
}

// List returns all NICs for a VM.
func (s *VMNICService) List(ctx context.Context, vmID int) ([]VMNIC, error) {
	params := url.Values{}
	params.Set("fields", nicListFields)
	params.Set("filter", fmt.Sprintf("machine eq %d", vmID))

	var nics []VMNIC
	if err := s.client.get(ctx, "/machine_nics", params, &nics); err != nil {
		return nil, err
	}

	return nics, nil
}

// Get returns a single NIC by ID.
func (s *VMNICService) Get(ctx context.Context, nicID int) (*VMNIC, error) {
	params := url.Values{}
	params.Set("fields", nicGetFields)

	var nic VMNIC
	endpoint := fmt.Sprintf("/machine_nics/%d", nicID)
	if err := s.client.get(ctx, endpoint, params, &nic); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "VMNIC", ID: nicID}
		}
		return nil, err
	}

	return &nic, nil
}

// Create creates a new NIC and returns the created NIC.
func (s *VMNICService) Create(ctx context.Context, vmID int, req *VMNICCreateRequest) (*VMNIC, error) {
	if req == nil {
		return nil, &ValidationError{Message: "create request is required"}
	}
	if req.Name == "" {
		return nil, &ValidationError{Field: "name", Message: "name is required"}
	}

	// Set the machine ID
	req.Machine = vmID

	// Set defaults
	if req.Enabled == nil {
		enabled := true
		req.Enabled = &enabled
	}

	var resp apiResponse
	if err := s.client.post(ctx, "/machine_nics", req, &resp); err != nil {
		return nil, err
	}

	// Extract the created NIC's ID
	id, err := getKey(resp)
	if err != nil {
		return nil, err
	}

	// Read back the created NIC
	nic, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	// Assign IP address if requested
	if req.AssignIPAddress && req.VNET > 0 && nic.MAC != "" {
		addrReq := vnetAddressRequest{
			VNET: req.VNET,
			MAC:  nic.MAC,
			Type: "static",
		}
		if err := s.client.post(ctx, "/vnet_addresses", addrReq, nil); err != nil {
			// Non-fatal: NIC was created but IP assignment failed
			// The caller can retry IP assignment
		}
	}

	return nic, nil
}

// Update updates a NIC and returns the updated NIC.
func (s *VMNICService) Update(ctx context.Context, nicID int, req *VMNICUpdateRequest) (*VMNIC, error) {
	if req == nil {
		return nil, &ValidationError{Message: "update request is required"}
	}

	endpoint := fmt.Sprintf("/machine_nics/%d", nicID)
	if err := s.client.put(ctx, endpoint, req, nil); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "VMNIC", ID: nicID}
		}
		return nil, err
	}

	// Read back the updated NIC
	return s.Get(ctx, nicID)
}

// Delete deletes a NIC.
// If the NIC is currently plugged in (VM running), it will be hot-unplugged first.
func (s *VMNICService) Delete(ctx context.Context, nicID int) error {
	// Get NIC to check power state and get VM ID
	nic, err := s.Get(ctx, nicID)
	if err != nil {
		if IsNotFoundError(err) {
			return nil // Already deleted
		}
		return err
	}

	// If NIC is up, hot-unplug it first
	if nic.PowerState != "" && nic.PowerState != "down" {
		if err := s.hotUnplug(ctx, nic.Machine, nicID); err != nil {
			return fmt.Errorf("vergeos: failed to unplug NIC before deletion: %w", err)
		}
	}

	endpoint := fmt.Sprintf("/machine_nics/%d", nicID)
	if err := s.client.delete(ctx, endpoint); err != nil {
		if IsNotFoundError(err) {
			return nil // Already deleted
		}
		return err
	}

	return nil
}

// hotUnplug hot-unplugs a NIC from a running VM.
func (s *VMNICService) hotUnplug(ctx context.Context, vmID, nicID int) error {
	action := vmAction{
		VM:     vmID,
		Action: vmActionHotplugNIC,
		Params: vmActionParams{
			Device: fmt.Sprintf("%d", nicID),
			Unplug: true,
		},
	}

	if err := s.client.post(ctx, "/vm_actions", action, nil); err != nil {
		return err
	}

	// Wait for NIC to be unplugged
	for i := 0; i < nicUnplugMaxRetries; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(nicUnplugPollInterval):
		}

		nic, err := s.Get(ctx, nicID)
		if err != nil {
			if IsNotFoundError(err) {
				return nil // NIC was removed
			}
			return err
		}

		if nic.PowerState == "down" || nic.PowerState == "" {
			return nil
		}
	}

	return fmt.Errorf("vergeos: timeout waiting for NIC %d to unplug", nicID)
}
