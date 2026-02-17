package vergeos

// VNetDNSView represents a VergeOS network DNS view.
// DNS views allow different DNS responses based on client IP addresses.
type VNetDNSView struct {
	// Key is the unique identifier for the view.
	Key FlexInt `json:"$key,omitempty"`
	// VNet is the parent network ID.
	VNet FlexInt `json:"vnet,omitempty"`
	// Name is the view name.
	Name string `json:"name"`
	// Recursion enables recursive DNS queries for this view.
	Recursion bool `json:"recursion,omitempty"`
	// MatchClients specifies which clients use this view (e.g., "10/8;172.16/16;").
	MatchClients string `json:"match_clients,omitempty"`
	// MatchDestinations specifies destination-based view matching.
	MatchDestinations string `json:"match_destinations,omitempty"`
	// MaxCacheSize is the maximum RAM for caching records in bytes (0 = unlimited).
	MaxCacheSize int64 `json:"max_cache_size,omitempty"`
	// OrderID determines the view processing order.
	OrderID int `json:"orderid,omitempty"`
	// QuerySource is the source IP address for outgoing queries.
	QuerySource FlexInt `json:"query_source,omitempty"`
	// Modified is the last modification timestamp (Unix epoch).
	Modified int64 `json:"modified,omitempty"`
}

// VNetDNSViewCreateRequest is the request body for creating a DNS view.
type VNetDNSViewCreateRequest struct {
	// VNet is the parent network ID (required).
	VNet int `json:"vnet"`
	// Name is the view name (required).
	Name string `json:"name"`
	// Recursion enables recursive DNS queries.
	Recursion *bool `json:"recursion,omitempty"`
	// MatchClients specifies which clients use this view.
	MatchClients *string `json:"match_clients,omitempty"`
	// MatchDestinations specifies destination-based view matching.
	MatchDestinations *string `json:"match_destinations,omitempty"`
	// MaxCacheSize is the maximum RAM for caching records in bytes.
	MaxCacheSize *int64 `json:"max_cache_size,omitempty"`
	// OrderID determines the view processing order.
	OrderID *int `json:"orderid,omitempty"`
	// QuerySource is the source IP address for outgoing queries.
	QuerySource *int `json:"query_source,omitempty"`
}

// VNetDNSViewUpdateRequest is the request body for updating a DNS view.
type VNetDNSViewUpdateRequest struct {
	// Name is the view name.
	Name *string `json:"name,omitempty"`
	// Recursion enables recursive DNS queries.
	Recursion *bool `json:"recursion,omitempty"`
	// MatchClients specifies which clients use this view.
	MatchClients *string `json:"match_clients,omitempty"`
	// MatchDestinations specifies destination-based view matching.
	MatchDestinations *string `json:"match_destinations,omitempty"`
	// MaxCacheSize is the maximum RAM for caching records in bytes.
	MaxCacheSize *int64 `json:"max_cache_size,omitempty"`
	// OrderID determines the view processing order.
	OrderID *int `json:"orderid,omitempty"`
	// QuerySource is the source IP address for outgoing queries.
	QuerySource *int `json:"query_source,omitempty"`
}

// vnetDNSViewListFields are the fields to request when listing views.
const vnetDNSViewListFields = "$key,vnet,name,recursion,match_clients,match_destinations,max_cache_size,orderid,query_source,modified"

// vnetDNSViewGetFields are the fields to request when getting a single view.
const vnetDNSViewGetFields = vnetDNSViewListFields

// VNetDNSZone represents a VergeOS network DNS zone.
type VNetDNSZone struct {
	// Key is the unique identifier for the zone.
	Key FlexInt `json:"$key,omitempty"`
	// View is the parent DNS view ID.
	View FlexInt `json:"view,omitempty"`
	// Domain is the zone domain name.
	Domain string `json:"domain"`
	// Type is the zone type (master, slave, redirect, forward, static-stub, stub).
	Type string `json:"type,omitempty"`
	// Nameserver is the primary name server for SOA record.
	Nameserver string `json:"nameserver,omitempty"`
	// Email is the admin email for SOA record.
	Email string `json:"email,omitempty"`
	// Notify controls NOTIFY messages (yes, no, explicit).
	Notify string `json:"notify,omitempty"`
	// AllowNotify specifies servers allowed to send NOTIFY (e.g., "none;").
	AllowNotify string `json:"allow_notify,omitempty"`
	// AlsoNotify specifies additional servers to send NOTIFY to.
	AlsoNotify string `json:"also_notify,omitempty"`
	// Masters specifies master servers for slave zones.
	Masters string `json:"masters,omitempty"`
	// AllowTransfer specifies servers allowed zone transfers (e.g., "none;").
	AllowTransfer string `json:"allow_transfer,omitempty"`
	// SerialNumber is the SOA serial number (readonly, auto-incremented).
	SerialNumber int64 `json:"serial_number,omitempty"`
	// DefaultTTL is the default record TTL (e.g., "1h", "30m", "2d").
	DefaultTTL string `json:"default_ttl,omitempty"`
	// RefreshInterval is the SOA refresh interval.
	RefreshInterval string `json:"refresh_interval,omitempty"`
	// RetryInterval is the SOA retry interval.
	RetryInterval string `json:"retry_interval,omitempty"`
	// ExpiryPeriod is the SOA expiry period.
	ExpiryPeriod string `json:"expiry_period,omitempty"`
	// NegativeTTL is the negative cache TTL.
	NegativeTTL string `json:"negative_ttl,omitempty"`
	// Forwarders is the list of forwarder servers (e.g., "8.8.8.8;8.8.4.4;").
	Forwarders string `json:"forwarders,omitempty"`
	// Modified is the last modification timestamp (Unix epoch).
	Modified int64 `json:"modified,omitempty"`
}

// VNetDNSZoneCreateRequest is the request body for creating a DNS zone.
type VNetDNSZoneCreateRequest struct {
	// View is the parent DNS view ID (required).
	View int `json:"view"`
	// Domain is the zone domain name (required).
	Domain string `json:"domain"`
	// Type is the zone type (default: master).
	Type *string `json:"type,omitempty"`
	// Nameserver is the primary name server.
	Nameserver *string `json:"nameserver,omitempty"`
	// Email is the admin email.
	Email *string `json:"email,omitempty"`
	// Notify controls NOTIFY messages.
	Notify *string `json:"notify,omitempty"`
	// AllowNotify specifies servers allowed to send NOTIFY.
	AllowNotify *string `json:"allow_notify,omitempty"`
	// AlsoNotify specifies additional servers to send NOTIFY to.
	AlsoNotify *string `json:"also_notify,omitempty"`
	// Masters specifies master servers for slave zones.
	Masters *string `json:"masters,omitempty"`
	// AllowTransfer specifies servers allowed zone transfers.
	AllowTransfer *string `json:"allow_transfer,omitempty"`
	// DefaultTTL is the default record TTL.
	DefaultTTL *string `json:"default_ttl,omitempty"`
	// RefreshInterval is the SOA refresh interval.
	RefreshInterval *string `json:"refresh_interval,omitempty"`
	// RetryInterval is the SOA retry interval.
	RetryInterval *string `json:"retry_interval,omitempty"`
	// ExpiryPeriod is the SOA expiry period.
	ExpiryPeriod *string `json:"expiry_period,omitempty"`
	// NegativeTTL is the negative cache TTL.
	NegativeTTL *string `json:"negative_ttl,omitempty"`
	// Forwarders is the list of forwarder servers.
	Forwarders *string `json:"forwarders,omitempty"`
}

// VNetDNSZoneUpdateRequest is the request body for updating a DNS zone.
type VNetDNSZoneUpdateRequest struct {
	// Domain is the zone domain name.
	Domain *string `json:"domain,omitempty"`
	// Type is the zone type.
	Type *string `json:"type,omitempty"`
	// Nameserver is the primary name server.
	Nameserver *string `json:"nameserver,omitempty"`
	// Email is the admin email.
	Email *string `json:"email,omitempty"`
	// Notify controls NOTIFY messages.
	Notify *string `json:"notify,omitempty"`
	// AllowNotify specifies servers allowed to send NOTIFY.
	AllowNotify *string `json:"allow_notify,omitempty"`
	// AlsoNotify specifies additional servers to send NOTIFY to.
	AlsoNotify *string `json:"also_notify,omitempty"`
	// Masters specifies master servers for slave zones.
	Masters *string `json:"masters,omitempty"`
	// AllowTransfer specifies servers allowed zone transfers.
	AllowTransfer *string `json:"allow_transfer,omitempty"`
	// DefaultTTL is the default record TTL.
	DefaultTTL *string `json:"default_ttl,omitempty"`
	// RefreshInterval is the SOA refresh interval.
	RefreshInterval *string `json:"refresh_interval,omitempty"`
	// RetryInterval is the SOA retry interval.
	RetryInterval *string `json:"retry_interval,omitempty"`
	// ExpiryPeriod is the SOA expiry period.
	ExpiryPeriod *string `json:"expiry_period,omitempty"`
	// NegativeTTL is the negative cache TTL.
	NegativeTTL *string `json:"negative_ttl,omitempty"`
	// Forwarders is the list of forwarder servers.
	Forwarders *string `json:"forwarders,omitempty"`
}

// DNS zone type constants
const (
	// DNSZoneTypeMaster indicates a primary zone.
	DNSZoneTypeMaster = "master"
	// DNSZoneTypeSlave indicates a secondary zone.
	DNSZoneTypeSlave = "slave"
	// DNSZoneTypeRedirect indicates a redirect zone.
	DNSZoneTypeRedirect = "redirect"
	// DNSZoneTypeForward indicates a forward zone.
	DNSZoneTypeForward = "forward"
	// DNSZoneTypeStaticStub indicates a static stub zone.
	DNSZoneTypeStaticStub = "static-stub"
	// DNSZoneTypeStub indicates a stub zone.
	DNSZoneTypeStub = "stub"
)

// vnetDNSZoneListFields are the fields to request when listing zones.
const vnetDNSZoneListFields = "$key,view,domain,type,nameserver,email,notify,allow_notify,also_notify,masters,allow_transfer,serial_number,default_ttl,refresh_interval,retry_interval,expiry_period,negative_ttl,forwarders,modified"

// vnetDNSZoneGetFields are the fields to request when getting a single zone.
const vnetDNSZoneGetFields = vnetDNSZoneListFields

// VNetDNSRecord represents a VergeOS network DNS zone record.
type VNetDNSRecord struct {
	// Key is the unique identifier for the record.
	Key FlexInt `json:"$key,omitempty"`
	// Zone is the parent DNS zone ID.
	Zone FlexInt `json:"zone,omitempty"`
	// Host is the record hostname (empty inherits from previous record or domain).
	Host string `json:"host,omitempty"`
	// TTL is the record time-to-live (e.g., "1h", "30m").
	TTL string `json:"ttl,omitempty"`
	// Type is the record type (A, AAAA, CNAME, MX, NS, PTR, SRV, TXT, CAA).
	Type string `json:"type,omitempty"`
	// Value is the record value (IP for A, hostname for CNAME, etc.).
	Value string `json:"value"`
	// Description is an optional description.
	Description string `json:"description,omitempty"`
	// MXPreference is the MX record preference (0-65535).
	MXPreference int `json:"mx_preference,omitempty"`
	// Weight is the SRV record weight (0-65535).
	Weight int `json:"weight,omitempty"`
	// Port is the SRV record port (0-65535).
	Port int `json:"port,omitempty"`
	// IssueWildcard is for CAA records - issue only wildcard certificates.
	IssueWildcard bool `json:"issue_wildcard,omitempty"`
	// OrderID determines the record display order.
	OrderID int `json:"orderid,omitempty"`
	// Modified is the last modification timestamp (Unix epoch).
	Modified int64 `json:"modified,omitempty"`
}

// VNetDNSRecordCreateRequest is the request body for creating a DNS record.
type VNetDNSRecordCreateRequest struct {
	// Zone is the parent DNS zone ID (required).
	Zone int `json:"zone"`
	// Host is the record hostname.
	Host string `json:"host,omitempty"`
	// TTL is the record time-to-live.
	TTL *string `json:"ttl,omitempty"`
	// Type is the record type (required: A, AAAA, CNAME, MX, NS, PTR, SRV, TXT, CAA).
	Type string `json:"type"`
	// Value is the record value (required).
	Value string `json:"value"`
	// Description is an optional description.
	Description string `json:"description,omitempty"`
	// MXPreference is the MX record preference.
	MXPreference *int `json:"mx_preference,omitempty"`
	// Weight is the SRV record weight.
	Weight *int `json:"weight,omitempty"`
	// Port is the SRV record port.
	Port *int `json:"port,omitempty"`
	// IssueWildcard is for CAA records.
	IssueWildcard *bool `json:"issue_wildcard,omitempty"`
	// OrderID determines the record display order.
	OrderID *int `json:"orderid,omitempty"`
}

// VNetDNSRecordUpdateRequest is the request body for updating a DNS record.
type VNetDNSRecordUpdateRequest struct {
	// Host is the record hostname.
	Host *string `json:"host,omitempty"`
	// TTL is the record time-to-live.
	TTL *string `json:"ttl,omitempty"`
	// Type is the record type.
	Type *string `json:"type,omitempty"`
	// Value is the record value.
	Value *string `json:"value,omitempty"`
	// Description is the record description.
	Description *string `json:"description,omitempty"`
	// MXPreference is the MX record preference.
	MXPreference *int `json:"mx_preference,omitempty"`
	// Weight is the SRV record weight.
	Weight *int `json:"weight,omitempty"`
	// Port is the SRV record port.
	Port *int `json:"port,omitempty"`
	// IssueWildcard is for CAA records.
	IssueWildcard *bool `json:"issue_wildcard,omitempty"`
	// OrderID determines the record display order.
	OrderID *int `json:"orderid,omitempty"`
}

// DNS record type constants
const (
	// DNSRecordTypeA indicates an A (IPv4 address) record.
	DNSRecordTypeA = "A"
	// DNSRecordTypeAAAA indicates an AAAA (IPv6 address) record.
	DNSRecordTypeAAAA = "AAAA"
	// DNSRecordTypeCNAME indicates a CNAME (canonical name) record.
	DNSRecordTypeCNAME = "CNAME"
	// DNSRecordTypeMX indicates an MX (mail exchange) record.
	DNSRecordTypeMX = "MX"
	// DNSRecordTypeNS indicates an NS (name server) record.
	DNSRecordTypeNS = "NS"
	// DNSRecordTypePTR indicates a PTR (pointer) record.
	DNSRecordTypePTR = "PTR"
	// DNSRecordTypeSRV indicates an SRV (service) record.
	DNSRecordTypeSRV = "SRV"
	// DNSRecordTypeTXT indicates a TXT (text) record.
	DNSRecordTypeTXT = "TXT"
	// DNSRecordTypeCAA indicates a CAA (certificate authority authorization) record.
	DNSRecordTypeCAA = "CAA"
)

// vnetDNSRecordListFields are the fields to request when listing records.
const vnetDNSRecordListFields = "$key,zone,host,ttl,type,value,description,mx_preference,weight,port,issue_wildcard,orderid,modified"

// vnetDNSRecordGetFields are the fields to request when getting a single record.
const vnetDNSRecordGetFields = vnetDNSRecordListFields
