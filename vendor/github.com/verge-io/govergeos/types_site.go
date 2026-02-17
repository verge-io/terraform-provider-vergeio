package vergeos

// Site represents a remote VergeOS site for DR, backup, and synchronization.
type Site struct {
	Key                       FlexInt `json:"$key,omitempty"`
	Name                      string  `json:"name,omitempty"`
	ID                        string  `json:"id,omitempty"` // 40-char SHA1 unique ID
	Description               string  `json:"description,omitempty"`
	Enabled                   bool    `json:"enabled,omitempty"`
	Domain                    string  `json:"domain,omitempty"`
	City                      string  `json:"city,omitempty"`
	Country                   string  `json:"country,omitempty"` // 2-letter country code (e.g., "US")
	Latitude                  float64 `json:"latitude,omitempty"`
	Longitude                 float64 `json:"longitude,omitempty"`
	Timezone                  string  `json:"timezone,omitempty"`
	URL                       string  `json:"url,omitempty"`            // Required: remote site URL
	AllowInsecure             bool    `json:"allow_insecure,omitempty"` // Allow insecure SSL connection
	Status                    string  `json:"status,omitempty"`         // idle, authenticating, syncing, error, warning
	StatusInfo                string  `json:"status_info,omitempty"`
	AuthenticationStatus      string  `json:"authentication_status,omitempty"`       // unauthenticated, authenticated, legacy
	VSANHost                  string  `json:"vsan_host,omitempty"`                   // vSAN connection host
	VSANPort                  int     `json:"vsan_port,omitempty"`                   // vSAN port (default 14201)
	IsTenant                  bool    `json:"is_tenant,omitempty"`                   // Site is a tenant
	ConfigCloudSnapshots      string  `json:"config_cloud_snapshots,omitempty"`      // disabled, send, receive, both
	ConfigStatistics          string  `json:"config_statistics,omitempty"`           // disabled, send, receive, both
	ConfigManagement          string  `json:"config_management,omitempty"`           // disabled, manage, managed, both
	ConfigRepairServer        string  `json:"config_repair_server,omitempty"`        // disabled, send, receive, both
	IncomingSyncsEnabled      bool    `json:"incoming_syncs_enabled,omitempty"`      // Read-only
	OutgoingSyncsEnabled      bool    `json:"outgoing_syncs_enabled,omitempty"`      // Read-only
	RepairsOutgoingEnabled    bool    `json:"repairs_outgoing_enabled,omitempty"`    // Read-only
	IncomingStatsEnabled      bool    `json:"incoming_stats_enabled,omitempty"`      // Read-only
	OutgoingStatsEnabled      bool    `json:"outgoing_stats_enabled,omitempty"`      // Read-only
	OutgoingManagementEnabled bool    `json:"outgoing_management_enabled,omitempty"` // Read-only
	IncomingManagementEnabled bool    `json:"incoming_management_enabled,omitempty"` // Read-only
	StatisticsInterval        int     `json:"statistics_interval,omitempty"`         // Seconds (default 600, min 300)
	StatisticsRetention       int     `json:"statistics_retention,omitempty"`        // Seconds (default 3888000 = 45 days)
	RequestURL                string  `json:"request_url,omitempty"`                 // URL remote system uses to connect
	RemoteUser                string  `json:"remote_user,omitempty"`                 // Remote user for authentication
	LogoURL                   string  `json:"logo_url,omitempty"`                    // Logo URL (144x36)
	HeaderBG                  string  `json:"header_bg,omitempty"`                   // Logo background color
	MapColor                  string  `json:"map_color,omitempty"`                   // Map pin color
	LastStatUpdate            int64   `json:"last_stat_update,omitempty"`
	Modified                  int64   `json:"modified,omitempty"`
	Created                   int64   `json:"created,omitempty"`
	Creator                   string  `json:"creator,omitempty"`
}

// SiteCreateRequest represents the request body for creating a site.
type SiteCreateRequest struct {
	Name                     string   `json:"name,omitempty"`
	Description              string   `json:"description,omitempty"`
	Enabled                  *bool    `json:"enabled,omitempty"`
	Domain                   string   `json:"domain,omitempty"`
	City                     string   `json:"city,omitempty"`
	Country                  string   `json:"country,omitempty"` // 2-letter country code
	Latitude                 *float64 `json:"latitude,omitempty"`
	Longitude                *float64 `json:"longitude,omitempty"`
	Timezone                 string   `json:"timezone,omitempty"`
	URL                      string   `json:"url"` // Required
	AllowInsecure            *bool    `json:"allow_insecure,omitempty"`
	AuthUser                 string   `json:"auth_user,omitempty"`              // User for site creation/verification (not stored)
	AuthPassword             string   `json:"auth_password,omitempty"`          // Password for site creation/verification (not stored)
	ConfigCloudSnapshots     *string  `json:"config_cloud_snapshots,omitempty"` // disabled, send, receive, both
	ConfigStatistics         *string  `json:"config_statistics,omitempty"`      // disabled, send, receive, both
	ConfigManagement         *string  `json:"config_management,omitempty"`      // disabled, manage, managed, both
	ConfigRepairServer       *string  `json:"config_repair_server,omitempty"`   // disabled, send, receive, both
	StatisticsInterval       *int     `json:"statistics_interval,omitempty"`
	StatisticsRetention      *int     `json:"statistics_retention,omitempty"`
	RequestURL               *string  `json:"request_url,omitempty"`
	AutomaticallyCreateSyncs *bool    `json:"automatically_create_syncs,omitempty"` // Default true
}

// SiteUpdateRequest represents the request body for updating a site.
type SiteUpdateRequest struct {
	Name                 *string  `json:"name,omitempty"`
	Description          *string  `json:"description,omitempty"`
	Enabled              *bool    `json:"enabled,omitempty"`
	Domain               *string  `json:"domain,omitempty"`
	City                 *string  `json:"city,omitempty"`
	Country              *string  `json:"country,omitempty"`
	Latitude             *float64 `json:"latitude,omitempty"`
	Longitude            *float64 `json:"longitude,omitempty"`
	Timezone             *string  `json:"timezone,omitempty"`
	URL                  *string  `json:"url,omitempty"`
	AllowInsecure        *bool    `json:"allow_insecure,omitempty"`
	ConfigCloudSnapshots *string  `json:"config_cloud_snapshots,omitempty"`
	ConfigStatistics     *string  `json:"config_statistics,omitempty"`
	ConfigManagement     *string  `json:"config_management,omitempty"`
	ConfigRepairServer   *string  `json:"config_repair_server,omitempty"`
	StatisticsInterval   *int     `json:"statistics_interval,omitempty"`
	StatisticsRetention  *int     `json:"statistics_retention,omitempty"`
	RequestURL           *string  `json:"request_url,omitempty"`
	RemoteUser           *string  `json:"remote_user,omitempty"`
	RemotePassword       *string  `json:"remote_password,omitempty"`
	LogoURL              *string  `json:"logo_url,omitempty"`
	HeaderBG             *string  `json:"header_bg,omitempty"`
	MapColor             *string  `json:"map_color,omitempty"`
	ForceRefresh         *bool    `json:"force_refresh,omitempty"`
}

// siteAction represents an action request for a site.
type siteAction struct {
	Site   int                    `json:"site"`
	Action string                 `json:"action"`
	Params map[string]interface{} `json:"params,omitempty"`
}

// Site configuration options for sync direction
const (
	SiteConfigDisabled string = "disabled"
	SiteConfigSend     string = "send"
	SiteConfigReceive  string = "receive"
	SiteConfigBoth     string = "both"
)

// Site management config options
const (
	SiteManagementManage  string = "manage"  // Manage this site's machines
	SiteManagementManaged string = "managed" // This site can manage my machines
)

// Site status values
const (
	SiteStatusIdle           string = "idle"
	SiteStatusAuthenticating string = "authenticating"
	SiteStatusSyncing        string = "syncing"
	SiteStatusError          string = "error"
	SiteStatusWarning        string = "warning"
)

// Site authentication status values
const (
	SiteAuthUnauthenticated string = "unauthenticated"
	SiteAuthAuthenticated   string = "authenticated"
	SiteAuthLegacy          string = "legacy"
)

// siteListFields defines the default fields for listing sites.
const siteListFields = "$key,name,id,description,enabled,domain,city,country,timezone,url,status,status_info,authentication_status,is_tenant,config_cloud_snapshots,config_statistics,config_management,config_repair_server,incoming_syncs_enabled,outgoing_syncs_enabled,created,creator"

// siteGetFields defines the default fields for getting a single site.
const siteGetFields = "$key,name,id,description,enabled,domain,city,country,latitude,longitude,timezone,url,allow_insecure,status,status_info,authentication_status,vsan_host,vsan_port,is_tenant,config_cloud_snapshots,config_statistics,config_management,config_repair_server,incoming_syncs_enabled,outgoing_syncs_enabled,repairs_outgoing_enabled,incoming_stats_enabled,outgoing_stats_enabled,outgoing_management_enabled,incoming_management_enabled,statistics_interval,statistics_retention,request_url,remote_user,logo_url,header_bg,map_color,last_stat_update,modified,created,creator"

// SiteSyncIncoming represents an incoming sync configuration from a remote site.
type SiteSyncIncoming struct {
	Key              FlexInt `json:"$key,omitempty"`
	Site             FlexInt `json:"site,omitempty"`
	Name             string  `json:"name,omitempty"`
	SyncID           string  `json:"sync_id,omitempty"` // 40-char SHA1 unique ID
	Description      string  `json:"description,omitempty"`
	Enabled          bool    `json:"enabled,omitempty"`
	Status           string  `json:"status,omitempty"` // generating_reg, syncing, offline, error, regeneration_needed
	StatusInfo       string  `json:"status_info,omitempty"`
	State            string  `json:"state,omitempty"`             // online, offline, warning, error
	PublicIP         string  `json:"public_ip,omitempty"`         // IP/Domain of connecting system
	RegistrationCode string  `json:"registration_code,omitempty"` // Generated registration code
	ForceTier        string  `json:"force_tier,omitempty"`        // unspecified, 1-5
	VSANHost         string  `json:"vsan_host,omitempty"`
	VSANPort         int     `json:"vsan_port,omitempty"`
	RequestURL       string  `json:"request_url,omitempty"`
	MinSnapshots     int     `json:"min_snapshots,omitempty"` // Minimum snapshots to retain
	LastSync         int64   `json:"last_sync,omitempty"`
	SystemCreated    bool    `json:"system_created,omitempty"` // Auto-created by system
}

// SiteSyncIncomingCreateRequest represents the request body for creating an incoming sync.
type SiteSyncIncomingCreateRequest struct {
	Site         int     `json:"site"` // Required: parent site ID
	Name         string  `json:"name"` // Required
	Description  string  `json:"description,omitempty"`
	Enabled      *bool   `json:"enabled,omitempty"`
	PublicIP     *string `json:"public_ip,omitempty"`
	ForceTier    *string `json:"force_tier,omitempty"` // unspecified, 1-5
	VSANHost     *string `json:"vsan_host,omitempty"`
	VSANPort     *int    `json:"vsan_port,omitempty"`
	RequestURL   *string `json:"request_url,omitempty"`
	MinSnapshots *int    `json:"min_snapshots,omitempty"`
}

// SiteSyncIncomingUpdateRequest represents the request body for updating an incoming sync.
type SiteSyncIncomingUpdateRequest struct {
	Name         *string `json:"name,omitempty"`
	Description  *string `json:"description,omitempty"`
	Enabled      *bool   `json:"enabled,omitempty"`
	PublicIP     *string `json:"public_ip,omitempty"`
	ForceTier    *string `json:"force_tier,omitempty"`
	VSANHost     *string `json:"vsan_host,omitempty"`
	VSANPort     *int    `json:"vsan_port,omitempty"`
	RequestURL   *string `json:"request_url,omitempty"`
	MinSnapshots *int    `json:"min_snapshots,omitempty"`
}

// siteSyncIncomingAction represents an action request for an incoming sync.
type siteSyncIncomingAction struct {
	SiteSyncIncoming int                    `json:"site_syncs_incoming"`
	Action           string                 `json:"action"`
	Params           map[string]interface{} `json:"params,omitempty"`
}

// Incoming sync status values
const (
	SiteSyncIncomingStatusGeneratingReg      string = "generating_reg"
	SiteSyncIncomingStatusSyncing            string = "syncing"
	SiteSyncIncomingStatusOffline            string = "offline"
	SiteSyncIncomingStatusError              string = "error"
	SiteSyncIncomingStatusRegenerationNeeded string = "regeneration_needed"
)

// siteSyncIncomingListFields defines the default fields for listing incoming syncs.
const siteSyncIncomingListFields = "$key,site,name,sync_id,description,enabled,status,status_info,state,public_ip,registration_code,force_tier,last_sync,system_created"

// siteSyncIncomingGetFields defines the default fields for getting a single incoming sync.
const siteSyncIncomingGetFields = "$key,site,name,sync_id,description,enabled,status,status_info,state,public_ip,registration_code,force_tier,vsan_host,vsan_port,request_url,min_snapshots,last_sync,system_created"

// SiteSyncOutgoing represents an outgoing sync configuration to a remote site.
type SiteSyncOutgoing struct {
	Key                    FlexInt `json:"$key,omitempty"`
	Site                   FlexInt `json:"site,omitempty"`
	Name                   string  `json:"name,omitempty"`
	Description            string  `json:"description,omitempty"`
	Enabled                bool    `json:"enabled,omitempty"`
	Status                 string  `json:"status,omitempty"` // initializing, syncing, offline, error
	StatusInfo             string  `json:"status_info,omitempty"`
	State                  string  `json:"state,omitempty"` // online, offline, warning, error
	URL                    string  `json:"url,omitempty"`   // Remote URL
	RegistrationCode       string  `json:"registration_code,omitempty"`
	User                   string  `json:"user,omitempty"`             // Site user
	DestinationTier        string  `json:"destination_tier,omitempty"` // unspecified, 1-5
	RemoteSiteID           string  `json:"remote_site_id,omitempty"`
	RemoteVSANHost         string  `json:"remote_vsan_host,omitempty"`
	RemoteVSANPort         int     `json:"remote_vsan_port,omitempty"`
	RemoteSyncID           string  `json:"remote_sync_id,omitempty"`
	RemoteMinSnapshots     int     `json:"remote_min_snapshots,omitempty"`
	Threads                int     `json:"threads,omitempty"`           // Data threads (1-32, default 8)
	FileThreads            int     `json:"file_threads,omitempty"`      // File scanning threads (1-64, default 4)
	Encryption             bool    `json:"encryption,omitempty"`        // Default true
	Compression            bool    `json:"compression,omitempty"`       // Default true
	NetInteg               bool    `json:"netinteg,omitempty"`          // Checksum network traffic
	SendThrottle           int     `json:"sendthrottle,omitempty"`      // 0 = disabled
	QueueRetryCount        int     `json:"queue_retry_count,omitempty"` // Default 10
	QueueRetryIntervalSec  int     `json:"queue_retry_interval_seconds,omitempty"`
	QueueRetryIntervalMult bool    `json:"queue_retry_interval_multiplier,omitempty"`
	RemoteSnapsLastRefresh int64   `json:"remote_snaps_last_refresh,omitempty"`
	RemoteSnapsStatus      string  `json:"remote_snaps_status,omitempty"` // idle, unsupported, error, refreshing, updating
	RemoteSnapsStatusInfo  string  `json:"remote_snaps_status_info,omitempty"`
	LastRun                int64   `json:"last_run,omitempty"`
	Note                   string  `json:"note,omitempty"`
}

// SiteSyncOutgoingCreateRequest represents the request body for creating an outgoing sync.
type SiteSyncOutgoingCreateRequest struct {
	Site                   int     `json:"site"` // Required: parent site ID
	Name                   string  `json:"name"` // Required
	Description            string  `json:"description,omitempty"`
	Enabled                *bool   `json:"enabled,omitempty"`
	URL                    *string `json:"url,omitempty"`
	RegistrationCode       string  `json:"registration_code,omitempty"` // From incoming sync
	DestinationTier        *string `json:"destination_tier,omitempty"`  // unspecified, 1-5
	Threads                *int    `json:"threads,omitempty"`
	FileThreads            *int    `json:"file_threads,omitempty"`
	Encryption             *bool   `json:"encryption,omitempty"`
	Compression            *bool   `json:"compression,omitempty"`
	NetInteg               *bool   `json:"netinteg,omitempty"`
	SendThrottle           *int    `json:"sendthrottle,omitempty"`
	QueueRetryCount        *int    `json:"queue_retry_count,omitempty"`
	QueueRetryIntervalSec  *int    `json:"queue_retry_interval_seconds,omitempty"`
	QueueRetryIntervalMult *bool   `json:"queue_retry_interval_multiplier,omitempty"`
	Note                   *string `json:"note,omitempty"`
}

// SiteSyncOutgoingUpdateRequest represents the request body for updating an outgoing sync.
type SiteSyncOutgoingUpdateRequest struct {
	Name                   *string `json:"name,omitempty"`
	Description            *string `json:"description,omitempty"`
	Enabled                *bool   `json:"enabled,omitempty"`
	URL                    *string `json:"url,omitempty"`
	DestinationTier        *string `json:"destination_tier,omitempty"`
	Threads                *int    `json:"threads,omitempty"`
	FileThreads            *int    `json:"file_threads,omitempty"`
	Encryption             *bool   `json:"encryption,omitempty"`
	Compression            *bool   `json:"compression,omitempty"`
	NetInteg               *bool   `json:"netinteg,omitempty"`
	SendThrottle           *int    `json:"sendthrottle,omitempty"`
	QueueRetryCount        *int    `json:"queue_retry_count,omitempty"`
	QueueRetryIntervalSec  *int    `json:"queue_retry_interval_seconds,omitempty"`
	QueueRetryIntervalMult *bool   `json:"queue_retry_interval_multiplier,omitempty"`
	Note                   *string `json:"note,omitempty"`
}

// siteSyncOutgoingAction represents an action request for an outgoing sync.
type siteSyncOutgoingAction struct {
	SiteSyncOutgoing int                    `json:"site_syncs_outgoing"`
	Action           string                 `json:"action"`
	Params           map[string]interface{} `json:"params,omitempty"`
}

// Outgoing sync status values
const (
	SiteSyncOutgoingStatusInitializing string = "initializing"
	SiteSyncOutgoingStatusSyncing      string = "syncing"
	SiteSyncOutgoingStatusOffline      string = "offline"
	SiteSyncOutgoingStatusError        string = "error"
)

// siteSyncOutgoingListFields defines the default fields for listing outgoing syncs.
const siteSyncOutgoingListFields = "$key,site,name,description,enabled,status,status_info,state,url,destination_tier,encryption,compression,sendthrottle,last_run"

// siteSyncOutgoingGetFields defines the default fields for getting a single outgoing sync.
const siteSyncOutgoingGetFields = "$key,site,name,description,enabled,status,status_info,state,url,registration_code,user,destination_tier,remote_site_id,remote_vsan_host,remote_vsan_port,remote_sync_id,remote_min_snapshots,threads,file_threads,encryption,compression,netinteg,sendthrottle,queue_retry_count,queue_retry_interval_seconds,queue_retry_interval_multiplier,remote_snaps_last_refresh,remote_snaps_status,remote_snaps_status_info,last_run,note"

// Tier options for site syncs
const (
	SiteSyncTierUnspecified string = "unspecified"
	SiteSyncTier1           string = "1"
	SiteSyncTier2           string = "2"
	SiteSyncTier3           string = "3"
	SiteSyncTier4           string = "4"
	SiteSyncTier5           string = "5"
)

// SiteSyncProfilePeriod represents a schedule period for outgoing site syncs.
// This configures when snapshots are synced to remote sites based on snapshot profile periods.
type SiteSyncProfilePeriod struct {
	// Key is the unique identifier for this profile period.
	Key FlexInt `json:"$key,omitempty"`
	// SiteSyncsOutgoing is the parent outgoing sync ID.
	SiteSyncsOutgoing FlexInt `json:"site_syncs_outgoing,omitempty"`
	// ProfilePeriod is the snapshot profile period ID (FK to snapshot_profile_periods).
	ProfilePeriod FlexInt `json:"profile_period,omitempty"`
	// ScheduleTask is the associated schedule task ID.
	ScheduleTask FlexInt `json:"schedule_task,omitempty"`
	// Task is the associated task ID.
	Task FlexInt `json:"task,omitempty"`
	// Retention is the remote retention in seconds.
	Retention int `json:"retention,omitempty"`
	// Priority is the sync priority (lower = higher priority).
	Priority int `json:"priority,omitempty"`
	// DoNotExpire prevents the source snapshot from expiring until sent.
	DoNotExpire bool `json:"do_not_expire,omitempty"`
	// DestinationPrefix is prefixed to the snapshot name on the destination.
	DestinationPrefix string `json:"destination_prefix,omitempty"`
}

// SiteSyncProfilePeriodCreateRequest represents the request body for creating a site sync profile period.
type SiteSyncProfilePeriodCreateRequest struct {
	// SiteSyncsOutgoing is the parent outgoing sync ID (required).
	SiteSyncsOutgoing int `json:"site_syncs_outgoing"`
	// ProfilePeriod is the snapshot profile period ID (required).
	ProfilePeriod int `json:"profile_period"`
	// Retention is the remote retention in seconds (required).
	Retention int `json:"retention"`
	// Priority is the sync priority.
	Priority *int `json:"priority,omitempty"`
	// DoNotExpire prevents the source snapshot from expiring until sent.
	DoNotExpire *bool `json:"do_not_expire,omitempty"`
	// DestinationPrefix is prefixed to the snapshot name on the destination.
	DestinationPrefix *string `json:"destination_prefix,omitempty"`
}

// SiteSyncProfilePeriodUpdateRequest represents the request body for updating a site sync profile period.
type SiteSyncProfilePeriodUpdateRequest struct {
	// Retention is the remote retention in seconds.
	Retention *int `json:"retention,omitempty"`
	// Priority is the sync priority.
	Priority *int `json:"priority,omitempty"`
	// DoNotExpire prevents the source snapshot from expiring until sent.
	DoNotExpire *bool `json:"do_not_expire,omitempty"`
	// DestinationPrefix is prefixed to the snapshot name on the destination.
	DestinationPrefix *string `json:"destination_prefix,omitempty"`
}

// siteSyncProfilePeriodListFields defines the default fields for listing site sync profile periods.
const siteSyncProfilePeriodListFields = "$key,site_syncs_outgoing,profile_period,schedule_task,task,retention,priority,do_not_expire,destination_prefix"

// siteSyncProfilePeriodGetFields defines the default fields for getting a single site sync profile period.
const siteSyncProfilePeriodGetFields = siteSyncProfilePeriodListFields
