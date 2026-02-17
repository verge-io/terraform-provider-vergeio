package vergeos

import (
	"context"
	"fmt"
	"net/url"
)

// CloudSnapshotService handles cloud snapshot (system snapshot) operations.
type CloudSnapshotService struct {
	client *Client
}

// List returns all cloud snapshots, with optional filtering and pagination.
func (s *CloudSnapshotService) List(ctx context.Context, opts ...ListOption) ([]CloudSnapshot, error) {
	options := applyListOptions(opts)

	if options.Fields == "most" {
		options.Fields = cloudSnapshotListFields
	}

	params := options.toQueryParams()

	var snapshots []CloudSnapshot
	if err := s.client.get(ctx, "/cloud_snapshots", params, &snapshots); err != nil {
		return nil, err
	}

	return snapshots, nil
}

// ListExpiring returns cloud snapshots that have an expiration date (non-zero expires).
func (s *CloudSnapshotService) ListExpiring(ctx context.Context, opts ...ListOption) ([]CloudSnapshot, error) {
	opts = append(opts, WithFilter("expires gt 0"))
	return s.List(ctx, opts...)
}

// ListLocal returns cloud snapshots that were created locally (not from provider).
func (s *CloudSnapshotService) ListLocal(ctx context.Context, opts ...ListOption) ([]CloudSnapshot, error) {
	opts = append(opts, WithFilter("provider eq false"))
	return s.List(ctx, opts...)
}

// ListByProfile returns cloud snapshots created by a specific snapshot profile.
func (s *CloudSnapshotService) ListByProfile(ctx context.Context, profileID int, opts ...ListOption) ([]CloudSnapshot, error) {
	opts = append(opts, WithFilter(fmt.Sprintf("snapshot_profile eq %d", profileID)))
	return s.List(ctx, opts...)
}

// Get returns a single cloud snapshot by ID.
func (s *CloudSnapshotService) Get(ctx context.Context, id int) (*CloudSnapshot, error) {
	params := url.Values{}
	params.Set("fields", cloudSnapshotGetFields)

	var snapshot CloudSnapshot
	endpoint := fmt.Sprintf("/cloud_snapshots/%d", id)
	if err := s.client.get(ctx, endpoint, params, &snapshot); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "CloudSnapshot", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	return &snapshot, nil
}

// GetByName returns a cloud snapshot by name.
func (s *CloudSnapshotService) GetByName(ctx context.Context, name string) (*CloudSnapshot, error) {
	snapshots, err := s.List(ctx, WithFilter(fmt.Sprintf("name eq '%s'", name)))
	if err != nil {
		return nil, err
	}
	if len(snapshots) == 0 {
		return nil, &NotFoundError{Resource: "CloudSnapshot", ID: name}
	}
	return s.Get(ctx, int(snapshots[0].Key))
}

// Create creates a new cloud snapshot (system snapshot).
// This triggers the snapshot creation task, which runs asynchronously.
func (s *CloudSnapshotService) Create(ctx context.Context, req *CloudSnapshotCreateRequest) (*CloudSnapshot, error) {
	if req == nil {
		return nil, &ValidationError{Message: "create request is required"}
	}
	if req.Name == "" {
		return nil, &ValidationError{Field: "name", Message: "name is required"}
	}

	// Cloud snapshots use table_actions/create endpoint
	tableAction := map[string]interface{}{
		"name": req.Name,
	}
	if req.Description != "" {
		tableAction["description"] = req.Description
	}
	if req.Retention != nil {
		tableAction["retention"] = *req.Retention
	}
	if req.MinSnapshots != nil {
		tableAction["min_snapshots"] = *req.MinSnapshots
	}
	if req.Immutable != nil {
		tableAction["immutable"] = *req.Immutable
	}
	if req.Private != nil {
		tableAction["private"] = *req.Private
	}

	var resp apiResponse
	if err := s.client.post(ctx, "/cloud_snapshots?action=create", tableAction, &resp); err != nil {
		return nil, err
	}

	id, err := getKey(resp)
	if err != nil {
		return nil, err
	}

	return s.Get(ctx, id)
}

// Update updates a cloud snapshot and returns the updated snapshot.
func (s *CloudSnapshotService) Update(ctx context.Context, id int, req *CloudSnapshotUpdateRequest) (*CloudSnapshot, error) {
	if req == nil {
		return nil, &ValidationError{Message: "update request is required"}
	}

	endpoint := fmt.Sprintf("/cloud_snapshots/%d", id)
	if err := s.client.put(ctx, endpoint, req, nil); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "CloudSnapshot", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	return s.Get(ctx, id)
}

// Delete deletes a cloud snapshot.
func (s *CloudSnapshotService) Delete(ctx context.Context, id int) error {
	endpoint := fmt.Sprintf("/cloud_snapshots/%d", id)
	if err := s.client.delete(ctx, endpoint); err != nil {
		if IsNotFoundError(err) {
			return &NotFoundError{Resource: "CloudSnapshot", ID: fmt.Sprintf("%d", id)}
		}
		return err
	}
	return nil
}

// Refresh refreshes the cloud snapshot metadata.
func (s *CloudSnapshotService) Refresh(ctx context.Context, id int) error {
	action := cloudSnapshotAction{
		CloudSnapshot: id,
		Action:        "refresh",
	}

	if err := s.client.post(ctx, "/cloud_snapshot_actions", action, nil); err != nil {
		return fmt.Errorf("vergeos: failed to refresh cloud snapshot %d: %w", id, err)
	}
	return nil
}

// Clone creates a copy of a cloud snapshot with a new name.
func (s *CloudSnapshotService) Clone(ctx context.Context, id int, opts *CloudSnapshotCloneOptions) error {
	if opts == nil {
		return &ValidationError{Message: "clone options are required"}
	}
	if opts.Name == "" {
		return &ValidationError{Field: "name", Message: "name is required for clone"}
	}

	action := cloudSnapshotAction{
		CloudSnapshot: id,
		Action:        "clone",
		Params: map[string]interface{}{
			"name": opts.Name,
		},
	}

	if err := s.client.post(ctx, "/cloud_snapshot_actions", action, nil); err != nil {
		return fmt.Errorf("vergeos: failed to clone cloud snapshot %d: %w", id, err)
	}
	return nil
}

// RequestFromProvider requests a cloud snapshot from the provider tenant.
func (s *CloudSnapshotService) RequestFromProvider(ctx context.Context, id int) error {
	action := cloudSnapshotAction{
		CloudSnapshot: id,
		Action:        "request",
	}

	if err := s.client.post(ctx, "/cloud_snapshot_actions", action, nil); err != nil {
		return fmt.Errorf("vergeos: failed to request cloud snapshot %d from provider: %w", id, err)
	}
	return nil
}

// FindTenants discovers and lists tenants within a cloud snapshot.
func (s *CloudSnapshotService) FindTenants(ctx context.Context, id int) error {
	action := cloudSnapshotAction{
		CloudSnapshot: id,
		Action:        "find_tenants",
	}

	if err := s.client.post(ctx, "/cloud_snapshot_actions", action, nil); err != nil {
		return fmt.Errorf("vergeos: failed to find tenants in cloud snapshot %d: %w", id, err)
	}
	return nil
}

// FindVMs discovers and lists VMs within a cloud snapshot.
func (s *CloudSnapshotService) FindVMs(ctx context.Context, id int) error {
	action := cloudSnapshotAction{
		CloudSnapshot: id,
		Action:        "find_vms",
	}

	if err := s.client.post(ctx, "/cloud_snapshot_actions", action, nil); err != nil {
		return fmt.Errorf("vergeos: failed to find VMs in cloud snapshot %d: %w", id, err)
	}
	return nil
}

// CloudSnapshotVMService handles VM listings within cloud snapshots.
type CloudSnapshotVMService struct {
	client *Client
}

// List returns all VMs within cloud snapshots, with optional filtering.
func (s *CloudSnapshotVMService) List(ctx context.Context, opts ...ListOption) ([]CloudSnapshotVM, error) {
	options := applyListOptions(opts)

	if options.Fields == "most" {
		options.Fields = cloudSnapshotVMListFields
	}

	params := options.toQueryParams()

	var vms []CloudSnapshotVM
	if err := s.client.get(ctx, "/cloud_snapshot_vms", params, &vms); err != nil {
		return nil, err
	}

	return vms, nil
}

// ListBySnapshot returns all VMs within a specific cloud snapshot.
func (s *CloudSnapshotVMService) ListBySnapshot(ctx context.Context, snapshotID int, opts ...ListOption) ([]CloudSnapshotVM, error) {
	opts = append(opts, WithFilter(fmt.Sprintf("cloud_snapshot eq %d", snapshotID)))
	return s.List(ctx, opts...)
}

// Get returns a single cloud snapshot VM by ID.
func (s *CloudSnapshotVMService) Get(ctx context.Context, id int) (*CloudSnapshotVM, error) {
	params := url.Values{}
	params.Set("fields", cloudSnapshotVMListFields)

	var vm CloudSnapshotVM
	endpoint := fmt.Sprintf("/cloud_snapshot_vms/%d", id)
	if err := s.client.get(ctx, endpoint, params, &vm); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "CloudSnapshotVM", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	return &vm, nil
}

// CloudSnapshotTenantService handles tenant listings within cloud snapshots.
type CloudSnapshotTenantService struct {
	client *Client
}

// List returns all tenants within cloud snapshots, with optional filtering.
func (s *CloudSnapshotTenantService) List(ctx context.Context, opts ...ListOption) ([]CloudSnapshotTenant, error) {
	options := applyListOptions(opts)

	if options.Fields == "most" {
		options.Fields = cloudSnapshotTenantListFields
	}

	params := options.toQueryParams()

	var tenants []CloudSnapshotTenant
	if err := s.client.get(ctx, "/cloud_snapshot_tenants", params, &tenants); err != nil {
		return nil, err
	}

	return tenants, nil
}

// ListBySnapshot returns all tenants within a specific cloud snapshot.
func (s *CloudSnapshotTenantService) ListBySnapshot(ctx context.Context, snapshotID int, opts ...ListOption) ([]CloudSnapshotTenant, error) {
	opts = append(opts, WithFilter(fmt.Sprintf("cloud_snapshot eq %d", snapshotID)))
	return s.List(ctx, opts...)
}

// Get returns a single cloud snapshot tenant by ID.
func (s *CloudSnapshotTenantService) Get(ctx context.Context, id int) (*CloudSnapshotTenant, error) {
	params := url.Values{}
	params.Set("fields", cloudSnapshotTenantListFields)

	var tenant CloudSnapshotTenant
	endpoint := fmt.Sprintf("/cloud_snapshot_tenants/%d", id)
	if err := s.client.get(ctx, endpoint, params, &tenant); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "CloudSnapshotTenant", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	return &tenant, nil
}
