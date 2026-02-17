package vergeos

// VNetRule represents a VergeOS network firewall rule.
type VNetRule struct {
	// Key is the unique identifier for the rule.
	Key FlexInt `json:"$key,omitempty"`
	// VNet is the parent network ID (readonly after creation).
	VNet FlexInt `json:"vnet,omitempty"`
	// Name is the rule name (unique per network).
	Name string `json:"name"`
	// Description is an optional description of the rule.
	Description string `json:"description,omitempty"`
	// OrderID determines the rule processing order (lower = earlier).
	OrderID int `json:"orderid,omitempty"`
	// Pin automatically moves this rule above/below non-pinned rules (no, top, bottom).
	Pin string `json:"pin,omitempty"`
	// Enabled indicates whether the rule is active.
	Enabled bool `json:"enabled"`
	// Trace enables packet tracing for diagnostic purposes.
	Trace bool `json:"trace,omitempty"`
	// SystemRule indicates this is a system-managed rule (readonly).
	SystemRule bool `json:"system_rule,omitempty"`
	// Owner is the owner reference path (e.g., "vnet_addresses/28") (readonly).
	Owner string `json:"owner,omitempty"`

	// Traffic matching criteria
	// Protocol to match (tcp, tcpudp, udp, icmp, 89=OSPF, 2=IGMP, 47=GRE, 50=ESP, 51=AH, any).
	Protocol string `json:"protocol,omitempty"`
	// Direction of traffic (incoming, outgoing).
	Direction string `json:"direction,omitempty"`
	// CTState is the connection tracking state filter (new, established, related, untracked).
	CTState string `json:"ct_state,omitempty"`
	// Interface to match (auto, router, dmz, wireguard, any).
	Interface string `json:"interface,omitempty"`

	// Source filtering
	// SourceIP is the source IP filter (e.g., "192.168.0.1,192.168.1.0/24,vnetself,router").
	SourceIP string `json:"source_ip,omitempty"`
	// SourcePorts is the source port filter (e.g., "80,22,1600-1800").
	SourcePorts string `json:"source_ports,omitempty"`

	// Destination filtering
	// DestinationIP is the destination IP filter.
	DestinationIP string `json:"destination_ip,omitempty"`
	// DestinationPorts is the destination port filter.
	DestinationPorts string `json:"destination_ports,omitempty"`

	// Action determines what happens to matched traffic (accept, drop, reject, translate, route).
	Action string `json:"action,omitempty"`

	// NAT/Routing targets (for translate/route actions)
	// TargetIP is the target IP for translation/routing.
	TargetIP string `json:"target_ip,omitempty"`
	// TargetPorts is the target port for translation.
	TargetPorts string `json:"target_ports,omitempty"`

	// Rate limiting
	// Throttle defines rate limiting (e.g., "1000 kbytes/second", "100/minute burst 10").
	Throttle string `json:"throttle,omitempty"`
	// DropThrottle creates an automatic drop rule when throttle is exceeded.
	DropThrottle bool `json:"drop_throttle,omitempty"`

	// Logging and statistics
	// Statistics enables rule hit tracking.
	Statistics bool `json:"statistics,omitempty"`
	// Log enables logging of matched traffic.
	Log bool `json:"log,omitempty"`
	// Packets is the number of packets matched (when statistics enabled).
	Packets int64 `json:"packets,omitempty"`
	// Bytes is the number of bytes matched (when statistics enabled).
	Bytes int64 `json:"bytes,omitempty"`

	// Metadata
	// Modified is the last modification timestamp (Unix epoch).
	Modified int64 `json:"modified,omitempty"`
	// Creator is the username that created this rule.
	Creator string `json:"creator,omitempty"`
}

// VNetRuleCreateRequest is the request body for creating a firewall rule.
type VNetRuleCreateRequest struct {
	// VNet is the parent network ID (required).
	VNet int `json:"vnet"`
	// Name is the rule name (required, unique per network).
	Name string `json:"name"`
	// Description is an optional description.
	Description string `json:"description,omitempty"`
	// OrderID determines the rule processing order.
	OrderID *int `json:"orderid,omitempty"`
	// Pin automatically moves this rule (no, top, bottom).
	Pin *string `json:"pin,omitempty"`
	// Enabled indicates whether the rule is active.
	Enabled *bool `json:"enabled,omitempty"`
	// Trace enables packet tracing.
	Trace *bool `json:"trace,omitempty"`

	// Traffic matching
	// Protocol to match.
	Protocol *string `json:"protocol,omitempty"`
	// Direction of traffic.
	Direction *string `json:"direction,omitempty"`
	// CTState is the connection tracking state filter.
	CTState *string `json:"ct_state,omitempty"`
	// Interface to match.
	Interface *string `json:"interface,omitempty"`

	// Source filtering
	// SourceIP is the source IP filter.
	SourceIP *string `json:"source_ip,omitempty"`
	// SourcePorts is the source port filter.
	SourcePorts *string `json:"source_ports,omitempty"`

	// Destination filtering
	// DestinationIP is the destination IP filter.
	DestinationIP *string `json:"destination_ip,omitempty"`
	// DestinationPorts is the destination port filter.
	DestinationPorts *string `json:"destination_ports,omitempty"`

	// Action determines what happens to matched traffic.
	Action *string `json:"action,omitempty"`

	// NAT/Routing targets
	// TargetIP is the target IP for translation/routing.
	TargetIP *string `json:"target_ip,omitempty"`
	// TargetPorts is the target port for translation.
	TargetPorts *string `json:"target_ports,omitempty"`

	// Rate limiting
	// Throttle defines rate limiting.
	Throttle *string `json:"throttle,omitempty"`
	// DropThrottle creates an automatic drop rule when throttle exceeded.
	DropThrottle *bool `json:"drop_throttle,omitempty"`

	// Logging and statistics
	// Statistics enables rule hit tracking.
	Statistics *bool `json:"statistics,omitempty"`
	// Log enables logging of matched traffic.
	Log *bool `json:"log,omitempty"`
}

// VNetRuleUpdateRequest is the request body for updating a firewall rule.
type VNetRuleUpdateRequest struct {
	// Name is the rule name.
	Name *string `json:"name,omitempty"`
	// Description is the rule description.
	Description *string `json:"description,omitempty"`
	// OrderID determines the rule processing order.
	OrderID *int `json:"orderid,omitempty"`
	// Pin automatically moves this rule (no, top, bottom).
	Pin *string `json:"pin,omitempty"`
	// Enabled indicates whether the rule is active.
	Enabled *bool `json:"enabled,omitempty"`
	// Trace enables packet tracing.
	Trace *bool `json:"trace,omitempty"`

	// Traffic matching
	// Protocol to match.
	Protocol *string `json:"protocol,omitempty"`
	// Direction of traffic.
	Direction *string `json:"direction,omitempty"`
	// CTState is the connection tracking state filter.
	CTState *string `json:"ct_state,omitempty"`
	// Interface to match.
	Interface *string `json:"interface,omitempty"`

	// Source filtering
	// SourceIP is the source IP filter.
	SourceIP *string `json:"source_ip,omitempty"`
	// SourcePorts is the source port filter.
	SourcePorts *string `json:"source_ports,omitempty"`

	// Destination filtering
	// DestinationIP is the destination IP filter.
	DestinationIP *string `json:"destination_ip,omitempty"`
	// DestinationPorts is the destination port filter.
	DestinationPorts *string `json:"destination_ports,omitempty"`

	// Action determines what happens to matched traffic.
	Action *string `json:"action,omitempty"`

	// NAT/Routing targets
	// TargetIP is the target IP for translation/routing.
	TargetIP *string `json:"target_ip,omitempty"`
	// TargetPorts is the target port for translation.
	TargetPorts *string `json:"target_ports,omitempty"`

	// Rate limiting
	// Throttle defines rate limiting.
	Throttle *string `json:"throttle,omitempty"`
	// DropThrottle creates an automatic drop rule when throttle exceeded.
	DropThrottle *bool `json:"drop_throttle,omitempty"`

	// Logging and statistics
	// Statistics enables rule hit tracking.
	Statistics *bool `json:"statistics,omitempty"`
	// Log enables logging of matched traffic.
	Log *bool `json:"log,omitempty"`
}

// vnetRuleListFields are the fields to request when listing rules.
const vnetRuleListFields = "$key,vnet,name,description,orderid,pin,enabled,trace,system_rule,owner,protocol,direction,ct_state,interface,source_ip,source_ports,destination_ip,destination_ports,action,target_ip,target_ports,throttle,drop_throttle,statistics,log,packets,bytes,modified,creator"

// vnetRuleGetFields are the fields to request when getting a single rule.
const vnetRuleGetFields = vnetRuleListFields

// VNetRuleAlias represents a VergeOS network rule alias.
// Aliases allow you to define reusable address lists for firewall rules.
type VNetRuleAlias struct {
	// Key is the unique identifier for the alias.
	Key FlexInt `json:"$key,omitempty"`
	// Name is the alias name (unique globally).
	Name string `json:"name"`
	// ID is the unique SHA1 hash identifier (readonly).
	ID string `json:"id,omitempty"`
	// Description is an optional description.
	Description string `json:"description,omitempty"`
	// Value is the comma-delimited list of addresses (e.g., "192.168.1.1,10.0.0.0/8").
	// Each entry can have a description separated by pipe: "192.168.1.1|Server1,10.0.0.0/8|Internal".
	Value string `json:"value"`
	// PublishingScope determines visibility (private, global, tenant, none).
	PublishingScope string `json:"publishing_scope,omitempty"`
	// Owner is the owner reference path (readonly).
	Owner string `json:"owner,omitempty"`
}

// VNetRuleAliasCreateRequest is the request body for creating a rule alias.
type VNetRuleAliasCreateRequest struct {
	// Name is the alias name (required, unique globally).
	Name string `json:"name"`
	// Description is an optional description.
	Description string `json:"description,omitempty"`
	// Value is the comma-delimited list of addresses (required).
	Value string `json:"value"`
	// PublishingScope determines visibility (private, global, tenant, none).
	PublishingScope *string `json:"publishing_scope,omitempty"`
}

// VNetRuleAliasUpdateRequest is the request body for updating a rule alias.
type VNetRuleAliasUpdateRequest struct {
	// Name is the alias name.
	Name *string `json:"name,omitempty"`
	// Description is the alias description.
	Description *string `json:"description,omitempty"`
	// Value is the comma-delimited list of addresses.
	Value *string `json:"value,omitempty"`
	// PublishingScope determines visibility.
	PublishingScope *string `json:"publishing_scope,omitempty"`
}

// vnetRuleAliasListFields are the fields to request when listing aliases.
const vnetRuleAliasListFields = "$key,name,id,description,value,publishing_scope,owner"

// vnetRuleAliasGetFields are the fields to request when getting a single alias.
const vnetRuleAliasGetFields = vnetRuleAliasListFields
