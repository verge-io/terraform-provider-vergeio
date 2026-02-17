package vergeos

// VNetAddress represents a VergeOS network IP address.
// Addresses can be dynamic (DHCP), static, IP aliases, proxy ARP, or virtual IPs.
type VNetAddress struct {
	// Key is the unique identifier for the address.
	Key FlexInt `json:"$key,omitempty"`
	// VNet is the parent network ID.
	VNet FlexInt `json:"vnet,omitempty"`
	// IP is the IP address (e.g., "192.168.1.100").
	IP string `json:"ip,omitempty"`
	// MAC is the MAC address associated with this IP.
	MAC string `json:"mac,omitempty"`
	// Type indicates the address type (dynamic, static, ipalias, proxy, virtual).
	Type string `json:"type,omitempty"`
	// Hostname is the hostname associated with this IP.
	Hostname string `json:"hostname,omitempty"`
	// Expiration is the lease expiration timestamp for dynamic addresses (Unix epoch).
	Expiration int64 `json:"expiration,omitempty"`
	// Owner is the owner reference path (e.g., "machine_nics/123") (readonly).
	Owner string `json:"owner,omitempty"`
	// Vendor is the NIC vendor name derived from the MAC address (readonly).
	Vendor string `json:"vendor,omitempty"`
	// Description is an optional description for the address.
	Description string `json:"description,omitempty"`
}

// VNetAddressCreateRequest is the request body for creating a network address.
type VNetAddressCreateRequest struct {
	// VNet is the parent network ID (required).
	VNet int `json:"vnet"`
	// IP is the IP address (required for static addresses, auto-assigned for dynamic).
	IP string `json:"ip,omitempty"`
	// MAC is the MAC address (optional).
	MAC string `json:"mac,omitempty"`
	// Type indicates the address type (required: dynamic, static, ipalias, proxy, virtual).
	Type string `json:"type"`
	// Hostname is the hostname (optional).
	Hostname string `json:"hostname,omitempty"`
	// Description is an optional description.
	Description string `json:"description,omitempty"`
}

// VNetAddressUpdateRequest is the request body for updating a network address.
type VNetAddressUpdateRequest struct {
	// IP is the IP address.
	IP *string `json:"ip,omitempty"`
	// MAC is the MAC address.
	MAC *string `json:"mac,omitempty"`
	// Type indicates the address type.
	Type *string `json:"type,omitempty"`
	// Hostname is the hostname.
	Hostname *string `json:"hostname,omitempty"`
	// Description is the description.
	Description *string `json:"description,omitempty"`
}

// Address type constants
const (
	// AddressTypeDynamic indicates a DHCP-assigned address.
	AddressTypeDynamic = "dynamic"
	// AddressTypeStatic indicates a statically assigned address.
	AddressTypeStatic = "static"
	// AddressTypeIPAlias indicates an IP alias.
	AddressTypeIPAlias = "ipalias"
	// AddressTypeProxy indicates a proxy ARP address.
	AddressTypeProxy = "proxy"
	// AddressTypeVirtual indicates a virtual IP address.
	AddressTypeVirtual = "virtual"
)

// vnetAddressListFields are the fields to request when listing addresses.
const vnetAddressListFields = "$key,vnet,ip,mac,type,hostname,expiration,owner,vendor,description"

// vnetAddressGetFields are the fields to request when getting a single address.
const vnetAddressGetFields = vnetAddressListFields
