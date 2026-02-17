package vergeos

import (
	"context"
	"fmt"
	"net/url"
)

// PermissionService handles permission operations.
// Permissions grant identities (users/groups) access to specific resources.
type PermissionService struct {
	client *Client
}

// List returns all permissions, with optional filtering and pagination.
func (s *PermissionService) List(ctx context.Context, opts ...ListOption) ([]Permission, error) {
	options := applyListOptions(opts)

	if options.Fields == "most" {
		options.Fields = permissionListFields
	}

	params := options.toQueryParams()

	var permissions []Permission
	if err := s.client.get(ctx, "/permissions", params, &permissions); err != nil {
		return nil, err
	}

	return permissions, nil
}

// ListByIdentity returns all permissions for a specific identity (user or group).
func (s *PermissionService) ListByIdentity(ctx context.Context, identityID int, opts ...ListOption) ([]Permission, error) {
	opts = append([]ListOption{WithFilter(fmt.Sprintf("identity eq %d", identityID))}, opts...)
	return s.List(ctx, opts...)
}

// ListByTable returns all permissions for a specific resource type.
func (s *PermissionService) ListByTable(ctx context.Context, table string, opts ...ListOption) ([]Permission, error) {
	opts = append([]ListOption{WithFilter(fmt.Sprintf("table eq '%s'", table))}, opts...)
	return s.List(ctx, opts...)
}

// ListByResource returns all permissions for a specific resource instance.
func (s *PermissionService) ListByResource(ctx context.Context, table string, rowID int64, opts ...ListOption) ([]Permission, error) {
	opts = append([]ListOption{WithFilter(fmt.Sprintf("table eq '%s' and row eq %d", table, rowID))}, opts...)
	return s.List(ctx, opts...)
}

// Get returns a single permission by ID.
func (s *PermissionService) Get(ctx context.Context, id int) (*Permission, error) {
	params := url.Values{}
	params.Set("fields", permissionGetFields)

	var permission Permission
	endpoint := fmt.Sprintf("/permissions/%d", id)
	if err := s.client.get(ctx, endpoint, params, &permission); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "Permission", ID: id}
		}
		return nil, err
	}

	return &permission, nil
}

// GetByIdentityAndResource returns the permission for a specific identity and resource.
func (s *PermissionService) GetByIdentityAndResource(ctx context.Context, identityID int, table string, rowID int64) (*Permission, error) {
	permissions, err := s.List(ctx, WithFilter(fmt.Sprintf("identity eq %d and table eq '%s' and row eq %d", identityID, table, rowID)))
	if err != nil {
		return nil, err
	}
	if len(permissions) == 0 {
		return nil, &NotFoundError{Resource: "Permission", ID: fmt.Sprintf("identity=%d,table=%s,row=%d", identityID, table, rowID)}
	}
	// Get full details
	return s.Get(ctx, int(permissions[0].Key))
}

// Create creates a new permission and returns the created permission.
func (s *PermissionService) Create(ctx context.Context, req *PermissionCreateRequest) (*Permission, error) {
	if req == nil {
		return nil, &ValidationError{Message: "create request is required"}
	}
	if req.Identity <= 0 {
		return nil, &ValidationError{Field: "identity", Message: "identity is required"}
	}
	if req.Table == "" {
		return nil, &ValidationError{Field: "table", Message: "table is required"}
	}
	if req.Row <= 0 {
		return nil, &ValidationError{Field: "row", Message: "row is required"}
	}

	var resp apiResponse
	if err := s.client.post(ctx, "/permissions", req, &resp); err != nil {
		return nil, err
	}

	id, err := getKey(resp)
	if err != nil {
		return nil, err
	}

	return s.Get(ctx, id)
}

// Update updates a permission and returns the updated permission.
func (s *PermissionService) Update(ctx context.Context, id int, req *PermissionUpdateRequest) (*Permission, error) {
	if req == nil {
		return nil, &ValidationError{Message: "update request is required"}
	}

	endpoint := fmt.Sprintf("/permissions/%d", id)
	if err := s.client.put(ctx, endpoint, req, nil); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "Permission", ID: id}
		}
		return nil, err
	}

	return s.Get(ctx, id)
}

// Delete deletes a permission.
func (s *PermissionService) Delete(ctx context.Context, id int) error {
	endpoint := fmt.Sprintf("/permissions/%d", id)
	if err := s.client.delete(ctx, endpoint); err != nil {
		if IsNotFoundError(err) {
			return &NotFoundError{Resource: "Permission", ID: id}
		}
		return err
	}
	return nil
}

// Grant creates a permission granting an identity access to a resource.
// This is a convenience method that creates a permission with common defaults.
func (s *PermissionService) Grant(ctx context.Context, identityID int, table string, rowID int64, read, modify, delete bool) (*Permission, error) {
	// If read is true, list is automatically set by the API
	list := read
	return s.Create(ctx, &PermissionCreateRequest{
		Identity: identityID,
		Table:    table,
		Row:      rowID,
		List:     &list,
		Read:     &read,
		Modify:   &modify,
		Delete:   &delete,
	})
}

// GrantReadOnly grants read-only access to a resource.
func (s *PermissionService) GrantReadOnly(ctx context.Context, identityID int, table string, rowID int64) (*Permission, error) {
	return s.Grant(ctx, identityID, table, rowID, true, false, false)
}

// GrantFullAccess grants full access (read, modify, delete) to a resource.
func (s *PermissionService) GrantFullAccess(ctx context.Context, identityID int, table string, rowID int64) (*Permission, error) {
	t := true
	return s.Create(ctx, &PermissionCreateRequest{
		Identity: identityID,
		Table:    table,
		Row:      rowID,
		List:     &t,
		Read:     &t,
		Create:   &t,
		Modify:   &t,
		Delete:   &t,
	})
}

// Revoke deletes the permission for a specific identity and resource.
func (s *PermissionService) Revoke(ctx context.Context, identityID int, table string, rowID int64) error {
	perm, err := s.GetByIdentityAndResource(ctx, identityID, table, rowID)
	if err != nil {
		if IsNotFoundError(err) {
			// Permission doesn't exist, nothing to revoke
			return nil
		}
		return err
	}
	return s.Delete(ctx, int(perm.Key))
}
