package vergeos

import (
	"context"
	"fmt"
	"net/url"
)

// TenantSnapshotService handles tenant snapshot operations.
// Tenant snapshots are created automatically by snapshot profiles or manually via tenant actions.
type TenantSnapshotService struct {
	client *Client
}

// List returns all tenant snapshots, with optional filtering and pagination.
func (s *TenantSnapshotService) List(ctx context.Context, opts ...ListOption) ([]TenantSnapshot, error) {
	options := applyListOptions(opts)

	if options.Fields == "most" {
		options.Fields = tenantSnapshotListFields
	}

	params := options.toQueryParams()

	var snapshots []TenantSnapshot
	if err := s.client.get(ctx, "/tenant_snapshots", params, &snapshots); err != nil {
		return nil, err
	}

	return snapshots, nil
}

// ListByTenant returns all snapshots for a specific tenant.
func (s *TenantSnapshotService) ListByTenant(ctx context.Context, tenantID int, opts ...ListOption) ([]TenantSnapshot, error) {
	opts = append(opts, WithFilter(fmt.Sprintf("tenant eq %d", tenantID)))
	return s.List(ctx, opts...)
}

// ListExpiring returns snapshots expiring within the specified number of days.
func (s *TenantSnapshotService) ListExpiring(ctx context.Context, days int, opts ...ListOption) ([]TenantSnapshot, error) {
	opts = append(opts, WithFilter(fmt.Sprintf("expires ne 0 and expires lt {$add({$now},%d)}", days*86400)))
	return s.List(ctx, opts...)
}

// Get returns a single tenant snapshot by ID.
func (s *TenantSnapshotService) Get(ctx context.Context, id int) (*TenantSnapshot, error) {
	params := url.Values{}
	params.Set("fields", tenantSnapshotGetFields)

	var snapshot TenantSnapshot
	endpoint := fmt.Sprintf("/tenant_snapshots/%d", id)
	if err := s.client.get(ctx, endpoint, params, &snapshot); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "TenantSnapshot", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	return &snapshot, nil
}

// GetByName returns a tenant snapshot by name within a specific tenant.
func (s *TenantSnapshotService) GetByName(ctx context.Context, tenantID int, name string) (*TenantSnapshot, error) {
	snapshots, err := s.List(ctx,
		WithFilter(fmt.Sprintf("tenant eq %d and name eq '%s'", tenantID, name)),
	)
	if err != nil {
		return nil, err
	}
	if len(snapshots) == 0 {
		return nil, &NotFoundError{Resource: "TenantSnapshot", ID: name}
	}
	return s.Get(ctx, int(snapshots[0].Key))
}

// Update updates a tenant snapshot and returns the updated snapshot.
func (s *TenantSnapshotService) Update(ctx context.Context, id int, req *TenantSnapshotUpdateRequest) (*TenantSnapshot, error) {
	if req == nil {
		return nil, &ValidationError{Message: "update request is required"}
	}

	endpoint := fmt.Sprintf("/tenant_snapshots/%d", id)
	if err := s.client.put(ctx, endpoint, req, nil); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "TenantSnapshot", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	return s.Get(ctx, id)
}

// Delete deletes a tenant snapshot.
func (s *TenantSnapshotService) Delete(ctx context.Context, id int) error {
	endpoint := fmt.Sprintf("/tenant_snapshots/%d", id)
	if err := s.client.delete(ctx, endpoint); err != nil {
		if IsNotFoundError(err) {
			return &NotFoundError{Resource: "TenantSnapshot", ID: fmt.Sprintf("%d", id)}
		}
		return err
	}
	return nil
}

// Refresh refreshes the tenant snapshots list from the snapshot profile.
// This triggers a refresh of snapshots for the specified tenant.
func (s *TenantSnapshotService) Refresh(ctx context.Context, tenantID int) error {
	action := tenantSnapshotAction{
		Tenant: tenantID,
		Action: "refresh",
	}

	if err := s.client.post(ctx, "/tenant_snapshot_actions", action, nil); err != nil {
		return fmt.Errorf("vergeos: failed to refresh tenant snapshots for tenant %d: %w", tenantID, err)
	}
	return nil
}

// SetNeverExpires sets a tenant snapshot to never expire.
func (s *TenantSnapshotService) SetNeverExpires(ctx context.Context, id int) (*TenantSnapshot, error) {
	expires := int64(0)
	return s.Update(ctx, id, &TenantSnapshotUpdateRequest{Expires: &expires})
}

// SetExpires sets the expiration timestamp for a tenant snapshot.
func (s *TenantSnapshotService) SetExpires(ctx context.Context, id int, expires int64) (*TenantSnapshot, error) {
	return s.Update(ctx, id, &TenantSnapshotUpdateRequest{Expires: &expires})
}

// tenantSnapshotAction represents a tenant snapshot action request.
type tenantSnapshotAction struct {
	Tenant int                    `json:"tenant"`
	Action string                 `json:"action"`
	Params map[string]interface{} `json:"params,omitempty"`
}
