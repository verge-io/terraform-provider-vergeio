package vergeos

import (
	"context"
	"fmt"
	"net/url"
)

// VolumeService handles NAS volume operations.
// Note: Unlike other resources, volumes use SHA1 hash strings as keys instead of integers.
type VolumeService struct {
	client *Client
}

// List returns all volumes, with optional filtering and pagination.
func (s *VolumeService) List(ctx context.Context, opts ...ListOption) ([]Volume, error) {
	options := applyListOptions(opts)

	// Use volume-specific fields if not specified
	if options.Fields == "most" {
		options.Fields = volumeListFields
	}

	params := options.toQueryParams()

	var volumes []Volume
	if err := s.client.get(ctx, "/volumes", params, &volumes); err != nil {
		return nil, err
	}

	return volumes, nil
}

// ListByService returns all volumes belonging to a specific NAS service.
func (s *VolumeService) ListByService(ctx context.Context, serviceID int) ([]Volume, error) {
	return s.List(ctx, WithFilter(fmt.Sprintf("service eq %d", serviceID)))
}

// Get returns a single volume by its SHA1 ID.
func (s *VolumeService) Get(ctx context.Context, id string) (*Volume, error) {
	params := url.Values{}
	params.Set("fields", volumeGetFields)

	var volume Volume
	endpoint := fmt.Sprintf("/volumes/%s", id)
	if err := s.client.get(ctx, endpoint, params, &volume); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "Volume", ID: id}
		}
		return nil, err
	}

	return &volume, nil
}

// GetByName returns a single volume by name within a service.
func (s *VolumeService) GetByName(ctx context.Context, serviceID int, name string) (*Volume, error) {
	volumes, err := s.List(ctx, WithFilter(fmt.Sprintf("service eq %d and name eq '%s'", serviceID, name)))
	if err != nil {
		return nil, err
	}
	if len(volumes) == 0 {
		return nil, &NotFoundError{Resource: "Volume", ID: name}
	}
	// Get full details
	return s.Get(ctx, volumes[0].ID)
}

// Create creates a new volume and returns the created volume.
func (s *VolumeService) Create(ctx context.Context, req *VolumeCreateRequest) (*Volume, error) {
	if req == nil {
		return nil, &ValidationError{Message: "create request is required"}
	}
	if req.Name == "" {
		return nil, &ValidationError{Field: "name", Message: "name is required"}
	}
	if req.Service <= 0 {
		return nil, &ValidationError{Field: "service", Message: "service is required"}
	}

	// Set defaults
	if req.Enabled == nil {
		enabled := true
		req.Enabled = &enabled
	}

	var resp apiResponse
	if err := s.client.post(ctx, "/volumes", req, &resp); err != nil {
		return nil, err
	}

	// Extract the created volume's ID (SHA1 hash string)
	id, err := getStringKey(resp)
	if err != nil {
		return nil, err
	}

	// Read back the created volume
	return s.Get(ctx, id)
}

// Update updates a volume and returns the updated volume.
func (s *VolumeService) Update(ctx context.Context, id string, req *VolumeUpdateRequest) (*Volume, error) {
	if req == nil {
		return nil, &ValidationError{Message: "update request is required"}
	}

	endpoint := fmt.Sprintf("/volumes/%s", id)
	if err := s.client.put(ctx, endpoint, req, nil); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "Volume", ID: id}
		}
		return nil, err
	}

	// Read back the updated volume
	return s.Get(ctx, id)
}

// Delete deletes a volume.
func (s *VolumeService) Delete(ctx context.Context, id string) error {
	endpoint := fmt.Sprintf("/volumes/%s", id)
	if err := s.client.delete(ctx, endpoint); err != nil {
		if IsNotFoundError(err) {
			return &NotFoundError{Resource: "Volume", ID: id}
		}
		return err
	}
	return nil
}

// Enable enables a volume.
func (s *VolumeService) Enable(ctx context.Context, id string) error {
	action := volumeAction{
		Volume: id,
		Action: "enable",
		Params: map[string]interface{}{},
	}

	if err := s.client.post(ctx, "/volume_actions", action, nil); err != nil {
		return fmt.Errorf("vergeos: failed to enable volume %s: %w", id, err)
	}
	return nil
}

// Disable disables a volume.
func (s *VolumeService) Disable(ctx context.Context, id string) error {
	action := volumeAction{
		Volume: id,
		Action: "disable",
		Params: map[string]interface{}{},
	}

	if err := s.client.post(ctx, "/volume_actions", action, nil); err != nil {
		return fmt.Errorf("vergeos: failed to disable volume %s: %w", id, err)
	}
	return nil
}

// Reset resets a volume (remounts it).
func (s *VolumeService) Reset(ctx context.Context, id string) error {
	action := volumeAction{
		Volume: id,
		Action: "reset",
		Params: map[string]interface{}{},
	}

	if err := s.client.post(ctx, "/volume_actions", action, nil); err != nil {
		return fmt.Errorf("vergeos: failed to reset volume %s: %w", id, err)
	}
	return nil
}

// volumeAction represents a volume action request.
type volumeAction struct {
	Volume string                 `json:"volume"`
	Action string                 `json:"action"`
	Params map[string]interface{} `json:"params"`
}

// getStringKey extracts a string key from an API response.
func getStringKey(resp apiResponse) (string, error) {
	if resp.Key == nil {
		return "", fmt.Errorf("vergeos: response missing $key field")
	}

	switch v := resp.Key.(type) {
	case string:
		return v, nil
	default:
		return "", fmt.Errorf("vergeos: expected string $key, got %T", v)
	}
}
