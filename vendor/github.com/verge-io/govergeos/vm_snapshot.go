package vergeos

import (
	"context"
	"fmt"
	"net/url"
)

// VMSnapshotService handles VM snapshot operations.
// VM Snapshots are point-in-time copies of a VM's state.
type VMSnapshotService struct {
	client *Client
}

// List returns all VM snapshots, with optional filtering and pagination.
func (s *VMSnapshotService) List(ctx context.Context, opts ...ListOption) ([]VMSnapshot, error) {
	options := applyListOptions(opts)

	if options.Fields == "most" {
		options.Fields = vmSnapshotListFields
	}

	params := options.toQueryParams()

	var snapshots []VMSnapshot
	if err := s.client.get(ctx, "/machine_snapshots", params, &snapshots); err != nil {
		return nil, err
	}

	return snapshots, nil
}

// ListByVM returns all snapshots for a specific VM.
func (s *VMSnapshotService) ListByVM(ctx context.Context, vmID int, opts ...ListOption) ([]VMSnapshot, error) {
	options := applyListOptions(opts)

	if options.Fields == "most" {
		options.Fields = vmSnapshotListFields
	}

	// Add machine filter
	if options.Filter != "" {
		options.Filter = fmt.Sprintf("(%s) and machine eq %d", options.Filter, vmID)
	} else {
		options.Filter = fmt.Sprintf("machine eq %d", vmID)
	}

	params := options.toQueryParams()

	var snapshots []VMSnapshot
	if err := s.client.get(ctx, "/machine_snapshots", params, &snapshots); err != nil {
		return nil, err
	}

	return snapshots, nil
}

// ListExpiring returns snapshots expiring within the specified number of days.
func (s *VMSnapshotService) ListExpiring(ctx context.Context, days int, opts ...ListOption) ([]VMSnapshot, error) {
	options := applyListOptions(opts)

	if options.Fields == "most" {
		options.Fields = vmSnapshotListFields
	}

	// Add expiring filter using API timestamp math
	if options.Filter != "" {
		options.Filter = fmt.Sprintf("(%s) and expires lt {$add({$now},%d)} and expires gt 0", options.Filter, days*86400)
	} else {
		options.Filter = fmt.Sprintf("expires lt {$add({$now},%d)} and expires gt 0", days*86400)
	}

	params := options.toQueryParams()

	var snapshots []VMSnapshot
	if err := s.client.get(ctx, "/machine_snapshots", params, &snapshots); err != nil {
		return nil, err
	}

	return snapshots, nil
}

// Get returns a single VM snapshot by ID.
func (s *VMSnapshotService) Get(ctx context.Context, id int) (*VMSnapshot, error) {
	params := url.Values{}
	params.Set("fields", vmSnapshotGetFields)

	var snapshot VMSnapshot
	endpoint := fmt.Sprintf("/machine_snapshots/%d", id)
	if err := s.client.get(ctx, endpoint, params, &snapshot); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "VMSnapshot", ID: id}
		}
		return nil, err
	}

	return &snapshot, nil
}

// GetByName returns a VM snapshot by name within a specific VM.
func (s *VMSnapshotService) GetByName(ctx context.Context, vmID int, name string) (*VMSnapshot, error) {
	snapshots, err := s.ListByVM(ctx, vmID, WithFilter(fmt.Sprintf("name eq '%s'", name)))
	if err != nil {
		return nil, err
	}

	if len(snapshots) == 0 {
		return nil, &NotFoundError{Resource: "VMSnapshot", ID: name}
	}

	return &snapshots[0], nil
}

// Create creates a new VM snapshot and returns the created snapshot.
func (s *VMSnapshotService) Create(ctx context.Context, req *VMSnapshotCreateRequest) (*VMSnapshot, error) {
	if req == nil {
		return nil, &ValidationError{Message: "create request is required"}
	}
	if req.Machine <= 0 {
		return nil, &ValidationError{Field: "machine", Message: "machine (VM ID) is required"}
	}
	if req.Name == "" {
		return nil, &ValidationError{Field: "name", Message: "name is required"}
	}

	// Set defaults
	if req.ExpiresType == "" {
		req.ExpiresType = "date"
	}

	var resp apiResponse
	if err := s.client.post(ctx, "/machine_snapshots", req, &resp); err != nil {
		return nil, err
	}

	// Extract the created snapshot's ID
	id, err := getKey(resp)
	if err != nil {
		return nil, err
	}

	// Read back the created snapshot
	return s.Get(ctx, id)
}

// Update updates a VM snapshot and returns the updated snapshot.
func (s *VMSnapshotService) Update(ctx context.Context, id int, req *VMSnapshotUpdateRequest) (*VMSnapshot, error) {
	if req == nil {
		return nil, &ValidationError{Message: "update request is required"}
	}

	endpoint := fmt.Sprintf("/machine_snapshots/%d", id)
	if err := s.client.put(ctx, endpoint, req, nil); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "VMSnapshot", ID: id}
		}
		return nil, err
	}

	// Read back the updated snapshot
	return s.Get(ctx, id)
}

// Delete deletes a VM snapshot.
func (s *VMSnapshotService) Delete(ctx context.Context, id int) error {
	endpoint := fmt.Sprintf("/machine_snapshots/%d", id)
	if err := s.client.delete(ctx, endpoint); err != nil {
		if IsNotFoundError(err) {
			return &NotFoundError{Resource: "VMSnapshot", ID: id}
		}
		return err
	}
	return nil
}

// Restore restores a VM from a snapshot.
// The VM will be reverted to the state at the time of the snapshot.
func (s *VMSnapshotService) Restore(ctx context.Context, id int, opts *VMSnapshotRestoreOptions) error {
	// Get the snapshot to find the parent machine
	snapshot, err := s.Get(ctx, id)
	if err != nil {
		return err
	}

	params := map[string]interface{}{}
	if opts != nil {
		params["poweron"] = opts.PowerOn
	}

	action := struct {
		VM     int                    `json:"vm"`
		Action string                 `json:"action"`
		Params map[string]interface{} `json:"params"`
	}{
		VM:     int(snapshot.Machine),
		Action: "restore",
		Params: params,
	}
	action.Params["snapshot"] = id

	if err := s.client.post(ctx, "/vm_actions", action, nil); err != nil {
		return fmt.Errorf("vergeos: failed to restore VM snapshot %d: %w", id, err)
	}
	return nil
}

// SetNeverExpires sets a snapshot to never expire.
func (s *VMSnapshotService) SetNeverExpires(ctx context.Context, id int) (*VMSnapshot, error) {
	expiresType := "never"
	return s.Update(ctx, id, &VMSnapshotUpdateRequest{
		ExpiresType: &expiresType,
	})
}

// SetExpires sets the expiration timestamp for a snapshot.
func (s *VMSnapshotService) SetExpires(ctx context.Context, id int, expires int64) (*VMSnapshot, error) {
	expiresType := "date"
	return s.Update(ctx, id, &VMSnapshotUpdateRequest{
		ExpiresType: &expiresType,
		Expires:     &expires,
	})
}
