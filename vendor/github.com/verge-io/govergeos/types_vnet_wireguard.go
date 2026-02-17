package vergeos

// VNetWireGuard represents a VergeOS WireGuard VPN interface on a network.
type VNetWireGuard struct {
	// Key is the unique identifier for the WireGuard interface.
	Key FlexInt `json:"$key,omitempty"`
	// VNet is the parent network ID.
	VNet FlexInt `json:"vnet,omitempty"`
	// Name is the interface name.
	Name string `json:"name"`
	// Description is an optional description.
	Description string `json:"description,omitempty"`
	// Enabled indicates if the interface is active.
	Enabled bool `json:"enabled,omitempty"`
	// IP is the WireGuard interface IP address with CIDR (e.g., "192.168.255.1/24").
	IP string `json:"ip,omitempty"`
	// ListenPort is the UDP port for WireGuard (default: 51820).
	ListenPort int `json:"listenport,omitempty"`
	// MTU is the interface MTU (0 = auto-configure).
	MTU int `json:"mtu,omitempty"`
	// PublicKey is the WireGuard public key (base64, read-only).
	PublicKey string `json:"public_key,omitempty"`
	// PrivateKey is the WireGuard private key (base64, hidden in responses).
	PrivateKey string `json:"private_key,omitempty"`
	// EndpointIP is the public-facing endpoint IP for peer configurations.
	EndpointIP string `json:"endpoint_ip,omitempty"`
	// NIC is the underlying machine NIC ID (read-only).
	NIC FlexInt `json:"nic,omitempty"`
	// Modified is the last modification timestamp (Unix epoch).
	Modified int64 `json:"modified,omitempty"`
}

// VNetWireGuardCreateRequest is the request body for creating a WireGuard interface.
type VNetWireGuardCreateRequest struct {
	// VNet is the parent network ID (required).
	VNet int `json:"vnet"`
	// Name is the interface name (required, max 128 chars).
	Name string `json:"name"`
	// Description is an optional description (max 2048 chars).
	Description string `json:"description,omitempty"`
	// Enabled indicates if the interface is active (default: true).
	Enabled *bool `json:"enabled,omitempty"`
	// IP is the WireGuard interface IP address with CIDR (required, e.g., "192.168.255.1/24").
	IP string `json:"ip"`
	// ListenPort is the UDP port for WireGuard (default: 51820).
	ListenPort *int `json:"listenport,omitempty"`
	// MTU is the interface MTU (0 = auto-configure).
	MTU *int `json:"mtu,omitempty"`
	// PrivateKey is the WireGuard private key (base64, leave blank to auto-generate).
	PrivateKey string `json:"private_key,omitempty"`
	// EndpointIP is the public-facing endpoint IP (auto-detected if blank).
	EndpointIP string `json:"endpoint_ip,omitempty"`
	// ConfigureFirewall creates a PAT rule on the external network.
	ConfigureFirewall *bool `json:"configure_firewall,omitempty"`
	// ExternalIP is the external IP address ID for firewall configuration.
	ExternalIP *int `json:"external_ip,omitempty"`
	// AutoApplyFirewall automatically applies firewall rules.
	AutoApplyFirewall *bool `json:"auto_apply_firewall,omitempty"`
}

// VNetWireGuardUpdateRequest is the request body for updating a WireGuard interface.
type VNetWireGuardUpdateRequest struct {
	// Name is the interface name.
	Name *string `json:"name,omitempty"`
	// Description is the interface description.
	Description *string `json:"description,omitempty"`
	// Enabled indicates if the interface is active.
	Enabled *bool `json:"enabled,omitempty"`
	// IP is the WireGuard interface IP address with CIDR.
	IP *string `json:"ip,omitempty"`
	// ListenPort is the UDP port for WireGuard.
	ListenPort *int `json:"listenport,omitempty"`
	// MTU is the interface MTU.
	MTU *int `json:"mtu,omitempty"`
	// PrivateKey is the WireGuard private key.
	PrivateKey *string `json:"private_key,omitempty"`
	// EndpointIP is the public-facing endpoint IP.
	EndpointIP *string `json:"endpoint_ip,omitempty"`
}

// vnetWireGuardListFields are the fields to request when listing WireGuard interfaces.
const vnetWireGuardListFields = "$key,vnet,name,description,enabled,ip,listenport,mtu,public_key,endpoint_ip,nic,modified"

// vnetWireGuardGetFields are the fields to request when getting a single WireGuard interface.
const vnetWireGuardGetFields = vnetWireGuardListFields

// VNetWireGuardPeer represents a WireGuard peer connection.
type VNetWireGuardPeer struct {
	// Key is the unique identifier for the peer.
	Key FlexInt `json:"$key,omitempty"`
	// WireGuard is the parent WireGuard interface ID.
	WireGuard FlexInt `json:"wireguard,omitempty"`
	// Name is the peer name.
	Name string `json:"name"`
	// Description is an optional description.
	Description string `json:"description,omitempty"`
	// Enabled indicates if the peer is active.
	Enabled bool `json:"enabled,omitempty"`
	// Endpoint is the peer's external IP or hostname (blank for roaming clients).
	Endpoint string `json:"endpoint,omitempty"`
	// Port is the peer's WireGuard port (default: 51820).
	Port int `json:"port,omitempty"`
	// PeerIP is the IP address for routing between endpoints.
	PeerIP string `json:"peer_ip,omitempty"`
	// PublicKey is the peer's WireGuard public key (base64, required).
	PublicKey string `json:"public_key,omitempty"`
	// PrivateKey is the peer's WireGuard private key (hidden).
	PrivateKey string `json:"private_key,omitempty"`
	// PresharedKey is an optional preshared key for post-quantum security.
	PresharedKey string `json:"preshared_key,omitempty"`
	// AllowedIPs is the list of allowed IP ranges (e.g., "10.1.2.0/24,192.168.0.0/16").
	AllowedIPs string `json:"allowed_ips,omitempty"`
	// ConfigureFirewall defines automatic firewall rule creation.
	// Options: "site-to-site", "remote-user", "none"
	ConfigureFirewall string `json:"configure_firewall,omitempty"`
	// Keepalive is the persistent keepalive interval in seconds (0 = disabled).
	Keepalive int `json:"keepalive,omitempty"`
	// AutogeneratePeer enables downloadable peer configuration generation.
	AutogeneratePeer bool `json:"autogenerate_peer,omitempty"`
	// Status is the peer status reference (read-only).
	Status FlexInt `json:"status,omitempty"`
	// Modified is the last modification timestamp (Unix epoch).
	Modified int64 `json:"modified,omitempty"`
}

// VNetWireGuardPeerCreateRequest is the request body for creating a WireGuard peer.
type VNetWireGuardPeerCreateRequest struct {
	// WireGuard is the parent WireGuard interface ID (required).
	WireGuard int `json:"wireguard"`
	// Name is the peer name (required, max 128 chars).
	Name string `json:"name"`
	// Description is an optional description (max 2048 chars).
	Description string `json:"description,omitempty"`
	// Enabled indicates if the peer is active (default: true).
	Enabled *bool `json:"enabled,omitempty"`
	// Endpoint is the peer's external IP or hostname.
	Endpoint string `json:"endpoint,omitempty"`
	// Port is the peer's WireGuard port (default: 51820).
	Port *int `json:"port,omitempty"`
	// PeerIP is the IP address for routing between endpoints (required).
	PeerIP string `json:"peer_ip"`
	// PublicKey is the peer's WireGuard public key (required).
	PublicKey string `json:"public_key"`
	// PresharedKey is an optional preshared key for post-quantum security.
	PresharedKey string `json:"preshared_key,omitempty"`
	// AllowedIPs is the list of allowed IP ranges (required).
	AllowedIPs string `json:"allowed_ips"`
	// ConfigureFirewall defines automatic firewall rule creation.
	// Options: "site-to-site" (default), "remote-user", "none"
	ConfigureFirewall *string `json:"configure_firewall,omitempty"`
	// Keepalive is the persistent keepalive interval in seconds.
	Keepalive *int `json:"keepalive,omitempty"`
	// AutogeneratePeer enables downloadable peer configuration generation.
	AutogeneratePeer *bool `json:"autogenerate_peer,omitempty"`
}

// VNetWireGuardPeerUpdateRequest is the request body for updating a WireGuard peer.
type VNetWireGuardPeerUpdateRequest struct {
	// Name is the peer name.
	Name *string `json:"name,omitempty"`
	// Description is the peer description.
	Description *string `json:"description,omitempty"`
	// Enabled indicates if the peer is active.
	Enabled *bool `json:"enabled,omitempty"`
	// Endpoint is the peer's external IP or hostname.
	Endpoint *string `json:"endpoint,omitempty"`
	// Port is the peer's WireGuard port.
	Port *int `json:"port,omitempty"`
	// PeerIP is the IP address for routing between endpoints.
	PeerIP *string `json:"peer_ip,omitempty"`
	// PublicKey is the peer's WireGuard public key.
	PublicKey *string `json:"public_key,omitempty"`
	// PresharedKey is the preshared key.
	PresharedKey *string `json:"preshared_key,omitempty"`
	// AllowedIPs is the list of allowed IP ranges.
	AllowedIPs *string `json:"allowed_ips,omitempty"`
	// ConfigureFirewall defines automatic firewall rule creation.
	ConfigureFirewall *string `json:"configure_firewall,omitempty"`
	// Keepalive is the persistent keepalive interval in seconds.
	Keepalive *int `json:"keepalive,omitempty"`
	// AutogeneratePeer enables downloadable peer configuration generation.
	AutogeneratePeer *bool `json:"autogenerate_peer,omitempty"`
}

// vnetWireGuardPeerListFields are the fields to request when listing WireGuard peers.
const vnetWireGuardPeerListFields = "$key,wireguard,name,description,enabled,endpoint,port,peer_ip,public_key,preshared_key,allowed_ips,configure_firewall,keepalive,autogenerate_peer,status,modified"

// vnetWireGuardPeerGetFields are the fields to request when getting a single WireGuard peer.
const vnetWireGuardPeerGetFields = vnetWireGuardPeerListFields

// WireGuard peer firewall configuration constants
const (
	// WireGuardPeerFirewallSiteToSite creates routes and accept rules for allowed IPs.
	WireGuardPeerFirewallSiteToSite = "site-to-site"
	// WireGuardPeerFirewallRemoteUser creates rules and SNATs outbound traffic.
	WireGuardPeerFirewallRemoteUser = "remote-user"
	// WireGuardPeerFirewallNone doesn't create any firewall rules.
	WireGuardPeerFirewallNone = "none"
)

// VNetWireGuardPeerStatus represents the status of a WireGuard peer connection.
type VNetWireGuardPeerStatus struct {
	// Key is the unique identifier for the status record.
	Key FlexInt `json:"$key,omitempty"`
	// Peer is the parent peer ID.
	Peer FlexInt `json:"peer,omitempty"`
	// LastHandshake is the timestamp of the last successful handshake (Unix epoch).
	LastHandshake int64 `json:"last_handshake,omitempty"`
	// TXBytes is the total bytes transmitted to this peer.
	TXBytes int64 `json:"tx_bytes,omitempty"`
	// RXBytes is the total bytes received from this peer.
	RXBytes int64 `json:"rx_bytes,omitempty"`
	// LastUpdate is the timestamp of the last status update (Unix epoch).
	LastUpdate int64 `json:"last_update,omitempty"`
}

// vnetWireGuardPeerStatusListFields are the fields to request when listing peer statuses.
const vnetWireGuardPeerStatusListFields = "$key,peer,last_handshake,tx_bytes,rx_bytes,last_update"

// vnetWireGuardPeerStatusGetFields are the fields to request when getting a single peer status.
const vnetWireGuardPeerStatusGetFields = vnetWireGuardPeerStatusListFields
