package vergeos

import (
	"context"
	"fmt"
	"net/url"
	"time"
)

const (
	// Network action constants
	networkActionPowerOn  = "poweron"
	networkActionPowerOff = "poweroff"
	networkActionKill     = "killpower"
	networkActionReset    = "reset"
	networkActionApply    = "apply"
	networkActionApplyDNS = "applydns"

	// Network power state polling
	networkPowerStateMaxRetries   = 30
	networkPowerStatePollInterval = 5 * time.Second
)

// NetworkService handles network operations.
type NetworkService struct {
	client *Client
}

// List returns all networks, with optional filtering and pagination.
func (s *NetworkService) List(ctx context.Context, opts ...ListOption) ([]Network, error) {
	options := applyListOptions(opts)

	// Use network-specific fields if not specified
	if options.Fields == "most" {
		options.Fields = networkListFields
	}

	params := options.toQueryParams()

	var networks []Network
	if err := s.client.get(ctx, "/vnets", params, &networks); err != nil {
		return nil, err
	}

	return networks, nil
}

// Get returns a single network by ID.
func (s *NetworkService) Get(ctx context.Context, id int) (*Network, error) {
	params := url.Values{}
	params.Set("fields", networkGetFields)

	var network Network
	endpoint := fmt.Sprintf("/vnets/%d", id)
	if err := s.client.get(ctx, endpoint, params, &network); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "Network", ID: id}
		}
		return nil, err
	}

	return &network, nil
}

// Create creates a new network and returns the created network.
func (s *NetworkService) Create(ctx context.Context, req *NetworkCreateRequest) (*Network, error) {
	if req == nil {
		return nil, &ValidationError{Message: "create request is required"}
	}
	if req.Name == "" {
		return nil, &ValidationError{Field: "name", Message: "name is required"}
	}

	// Set defaults
	if req.Enabled == nil {
		enabled := true
		req.Enabled = &enabled
	}

	var resp apiResponse
	if err := s.client.post(ctx, "/vnets", req, &resp); err != nil {
		return nil, err
	}

	// Extract the created network's ID
	id, err := getKey(resp)
	if err != nil {
		return nil, err
	}

	// Read back the created network
	return s.Get(ctx, id)
}

// Update updates a network and returns the updated network.
func (s *NetworkService) Update(ctx context.Context, id int, req *NetworkUpdateRequest) (*Network, error) {
	if req == nil {
		return nil, &ValidationError{Message: "update request is required"}
	}

	endpoint := fmt.Sprintf("/vnets/%d", id)
	if err := s.client.put(ctx, endpoint, req, nil); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "Network", ID: id}
		}
		return nil, err
	}

	// Read back the updated network
	return s.Get(ctx, id)
}

// Delete deletes a network.
func (s *NetworkService) Delete(ctx context.Context, id int) error {
	endpoint := fmt.Sprintf("/vnets/%d", id)
	if err := s.client.delete(ctx, endpoint); err != nil {
		if IsNotFoundError(err) {
			return nil // Already deleted
		}
		return err
	}
	return nil
}

// PowerOn powers on a network and waits for it to start.
func (s *NetworkService) PowerOn(ctx context.Context, id int) error {
	// Get current state
	network, err := s.Get(ctx, id)
	if err != nil {
		return err
	}

	// Already running
	if network.PowerState {
		return nil
	}

	// Power on is typically done by enabling the network
	enabled := true
	updateReq := &NetworkUpdateRequest{
		Enabled: &enabled,
	}

	_, err = s.Update(ctx, id, updateReq)
	if err != nil {
		return fmt.Errorf("vergeos: failed to power on network %d: %w", id, err)
	}

	// Wait for network to start
	return s.waitForPowerState(ctx, id, true)
}

// PowerOff powers off a network and waits for it to stop.
func (s *NetworkService) PowerOff(ctx context.Context, id int) error {
	// Get current state
	network, err := s.Get(ctx, id)
	if err != nil {
		return err
	}

	// Already stopped
	if !network.PowerState {
		return nil
	}

	// Send kill action
	action := vnetAction{
		VNet:   id,
		Action: networkActionKill,
		Params: struct{}{},
	}

	if err := s.client.post(ctx, "/vnet_actions", action, nil); err != nil {
		return fmt.Errorf("vergeos: failed to power off network %d: %w", id, err)
	}

	// Wait for network to stop
	return s.waitForPowerState(ctx, id, false)
}

// waitForPowerState waits for a network to reach the desired power state.
func (s *NetworkService) waitForPowerState(ctx context.Context, id int, desiredState bool) error {
	stateDesc := "stopped"
	if desiredState {
		stateDesc = "running"
	}

	for i := 0; i < networkPowerStateMaxRetries; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(networkPowerStatePollInterval):
		}

		network, err := s.Get(ctx, id)
		if err != nil {
			return err
		}

		if network.PowerState == desiredState {
			return nil
		}
	}

	return fmt.Errorf("vergeos: timeout waiting for network %d to become %s", id, stateDesc)
}

// Kill forcefully powers off a network (hard power off).
func (s *NetworkService) Kill(ctx context.Context, id int) error {
	action := vnetAction{
		VNet:   id,
		Action: networkActionKill,
		Params: struct{}{},
	}

	if err := s.client.post(ctx, "/vnet_actions", action, nil); err != nil {
		return fmt.Errorf("vergeos: failed to kill network %d: %w", id, err)
	}

	return nil
}

// Reset restarts a network.
func (s *NetworkService) Reset(ctx context.Context, id int, applyFirewall bool) error {
	params := struct {
		Apply bool `json:"apply"`
	}{
		Apply: applyFirewall,
	}

	action := vnetAction{
		VNet:   id,
		Action: networkActionReset,
		Params: params,
	}

	if err := s.client.post(ctx, "/vnet_actions", action, nil); err != nil {
		return fmt.Errorf("vergeos: failed to reset network %d: %w", id, err)
	}

	return nil
}

// ApplyRules applies firewall rules to a running network.
func (s *NetworkService) ApplyRules(ctx context.Context, id int) error {
	action := vnetAction{
		VNet:   id,
		Action: networkActionApply,
		Params: struct{}{},
	}

	if err := s.client.post(ctx, "/vnet_actions", action, nil); err != nil {
		return fmt.Errorf("vergeos: failed to apply rules to network %d: %w", id, err)
	}

	return nil
}

// ApplyDNS applies DNS configuration to a running network.
func (s *NetworkService) ApplyDNS(ctx context.Context, id int) error {
	action := vnetAction{
		VNet:   id,
		Action: networkActionApplyDNS,
		Params: struct{}{},
	}

	if err := s.client.post(ctx, "/vnet_actions", action, nil); err != nil {
		return fmt.Errorf("vergeos: failed to apply DNS to network %d: %w", id, err)
	}

	return nil
}

// RunQuery runs a diagnostic query on a network and returns the query job.
// The query runs asynchronously - use GetQuery to poll for results, or use
// RunQueryWait to wait for completion.
func (s *NetworkService) RunQuery(ctx context.Context, req *NetworkQueryRequest) (*NetworkQuery, error) {
	if req == nil {
		return nil, &ValidationError{Message: "query request is required"}
	}
	if req.VNet == 0 {
		return nil, &ValidationError{Field: "vnet", Message: "network ID is required"}
	}
	if req.Query == "" {
		return nil, &ValidationError{Field: "query", Message: "query type is required"}
	}

	var resp struct {
		ID string `json:"id"`
	}
	if err := s.client.post(ctx, "/vnet_queries", req, &resp); err != nil {
		return nil, err
	}

	// Read back the created query
	return s.GetQuery(ctx, resp.ID)
}

// GetQuery returns a network diagnostic query by ID.
func (s *NetworkService) GetQuery(ctx context.Context, id string) (*NetworkQuery, error) {
	params := url.Values{}
	params.Set("fields", networkQueryFields)

	var query NetworkQuery
	endpoint := fmt.Sprintf("/vnet_queries/%s", id)
	if err := s.client.get(ctx, endpoint, params, &query); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "NetworkQuery", ID: id}
		}
		return nil, err
	}

	return &query, nil
}

// RunQueryWait runs a diagnostic query and waits for it to complete.
// It polls the query status until it reaches "complete" or "error", or until
// the context is cancelled.
func (s *NetworkService) RunQueryWait(ctx context.Context, req *NetworkQueryRequest) (*NetworkQuery, error) {
	query, err := s.RunQuery(ctx, req)
	if err != nil {
		return nil, err
	}

	// Poll until complete or error
	for query.Status == NetworkQueryStatusRunning {
		select {
		case <-ctx.Done():
			return query, ctx.Err()
		case <-time.After(time.Second):
		}

		query, err = s.GetQuery(ctx, query.ID)
		if err != nil {
			return nil, err
		}
	}

	return query, nil
}

// Ping runs a ping diagnostic on a network.
// The target can be an IP address or hostname.
func (s *NetworkService) Ping(ctx context.Context, networkID int, target string, count int) (*NetworkQuery, error) {
	if count <= 0 {
		count = 4
	}
	params := map[string]interface{}{
		"address": target,
		"count":   count,
	}
	return s.RunQueryWait(ctx, &NetworkQueryRequest{
		VNet:   networkID,
		Query:  NetworkQueryPing,
		Params: params,
	})
}

// Traceroute runs a traceroute diagnostic on a network.
func (s *NetworkService) Traceroute(ctx context.Context, networkID int, target string) (*NetworkQuery, error) {
	params := map[string]interface{}{
		"address": target,
	}
	return s.RunQueryWait(ctx, &NetworkQueryRequest{
		VNet:   networkID,
		Query:  NetworkQueryTraceroute,
		Params: params,
	})
}

// DNSLookup runs a DNS lookup diagnostic on a network.
func (s *NetworkService) DNSLookup(ctx context.Context, networkID int, hostname string) (*NetworkQuery, error) {
	params := map[string]interface{}{
		"hostname": hostname,
	}
	return s.RunQueryWait(ctx, &NetworkQueryRequest{
		VNet:   networkID,
		Query:  NetworkQueryDNS,
		Params: params,
	})
}

// GetDiagnostics is a convenience method that returns basic diagnostic info for a network.
// It runs a "whatsmyip" query to verify network connectivity.
func (s *NetworkService) GetDiagnostics(ctx context.Context, id int) (*NetworkQuery, error) {
	return s.RunQueryWait(ctx, &NetworkQueryRequest{
		VNet:  id,
		Query: NetworkQueryWhatsMyIP,
	})
}

// GetStatistics returns the latest monitoring statistics for a network.
// Requires gateway monitoring to be enabled on the network.
func (s *NetworkService) GetStatistics(ctx context.Context, id int) ([]NetworkMonitorStats, error) {
	params := url.Values{}
	params.Set("fields", networkMonitorStatsFields)
	params.Set("filter", fmt.Sprintf("vnet eq %d", id))
	params.Set("sort", "-timestamp")
	params.Set("limit", "100")

	var stats []NetworkMonitorStats
	if err := s.client.get(ctx, "/vnet_monitor_stats_history_short", params, &stats); err != nil {
		return nil, err
	}

	return stats, nil
}

// GetLatestStatistics returns the most recent monitoring statistics for a network.
// Returns nil if no statistics are available.
func (s *NetworkService) GetLatestStatistics(ctx context.Context, id int) (*NetworkMonitorStats, error) {
	stats, err := s.GetStatistics(ctx, id)
	if err != nil {
		return nil, err
	}
	if len(stats) == 0 {
		return nil, nil
	}
	return &stats[0], nil
}
