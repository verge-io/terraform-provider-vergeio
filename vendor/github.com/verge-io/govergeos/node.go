package vergeos

import (
	"context"
	"fmt"
	"net/url"
)

const (
	// Node action constants
	nodeActionEnableMaintenance  = "enable_maintenance"
	nodeActionDisableMaintenance = "disable_maintenance"
	nodeActionMaintenanceReboot  = "maintenance_reboot"
	nodeActionClearPStore        = "clear_pstore"
)

// nodeAction represents a node action request.
type nodeAction struct {
	Node   int         `json:"node"`
	Action string      `json:"action"`
	Params interface{} `json:"params"`
}

// NodeService handles node read operations.
type NodeService struct {
	client *Client
}

// List returns all nodes, with optional filtering and pagination.
func (s *NodeService) List(ctx context.Context, opts ...ListOption) ([]Node, error) {
	options := applyListOptions(opts)

	if options.Fields == "most" {
		options.Fields = nodeListFields
	}

	params := options.toQueryParams()

	var nodes []Node
	if err := s.client.get(ctx, "/nodes", params, &nodes); err != nil {
		return nil, err
	}

	return nodes, nil
}

// ListPhysical returns all physical nodes.
func (s *NodeService) ListPhysical(ctx context.Context, opts ...ListOption) ([]Node, error) {
	// Prepend physical filter
	opts = append([]ListOption{WithFilter("physical eq true")}, opts...)
	return s.List(ctx, opts...)
}

// Get returns a single node by ID.
func (s *NodeService) Get(ctx context.Context, id int) (*Node, error) {
	params := url.Values{}
	params.Set("fields", nodeListFields)

	var node Node
	endpoint := fmt.Sprintf("/nodes/%d", id)
	if err := s.client.get(ctx, endpoint, params, &node); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "Node", ID: id}
		}
		return nil, err
	}

	return &node, nil
}

// GetByName returns a node by name.
func (s *NodeService) GetByName(ctx context.Context, name string) (*Node, error) {
	nodes, err := s.List(ctx, WithFilter(fmt.Sprintf("name eq '%s'", name)))
	if err != nil {
		return nil, err
	}

	if len(nodes) == 0 {
		return nil, &NotFoundError{Resource: "Node", ID: name}
	}

	return &nodes[0], nil
}

// GetDashboard returns detailed dashboard information for a node.
func (s *NodeService) GetDashboard(ctx context.Context, id int) (*Node, error) {
	params := url.Values{}
	params.Set("fields", "dashboard")

	var node Node
	endpoint := fmt.Sprintf("/nodes/%d", id)
	if err := s.client.get(ctx, endpoint, params, &node); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "Node", ID: id}
		}
		return nil, err
	}

	return &node, nil
}

// EnableMaintenance puts a node into maintenance mode.
// In maintenance mode, VMs are migrated off the node and no new VMs will be scheduled to it.
func (s *NodeService) EnableMaintenance(ctx context.Context, id int) error {
	action := nodeAction{
		Node:   id,
		Action: nodeActionEnableMaintenance,
		Params: struct{}{},
	}

	if err := s.client.post(ctx, "/node_actions", action, nil); err != nil {
		return fmt.Errorf("vergeos: failed to enable maintenance for node %d: %w", id, err)
	}

	return nil
}

// DisableMaintenance takes a node out of maintenance mode.
func (s *NodeService) DisableMaintenance(ctx context.Context, id int) error {
	action := nodeAction{
		Node:   id,
		Action: nodeActionDisableMaintenance,
		Params: struct{}{},
	}

	if err := s.client.post(ctx, "/node_actions", action, nil); err != nil {
		return fmt.Errorf("vergeos: failed to disable maintenance for node %d: %w", id, err)
	}

	return nil
}

// MaintenanceReboot reboots a node while in maintenance mode.
// The node must already be in maintenance mode.
func (s *NodeService) MaintenanceReboot(ctx context.Context, id int) error {
	action := nodeAction{
		Node:   id,
		Action: nodeActionMaintenanceReboot,
		Params: struct{}{},
	}

	if err := s.client.post(ctx, "/node_actions", action, nil); err != nil {
		return fmt.Errorf("vergeos: failed to reboot node %d: %w", id, err)
	}

	return nil
}

// ClearPStore clears the persistent storage on a node.
func (s *NodeService) ClearPStore(ctx context.Context, id int) error {
	action := nodeAction{
		Node:   id,
		Action: nodeActionClearPStore,
		Params: struct{}{},
	}

	if err := s.client.post(ctx, "/node_actions", action, nil); err != nil {
		return fmt.Errorf("vergeos: failed to clear pstore for node %d: %w", id, err)
	}

	return nil
}
