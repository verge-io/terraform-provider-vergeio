package vergeos

import (
	"context"
	"fmt"
	"net/url"
)

// TenantService handles tenant operations.
type TenantService struct {
	client *Client
}

// List returns all tenants, with optional filtering and pagination.
func (s *TenantService) List(ctx context.Context, opts ...ListOption) ([]Tenant, error) {
	options := applyListOptions(opts)

	// Use tenant-specific fields if not specified
	if options.Fields == "most" {
		options.Fields = tenantListFields
	}

	params := options.toQueryParams()

	var tenants []Tenant
	if err := s.client.get(ctx, "/tenants", params, &tenants); err != nil {
		return nil, err
	}

	return tenants, nil
}

// Get returns a single tenant by ID.
func (s *TenantService) Get(ctx context.Context, id int) (*Tenant, error) {
	params := url.Values{}
	params.Set("fields", tenantGetFields)

	var tenant Tenant
	endpoint := fmt.Sprintf("/tenants/%d", id)
	if err := s.client.get(ctx, endpoint, params, &tenant); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "Tenant", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	return &tenant, nil
}

// GetByName returns a tenant by name.
func (s *TenantService) GetByName(ctx context.Context, name string) (*Tenant, error) {
	tenants, err := s.List(ctx, WithFilter(fmt.Sprintf("name eq '%s'", name)))
	if err != nil {
		return nil, err
	}
	if len(tenants) == 0 {
		return nil, &NotFoundError{Resource: "Tenant", ID: name}
	}
	// Get full details
	return s.Get(ctx, int(tenants[0].Key))
}

// Create creates a new tenant and returns the created tenant.
func (s *TenantService) Create(ctx context.Context, req *TenantCreateRequest) (*Tenant, error) {
	if req == nil {
		return nil, &ValidationError{Message: "create request is required"}
	}
	if req.Name == "" {
		return nil, &ValidationError{Field: "name", Message: "name is required"}
	}

	var resp apiResponse
	if err := s.client.post(ctx, "/tenants", req, &resp); err != nil {
		return nil, err
	}

	// Extract the created tenant's ID
	id, err := getKey(resp)
	if err != nil {
		return nil, err
	}

	// Read back the created tenant
	return s.Get(ctx, id)
}

// Update updates a tenant and returns the updated tenant.
func (s *TenantService) Update(ctx context.Context, id int, req *TenantUpdateRequest) (*Tenant, error) {
	if req == nil {
		return nil, &ValidationError{Message: "update request is required"}
	}

	endpoint := fmt.Sprintf("/tenants/%d", id)
	if err := s.client.put(ctx, endpoint, req, nil); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "Tenant", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	// Read back the updated tenant
	return s.Get(ctx, id)
}

// Delete deletes a tenant.
func (s *TenantService) Delete(ctx context.Context, id int) error {
	endpoint := fmt.Sprintf("/tenants/%d", id)
	if err := s.client.delete(ctx, endpoint); err != nil {
		if IsNotFoundError(err) {
			return &NotFoundError{Resource: "Tenant", ID: fmt.Sprintf("%d", id)}
		}
		return err
	}
	return nil
}

// PowerOn powers on a tenant.
func (s *TenantService) PowerOn(ctx context.Context, id int) error {
	return s.PowerOnWithNode(ctx, id, 0)
}

// PowerOnWithNode powers on a tenant on a specific preferred node.
// If preferredNode is 0, the system chooses the node.
func (s *TenantService) PowerOnWithNode(ctx context.Context, id int, preferredNode int) error {
	action := tenantAction{
		Tenant: id,
		Action: "poweron",
	}
	if preferredNode > 0 {
		action.Params = map[string]interface{}{
			"preferred_node": preferredNode,
		}
	}

	if err := s.client.post(ctx, "/tenant_actions", action, nil); err != nil {
		return fmt.Errorf("vergeos: failed to power on tenant %d: %w", id, err)
	}
	return nil
}

// PowerOff powers off a tenant.
func (s *TenantService) PowerOff(ctx context.Context, id int) error {
	action := tenantAction{
		Tenant: id,
		Action: "poweroff",
	}

	if err := s.client.post(ctx, "/tenant_actions", action, nil); err != nil {
		return fmt.Errorf("vergeos: failed to power off tenant %d: %w", id, err)
	}
	return nil
}

// Reset resets a tenant (restart).
func (s *TenantService) Reset(ctx context.Context, id int) error {
	action := tenantAction{
		Tenant: id,
		Action: "reset",
	}

	if err := s.client.post(ctx, "/tenant_actions", action, nil); err != nil {
		return fmt.Errorf("vergeos: failed to reset tenant %d: %w", id, err)
	}
	return nil
}

// Clone clones a tenant.
func (s *TenantService) Clone(ctx context.Context, id int, opts *TenantCloneOptions) error {
	if opts == nil {
		return &ValidationError{Message: "clone options are required"}
	}
	if opts.Name == "" {
		return &ValidationError{Field: "name", Message: "name is required for clone"}
	}

	action := tenantAction{
		Tenant: id,
		Action: "clone",
		Params: map[string]interface{}{
			"name":       opts.Name,
			"no_vnet":    opts.NoVNet,
			"no_storage": opts.NoStorage,
			"no_nodes":   opts.NoNodes,
		},
	}

	if err := s.client.post(ctx, "/tenant_actions", action, nil); err != nil {
		return fmt.Errorf("vergeos: failed to clone tenant %d: %w", id, err)
	}
	return nil
}

// IsolateOn enables network isolation for a tenant.
func (s *TenantService) IsolateOn(ctx context.Context, id int) error {
	action := tenantAction{
		Tenant: id,
		Action: "isolateon",
	}

	if err := s.client.post(ctx, "/tenant_actions", action, nil); err != nil {
		return fmt.Errorf("vergeos: failed to isolate tenant %d: %w", id, err)
	}
	return nil
}

// IsolateOff disables network isolation for a tenant.
func (s *TenantService) IsolateOff(ctx context.Context, id int) error {
	action := tenantAction{
		Tenant: id,
		Action: "isolateoff",
	}

	if err := s.client.post(ctx, "/tenant_actions", action, nil); err != nil {
		return fmt.Errorf("vergeos: failed to un-isolate tenant %d: %w", id, err)
	}
	return nil
}

// tenantAction represents a tenant action request.
type tenantAction struct {
	Tenant int                    `json:"tenant"`
	Action string                 `json:"action"`
	Params map[string]interface{} `json:"params,omitempty"`
}

// TenantNodeService handles tenant node operations.
type TenantNodeService struct {
	client *Client
}

// List returns all tenant nodes, with optional filtering and pagination.
func (s *TenantNodeService) List(ctx context.Context, opts ...ListOption) ([]TenantNode, error) {
	options := applyListOptions(opts)

	// Use node-specific fields if not specified
	if options.Fields == "most" {
		options.Fields = tenantNodeListFields
	}

	params := options.toQueryParams()

	var nodes []TenantNode
	if err := s.client.get(ctx, "/tenant_nodes", params, &nodes); err != nil {
		return nil, err
	}

	return nodes, nil
}

// ListByTenant returns all nodes for a specific tenant.
func (s *TenantNodeService) ListByTenant(ctx context.Context, tenantID int, opts ...ListOption) ([]TenantNode, error) {
	// Prepend tenant filter to any existing filters
	filterOpts := []ListOption{WithFilter(fmt.Sprintf("tenant eq %d", tenantID))}
	filterOpts = append(filterOpts, opts...)
	return s.List(ctx, filterOpts...)
}

// Get returns a single tenant node by ID.
func (s *TenantNodeService) Get(ctx context.Context, id int) (*TenantNode, error) {
	params := url.Values{}
	params.Set("fields", tenantNodeGetFields)

	var node TenantNode
	endpoint := fmt.Sprintf("/tenant_nodes/%d", id)
	if err := s.client.get(ctx, endpoint, params, &node); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "TenantNode", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	return &node, nil
}

// GetByName returns a tenant node by name within a specific tenant.
func (s *TenantNodeService) GetByName(ctx context.Context, tenantID int, name string) (*TenantNode, error) {
	nodes, err := s.List(ctx, WithFilter(fmt.Sprintf("tenant eq %d and name eq '%s'", tenantID, name)))
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, &NotFoundError{Resource: "TenantNode", ID: name}
	}
	// Get full details
	return s.Get(ctx, int(nodes[0].Key))
}

// Create creates a new tenant node and returns the created node.
func (s *TenantNodeService) Create(ctx context.Context, req *TenantNodeCreateRequest) (*TenantNode, error) {
	if req == nil {
		return nil, &ValidationError{Message: "create request is required"}
	}
	if req.Tenant <= 0 {
		return nil, &ValidationError{Field: "tenant", Message: "tenant is required"}
	}
	if req.CPUCores <= 0 {
		return nil, &ValidationError{Field: "cpu_cores", Message: "cpu_cores is required"}
	}
	if req.RAM < 2048 {
		return nil, &ValidationError{Field: "ram", Message: "ram must be at least 2048 MB"}
	}

	var resp apiResponse
	if err := s.client.post(ctx, "/tenant_nodes", req, &resp); err != nil {
		return nil, err
	}

	// Extract the created node's ID
	id, err := getKey(resp)
	if err != nil {
		return nil, err
	}

	// Read back the created node
	return s.Get(ctx, id)
}

// Update updates a tenant node and returns the updated node.
func (s *TenantNodeService) Update(ctx context.Context, id int, req *TenantNodeUpdateRequest) (*TenantNode, error) {
	if req == nil {
		return nil, &ValidationError{Message: "update request is required"}
	}

	endpoint := fmt.Sprintf("/tenant_nodes/%d", id)
	if err := s.client.put(ctx, endpoint, req, nil); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "TenantNode", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	// Read back the updated node
	return s.Get(ctx, id)
}

// Delete deletes a tenant node.
func (s *TenantNodeService) Delete(ctx context.Context, id int) error {
	endpoint := fmt.Sprintf("/tenant_nodes/%d", id)
	if err := s.client.delete(ctx, endpoint); err != nil {
		if IsNotFoundError(err) {
			return &NotFoundError{Resource: "TenantNode", ID: fmt.Sprintf("%d", id)}
		}
		return err
	}
	return nil
}

// PowerOn powers on a tenant node.
func (s *TenantNodeService) PowerOn(ctx context.Context, id int) error {
	action := tenantNodeAction{
		TenantNode: id,
		Action:     "poweron",
	}

	if err := s.client.post(ctx, "/tenant_node_actions", action, nil); err != nil {
		return fmt.Errorf("vergeos: failed to power on tenant node %d: %w", id, err)
	}
	return nil
}

// PowerOff powers off a tenant node.
func (s *TenantNodeService) PowerOff(ctx context.Context, id int) error {
	action := tenantNodeAction{
		TenantNode: id,
		Action:     "poweroff",
	}

	if err := s.client.post(ctx, "/tenant_node_actions", action, nil); err != nil {
		return fmt.Errorf("vergeos: failed to power off tenant node %d: %w", id, err)
	}
	return nil
}

// Reset resets a tenant node.
func (s *TenantNodeService) Reset(ctx context.Context, id int) error {
	action := tenantNodeAction{
		TenantNode: id,
		Action:     "reset",
	}

	if err := s.client.post(ctx, "/tenant_node_actions", action, nil); err != nil {
		return fmt.Errorf("vergeos: failed to reset tenant node %d: %w", id, err)
	}
	return nil
}

// Kill forcefully terminates a tenant node.
func (s *TenantNodeService) Kill(ctx context.Context, id int) error {
	action := tenantNodeAction{
		TenantNode: id,
		Action:     "kill",
	}

	if err := s.client.post(ctx, "/tenant_node_actions", action, nil); err != nil {
		return fmt.Errorf("vergeos: failed to kill tenant node %d: %w", id, err)
	}
	return nil
}

// Migrate migrates a tenant node to another host.
func (s *TenantNodeService) Migrate(ctx context.Context, id int, targetNode int) error {
	action := tenantNodeAction{
		TenantNode: id,
		Action:     "migrate",
	}
	if targetNode > 0 {
		action.Params = map[string]interface{}{
			"node": targetNode,
		}
	}

	if err := s.client.post(ctx, "/tenant_node_actions", action, nil); err != nil {
		return fmt.Errorf("vergeos: failed to migrate tenant node %d: %w", id, err)
	}
	return nil
}

// tenantNodeAction represents a tenant node action request.
type tenantNodeAction struct {
	TenantNode int                    `json:"tenant_node"`
	Action     string                 `json:"action"`
	Params     map[string]interface{} `json:"params,omitempty"`
}

// TenantStorageService handles tenant storage allocation operations.
type TenantStorageService struct {
	client *Client
}

// List returns all tenant storage allocations, with optional filtering and pagination.
func (s *TenantStorageService) List(ctx context.Context, opts ...ListOption) ([]TenantStorage, error) {
	options := applyListOptions(opts)

	// Use storage-specific fields if not specified
	if options.Fields == "most" {
		options.Fields = tenantStorageListFields
	}

	params := options.toQueryParams()

	var storage []TenantStorage
	if err := s.client.get(ctx, "/tenant_storage", params, &storage); err != nil {
		return nil, err
	}

	return storage, nil
}

// ListByTenant returns all storage allocations for a specific tenant.
func (s *TenantStorageService) ListByTenant(ctx context.Context, tenantID int, opts ...ListOption) ([]TenantStorage, error) {
	// Prepend tenant filter to any existing filters
	filterOpts := []ListOption{WithFilter(fmt.Sprintf("tenant eq %d", tenantID))}
	filterOpts = append(filterOpts, opts...)
	return s.List(ctx, filterOpts...)
}

// Get returns a single tenant storage allocation by ID.
func (s *TenantStorageService) Get(ctx context.Context, id int) (*TenantStorage, error) {
	params := url.Values{}
	params.Set("fields", tenantStorageGetFields)

	var storage TenantStorage
	endpoint := fmt.Sprintf("/tenant_storage/%d", id)
	if err := s.client.get(ctx, endpoint, params, &storage); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "TenantStorage", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	return &storage, nil
}

// Create creates a new tenant storage allocation and returns the created allocation.
func (s *TenantStorageService) Create(ctx context.Context, req *TenantStorageCreateRequest) (*TenantStorage, error) {
	if req == nil {
		return nil, &ValidationError{Message: "create request is required"}
	}
	if req.Tenant <= 0 {
		return nil, &ValidationError{Field: "tenant", Message: "tenant is required"}
	}
	if req.Tier <= 0 {
		return nil, &ValidationError{Field: "tier", Message: "tier is required"}
	}
	if req.Provisioned <= 0 {
		return nil, &ValidationError{Field: "provisioned", Message: "provisioned storage is required"}
	}

	var resp apiResponse
	if err := s.client.post(ctx, "/tenant_storage", req, &resp); err != nil {
		return nil, err
	}

	// Extract the created storage's ID
	id, err := getKey(resp)
	if err != nil {
		return nil, err
	}

	// Read back the created storage
	return s.Get(ctx, id)
}

// Update updates a tenant storage allocation and returns the updated allocation.
func (s *TenantStorageService) Update(ctx context.Context, id int, req *TenantStorageUpdateRequest) (*TenantStorage, error) {
	if req == nil {
		return nil, &ValidationError{Message: "update request is required"}
	}

	endpoint := fmt.Sprintf("/tenant_storage/%d", id)
	if err := s.client.put(ctx, endpoint, req, nil); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "TenantStorage", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	// Read back the updated storage
	return s.Get(ctx, id)
}

// Delete deletes a tenant storage allocation.
func (s *TenantStorageService) Delete(ctx context.Context, id int) error {
	endpoint := fmt.Sprintf("/tenant_storage/%d", id)
	if err := s.client.delete(ctx, endpoint); err != nil {
		if IsNotFoundError(err) {
			return &NotFoundError{Resource: "TenantStorage", ID: fmt.Sprintf("%d", id)}
		}
		return err
	}
	return nil
}
