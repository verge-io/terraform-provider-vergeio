package vergeos

import (
	"context"
	"fmt"
	"net/url"
)

// VNetIPSecService handles IPSec VPN configuration operations.
type VNetIPSecService struct {
	client *Client
}

// List returns all IPSec configurations, with optional filtering and pagination.
func (s *VNetIPSecService) List(ctx context.Context, opts ...ListOption) ([]VNetIPSec, error) {
	options := applyListOptions(opts)

	if options.Fields == "most" {
		options.Fields = vnetIPSecListFields
	}

	params := options.toQueryParams()

	var ipsecs []VNetIPSec
	if err := s.client.get(ctx, "/vnet_ipsecs", params, &ipsecs); err != nil {
		return nil, err
	}

	return ipsecs, nil
}

// Get returns a single IPSec configuration by ID.
func (s *VNetIPSecService) Get(ctx context.Context, id int) (*VNetIPSec, error) {
	params := url.Values{}
	params.Set("fields", vnetIPSecGetFields)

	var ipsec VNetIPSec
	endpoint := fmt.Sprintf("/vnet_ipsecs/%d", id)
	if err := s.client.get(ctx, endpoint, params, &ipsec); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "VNetIPSec", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	return &ipsec, nil
}

// GetByNetwork returns the IPSec configuration for a specific network.
func (s *VNetIPSecService) GetByNetwork(ctx context.Context, vnetID int) (*VNetIPSec, error) {
	ipsecs, err := s.List(ctx, WithFilter(fmt.Sprintf("vnet eq %d", vnetID)))
	if err != nil {
		return nil, err
	}
	if len(ipsecs) == 0 {
		return nil, &NotFoundError{Resource: "VNetIPSec", ID: fmt.Sprintf("vnet:%d", vnetID)}
	}
	return s.Get(ctx, int(ipsecs[0].Key))
}

// Create creates a new IPSec configuration and returns it.
func (s *VNetIPSecService) Create(ctx context.Context, req *VNetIPSecCreateRequest) (*VNetIPSec, error) {
	if req == nil {
		return nil, &ValidationError{Message: "create request is required"}
	}
	if req.VNet <= 0 {
		return nil, &ValidationError{Field: "vnet", Message: "vnet is required"}
	}

	var resp apiResponse
	if err := s.client.post(ctx, "/vnet_ipsecs", req, &resp); err != nil {
		return nil, err
	}

	id, err := getKey(resp)
	if err != nil {
		return nil, err
	}

	return s.Get(ctx, id)
}

// Update updates an IPSec configuration and returns the updated configuration.
func (s *VNetIPSecService) Update(ctx context.Context, id int, req *VNetIPSecUpdateRequest) (*VNetIPSec, error) {
	if req == nil {
		return nil, &ValidationError{Message: "update request is required"}
	}

	endpoint := fmt.Sprintf("/vnet_ipsecs/%d", id)
	if err := s.client.put(ctx, endpoint, req, nil); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "VNetIPSec", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	return s.Get(ctx, id)
}

// Delete deletes an IPSec configuration.
func (s *VNetIPSecService) Delete(ctx context.Context, id int) error {
	endpoint := fmt.Sprintf("/vnet_ipsecs/%d", id)
	if err := s.client.delete(ctx, endpoint); err != nil {
		if IsNotFoundError(err) {
			return &NotFoundError{Resource: "VNetIPSec", ID: fmt.Sprintf("%d", id)}
		}
		return err
	}
	return nil
}

// VNetIPSecPhase1Service handles IPSec Phase 1 (IKE SA) configuration operations.
type VNetIPSecPhase1Service struct {
	client *Client
}

// List returns all Phase 1 configurations, with optional filtering and pagination.
func (s *VNetIPSecPhase1Service) List(ctx context.Context, opts ...ListOption) ([]VNetIPSecPhase1, error) {
	options := applyListOptions(opts)

	if options.Fields == "most" {
		options.Fields = vnetIPSecPhase1ListFields
	}

	params := options.toQueryParams()

	var phase1s []VNetIPSecPhase1
	if err := s.client.get(ctx, "/vnet_ipsec_phase1s", params, &phase1s); err != nil {
		return nil, err
	}

	return phase1s, nil
}

// ListByIPSec returns all Phase 1 configurations for a specific IPSec configuration.
func (s *VNetIPSecPhase1Service) ListByIPSec(ctx context.Context, ipsecID int, opts ...ListOption) ([]VNetIPSecPhase1, error) {
	filterOpts := []ListOption{WithFilter(fmt.Sprintf("ipsec eq %d", ipsecID))}
	filterOpts = append(filterOpts, opts...)
	return s.List(ctx, filterOpts...)
}

// Get returns a single Phase 1 configuration by ID.
func (s *VNetIPSecPhase1Service) Get(ctx context.Context, id int) (*VNetIPSecPhase1, error) {
	params := url.Values{}
	params.Set("fields", vnetIPSecPhase1GetFields)

	var phase1 VNetIPSecPhase1
	endpoint := fmt.Sprintf("/vnet_ipsec_phase1s/%d", id)
	if err := s.client.get(ctx, endpoint, params, &phase1); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "VNetIPSecPhase1", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	return &phase1, nil
}

// GetByName returns a Phase 1 configuration by name within an IPSec configuration.
func (s *VNetIPSecPhase1Service) GetByName(ctx context.Context, ipsecID int, name string) (*VNetIPSecPhase1, error) {
	phase1s, err := s.List(ctx, WithFilter(fmt.Sprintf("ipsec eq %d and name eq '%s'", ipsecID, name)))
	if err != nil {
		return nil, err
	}
	if len(phase1s) == 0 {
		return nil, &NotFoundError{Resource: "VNetIPSecPhase1", ID: name}
	}
	return s.Get(ctx, int(phase1s[0].Key))
}

// Create creates a new Phase 1 configuration and returns it.
func (s *VNetIPSecPhase1Service) Create(ctx context.Context, req *VNetIPSecPhase1CreateRequest) (*VNetIPSecPhase1, error) {
	if req == nil {
		return nil, &ValidationError{Message: "create request is required"}
	}
	if req.IPSec <= 0 {
		return nil, &ValidationError{Field: "ipsec", Message: "ipsec is required"}
	}
	if req.Name == "" {
		return nil, &ValidationError{Field: "name", Message: "name is required"}
	}
	if req.RemoteGateway == "" {
		return nil, &ValidationError{Field: "remote_gateway", Message: "remote_gateway is required"}
	}

	var resp apiResponse
	if err := s.client.post(ctx, "/vnet_ipsec_phase1s", req, &resp); err != nil {
		return nil, err
	}

	id, err := getKey(resp)
	if err != nil {
		return nil, err
	}

	return s.Get(ctx, id)
}

// Update updates a Phase 1 configuration and returns the updated configuration.
func (s *VNetIPSecPhase1Service) Update(ctx context.Context, id int, req *VNetIPSecPhase1UpdateRequest) (*VNetIPSecPhase1, error) {
	if req == nil {
		return nil, &ValidationError{Message: "update request is required"}
	}

	endpoint := fmt.Sprintf("/vnet_ipsec_phase1s/%d", id)
	if err := s.client.put(ctx, endpoint, req, nil); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "VNetIPSecPhase1", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	return s.Get(ctx, id)
}

// Delete deletes a Phase 1 configuration.
func (s *VNetIPSecPhase1Service) Delete(ctx context.Context, id int) error {
	endpoint := fmt.Sprintf("/vnet_ipsec_phase1s/%d", id)
	if err := s.client.delete(ctx, endpoint); err != nil {
		if IsNotFoundError(err) {
			return &NotFoundError{Resource: "VNetIPSecPhase1", ID: fmt.Sprintf("%d", id)}
		}
		return err
	}
	return nil
}

// VNetIPSecPhase2Service handles IPSec Phase 2 (IPsec SA) configuration operations.
type VNetIPSecPhase2Service struct {
	client *Client
}

// List returns all Phase 2 configurations, with optional filtering and pagination.
func (s *VNetIPSecPhase2Service) List(ctx context.Context, opts ...ListOption) ([]VNetIPSecPhase2, error) {
	options := applyListOptions(opts)

	if options.Fields == "most" {
		options.Fields = vnetIPSecPhase2ListFields
	}

	params := options.toQueryParams()

	var phase2s []VNetIPSecPhase2
	if err := s.client.get(ctx, "/vnet_ipsec_phase2s", params, &phase2s); err != nil {
		return nil, err
	}

	return phase2s, nil
}

// ListByPhase1 returns all Phase 2 configurations for a specific Phase 1.
func (s *VNetIPSecPhase2Service) ListByPhase1(ctx context.Context, phase1ID int, opts ...ListOption) ([]VNetIPSecPhase2, error) {
	filterOpts := []ListOption{WithFilter(fmt.Sprintf("phase1 eq %d", phase1ID))}
	filterOpts = append(filterOpts, opts...)
	return s.List(ctx, filterOpts...)
}

// Get returns a single Phase 2 configuration by ID.
func (s *VNetIPSecPhase2Service) Get(ctx context.Context, id int) (*VNetIPSecPhase2, error) {
	params := url.Values{}
	params.Set("fields", vnetIPSecPhase2GetFields)

	var phase2 VNetIPSecPhase2
	endpoint := fmt.Sprintf("/vnet_ipsec_phase2s/%d", id)
	if err := s.client.get(ctx, endpoint, params, &phase2); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "VNetIPSecPhase2", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	return &phase2, nil
}

// GetByName returns a Phase 2 configuration by name within a Phase 1.
func (s *VNetIPSecPhase2Service) GetByName(ctx context.Context, phase1ID int, name string) (*VNetIPSecPhase2, error) {
	phase2s, err := s.List(ctx, WithFilter(fmt.Sprintf("phase1 eq %d and name eq '%s'", phase1ID, name)))
	if err != nil {
		return nil, err
	}
	if len(phase2s) == 0 {
		return nil, &NotFoundError{Resource: "VNetIPSecPhase2", ID: name}
	}
	return s.Get(ctx, int(phase2s[0].Key))
}

// Create creates a new Phase 2 configuration and returns it.
func (s *VNetIPSecPhase2Service) Create(ctx context.Context, req *VNetIPSecPhase2CreateRequest) (*VNetIPSecPhase2, error) {
	if req == nil {
		return nil, &ValidationError{Message: "create request is required"}
	}
	if req.Phase1 <= 0 {
		return nil, &ValidationError{Field: "phase1", Message: "phase1 is required"}
	}
	if req.Name == "" {
		return nil, &ValidationError{Field: "name", Message: "name is required"}
	}
	if req.Local == "" {
		return nil, &ValidationError{Field: "local", Message: "local is required"}
	}

	var resp apiResponse
	if err := s.client.post(ctx, "/vnet_ipsec_phase2s", req, &resp); err != nil {
		return nil, err
	}

	id, err := getKey(resp)
	if err != nil {
		return nil, err
	}

	return s.Get(ctx, id)
}

// Update updates a Phase 2 configuration and returns the updated configuration.
func (s *VNetIPSecPhase2Service) Update(ctx context.Context, id int, req *VNetIPSecPhase2UpdateRequest) (*VNetIPSecPhase2, error) {
	if req == nil {
		return nil, &ValidationError{Message: "update request is required"}
	}

	endpoint := fmt.Sprintf("/vnet_ipsec_phase2s/%d", id)
	if err := s.client.put(ctx, endpoint, req, nil); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "VNetIPSecPhase2", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	return s.Get(ctx, id)
}

// Delete deletes a Phase 2 configuration.
func (s *VNetIPSecPhase2Service) Delete(ctx context.Context, id int) error {
	endpoint := fmt.Sprintf("/vnet_ipsec_phase2s/%d", id)
	if err := s.client.delete(ctx, endpoint); err != nil {
		if IsNotFoundError(err) {
			return &NotFoundError{Resource: "VNetIPSecPhase2", ID: fmt.Sprintf("%d", id)}
		}
		return err
	}
	return nil
}

// VNetIPSecConnectionService handles IPSec connection status operations (read-only).
type VNetIPSecConnectionService struct {
	client *Client
}

// List returns all active IPSec connections, with optional filtering.
func (s *VNetIPSecConnectionService) List(ctx context.Context, opts ...ListOption) ([]VNetIPSecConnection, error) {
	options := applyListOptions(opts)

	if options.Fields == "most" {
		options.Fields = vnetIPSecConnectionListFields
	}

	params := options.toQueryParams()

	var conns []VNetIPSecConnection
	if err := s.client.get(ctx, "/vnet_ipsec_connections", params, &conns); err != nil {
		return nil, err
	}

	return conns, nil
}

// ListByNetwork returns all active connections for a specific network.
func (s *VNetIPSecConnectionService) ListByNetwork(ctx context.Context, vnetID int, opts ...ListOption) ([]VNetIPSecConnection, error) {
	filterOpts := []ListOption{WithFilter(fmt.Sprintf("vnet eq %d", vnetID))}
	filterOpts = append(filterOpts, opts...)
	return s.List(ctx, filterOpts...)
}

// ListByPhase1 returns all active connections for a specific Phase 1.
func (s *VNetIPSecConnectionService) ListByPhase1(ctx context.Context, phase1ID int, opts ...ListOption) ([]VNetIPSecConnection, error) {
	filterOpts := []ListOption{WithFilter(fmt.Sprintf("phase1 eq %d", phase1ID))}
	filterOpts = append(filterOpts, opts...)
	return s.List(ctx, filterOpts...)
}

// Get returns a single IPSec connection by ID.
func (s *VNetIPSecConnectionService) Get(ctx context.Context, id int) (*VNetIPSecConnection, error) {
	params := url.Values{}
	params.Set("fields", vnetIPSecConnectionListFields)

	var conn VNetIPSecConnection
	endpoint := fmt.Sprintf("/vnet_ipsec_connections/%d", id)
	if err := s.client.get(ctx, endpoint, params, &conn); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "VNetIPSecConnection", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	return &conn, nil
}
