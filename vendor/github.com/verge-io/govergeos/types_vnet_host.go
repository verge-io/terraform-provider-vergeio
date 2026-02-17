package vergeos

// VNetHost represents a VergeOS network DNS/DHCP host override.
// Host overrides provide static hostname-to-IP mappings for DNS and DHCP reservations.
type VNetHost struct {
	// Key is the unique identifier for the host.
	Key FlexInt `json:"$key,omitempty"`
	// VNet is the parent network ID.
	VNet FlexInt `json:"vnet,omitempty"`
	// Type is the override type (host, domain).
	Type string `json:"type,omitempty"`
	// Host is the hostname or domain name.
	Host string `json:"host"`
	// IP is the IP address to resolve to.
	IP string `json:"ip"`
}

// VNetHostCreateRequest is the request body for creating a host override.
type VNetHostCreateRequest struct {
	// VNet is the parent network ID (required).
	VNet int `json:"vnet"`
	// Type is the override type (host, domain). Default: host.
	Type *string `json:"type,omitempty"`
	// Host is the hostname or domain name (required).
	Host string `json:"host"`
	// IP is the IP address (required).
	IP string `json:"ip"`
}

// VNetHostUpdateRequest is the request body for updating a host override.
type VNetHostUpdateRequest struct {
	// Type is the override type.
	Type *string `json:"type,omitempty"`
	// Host is the hostname or domain name.
	Host *string `json:"host,omitempty"`
	// IP is the IP address.
	IP *string `json:"ip,omitempty"`
}

// Host override type constants
const (
	// HostTypeHost indicates a single hostname override.
	HostTypeHost = "host"
	// HostTypeDomain indicates a domain-wide override.
	HostTypeDomain = "domain"
)

// vnetHostListFields are the fields to request when listing hosts.
const vnetHostListFields = "$key,vnet,type,host,ip"

// vnetHostGetFields are the fields to request when getting a single host.
const vnetHostGetFields = vnetHostListFields
