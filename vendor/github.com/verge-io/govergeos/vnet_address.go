package vergeos

import (
	"context"
	"fmt"
	"net/url"
)

// VNetAddressService handles network IP address operations.
type VNetAddressService struct {
	client *Client
}

// List returns all network addresses, with optional filtering and pagination.
func (s *VNetAddressService) List(ctx context.Context, opts ...ListOption) ([]VNetAddress, error) {
	options := applyListOptions(opts)

	if options.Fields == "most" {
		options.Fields = vnetAddressListFields
	}

	params := options.toQueryParams()

	var addresses []VNetAddress
	if err := s.client.get(ctx, "/vnet_addresses", params, &addresses); err != nil {
		return nil, err
	}

	return addresses, nil
}

// ListByNetwork returns all addresses for a specific network.
func (s *VNetAddressService) ListByNetwork(ctx context.Context, vnetID int, opts ...ListOption) ([]VNetAddress, error) {
	filterOpts := []ListOption{WithFilter(fmt.Sprintf("vnet eq %d", vnetID))}
	filterOpts = append(filterOpts, opts...)
	return s.List(ctx, filterOpts...)
}

// ListByType returns all addresses of a specific type within a network.
func (s *VNetAddressService) ListByType(ctx context.Context, vnetID int, addrType string, opts ...ListOption) ([]VNetAddress, error) {
	filterOpts := []ListOption{WithFilter(fmt.Sprintf("vnet eq %d and type eq '%s'", vnetID, addrType))}
	filterOpts = append(filterOpts, opts...)
	return s.List(ctx, filterOpts...)
}

// Get returns a single address by ID.
func (s *VNetAddressService) Get(ctx context.Context, id int) (*VNetAddress, error) {
	params := url.Values{}
	params.Set("fields", vnetAddressGetFields)

	var address VNetAddress
	endpoint := fmt.Sprintf("/vnet_addresses/%d", id)
	if err := s.client.get(ctx, endpoint, params, &address); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "VNetAddress", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	return &address, nil
}

// GetByIP returns an address by IP within a specific network.
func (s *VNetAddressService) GetByIP(ctx context.Context, vnetID int, ip string) (*VNetAddress, error) {
	addresses, err := s.List(ctx, WithFilter(fmt.Sprintf("vnet eq %d and ip eq '%s'", vnetID, ip)))
	if err != nil {
		return nil, err
	}
	if len(addresses) == 0 {
		return nil, &NotFoundError{Resource: "VNetAddress", ID: ip}
	}
	return s.Get(ctx, int(addresses[0].Key))
}

// GetByMAC returns an address by MAC address within a specific network.
func (s *VNetAddressService) GetByMAC(ctx context.Context, vnetID int, mac string) (*VNetAddress, error) {
	addresses, err := s.List(ctx, WithFilter(fmt.Sprintf("vnet eq %d and mac eq '%s'", vnetID, mac)))
	if err != nil {
		return nil, err
	}
	if len(addresses) == 0 {
		return nil, &NotFoundError{Resource: "VNetAddress", ID: mac}
	}
	return s.Get(ctx, int(addresses[0].Key))
}

// Create creates a new network address and returns the created address.
func (s *VNetAddressService) Create(ctx context.Context, req *VNetAddressCreateRequest) (*VNetAddress, error) {
	if req == nil {
		return nil, &ValidationError{Message: "create request is required"}
	}
	if req.VNet <= 0 {
		return nil, &ValidationError{Field: "vnet", Message: "vnet is required"}
	}
	if req.Type == "" {
		return nil, &ValidationError{Field: "type", Message: "type is required"}
	}

	var resp apiResponse
	if err := s.client.post(ctx, "/vnet_addresses", req, &resp); err != nil {
		return nil, err
	}

	id, err := getKey(resp)
	if err != nil {
		return nil, err
	}

	return s.Get(ctx, id)
}

// Update updates an address and returns the updated address.
func (s *VNetAddressService) Update(ctx context.Context, id int, req *VNetAddressUpdateRequest) (*VNetAddress, error) {
	if req == nil {
		return nil, &ValidationError{Message: "update request is required"}
	}

	endpoint := fmt.Sprintf("/vnet_addresses/%d", id)
	if err := s.client.put(ctx, endpoint, req, nil); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "VNetAddress", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	return s.Get(ctx, id)
}

// Delete deletes a network address.
func (s *VNetAddressService) Delete(ctx context.Context, id int) error {
	endpoint := fmt.Sprintf("/vnet_addresses/%d", id)
	if err := s.client.delete(ctx, endpoint); err != nil {
		if IsNotFoundError(err) {
			return &NotFoundError{Resource: "VNetAddress", ID: fmt.Sprintf("%d", id)}
		}
		return err
	}
	return nil
}
