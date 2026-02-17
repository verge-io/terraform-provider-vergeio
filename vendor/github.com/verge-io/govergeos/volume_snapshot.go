package vergeos

import (
	"context"
	"fmt"
	"net/url"
	"time"
)

// VolumeSnapshotService handles NAS volume snapshot operations.
type VolumeSnapshotService struct {
	client *Client
}

// List returns all volume snapshots, with optional filtering and pagination.
func (s *VolumeSnapshotService) List(ctx context.Context, opts ...ListOption) ([]VolumeSnapshot, error) {
	options := applyListOptions(opts)

	if options.Fields == "most" {
		options.Fields = volumeSnapshotListFields
	}

	params := options.toQueryParams()

	var snapshots []VolumeSnapshot
	if err := s.client.get(ctx, "/volume_snapshots", params, &snapshots); err != nil {
		return nil, err
	}

	return snapshots, nil
}

// ListByVolume returns all snapshots for a specific volume.
func (s *VolumeSnapshotService) ListByVolume(ctx context.Context, volumeID int, opts ...ListOption) ([]VolumeSnapshot, error) {
	opts = append([]ListOption{WithFilter(fmt.Sprintf("volume eq %d", volumeID))}, opts...)
	return s.List(ctx, opts...)
}

// ListExpiring returns snapshots that will expire within the specified number of days.
func (s *VolumeSnapshotService) ListExpiring(ctx context.Context, days int, opts ...ListOption) ([]VolumeSnapshot, error) {
	expiresBy := time.Now().AddDate(0, 0, days).Unix()
	opts = append([]ListOption{
		WithFilter(fmt.Sprintf("expires_type eq 'date' and expires le %d", expiresBy)),
	}, opts...)
	return s.List(ctx, opts...)
}

// ListManual returns snapshots that were created manually (not scheduled).
func (s *VolumeSnapshotService) ListManual(ctx context.Context, opts ...ListOption) ([]VolumeSnapshot, error) {
	opts = append([]ListOption{WithFilter("created_manually eq true")}, opts...)
	return s.List(ctx, opts...)
}

// Get returns a single volume snapshot by ID.
func (s *VolumeSnapshotService) Get(ctx context.Context, id int) (*VolumeSnapshot, error) {
	params := url.Values{}
	params.Set("fields", volumeSnapshotGetFields)

	var snapshot VolumeSnapshot
	endpoint := fmt.Sprintf("/volume_snapshots/%d", id)
	if err := s.client.get(ctx, endpoint, params, &snapshot); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "VolumeSnapshot", ID: id}
		}
		return nil, err
	}

	return &snapshot, nil
}

// GetByName returns a volume snapshot by name within a specific volume.
func (s *VolumeSnapshotService) GetByName(ctx context.Context, volumeID int, name string) (*VolumeSnapshot, error) {
	snapshots, err := s.List(ctx, WithFilter(fmt.Sprintf("volume eq %d and name eq '%s'", volumeID, name)))
	if err != nil {
		return nil, err
	}
	if len(snapshots) == 0 {
		return nil, &NotFoundError{Resource: "VolumeSnapshot", ID: name}
	}
	// Get full details
	return s.Get(ctx, int(snapshots[0].Key))
}

// Create creates a new volume snapshot and returns the created snapshot.
func (s *VolumeSnapshotService) Create(ctx context.Context, req *VolumeSnapshotCreateRequest) (*VolumeSnapshot, error) {
	if req == nil {
		return nil, &ValidationError{Message: "create request is required"}
	}
	if req.Volume <= 0 {
		return nil, &ValidationError{Field: "volume", Message: "volume is required"}
	}
	if req.Name == "" {
		return nil, &ValidationError{Field: "name", Message: "name is required"}
	}

	var resp apiResponse
	if err := s.client.post(ctx, "/volume_snapshots", req, &resp); err != nil {
		return nil, err
	}

	id, err := getKey(resp)
	if err != nil {
		return nil, err
	}

	return s.Get(ctx, id)
}

// Update updates a volume snapshot and returns the updated snapshot.
func (s *VolumeSnapshotService) Update(ctx context.Context, id int, req *VolumeSnapshotUpdateRequest) (*VolumeSnapshot, error) {
	if req == nil {
		return nil, &ValidationError{Message: "update request is required"}
	}

	endpoint := fmt.Sprintf("/volume_snapshots/%d", id)
	if err := s.client.put(ctx, endpoint, req, nil); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "VolumeSnapshot", ID: id}
		}
		return nil, err
	}

	return s.Get(ctx, id)
}

// Delete deletes a volume snapshot.
func (s *VolumeSnapshotService) Delete(ctx context.Context, id int) error {
	endpoint := fmt.Sprintf("/volume_snapshots/%d", id)
	if err := s.client.delete(ctx, endpoint); err != nil {
		if IsNotFoundError(err) {
			return &NotFoundError{Resource: "VolumeSnapshot", ID: id}
		}
		return err
	}
	return nil
}

// Enable enables a volume snapshot.
func (s *VolumeSnapshotService) Enable(ctx context.Context, id int) error {
	enabled := true
	_, err := s.Update(ctx, id, &VolumeSnapshotUpdateRequest{Enabled: &enabled})
	return err
}

// Disable disables a volume snapshot.
func (s *VolumeSnapshotService) Disable(ctx context.Context, id int) error {
	enabled := false
	_, err := s.Update(ctx, id, &VolumeSnapshotUpdateRequest{Enabled: &enabled})
	return err
}

// SetNeverExpires sets the snapshot to never expire.
func (s *VolumeSnapshotService) SetNeverExpires(ctx context.Context, id int) (*VolumeSnapshot, error) {
	expiresType := "never"
	return s.Update(ctx, id, &VolumeSnapshotUpdateRequest{ExpiresType: &expiresType})
}

// SetExpires sets the snapshot expiration time.
func (s *VolumeSnapshotService) SetExpires(ctx context.Context, id int, expires int64) (*VolumeSnapshot, error) {
	expiresType := "date"
	return s.Update(ctx, id, &VolumeSnapshotUpdateRequest{
		ExpiresType: &expiresType,
		Expires:     &expires,
	})
}
