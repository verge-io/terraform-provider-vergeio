package vergeos

import (
	"context"
	"fmt"
	"net/url"
)

// CloudInitService handles cloud-init file operations.
type CloudInitService struct {
	client *Client
}

// List returns all cloud-init files, with optional filtering and pagination.
func (s *CloudInitService) List(ctx context.Context, opts ...ListOption) ([]CloudInitFile, error) {
	options := applyListOptions(opts)

	// Use cloud-init-specific fields if not specified
	if options.Fields == "most" {
		options.Fields = cloudInitListFields
	}

	params := options.toQueryParams()

	var files []CloudInitFile
	if err := s.client.get(ctx, "/cloudinit_files", params, &files); err != nil {
		return nil, err
	}

	return files, nil
}

// Get returns a single cloud-init file by ID.
func (s *CloudInitService) Get(ctx context.Context, id int) (*CloudInitFile, error) {
	params := url.Values{}
	params.Set("fields", cloudInitListFields)

	var file CloudInitFile
	endpoint := fmt.Sprintf("/cloudinit_files/%d", id)
	if err := s.client.get(ctx, endpoint, params, &file); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "CloudInitFile", ID: id}
		}
		return nil, err
	}

	return &file, nil
}

// GetByName returns a cloud-init file by name.
func (s *CloudInitService) GetByName(ctx context.Context, name string) (*CloudInitFile, error) {
	files, err := s.List(ctx, WithFilter(fmt.Sprintf("name eq '%s'", name)))
	if err != nil {
		return nil, err
	}

	if len(files) == 0 {
		return nil, &NotFoundError{Resource: "CloudInitFile", ID: name}
	}

	return &files[0], nil
}

// Create creates a new cloud-init file and returns the created file.
func (s *CloudInitService) Create(ctx context.Context, req *CloudInitFileCreateRequest) (*CloudInitFile, error) {
	if req == nil {
		return nil, &ValidationError{Message: "create request is required"}
	}
	if req.Name == "" {
		return nil, &ValidationError{Field: "name", Message: "name is required"}
	}

	var resp apiResponse
	if err := s.client.post(ctx, "/cloudinit_files", req, &resp); err != nil {
		return nil, err
	}

	// Extract the created file's ID
	id, err := getKey(resp)
	if err != nil {
		return nil, err
	}

	// Read back the created file
	return s.Get(ctx, id)
}

// Update updates a cloud-init file and returns the updated file.
func (s *CloudInitService) Update(ctx context.Context, id int, req *CloudInitFileUpdateRequest) (*CloudInitFile, error) {
	if req == nil {
		return nil, &ValidationError{Message: "update request is required"}
	}

	endpoint := fmt.Sprintf("/cloudinit_files/%d", id)
	if err := s.client.put(ctx, endpoint, req, nil); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "CloudInitFile", ID: id}
		}
		return nil, err
	}

	// Read back the updated file
	return s.Get(ctx, id)
}

// Delete deletes a cloud-init file.
func (s *CloudInitService) Delete(ctx context.Context, id int) error {
	endpoint := fmt.Sprintf("/cloudinit_files/%d", id)
	if err := s.client.delete(ctx, endpoint); err != nil {
		if IsNotFoundError(err) {
			return nil // Already deleted
		}
		return err
	}
	return nil
}
