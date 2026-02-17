package vergeos

import (
	"context"
	"fmt"
	"net/url"
)

// TenantLayer2NetworkService handles tenant layer2 network assignment operations.
// Layer 2 networks allow tenants to access host networks directly.
type TenantLayer2NetworkService struct {
	client *Client
}

// List returns all tenant layer2 network assignments, with optional filtering and pagination.
func (s *TenantLayer2NetworkService) List(ctx context.Context, opts ...ListOption) ([]TenantLayer2Network, error) {
	options := applyListOptions(opts)

	if options.Fields == "most" {
		options.Fields = tenantLayer2NetworkListFields
	}

	params := options.toQueryParams()

	var networks []TenantLayer2Network
	if err := s.client.get(ctx, "/tenant_layer2_vnets", params, &networks); err != nil {
		return nil, err
	}

	return networks, nil
}

// ListByTenant returns all layer2 network assignments for a specific tenant.
func (s *TenantLayer2NetworkService) ListByTenant(ctx context.Context, tenantID int, opts ...ListOption) ([]TenantLayer2Network, error) {
	opts = append(opts, WithFilter(fmt.Sprintf("tenant eq %d", tenantID)))
	return s.List(ctx, opts...)
}

// ListByNetwork returns all tenant assignments for a specific network.
func (s *TenantLayer2NetworkService) ListByNetwork(ctx context.Context, vnetID int, opts ...ListOption) ([]TenantLayer2Network, error) {
	opts = append(opts, WithFilter(fmt.Sprintf("vnet eq %d", vnetID)))
	return s.List(ctx, opts...)
}

// Get returns a single tenant layer2 network assignment by ID.
func (s *TenantLayer2NetworkService) Get(ctx context.Context, id int) (*TenantLayer2Network, error) {
	params := url.Values{}
	params.Set("fields", tenantLayer2NetworkGetFields)

	var network TenantLayer2Network
	endpoint := fmt.Sprintf("/tenant_layer2_vnets/%d", id)
	if err := s.client.get(ctx, endpoint, params, &network); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "TenantLayer2Network", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	return &network, nil
}

// GetByTenantAndNetwork returns a tenant layer2 network assignment by tenant and network IDs.
func (s *TenantLayer2NetworkService) GetByTenantAndNetwork(ctx context.Context, tenantID, vnetID int) (*TenantLayer2Network, error) {
	networks, err := s.List(ctx,
		WithFilter(fmt.Sprintf("tenant eq %d and vnet eq %d", tenantID, vnetID)),
	)
	if err != nil {
		return nil, err
	}
	if len(networks) == 0 {
		return nil, &NotFoundError{Resource: "TenantLayer2Network", ID: fmt.Sprintf("tenant:%d vnet:%d", tenantID, vnetID)}
	}
	return s.Get(ctx, int(networks[0].Key))
}

// Create creates a new tenant layer2 network assignment and returns it.
func (s *TenantLayer2NetworkService) Create(ctx context.Context, req *TenantLayer2NetworkCreateRequest) (*TenantLayer2Network, error) {
	if req == nil {
		return nil, &ValidationError{Message: "create request is required"}
	}
	if req.Tenant == 0 {
		return nil, &ValidationError{Field: "tenant", Message: "tenant is required"}
	}
	if req.VNet == 0 {
		return nil, &ValidationError{Field: "vnet", Message: "vnet is required"}
	}

	var resp apiResponse
	if err := s.client.post(ctx, "/tenant_layer2_vnets", req, &resp); err != nil {
		return nil, err
	}

	id, err := getKey(resp)
	if err != nil {
		return nil, err
	}

	return s.Get(ctx, id)
}

// Update updates a tenant layer2 network assignment and returns the updated assignment.
func (s *TenantLayer2NetworkService) Update(ctx context.Context, id int, req *TenantLayer2NetworkUpdateRequest) (*TenantLayer2Network, error) {
	if req == nil {
		return nil, &ValidationError{Message: "update request is required"}
	}

	endpoint := fmt.Sprintf("/tenant_layer2_vnets/%d", id)
	if err := s.client.put(ctx, endpoint, req, nil); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "TenantLayer2Network", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	return s.Get(ctx, id)
}

// Delete deletes a tenant layer2 network assignment.
func (s *TenantLayer2NetworkService) Delete(ctx context.Context, id int) error {
	endpoint := fmt.Sprintf("/tenant_layer2_vnets/%d", id)
	if err := s.client.delete(ctx, endpoint); err != nil {
		if IsNotFoundError(err) {
			return &NotFoundError{Resource: "TenantLayer2Network", ID: fmt.Sprintf("%d", id)}
		}
		return err
	}
	return nil
}

// Enable enables a tenant layer2 network assignment.
func (s *TenantLayer2NetworkService) Enable(ctx context.Context, id int) (*TenantLayer2Network, error) {
	enabled := true
	return s.Update(ctx, id, &TenantLayer2NetworkUpdateRequest{Enabled: &enabled})
}

// Disable disables a tenant layer2 network assignment.
func (s *TenantLayer2NetworkService) Disable(ctx context.Context, id int) (*TenantLayer2Network, error) {
	enabled := false
	return s.Update(ctx, id, &TenantLayer2NetworkUpdateRequest{Enabled: &enabled})
}

// Assign is a convenience method to assign a network to a tenant.
// If the assignment already exists, it returns the existing assignment.
func (s *TenantLayer2NetworkService) Assign(ctx context.Context, tenantID, vnetID int) (*TenantLayer2Network, error) {
	// Check if assignment already exists
	existing, err := s.GetByTenantAndNetwork(ctx, tenantID, vnetID)
	if err == nil {
		return existing, nil
	}
	if !IsNotFoundError(err) {
		return nil, err
	}

	// Create new assignment
	enabled := true
	return s.Create(ctx, &TenantLayer2NetworkCreateRequest{
		Tenant:  tenantID,
		VNet:    vnetID,
		Enabled: &enabled,
	})
}

// Unassign is a convenience method to remove a network assignment from a tenant.
func (s *TenantLayer2NetworkService) Unassign(ctx context.Context, tenantID, vnetID int) error {
	existing, err := s.GetByTenantAndNetwork(ctx, tenantID, vnetID)
	if err != nil {
		if IsNotFoundError(err) {
			return nil // Already unassigned
		}
		return err
	}
	return s.Delete(ctx, int(existing.Key))
}
