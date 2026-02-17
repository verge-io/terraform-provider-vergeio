package vergeos

import (
	"context"
	"fmt"
	"net/url"
)

// NASServiceService handles NAS service operations.
// A NAS service is a specialized VM that provides NAS functionality including
// volumes, CIFS/NFS shares, and sync operations.
type NASServiceService struct {
	client *Client
}

// List returns all NAS services, with optional filtering and pagination.
func (s *NASServiceService) List(ctx context.Context, opts ...ListOption) ([]NASService, error) {
	options := applyListOptions(opts)

	if options.Fields == "most" {
		options.Fields = nasServiceListFields
	}

	params := options.toQueryParams()

	var services []NASService
	if err := s.client.get(ctx, "/vm_services", params, &services); err != nil {
		return nil, err
	}

	return services, nil
}

// Get returns a single NAS service by ID.
func (s *NASServiceService) Get(ctx context.Context, id int) (*NASService, error) {
	params := url.Values{}
	params.Set("fields", nasServiceGetFields)

	var service NASService
	endpoint := fmt.Sprintf("/vm_services/%d", id)
	if err := s.client.get(ctx, endpoint, params, &service); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "NASService", ID: id}
		}
		return nil, err
	}

	return &service, nil
}

// GetByVM returns the NAS service for a specific VM.
func (s *NASServiceService) GetByVM(ctx context.Context, vmID int) (*NASService, error) {
	services, err := s.List(ctx, WithFilter(fmt.Sprintf("vm eq %d", vmID)))
	if err != nil {
		return nil, err
	}
	if len(services) == 0 {
		return nil, &NotFoundError{Resource: "NASService", ID: vmID}
	}
	// Get full details
	return s.Get(ctx, int(services[0].Key))
}

// GetByName returns a NAS service by name.
func (s *NASServiceService) GetByName(ctx context.Context, name string) (*NASService, error) {
	services, err := s.List(ctx, WithFilter(fmt.Sprintf("name eq '%s'", name)))
	if err != nil {
		return nil, err
	}
	if len(services) == 0 {
		return nil, &NotFoundError{Resource: "NASService", ID: name}
	}
	// Get full details
	return s.Get(ctx, int(services[0].Key))
}

// Create creates a new NAS service and returns the created service.
func (s *NASServiceService) Create(ctx context.Context, req *NASServiceCreateRequest) (*NASService, error) {
	if req == nil {
		return nil, &ValidationError{Message: "create request is required"}
	}
	if req.VM <= 0 {
		return nil, &ValidationError{Field: "vm", Message: "vm is required"}
	}

	var resp apiResponse
	if err := s.client.post(ctx, "/vm_services", req, &resp); err != nil {
		return nil, err
	}

	id, err := getKey(resp)
	if err != nil {
		return nil, err
	}

	return s.Get(ctx, id)
}

// Update updates a NAS service and returns the updated service.
func (s *NASServiceService) Update(ctx context.Context, id int, req *NASServiceUpdateRequest) (*NASService, error) {
	if req == nil {
		return nil, &ValidationError{Message: "update request is required"}
	}

	endpoint := fmt.Sprintf("/vm_services/%d", id)
	if err := s.client.put(ctx, endpoint, req, nil); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "NASService", ID: id}
		}
		return nil, err
	}

	return s.Get(ctx, id)
}

// Delete deletes a NAS service.
func (s *NASServiceService) Delete(ctx context.Context, id int) error {
	endpoint := fmt.Sprintf("/vm_services/%d", id)
	if err := s.client.delete(ctx, endpoint); err != nil {
		if IsNotFoundError(err) {
			return &NotFoundError{Resource: "NASService", ID: id}
		}
		return err
	}
	return nil
}

// NASServiceUserService handles NAS service user operations.
// NAS service users can access CIFS shares and have optional home directories.
type NASServiceUserService struct {
	client *Client
}

// List returns all NAS service users, with optional filtering and pagination.
func (s *NASServiceUserService) List(ctx context.Context, opts ...ListOption) ([]NASServiceUser, error) {
	options := applyListOptions(opts)

	if options.Fields == "most" {
		options.Fields = nasServiceUserListFields
	}

	params := options.toQueryParams()

	var users []NASServiceUser
	if err := s.client.get(ctx, "/vm_service_users", params, &users); err != nil {
		return nil, err
	}

	return users, nil
}

// ListByService returns all users belonging to a specific NAS service.
func (s *NASServiceUserService) ListByService(ctx context.Context, serviceID int) ([]NASServiceUser, error) {
	return s.List(ctx, WithFilter(fmt.Sprintf("service eq %d", serviceID)))
}

// Get returns a single NAS service user by its SHA1 ID.
func (s *NASServiceUserService) Get(ctx context.Context, id string) (*NASServiceUser, error) {
	params := url.Values{}
	params.Set("fields", nasServiceUserGetFields)

	var user NASServiceUser
	endpoint := fmt.Sprintf("/vm_service_users/%s", id)
	if err := s.client.get(ctx, endpoint, params, &user); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "NASServiceUser", ID: id}
		}
		return nil, err
	}

	return &user, nil
}

// GetByName returns a NAS service user by name within a specific service.
func (s *NASServiceUserService) GetByName(ctx context.Context, serviceID int, name string) (*NASServiceUser, error) {
	users, err := s.List(ctx, WithFilter(fmt.Sprintf("service eq %d and name eq '%s'", serviceID, name)))
	if err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return nil, &NotFoundError{Resource: "NASServiceUser", ID: name}
	}
	// Get full details
	return s.Get(ctx, users[0].ID)
}

// Create creates a new NAS service user and returns the created user.
func (s *NASServiceUserService) Create(ctx context.Context, req *NASServiceUserCreateRequest) (*NASServiceUser, error) {
	if req == nil {
		return nil, &ValidationError{Message: "create request is required"}
	}
	if req.Service <= 0 {
		return nil, &ValidationError{Field: "service", Message: "service is required"}
	}
	if req.Name == "" {
		return nil, &ValidationError{Field: "name", Message: "name is required"}
	}
	if req.Password == "" {
		return nil, &ValidationError{Field: "password", Message: "password is required"}
	}

	// Set default enabled to true
	if req.Enabled == nil {
		enabled := true
		req.Enabled = &enabled
	}

	var resp apiResponse
	if err := s.client.post(ctx, "/vm_service_users", req, &resp); err != nil {
		return nil, err
	}

	// Extract the created user's ID (SHA1 hash string)
	id, err := getStringKey(resp)
	if err != nil {
		return nil, err
	}

	return s.Get(ctx, id)
}

// Update updates a NAS service user and returns the updated user.
func (s *NASServiceUserService) Update(ctx context.Context, id string, req *NASServiceUserUpdateRequest) (*NASServiceUser, error) {
	if req == nil {
		return nil, &ValidationError{Message: "update request is required"}
	}

	endpoint := fmt.Sprintf("/vm_service_users/%s", id)
	if err := s.client.put(ctx, endpoint, req, nil); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "NASServiceUser", ID: id}
		}
		return nil, err
	}

	return s.Get(ctx, id)
}

// Delete deletes a NAS service user.
func (s *NASServiceUserService) Delete(ctx context.Context, id string) error {
	endpoint := fmt.Sprintf("/vm_service_users/%s", id)
	if err := s.client.delete(ctx, endpoint); err != nil {
		if IsNotFoundError(err) {
			return &NotFoundError{Resource: "NASServiceUser", ID: id}
		}
		return err
	}
	return nil
}

// Enable enables a NAS service user.
func (s *NASServiceUserService) Enable(ctx context.Context, id string) error {
	enabled := true
	_, err := s.Update(ctx, id, &NASServiceUserUpdateRequest{Enabled: &enabled})
	return err
}

// Disable disables a NAS service user.
func (s *NASServiceUserService) Disable(ctx context.Context, id string) error {
	enabled := false
	_, err := s.Update(ctx, id, &NASServiceUserUpdateRequest{Enabled: &enabled})
	return err
}
