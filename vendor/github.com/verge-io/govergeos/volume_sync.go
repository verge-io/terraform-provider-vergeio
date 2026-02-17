package vergeos

import (
	"context"
	"fmt"
	"net/url"
)

// VolumeSyncService handles volume sync job operations.
// Volume syncs replicate data between volumes using rsync or ysync.
type VolumeSyncService struct {
	client *Client
}

// List returns all volume sync jobs, with optional filtering and pagination.
func (s *VolumeSyncService) List(ctx context.Context, opts ...ListOption) ([]VolumeSync, error) {
	options := applyListOptions(opts)

	if options.Fields == "most" {
		options.Fields = volumeSyncListFields
	}

	params := options.toQueryParams()

	var syncs []VolumeSync
	if err := s.client.get(ctx, "/volume_syncs", params, &syncs); err != nil {
		return nil, err
	}

	return syncs, nil
}

// ListByService returns all volume syncs belonging to a specific NAS service.
func (s *VolumeSyncService) ListByService(ctx context.Context, serviceID int) ([]VolumeSync, error) {
	return s.List(ctx, WithFilter(fmt.Sprintf("service eq %d", serviceID)))
}

// ListEnabled returns all enabled volume syncs.
func (s *VolumeSyncService) ListEnabled(ctx context.Context, opts ...ListOption) ([]VolumeSync, error) {
	opts = append([]ListOption{WithFilter("enabled eq true")}, opts...)
	return s.List(ctx, opts...)
}

// Get returns a single volume sync by its SHA1 ID.
func (s *VolumeSyncService) Get(ctx context.Context, id string) (*VolumeSync, error) {
	params := url.Values{}
	params.Set("fields", volumeSyncGetFields)

	var sync VolumeSync
	endpoint := fmt.Sprintf("/volume_syncs/%s", id)
	if err := s.client.get(ctx, endpoint, params, &sync); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "VolumeSync", ID: id}
		}
		return nil, err
	}

	return &sync, nil
}

// GetByName returns a volume sync by name within a specific service.
func (s *VolumeSyncService) GetByName(ctx context.Context, serviceID int, name string) (*VolumeSync, error) {
	syncs, err := s.List(ctx, WithFilter(fmt.Sprintf("service eq %d and name eq '%s'", serviceID, name)))
	if err != nil {
		return nil, err
	}
	if len(syncs) == 0 {
		return nil, &NotFoundError{Resource: "VolumeSync", ID: name}
	}
	// Get full details
	return s.Get(ctx, syncs[0].ID)
}

// Create creates a new volume sync job and returns the created sync.
func (s *VolumeSyncService) Create(ctx context.Context, req *VolumeSyncCreateRequest) (*VolumeSync, error) {
	if req == nil {
		return nil, &ValidationError{Message: "create request is required"}
	}
	if req.Service <= 0 {
		return nil, &ValidationError{Field: "service", Message: "service is required"}
	}
	if req.Name == "" {
		return nil, &ValidationError{Field: "name", Message: "name is required"}
	}
	if req.SourceVolume <= 0 {
		return nil, &ValidationError{Field: "source_volume", Message: "source_volume is required"}
	}
	if req.DestinationVolume <= 0 {
		return nil, &ValidationError{Field: "destination_volume", Message: "destination_volume is required"}
	}

	// Set default enabled to true
	if req.Enabled == nil {
		enabled := true
		req.Enabled = &enabled
	}

	var resp apiResponse
	if err := s.client.post(ctx, "/volume_syncs", req, &resp); err != nil {
		return nil, err
	}

	// Extract the created sync's ID (SHA1 hash string)
	id, err := getStringKey(resp)
	if err != nil {
		return nil, err
	}

	return s.Get(ctx, id)
}

// Update updates a volume sync job and returns the updated sync.
func (s *VolumeSyncService) Update(ctx context.Context, id string, req *VolumeSyncUpdateRequest) (*VolumeSync, error) {
	if req == nil {
		return nil, &ValidationError{Message: "update request is required"}
	}

	endpoint := fmt.Sprintf("/volume_syncs/%s", id)
	if err := s.client.put(ctx, endpoint, req, nil); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "VolumeSync", ID: id}
		}
		return nil, err
	}

	return s.Get(ctx, id)
}

// Delete deletes a volume sync job.
func (s *VolumeSyncService) Delete(ctx context.Context, id string) error {
	endpoint := fmt.Sprintf("/volume_syncs/%s", id)
	if err := s.client.delete(ctx, endpoint); err != nil {
		if IsNotFoundError(err) {
			return &NotFoundError{Resource: "VolumeSync", ID: id}
		}
		return err
	}
	return nil
}

// Enable enables a volume sync job.
func (s *VolumeSyncService) Enable(ctx context.Context, id string) error {
	enabled := true
	_, err := s.Update(ctx, id, &VolumeSyncUpdateRequest{Enabled: &enabled})
	return err
}

// Disable disables a volume sync job.
func (s *VolumeSyncService) Disable(ctx context.Context, id string) error {
	enabled := false
	_, err := s.Update(ctx, id, &VolumeSyncUpdateRequest{Enabled: &enabled})
	return err
}

// volumeSyncAction represents a volume sync action request.
type volumeSyncAction struct {
	Sync   string                 `json:"sync"`
	Action string                 `json:"action"`
	Params map[string]interface{} `json:"params"`
}

// Start starts a volume sync job immediately.
func (s *VolumeSyncService) Start(ctx context.Context, id string) error {
	action := volumeSyncAction{
		Sync:   id,
		Action: "start",
		Params: map[string]interface{}{},
	}

	if err := s.client.post(ctx, "/volume_sync_actions", action, nil); err != nil {
		return fmt.Errorf("vergeos: failed to start volume sync %s: %w", id, err)
	}
	return nil
}

// Stop stops a running volume sync job.
func (s *VolumeSyncService) Stop(ctx context.Context, id string) error {
	action := volumeSyncAction{
		Sync:   id,
		Action: "stop",
		Params: map[string]interface{}{},
	}

	if err := s.client.post(ctx, "/volume_sync_actions", action, nil); err != nil {
		return fmt.Errorf("vergeos: failed to stop volume sync %s: %w", id, err)
	}
	return nil
}
