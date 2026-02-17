package vergeos

import (
	"context"
	"fmt"
	"net/url"
)

// VNetWireGuardService handles WireGuard VPN interface operations.
type VNetWireGuardService struct {
	client *Client
}

// List returns all WireGuard interfaces, with optional filtering and pagination.
func (s *VNetWireGuardService) List(ctx context.Context, opts ...ListOption) ([]VNetWireGuard, error) {
	options := applyListOptions(opts)

	if options.Fields == "most" {
		options.Fields = vnetWireGuardListFields
	}

	params := options.toQueryParams()

	var wgs []VNetWireGuard
	if err := s.client.get(ctx, "/vnet_wireguards", params, &wgs); err != nil {
		return nil, err
	}

	return wgs, nil
}

// ListByNetwork returns all WireGuard interfaces for a specific network.
func (s *VNetWireGuardService) ListByNetwork(ctx context.Context, vnetID int, opts ...ListOption) ([]VNetWireGuard, error) {
	filterOpts := []ListOption{WithFilter(fmt.Sprintf("vnet eq %d", vnetID))}
	filterOpts = append(filterOpts, opts...)
	return s.List(ctx, filterOpts...)
}

// Get returns a single WireGuard interface by ID.
func (s *VNetWireGuardService) Get(ctx context.Context, id int) (*VNetWireGuard, error) {
	params := url.Values{}
	params.Set("fields", vnetWireGuardGetFields)

	var wg VNetWireGuard
	endpoint := fmt.Sprintf("/vnet_wireguards/%d", id)
	if err := s.client.get(ctx, endpoint, params, &wg); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "VNetWireGuard", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	return &wg, nil
}

// GetByName returns a WireGuard interface by name within a specific network.
func (s *VNetWireGuardService) GetByName(ctx context.Context, vnetID int, name string) (*VNetWireGuard, error) {
	wgs, err := s.List(ctx, WithFilter(fmt.Sprintf("vnet eq %d and name eq '%s'", vnetID, name)))
	if err != nil {
		return nil, err
	}
	if len(wgs) == 0 {
		return nil, &NotFoundError{Resource: "VNetWireGuard", ID: name}
	}
	return s.Get(ctx, int(wgs[0].Key))
}

// Create creates a new WireGuard interface and returns the created interface.
func (s *VNetWireGuardService) Create(ctx context.Context, req *VNetWireGuardCreateRequest) (*VNetWireGuard, error) {
	if req == nil {
		return nil, &ValidationError{Message: "create request is required"}
	}
	if req.VNet <= 0 {
		return nil, &ValidationError{Field: "vnet", Message: "vnet is required"}
	}
	if req.Name == "" {
		return nil, &ValidationError{Field: "name", Message: "name is required"}
	}
	if req.IP == "" {
		return nil, &ValidationError{Field: "ip", Message: "ip is required"}
	}

	var resp apiResponse
	if err := s.client.post(ctx, "/vnet_wireguards", req, &resp); err != nil {
		return nil, err
	}

	id, err := getKey(resp)
	if err != nil {
		return nil, err
	}

	return s.Get(ctx, id)
}

// Update updates a WireGuard interface and returns the updated interface.
func (s *VNetWireGuardService) Update(ctx context.Context, id int, req *VNetWireGuardUpdateRequest) (*VNetWireGuard, error) {
	if req == nil {
		return nil, &ValidationError{Message: "update request is required"}
	}

	endpoint := fmt.Sprintf("/vnet_wireguards/%d", id)
	if err := s.client.put(ctx, endpoint, req, nil); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "VNetWireGuard", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	return s.Get(ctx, id)
}

// Delete deletes a WireGuard interface.
func (s *VNetWireGuardService) Delete(ctx context.Context, id int) error {
	endpoint := fmt.Sprintf("/vnet_wireguards/%d", id)
	if err := s.client.delete(ctx, endpoint); err != nil {
		if IsNotFoundError(err) {
			return &NotFoundError{Resource: "VNetWireGuard", ID: fmt.Sprintf("%d", id)}
		}
		return err
	}
	return nil
}

// VNetWireGuardPeerService handles WireGuard peer operations.
type VNetWireGuardPeerService struct {
	client *Client
}

// List returns all WireGuard peers, with optional filtering and pagination.
func (s *VNetWireGuardPeerService) List(ctx context.Context, opts ...ListOption) ([]VNetWireGuardPeer, error) {
	options := applyListOptions(opts)

	if options.Fields == "most" {
		options.Fields = vnetWireGuardPeerListFields
	}

	params := options.toQueryParams()

	var peers []VNetWireGuardPeer
	if err := s.client.get(ctx, "/vnet_wireguard_peers", params, &peers); err != nil {
		return nil, err
	}

	return peers, nil
}

// ListByWireGuard returns all peers for a specific WireGuard interface.
func (s *VNetWireGuardPeerService) ListByWireGuard(ctx context.Context, wireguardID int, opts ...ListOption) ([]VNetWireGuardPeer, error) {
	filterOpts := []ListOption{WithFilter(fmt.Sprintf("wireguard eq %d", wireguardID))}
	filterOpts = append(filterOpts, opts...)
	return s.List(ctx, filterOpts...)
}

// Get returns a single WireGuard peer by ID.
func (s *VNetWireGuardPeerService) Get(ctx context.Context, id int) (*VNetWireGuardPeer, error) {
	params := url.Values{}
	params.Set("fields", vnetWireGuardPeerGetFields)

	var peer VNetWireGuardPeer
	endpoint := fmt.Sprintf("/vnet_wireguard_peers/%d", id)
	if err := s.client.get(ctx, endpoint, params, &peer); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "VNetWireGuardPeer", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	return &peer, nil
}

// GetByName returns a WireGuard peer by name within a specific WireGuard interface.
func (s *VNetWireGuardPeerService) GetByName(ctx context.Context, wireguardID int, name string) (*VNetWireGuardPeer, error) {
	peers, err := s.List(ctx, WithFilter(fmt.Sprintf("wireguard eq %d and name eq '%s'", wireguardID, name)))
	if err != nil {
		return nil, err
	}
	if len(peers) == 0 {
		return nil, &NotFoundError{Resource: "VNetWireGuardPeer", ID: name}
	}
	return s.Get(ctx, int(peers[0].Key))
}

// Create creates a new WireGuard peer and returns the created peer.
func (s *VNetWireGuardPeerService) Create(ctx context.Context, req *VNetWireGuardPeerCreateRequest) (*VNetWireGuardPeer, error) {
	if req == nil {
		return nil, &ValidationError{Message: "create request is required"}
	}
	if req.WireGuard <= 0 {
		return nil, &ValidationError{Field: "wireguard", Message: "wireguard is required"}
	}
	if req.Name == "" {
		return nil, &ValidationError{Field: "name", Message: "name is required"}
	}
	if req.PeerIP == "" {
		return nil, &ValidationError{Field: "peer_ip", Message: "peer_ip is required"}
	}
	if req.PublicKey == "" {
		return nil, &ValidationError{Field: "public_key", Message: "public_key is required"}
	}
	if req.AllowedIPs == "" {
		return nil, &ValidationError{Field: "allowed_ips", Message: "allowed_ips is required"}
	}

	var resp apiResponse
	if err := s.client.post(ctx, "/vnet_wireguard_peers", req, &resp); err != nil {
		return nil, err
	}

	id, err := getKey(resp)
	if err != nil {
		return nil, err
	}

	return s.Get(ctx, id)
}

// Update updates a WireGuard peer and returns the updated peer.
func (s *VNetWireGuardPeerService) Update(ctx context.Context, id int, req *VNetWireGuardPeerUpdateRequest) (*VNetWireGuardPeer, error) {
	if req == nil {
		return nil, &ValidationError{Message: "update request is required"}
	}

	endpoint := fmt.Sprintf("/vnet_wireguard_peers/%d", id)
	if err := s.client.put(ctx, endpoint, req, nil); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "VNetWireGuardPeer", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	return s.Get(ctx, id)
}

// Delete deletes a WireGuard peer.
func (s *VNetWireGuardPeerService) Delete(ctx context.Context, id int) error {
	endpoint := fmt.Sprintf("/vnet_wireguard_peers/%d", id)
	if err := s.client.delete(ctx, endpoint); err != nil {
		if IsNotFoundError(err) {
			return &NotFoundError{Resource: "VNetWireGuardPeer", ID: fmt.Sprintf("%d", id)}
		}
		return err
	}
	return nil
}

// GetConfig retrieves the peer's WireGuard configuration file content.
// This is only available if autogenerate_peer is enabled on the peer.
func (s *VNetWireGuardPeerService) GetConfig(ctx context.Context, id int) (string, error) {
	params := url.Values{}
	params.Set("fields", "wg_config")

	var result struct {
		WGConfig string `json:"wg_config"`
	}
	endpoint := fmt.Sprintf("/vnet_wireguard_peers/%d", id)
	if err := s.client.get(ctx, endpoint, params, &result); err != nil {
		if IsNotFoundError(err) {
			return "", &NotFoundError{Resource: "VNetWireGuardPeer", ID: fmt.Sprintf("%d", id)}
		}
		return "", err
	}

	return result.WGConfig, nil
}

// VNetWireGuardPeerStatusService handles WireGuard peer status operations (read-only).
type VNetWireGuardPeerStatusService struct {
	client *Client
}

// List returns all WireGuard peer statuses, with optional filtering.
func (s *VNetWireGuardPeerStatusService) List(ctx context.Context, opts ...ListOption) ([]VNetWireGuardPeerStatus, error) {
	options := applyListOptions(opts)

	if options.Fields == "most" {
		options.Fields = vnetWireGuardPeerStatusListFields
	}

	params := options.toQueryParams()

	var statuses []VNetWireGuardPeerStatus
	if err := s.client.get(ctx, "/vnet_wireguard_peer_status", params, &statuses); err != nil {
		return nil, err
	}

	return statuses, nil
}

// Get returns a single WireGuard peer status by ID.
func (s *VNetWireGuardPeerStatusService) Get(ctx context.Context, id int) (*VNetWireGuardPeerStatus, error) {
	params := url.Values{}
	params.Set("fields", vnetWireGuardPeerStatusGetFields)

	var status VNetWireGuardPeerStatus
	endpoint := fmt.Sprintf("/vnet_wireguard_peer_status/%d", id)
	if err := s.client.get(ctx, endpoint, params, &status); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "VNetWireGuardPeerStatus", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	return &status, nil
}

// GetByPeer returns the status for a specific WireGuard peer.
func (s *VNetWireGuardPeerStatusService) GetByPeer(ctx context.Context, peerID int) (*VNetWireGuardPeerStatus, error) {
	statuses, err := s.List(ctx, WithFilter(fmt.Sprintf("peer eq %d", peerID)))
	if err != nil {
		return nil, err
	}
	if len(statuses) == 0 {
		return nil, &NotFoundError{Resource: "VNetWireGuardPeerStatus", ID: fmt.Sprintf("peer:%d", peerID)}
	}
	return &statuses[0], nil
}
