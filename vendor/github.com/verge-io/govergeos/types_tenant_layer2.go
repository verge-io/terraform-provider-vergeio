package vergeos

// TenantLayer2Network represents a Layer 2 network exposed to a tenant.
// This allows tenants to access host networks at Layer 2 for direct connectivity.
type TenantLayer2Network struct {
	// Key is the unique identifier for this tenant layer2 network assignment.
	Key FlexInt `json:"$key,omitempty"`
	// Tenant is the tenant ID this network is assigned to.
	Tenant FlexInt `json:"tenant,omitempty"`
	// VNet is the network ID being assigned.
	VNet FlexInt `json:"vnet,omitempty"`
	// Enabled indicates whether this assignment is active.
	Enabled bool `json:"enabled,omitempty"`
}

// TenantLayer2NetworkCreateRequest represents the request body for creating a tenant layer2 network assignment.
type TenantLayer2NetworkCreateRequest struct {
	// Tenant is the tenant ID (required).
	Tenant int `json:"tenant"`
	// VNet is the network ID to assign (required).
	VNet int `json:"vnet"`
	// Enabled indicates whether the assignment is active (default: true).
	Enabled *bool `json:"enabled,omitempty"`
}

// TenantLayer2NetworkUpdateRequest represents the request body for updating a tenant layer2 network assignment.
type TenantLayer2NetworkUpdateRequest struct {
	// Enabled indicates whether the assignment is active.
	Enabled *bool `json:"enabled,omitempty"`
}

// tenantLayer2NetworkListFields defines the default fields for listing tenant layer2 networks.
const tenantLayer2NetworkListFields = "$key,tenant,vnet,enabled"

// tenantLayer2NetworkGetFields defines the default fields for getting a single tenant layer2 network.
const tenantLayer2NetworkGetFields = tenantLayer2NetworkListFields
