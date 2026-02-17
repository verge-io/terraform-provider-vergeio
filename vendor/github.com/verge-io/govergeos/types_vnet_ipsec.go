package vergeos

// VNetIPSec represents IPSec VPN configuration for a network.
type VNetIPSec struct {
	// Key is the unique identifier for the IPSec configuration.
	Key FlexInt `json:"$key,omitempty"`
	// VNet is the parent network ID.
	VNet FlexInt `json:"vnet,omitempty"`
	// Enabled indicates if IPSec is active on this network.
	Enabled bool `json:"enabled,omitempty"`
	// Mode is the configuration mode: normal or advanced.
	Mode string `json:"mode,omitempty"`
	// StrongswanConf is the raw strongswan.conf content (advanced mode).
	StrongswanConf string `json:"strongswan_conf,omitempty"`
	// IPSecConf is the raw ipsec.conf content (advanced mode).
	IPSecConf string `json:"ipsec_conf,omitempty"`
	// IPSecSecrets is the raw ipsec.secrets content (advanced mode).
	IPSecSecrets string `json:"ipsec_secrets,omitempty"`
	// UniqueIDs controls unique participant ID handling: yes, no, never, replace, keep.
	UniqueIDs string `json:"uniqueids,omitempty"`
	// Compress proposes IPComp compression.
	Compress bool `json:"compress,omitempty"`
	// ExcludeNetwork excludes local subnet traffic from IPsec.
	ExcludeNetwork bool `json:"exclude_network,omitempty"`
	// CiscoUnity sends Cisco Unity vendor ID payload (IKEv1 only).
	CiscoUnity bool `json:"charon.cisco_unity,omitempty"`
	// AcceptUnencryptedMainMode accepts unencrypted ID/HASH in IKEv1 Main Mode.
	AcceptUnencryptedMainMode bool `json:"charon.accept_unencrypted_mainmode_messages,omitempty"`
	// MSSClamp is the MSS to set on installed routes (0 = disabled).
	MSSClamp int `json:"charon.plugins.kernel-netlink.mss,omitempty"`
	// StrictCRLPolicy defines CRL validation: yes, ifuri, no.
	StrictCRLPolicy string `json:"strictcrlpolicy,omitempty"`
	// MakeBeforeBreak uses make-before-break reauthentication (IKEv2).
	MakeBeforeBreak bool `json:"charon.make_before_break,omitempty"`
	// Modified is the last modification timestamp (Unix epoch).
	Modified int64 `json:"modified,omitempty"`
}

// VNetIPSecCreateRequest is the request body for creating an IPSec configuration.
type VNetIPSecCreateRequest struct {
	// VNet is the parent network ID (required).
	VNet int `json:"vnet"`
	// Enabled indicates if IPSec is active (default: true).
	Enabled *bool `json:"enabled,omitempty"`
	// Mode is the configuration mode: normal or advanced (default: normal).
	Mode *string `json:"mode,omitempty"`
	// StrongswanConf is the raw strongswan.conf content.
	StrongswanConf *string `json:"strongswan_conf,omitempty"`
	// IPSecConf is the raw ipsec.conf content.
	IPSecConf *string `json:"ipsec_conf,omitempty"`
	// IPSecSecrets is the raw ipsec.secrets content.
	IPSecSecrets *string `json:"ipsec_secrets,omitempty"`
	// UniqueIDs controls unique participant ID handling.
	UniqueIDs *string `json:"uniqueids,omitempty"`
	// Compress proposes IPComp compression.
	Compress *bool `json:"compress,omitempty"`
	// ExcludeNetwork excludes local subnet traffic from IPsec.
	ExcludeNetwork *bool `json:"exclude_network,omitempty"`
}

// VNetIPSecUpdateRequest is the request body for updating an IPSec configuration.
type VNetIPSecUpdateRequest struct {
	// Enabled indicates if IPSec is active.
	Enabled *bool `json:"enabled,omitempty"`
	// Mode is the configuration mode.
	Mode *string `json:"mode,omitempty"`
	// StrongswanConf is the raw strongswan.conf content.
	StrongswanConf *string `json:"strongswan_conf,omitempty"`
	// IPSecConf is the raw ipsec.conf content.
	IPSecConf *string `json:"ipsec_conf,omitempty"`
	// IPSecSecrets is the raw ipsec.secrets content.
	IPSecSecrets *string `json:"ipsec_secrets,omitempty"`
	// UniqueIDs controls unique participant ID handling.
	UniqueIDs *string `json:"uniqueids,omitempty"`
	// Compress proposes IPComp compression.
	Compress *bool `json:"compress,omitempty"`
	// ExcludeNetwork excludes local subnet traffic from IPsec.
	ExcludeNetwork *bool `json:"exclude_network,omitempty"`
	// CiscoUnity sends Cisco Unity vendor ID payload.
	CiscoUnity *bool `json:"charon.cisco_unity,omitempty"`
	// AcceptUnencryptedMainMode accepts unencrypted ID/HASH in IKEv1.
	AcceptUnencryptedMainMode *bool `json:"charon.accept_unencrypted_mainmode_messages,omitempty"`
	// MSSClamp is the MSS to set on installed routes.
	MSSClamp *int `json:"charon.plugins.kernel-netlink.mss,omitempty"`
	// StrictCRLPolicy defines CRL validation.
	StrictCRLPolicy *string `json:"strictcrlpolicy,omitempty"`
	// MakeBeforeBreak uses make-before-break reauthentication.
	MakeBeforeBreak *bool `json:"charon.make_before_break,omitempty"`
}

// vnetIPSecListFields are the fields to request when listing IPSec configurations.
const vnetIPSecListFields = "$key,vnet,enabled,mode,uniqueids,compress,exclude_network,strictcrlpolicy,modified"

// vnetIPSecGetFields are the fields to request when getting a single IPSec configuration.
const vnetIPSecGetFields = vnetIPSecListFields

// IPSec mode constants
const (
	// IPSecModeNormal uses the normal GUI-based configuration.
	IPSecModeNormal = "normal"
	// IPSecModeAdvanced uses raw configuration files.
	IPSecModeAdvanced = "advanced"
)

// IPSec uniqueids constants
const (
	// IPSecUniqueIDsYes replaces old IKE_SAs with same ID.
	IPSecUniqueIDsYes = "yes"
	// IPSecUniqueIDsNo keeps duplicate IKE_SAs.
	IPSecUniqueIDsNo = "no"
	// IPSecUniqueIDsNever ignores INITIAL_CONTACT notifies.
	IPSecUniqueIDsNever = "never"
	// IPSecUniqueIDsReplace is identical to yes.
	IPSecUniqueIDsReplace = "replace"
	// IPSecUniqueIDsKeep rejects new setups, keeps old.
	IPSecUniqueIDsKeep = "keep"
)

// VNetIPSecPhase1 represents an IKE SA (Phase 1) configuration.
type VNetIPSecPhase1 struct {
	// Key is the unique identifier for the Phase 1 configuration.
	Key FlexInt `json:"$key,omitempty"`
	// IPSec is the parent IPSec configuration ID.
	IPSec FlexInt `json:"ipsec,omitempty"`
	// Name is the Phase 1 configuration name.
	Name string `json:"name"`
	// Description is an optional description.
	Description string `json:"description,omitempty"`
	// Enabled indicates if this Phase 1 is active.
	Enabled bool `json:"enabled,omitempty"`
	// KeyExchange is the IKE version: ikev1, ikev2, ike (auto).
	KeyExchange string `json:"keyexchange,omitempty"`
	// RemoteGateway is the remote peer IP or hostname.
	RemoteGateway string `json:"remote_gateway,omitempty"`
	// Auth is the authentication method: psk (mutual PSK), pubkey (mutual RSA).
	Auth string `json:"auth,omitempty"`
	// Negotiation is the negotiation mode: main, aggressive.
	Negotiation string `json:"negotiation,omitempty"`
	// Identifier is the local identifier (blank = current IP).
	Identifier string `json:"identifier,omitempty"`
	// PeerIdentifier is the peer identifier (blank = remote gateway).
	PeerIdentifier string `json:"peer_identifier,omitempty"`
	// PSK is the pre-shared key (hidden in responses).
	PSK string `json:"psk,omitempty"`
	// IKE is the encryption algorithm(s) for IKE SA.
	IKE string `json:"ike,omitempty"`
	// IKELifetime is how long the IKE SA lasts before rekeying (seconds).
	IKELifetime int `json:"ikelifetime,omitempty"`
	// Auto is the connection behavior: add, route, start.
	Auto string `json:"auto,omitempty"`
	// MOBIKE enables IKEv2 MOBIKE protocol.
	MOBIKE bool `json:"mobike,omitempty"`
	// SplitConnections creates separate connections for each Phase 2.
	SplitConnections bool `json:"split_connections,omitempty"`
	// ForceEncaps forces UDP encapsulation even without NAT.
	ForceEncaps bool `json:"forceencaps,omitempty"`
	// KeyingTries is the number of negotiation attempts (0 = never give up).
	KeyingTries int `json:"keyingtries,omitempty"`
	// Rekey enables renegotiation before expiry.
	Rekey bool `json:"rekey,omitempty"`
	// Reauth enables reauthentication during rekey (IKEv2).
	Reauth bool `json:"reauth,omitempty"`
	// MarginTime is how long before expiry to start rekeying (seconds).
	MarginTime int `json:"margintime,omitempty"`
	// DPDAction is the dead peer detection action: none, clear, hold, restart.
	DPDAction string `json:"dpdaction,omitempty"`
	// DPDDelay is the DPD check interval (seconds).
	DPDDelay int `json:"dpddelay,omitempty"`
	// DPDFailures is the max DPD failures before disconnect (IKEv1).
	DPDFailures int `json:"dpdfailures,omitempty"`
	// Modified is the last modification timestamp (Unix epoch).
	Modified int64 `json:"modified,omitempty"`
}

// VNetIPSecPhase1CreateRequest is the request body for creating a Phase 1 configuration.
type VNetIPSecPhase1CreateRequest struct {
	// IPSec is the parent IPSec configuration ID (required).
	IPSec int `json:"ipsec"`
	// Name is the Phase 1 configuration name (required).
	Name string `json:"name"`
	// Description is an optional description.
	Description string `json:"description,omitempty"`
	// Enabled indicates if this Phase 1 is active (default: true).
	Enabled *bool `json:"enabled,omitempty"`
	// KeyExchange is the IKE version (default: ike).
	KeyExchange *string `json:"keyexchange,omitempty"`
	// RemoteGateway is the remote peer IP or hostname (required).
	RemoteGateway string `json:"remote_gateway"`
	// Auth is the authentication method (default: psk).
	Auth *string `json:"auth,omitempty"`
	// Negotiation is the negotiation mode (default: main).
	Negotiation *string `json:"negotiation,omitempty"`
	// Identifier is the local identifier.
	Identifier *string `json:"identifier,omitempty"`
	// PeerIdentifier is the peer identifier.
	PeerIdentifier *string `json:"peer_identifier,omitempty"`
	// PSK is the pre-shared key (required for psk auth).
	PSK *string `json:"psk,omitempty"`
	// IKE is the encryption algorithm(s) (default: aes256-sha256-modp2048).
	IKE *string `json:"ike,omitempty"`
	// IKELifetime is the IKE SA lifetime in seconds (default: 10800).
	IKELifetime *int `json:"ikelifetime,omitempty"`
	// Auto is the connection behavior (default: route).
	Auto *string `json:"auto,omitempty"`
	// MOBIKE enables IKEv2 MOBIKE protocol.
	MOBIKE *bool `json:"mobike,omitempty"`
	// SplitConnections creates separate connections for each Phase 2.
	SplitConnections *bool `json:"split_connections,omitempty"`
	// ForceEncaps forces UDP encapsulation.
	ForceEncaps *bool `json:"forceencaps,omitempty"`
	// KeyingTries is the number of negotiation attempts (default: 3).
	KeyingTries *int `json:"keyingtries,omitempty"`
	// Rekey enables renegotiation (default: true).
	Rekey *bool `json:"rekey,omitempty"`
	// Reauth enables reauthentication (default: true).
	Reauth *bool `json:"reauth,omitempty"`
	// MarginTime is rekeying margin time in seconds (default: 540).
	MarginTime *int `json:"margintime,omitempty"`
	// DPDAction is the dead peer detection action (default: restart).
	DPDAction *string `json:"dpdaction,omitempty"`
	// DPDDelay is the DPD check interval (default: 30).
	DPDDelay *int `json:"dpddelay,omitempty"`
	// DPDFailures is the max DPD failures (default: 5).
	DPDFailures *int `json:"dpdfailures,omitempty"`
}

// VNetIPSecPhase1UpdateRequest is the request body for updating a Phase 1 configuration.
type VNetIPSecPhase1UpdateRequest struct {
	// Name is the Phase 1 configuration name.
	Name *string `json:"name,omitempty"`
	// Description is the description.
	Description *string `json:"description,omitempty"`
	// Enabled indicates if this Phase 1 is active.
	Enabled *bool `json:"enabled,omitempty"`
	// KeyExchange is the IKE version.
	KeyExchange *string `json:"keyexchange,omitempty"`
	// RemoteGateway is the remote peer IP or hostname.
	RemoteGateway *string `json:"remote_gateway,omitempty"`
	// Auth is the authentication method.
	Auth *string `json:"auth,omitempty"`
	// Negotiation is the negotiation mode.
	Negotiation *string `json:"negotiation,omitempty"`
	// Identifier is the local identifier.
	Identifier *string `json:"identifier,omitempty"`
	// PeerIdentifier is the peer identifier.
	PeerIdentifier *string `json:"peer_identifier,omitempty"`
	// PSK is the pre-shared key.
	PSK *string `json:"psk,omitempty"`
	// IKE is the encryption algorithm(s).
	IKE *string `json:"ike,omitempty"`
	// IKELifetime is the IKE SA lifetime in seconds.
	IKELifetime *int `json:"ikelifetime,omitempty"`
	// Auto is the connection behavior.
	Auto *string `json:"auto,omitempty"`
	// MOBIKE enables IKEv2 MOBIKE protocol.
	MOBIKE *bool `json:"mobike,omitempty"`
	// SplitConnections creates separate connections for each Phase 2.
	SplitConnections *bool `json:"split_connections,omitempty"`
	// ForceEncaps forces UDP encapsulation.
	ForceEncaps *bool `json:"forceencaps,omitempty"`
	// KeyingTries is the number of negotiation attempts.
	KeyingTries *int `json:"keyingtries,omitempty"`
	// Rekey enables renegotiation.
	Rekey *bool `json:"rekey,omitempty"`
	// Reauth enables reauthentication.
	Reauth *bool `json:"reauth,omitempty"`
	// MarginTime is rekeying margin time in seconds.
	MarginTime *int `json:"margintime,omitempty"`
	// DPDAction is the dead peer detection action.
	DPDAction *string `json:"dpdaction,omitempty"`
	// DPDDelay is the DPD check interval.
	DPDDelay *int `json:"dpddelay,omitempty"`
	// DPDFailures is the max DPD failures.
	DPDFailures *int `json:"dpdfailures,omitempty"`
}

// vnetIPSecPhase1ListFields are the fields to request when listing Phase 1 configurations.
const vnetIPSecPhase1ListFields = "$key,ipsec,name,description,enabled,keyexchange,remote_gateway,auth,negotiation,identifier,peer_identifier,ike,ikelifetime,auto,mobike,split_connections,forceencaps,keyingtries,rekey,reauth,margintime,dpdaction,dpddelay,dpdfailures,modified"

// vnetIPSecPhase1GetFields are the fields to request when getting a single Phase 1.
const vnetIPSecPhase1GetFields = vnetIPSecPhase1ListFields

// Phase 1 key exchange constants
const (
	// IPSecKeyExchangeIKEv1 uses IKEv1 only.
	IPSecKeyExchangeIKEv1 = "ikev1"
	// IPSecKeyExchangeIKEv2 uses IKEv2 only.
	IPSecKeyExchangeIKEv2 = "ikev2"
	// IPSecKeyExchangeAuto uses IKEv2 as initiator, accepts either as responder.
	IPSecKeyExchangeAuto = "ike"
)

// Phase 1 authentication constants
const (
	// IPSecAuthPSK uses mutual pre-shared key.
	IPSecAuthPSK = "psk"
	// IPSecAuthPubkey uses mutual RSA certificates.
	IPSecAuthPubkey = "pubkey"
)

// Phase 1 negotiation mode constants
const (
	// IPSecNegotiationMain uses main mode (more secure).
	IPSecNegotiationMain = "main"
	// IPSecNegotiationAggressive uses aggressive mode (more flexible, less secure).
	IPSecNegotiationAggressive = "aggressive"
)

// Phase 1 connection behavior constants
const (
	// IPSecAutoAdd loads without starting (responder only).
	IPSecAutoAdd = "add"
	// IPSecAutoRoute loads and installs traps (on-demand).
	IPSecAutoRoute = "route"
	// IPSecAutoStart loads and starts immediately.
	IPSecAutoStart = "start"
)

// Phase 1 DPD action constants
const (
	// IPSecDPDNone disables active DPD.
	IPSecDPDNone = "none"
	// IPSecDPDClear closes connection on timeout.
	IPSecDPDClear = "clear"
	// IPSecDPDHold installs trap policy on timeout.
	IPSecDPDHold = "hold"
	// IPSecDPDRestart renegotiates on timeout.
	IPSecDPDRestart = "restart"
)

// VNetIPSecPhase2 represents an IPsec SA (Phase 2) configuration.
type VNetIPSecPhase2 struct {
	// Key is the unique identifier for the Phase 2 configuration.
	Key FlexInt `json:"$key,omitempty"`
	// Phase1 is the parent Phase 1 configuration ID.
	Phase1 FlexInt `json:"phase1,omitempty"`
	// Name is the Phase 2 configuration name.
	Name string `json:"name"`
	// Description is an optional description.
	Description string `json:"description,omitempty"`
	// Enabled indicates if this Phase 2 is active.
	Enabled bool `json:"enabled,omitempty"`
	// Mode is the IPsec mode: tunnel or transport.
	Mode string `json:"mode,omitempty"`
	// Local is the local network/subnet.
	Local string `json:"local,omitempty"`
	// Remote is the remote network/subnet.
	Remote string `json:"remote,omitempty"`
	// Lifetime is how long the IPsec SA lasts (seconds).
	Lifetime int `json:"lifetime,omitempty"`
	// Protocol is the IPsec protocol: esp (encryption) or ah (auth only).
	Protocol string `json:"protocol,omitempty"`
	// Ciphers is the cipher suites for IPsec SA.
	Ciphers string `json:"ciphers,omitempty"`
	// Modified is the last modification timestamp (Unix epoch).
	Modified int64 `json:"modified,omitempty"`
}

// VNetIPSecPhase2CreateRequest is the request body for creating a Phase 2 configuration.
type VNetIPSecPhase2CreateRequest struct {
	// Phase1 is the parent Phase 1 configuration ID (required).
	Phase1 int `json:"phase1"`
	// Name is the Phase 2 configuration name (required).
	Name string `json:"name"`
	// Description is an optional description.
	Description string `json:"description,omitempty"`
	// Enabled indicates if this Phase 2 is active (default: true).
	Enabled *bool `json:"enabled,omitempty"`
	// Mode is the IPsec mode (default: tunnel).
	Mode *string `json:"mode,omitempty"`
	// Local is the local network/subnet (required).
	Local string `json:"local"`
	// Remote is the remote network/subnet.
	Remote string `json:"remote,omitempty"`
	// Lifetime is the IPsec SA lifetime in seconds (default: 3600).
	Lifetime *int `json:"lifetime,omitempty"`
	// Protocol is the IPsec protocol (default: esp).
	Protocol *string `json:"protocol,omitempty"`
	// Ciphers is the cipher suites (default: aes128-sha256-modp2048,aes128gcm128-sha256-modp2048).
	Ciphers *string `json:"ciphers,omitempty"`
}

// VNetIPSecPhase2UpdateRequest is the request body for updating a Phase 2 configuration.
type VNetIPSecPhase2UpdateRequest struct {
	// Name is the Phase 2 configuration name.
	Name *string `json:"name,omitempty"`
	// Description is the description.
	Description *string `json:"description,omitempty"`
	// Enabled indicates if this Phase 2 is active.
	Enabled *bool `json:"enabled,omitempty"`
	// Mode is the IPsec mode.
	Mode *string `json:"mode,omitempty"`
	// Local is the local network/subnet.
	Local *string `json:"local,omitempty"`
	// Remote is the remote network/subnet.
	Remote *string `json:"remote,omitempty"`
	// Lifetime is the IPsec SA lifetime in seconds.
	Lifetime *int `json:"lifetime,omitempty"`
	// Protocol is the IPsec protocol.
	Protocol *string `json:"protocol,omitempty"`
	// Ciphers is the cipher suites.
	Ciphers *string `json:"ciphers,omitempty"`
}

// vnetIPSecPhase2ListFields are the fields to request when listing Phase 2 configurations.
const vnetIPSecPhase2ListFields = "$key,phase1,name,description,enabled,mode,local,remote,lifetime,protocol,ciphers,modified"

// vnetIPSecPhase2GetFields are the fields to request when getting a single Phase 2.
const vnetIPSecPhase2GetFields = vnetIPSecPhase2ListFields

// Phase 2 mode constants
const (
	// IPSecModeTunnel encapsulates entire IP packets (subnet-to-subnet).
	IPSecModeTunnel = "tunnel"
	// IPSecModeTransport encrypts only payload (host-to-host).
	IPSecModeTransport = "transport"
)

// Phase 2 protocol constants
const (
	// IPSecProtocolESP uses Encapsulating Security Payload (encryption).
	IPSecProtocolESP = "esp"
	// IPSecProtocolAH uses Authentication Header (authentication only).
	IPSecProtocolAH = "ah"
)

// VNetIPSecConnection represents an active IPsec connection (read-only status).
type VNetIPSecConnection struct {
	// Key is the unique identifier for the connection.
	Key FlexInt `json:"$key,omitempty"`
	// VNet is the network ID.
	VNet FlexInt `json:"vnet,omitempty"`
	// Phase1 is the Phase 1 configuration ID.
	Phase1 FlexInt `json:"phase1,omitempty"`
	// Phase2 is the Phase 2 configuration ID.
	Phase2 FlexInt `json:"phase2,omitempty"`
	// UniqueID is the IKE SA unique identifier.
	UniqueID int `json:"uniqueid,omitempty"`
	// Local is the local endpoint IP.
	Local string `json:"local,omitempty"`
	// Remote is the remote endpoint IP.
	Remote string `json:"remote,omitempty"`
	// LocalNetwork is the local subnet.
	LocalNetwork string `json:"local_network,omitempty"`
	// RemoteNetwork is the remote subnet.
	RemoteNetwork string `json:"remote_network,omitempty"`
	// Connection is the connection name.
	Connection string `json:"connection,omitempty"`
	// ReqID is the request ID.
	ReqID string `json:"reqid,omitempty"`
	// Interface is the network interface.
	Interface string `json:"interface,omitempty"`
	// Protocol is the IPsec protocol in use.
	Protocol string `json:"protocol,omitempty"`
	// Created is the connection establishment timestamp (Unix epoch).
	Created int64 `json:"created,omitempty"`
}

// vnetIPSecConnectionListFields are the fields to request when listing connections.
const vnetIPSecConnectionListFields = "$key,vnet,phase1,phase2,uniqueid,local,remote,local_network,remote_network,connection,reqid,interface,protocol,created"
