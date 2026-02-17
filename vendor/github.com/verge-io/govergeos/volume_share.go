package vergeos

import (
	"context"
	"fmt"
	"net/url"
)

// VolumeCIFSShareService handles CIFS (SMB) share operations on NAS volumes.
// Note: Like volumes, CIFS shares use SHA1 hash strings as keys instead of integers.
type VolumeCIFSShareService struct {
	client *Client
}

// List returns all CIFS shares, with optional filtering and pagination.
func (s *VolumeCIFSShareService) List(ctx context.Context, opts ...ListOption) ([]VolumeCIFSShare, error) {
	options := applyListOptions(opts)

	if options.Fields == "most" {
		options.Fields = cifsShareListFields
	}

	params := options.toQueryParams()

	var shares []VolumeCIFSShare
	if err := s.client.get(ctx, "/volume_cifs_shares", params, &shares); err != nil {
		return nil, err
	}

	return shares, nil
}

// ListByVolume returns all CIFS shares belonging to a specific volume.
func (s *VolumeCIFSShareService) ListByVolume(ctx context.Context, volumeID string, opts ...ListOption) ([]VolumeCIFSShare, error) {
	return s.List(ctx, append(opts, WithFilter(fmt.Sprintf("volume eq '%s'", volumeID)))...)
}

// Get returns a single CIFS share by its SHA1 ID.
func (s *VolumeCIFSShareService) Get(ctx context.Context, id string) (*VolumeCIFSShare, error) {
	params := url.Values{}
	params.Set("fields", cifsShareGetFields)

	var share VolumeCIFSShare
	endpoint := fmt.Sprintf("/volume_cifs_shares/%s", id)
	if err := s.client.get(ctx, endpoint, params, &share); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "VolumeCIFSShare", ID: id}
		}
		return nil, err
	}

	return &share, nil
}

// GetByName returns a single CIFS share by name within a volume.
func (s *VolumeCIFSShareService) GetByName(ctx context.Context, volumeID, name string) (*VolumeCIFSShare, error) {
	shares, err := s.List(ctx, WithFilter(fmt.Sprintf("volume eq '%s' and name eq '%s'", volumeID, name)))
	if err != nil {
		return nil, err
	}
	if len(shares) == 0 {
		return nil, &NotFoundError{Resource: "VolumeCIFSShare", ID: name}
	}
	// Get full details
	return s.Get(ctx, shares[0].ID)
}

// Create creates a new CIFS share and returns the created share.
func (s *VolumeCIFSShareService) Create(ctx context.Context, req *VolumeCIFSShareCreateRequest) (*VolumeCIFSShare, error) {
	if req == nil {
		return nil, &ValidationError{Message: "create request is required"}
	}
	if req.Name == "" {
		return nil, &ValidationError{Field: "name", Message: "name is required"}
	}
	if req.Volume == "" {
		return nil, &ValidationError{Field: "volume", Message: "volume is required"}
	}

	// Set defaults
	if req.Enabled == nil {
		enabled := true
		req.Enabled = &enabled
	}

	var resp apiResponse
	if err := s.client.post(ctx, "/volume_cifs_shares", req, &resp); err != nil {
		return nil, err
	}

	// Extract the created share's ID (SHA1 hash string)
	id, err := getStringKey(resp)
	if err != nil {
		return nil, err
	}

	// Read back the created share
	return s.Get(ctx, id)
}

// Update updates a CIFS share and returns the updated share.
func (s *VolumeCIFSShareService) Update(ctx context.Context, id string, req *VolumeCIFSShareUpdateRequest) (*VolumeCIFSShare, error) {
	if req == nil {
		return nil, &ValidationError{Message: "update request is required"}
	}

	endpoint := fmt.Sprintf("/volume_cifs_shares/%s", id)
	if err := s.client.put(ctx, endpoint, req, nil); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "VolumeCIFSShare", ID: id}
		}
		return nil, err
	}

	// Read back the updated share
	return s.Get(ctx, id)
}

// Delete deletes a CIFS share.
func (s *VolumeCIFSShareService) Delete(ctx context.Context, id string) error {
	endpoint := fmt.Sprintf("/volume_cifs_shares/%s", id)
	if err := s.client.delete(ctx, endpoint); err != nil {
		if IsNotFoundError(err) {
			return &NotFoundError{Resource: "VolumeCIFSShare", ID: id}
		}
		return err
	}
	return nil
}

// VolumeNFSShareService handles NFS share operations on NAS volumes.
// Note: Like volumes, NFS shares use SHA1 hash strings as keys instead of integers.
type VolumeNFSShareService struct {
	client *Client
}

// List returns all NFS shares, with optional filtering and pagination.
func (s *VolumeNFSShareService) List(ctx context.Context, opts ...ListOption) ([]VolumeNFSShare, error) {
	options := applyListOptions(opts)

	if options.Fields == "most" {
		options.Fields = nfsShareListFields
	}

	params := options.toQueryParams()

	var shares []VolumeNFSShare
	if err := s.client.get(ctx, "/volume_nfs_shares", params, &shares); err != nil {
		return nil, err
	}

	return shares, nil
}

// ListByVolume returns all NFS shares belonging to a specific volume.
func (s *VolumeNFSShareService) ListByVolume(ctx context.Context, volumeID string, opts ...ListOption) ([]VolumeNFSShare, error) {
	return s.List(ctx, append(opts, WithFilter(fmt.Sprintf("volume eq '%s'", volumeID)))...)
}

// Get returns a single NFS share by its SHA1 ID.
func (s *VolumeNFSShareService) Get(ctx context.Context, id string) (*VolumeNFSShare, error) {
	params := url.Values{}
	params.Set("fields", nfsShareGetFields)

	var share VolumeNFSShare
	endpoint := fmt.Sprintf("/volume_nfs_shares/%s", id)
	if err := s.client.get(ctx, endpoint, params, &share); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "VolumeNFSShare", ID: id}
		}
		return nil, err
	}

	return &share, nil
}

// GetByName returns a single NFS share by name within a volume.
func (s *VolumeNFSShareService) GetByName(ctx context.Context, volumeID, name string) (*VolumeNFSShare, error) {
	shares, err := s.List(ctx, WithFilter(fmt.Sprintf("volume eq '%s' and name eq '%s'", volumeID, name)))
	if err != nil {
		return nil, err
	}
	if len(shares) == 0 {
		return nil, &NotFoundError{Resource: "VolumeNFSShare", ID: name}
	}
	// Get full details
	return s.Get(ctx, shares[0].ID)
}

// Create creates a new NFS share and returns the created share.
func (s *VolumeNFSShareService) Create(ctx context.Context, req *VolumeNFSShareCreateRequest) (*VolumeNFSShare, error) {
	if req == nil {
		return nil, &ValidationError{Message: "create request is required"}
	}
	if req.Name == "" {
		return nil, &ValidationError{Field: "name", Message: "name is required"}
	}
	if req.Volume == "" {
		return nil, &ValidationError{Field: "volume", Message: "volume is required"}
	}

	// Set defaults
	if req.Enabled == nil {
		enabled := true
		req.Enabled = &enabled
	}

	var resp apiResponse
	if err := s.client.post(ctx, "/volume_nfs_shares", req, &resp); err != nil {
		return nil, err
	}

	// Extract the created share's ID (SHA1 hash string)
	id, err := getStringKey(resp)
	if err != nil {
		return nil, err
	}

	// Read back the created share
	return s.Get(ctx, id)
}

// Update updates an NFS share and returns the updated share.
func (s *VolumeNFSShareService) Update(ctx context.Context, id string, req *VolumeNFSShareUpdateRequest) (*VolumeNFSShare, error) {
	if req == nil {
		return nil, &ValidationError{Message: "update request is required"}
	}

	endpoint := fmt.Sprintf("/volume_nfs_shares/%s", id)
	if err := s.client.put(ctx, endpoint, req, nil); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "VolumeNFSShare", ID: id}
		}
		return nil, err
	}

	// Read back the updated share
	return s.Get(ctx, id)
}

// Delete deletes an NFS share.
func (s *VolumeNFSShareService) Delete(ctx context.Context, id string) error {
	endpoint := fmt.Sprintf("/volume_nfs_shares/%s", id)
	if err := s.client.delete(ctx, endpoint); err != nil {
		if IsNotFoundError(err) {
			return &NotFoundError{Resource: "VolumeNFSShare", ID: id}
		}
		return err
	}
	return nil
}
