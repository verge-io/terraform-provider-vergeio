package vergeos

import (
	"context"
	"fmt"
	"net/url"
)

// VNetHostService handles network host override operations.
// Host overrides provide static hostname-to-IP mappings for DNS and DHCP.
type VNetHostService struct {
	client *Client
}

// List returns all host overrides, with optional filtering and pagination.
func (s *VNetHostService) List(ctx context.Context, opts ...ListOption) ([]VNetHost, error) {
	options := applyListOptions(opts)

	if options.Fields == "most" {
		options.Fields = vnetHostListFields
	}

	params := options.toQueryParams()

	var hosts []VNetHost
	if err := s.client.get(ctx, "/vnet_hosts", params, &hosts); err != nil {
		return nil, err
	}

	return hosts, nil
}

// ListByNetwork returns all host overrides for a specific network.
func (s *VNetHostService) ListByNetwork(ctx context.Context, vnetID int, opts ...ListOption) ([]VNetHost, error) {
	filterOpts := []ListOption{WithFilter(fmt.Sprintf("vnet eq %d", vnetID))}
	filterOpts = append(filterOpts, opts...)
	return s.List(ctx, filterOpts...)
}

// Get returns a single host override by ID.
func (s *VNetHostService) Get(ctx context.Context, id int) (*VNetHost, error) {
	params := url.Values{}
	params.Set("fields", vnetHostGetFields)

	var host VNetHost
	endpoint := fmt.Sprintf("/vnet_hosts/%d", id)
	if err := s.client.get(ctx, endpoint, params, &host); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "VNetHost", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	return &host, nil
}

// GetByHost returns a host override by hostname within a specific network.
func (s *VNetHostService) GetByHost(ctx context.Context, vnetID int, hostname string) (*VNetHost, error) {
	hosts, err := s.List(ctx, WithFilter(fmt.Sprintf("vnet eq %d and host eq '%s'", vnetID, hostname)))
	if err != nil {
		return nil, err
	}
	if len(hosts) == 0 {
		return nil, &NotFoundError{Resource: "VNetHost", ID: hostname}
	}
	return s.Get(ctx, int(hosts[0].Key))
}

// GetByIP returns a host override by IP address within a specific network.
func (s *VNetHostService) GetByIP(ctx context.Context, vnetID int, ip string) (*VNetHost, error) {
	hosts, err := s.List(ctx, WithFilter(fmt.Sprintf("vnet eq %d and ip eq '%s'", vnetID, ip)))
	if err != nil {
		return nil, err
	}
	if len(hosts) == 0 {
		return nil, &NotFoundError{Resource: "VNetHost", ID: ip}
	}
	return s.Get(ctx, int(hosts[0].Key))
}

// Create creates a new host override and returns the created host.
func (s *VNetHostService) Create(ctx context.Context, req *VNetHostCreateRequest) (*VNetHost, error) {
	if req == nil {
		return nil, &ValidationError{Message: "create request is required"}
	}
	if req.VNet <= 0 {
		return nil, &ValidationError{Field: "vnet", Message: "vnet is required"}
	}
	if req.Host == "" {
		return nil, &ValidationError{Field: "host", Message: "host is required"}
	}
	if req.IP == "" {
		return nil, &ValidationError{Field: "ip", Message: "ip is required"}
	}

	var resp apiResponse
	if err := s.client.post(ctx, "/vnet_hosts", req, &resp); err != nil {
		return nil, err
	}

	id, err := getKey(resp)
	if err != nil {
		return nil, err
	}

	return s.Get(ctx, id)
}

// Update updates a host override and returns the updated host.
func (s *VNetHostService) Update(ctx context.Context, id int, req *VNetHostUpdateRequest) (*VNetHost, error) {
	if req == nil {
		return nil, &ValidationError{Message: "update request is required"}
	}

	endpoint := fmt.Sprintf("/vnet_hosts/%d", id)
	if err := s.client.put(ctx, endpoint, req, nil); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "VNetHost", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	return s.Get(ctx, id)
}

// Delete deletes a host override.
func (s *VNetHostService) Delete(ctx context.Context, id int) error {
	endpoint := fmt.Sprintf("/vnet_hosts/%d", id)
	if err := s.client.delete(ctx, endpoint); err != nil {
		if IsNotFoundError(err) {
			return &NotFoundError{Resource: "VNetHost", ID: fmt.Sprintf("%d", id)}
		}
		return err
	}
	return nil
}
