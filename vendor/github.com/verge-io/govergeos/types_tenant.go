package vergeos

import "encoding/json"

// Tenant represents a VergeOS tenant (virtual data center).
// Tenants are isolated environments that can contain their own VMs, networks, and storage.
type Tenant struct {
	// Key is the unique identifier for the tenant.
	Key FlexInt `json:"$key,omitempty"`
	// Name is the tenant name (unique, required).
	Name string `json:"name"`
	// Description is an optional description of the tenant.
	Description string `json:"description,omitempty"`
	// URL is the optional URL for the tenant.
	URL string `json:"url,omitempty"`
	// UUID is the unique identifier (readonly, auto-generated).
	UUID string `json:"uuid,omitempty"`
	// VNet is the auto-created network ID for the tenant (readonly).
	VNet FlexInt `json:"vnet,omitempty"`
	// UIAddress is the IP address row ID for the tenant UI (readonly).
	UIAddress FlexInt `json:"ui_address,omitempty"`
	// OIDCApplication is the OIDC application ID for SSO configuration.
	OIDCApplication *FlexInt `json:"oidc_application,omitempty"`
	// IsSnapshot indicates whether this is a snapshot of a tenant.
	IsSnapshot bool `json:"is_snapshot,omitempty"`
	// Isolate indicates whether network isolation is enabled (readonly).
	Isolate bool `json:"isolate,omitempty"`
	// ExposeCloudSnapshots allows the tenant to request system snapshots.
	ExposeCloudSnapshots bool `json:"expose_cloud_snapshots,omitempty"`
	// AllowBranding allows the tenant to customize colors and logo.
	AllowBranding bool `json:"allow_branding,omitempty"`
	// ChangePassword requires password change on first login.
	ChangePassword bool `json:"change_password,omitempty"`
	// ThemeAccess determines theme visibility (specified, host_only, local_only, both).
	ThemeAccess string `json:"theme_access,omitempty"`
	// HelpURL is a custom help URL (blank to disable, "default" for default).
	HelpURL string `json:"help_url,omitempty"`
	// Note is a free-form note about the tenant.
	Note string `json:"note,omitempty"`
	// Meta stores arbitrary JSON metadata.
	Meta json.RawMessage `json:"meta,omitempty"`
	// Creator is the username that created this tenant (readonly).
	Creator string `json:"creator,omitempty"`
	// Created is the creation timestamp (Unix epoch, readonly).
	Created int64 `json:"created,omitempty"`
}

// TenantCreateRequest is the request body for creating a tenant.
type TenantCreateRequest struct {
	// Name is the tenant name (required, unique).
	Name string `json:"name"`
	// Password is the admin user password for the tenant.
	Password string `json:"password,omitempty"`
	// Description is an optional description.
	Description string `json:"description,omitempty"`
	// URL is the optional URL for the tenant.
	URL string `json:"url,omitempty"`
	// OIDCApplication is the OIDC application ID for SSO.
	OIDCApplication *int `json:"oidc_application,omitempty"`
	// ExposeCloudSnapshots allows the tenant to request system snapshots.
	ExposeCloudSnapshots *bool `json:"expose_cloud_snapshots,omitempty"`
	// AllowBranding allows the tenant to customize colors and logo.
	AllowBranding *bool `json:"allow_branding,omitempty"`
	// ChangePassword requires password change on first login.
	ChangePassword *bool `json:"change_password,omitempty"`
	// ThemeAccess determines theme visibility.
	ThemeAccess *string `json:"theme_access,omitempty"`
	// HelpURL is a custom help URL.
	HelpURL *string `json:"help_url,omitempty"`
	// Note is a free-form note.
	Note *string `json:"note,omitempty"`
}

// TenantUpdateRequest is the request body for updating a tenant.
type TenantUpdateRequest struct {
	// Name is the tenant name.
	Name *string `json:"name,omitempty"`
	// Password is the admin user password.
	Password *string `json:"password,omitempty"`
	// Description is the tenant description.
	Description *string `json:"description,omitempty"`
	// URL is the tenant URL.
	URL *string `json:"url,omitempty"`
	// OIDCApplication is the OIDC application ID.
	OIDCApplication *int `json:"oidc_application,omitempty"`
	// ExposeCloudSnapshots allows the tenant to request system snapshots.
	ExposeCloudSnapshots *bool `json:"expose_cloud_snapshots,omitempty"`
	// AllowBranding allows the tenant to customize colors and logo.
	AllowBranding *bool `json:"allow_branding,omitempty"`
	// ThemeAccess determines theme visibility.
	ThemeAccess *string `json:"theme_access,omitempty"`
	// HelpURL is a custom help URL.
	HelpURL *string `json:"help_url,omitempty"`
	// Note is a free-form note.
	Note *string `json:"note,omitempty"`
}

// TenantCloneOptions are options for cloning a tenant.
type TenantCloneOptions struct {
	// Name is the name for the cloned tenant (required).
	Name string `json:"name"`
	// NoVNet skips cloning the network.
	NoVNet bool `json:"no_vnet,omitempty"`
	// NoStorage skips cloning the storage.
	NoStorage bool `json:"no_storage,omitempty"`
	// NoNodes skips cloning the nodes.
	NoNodes bool `json:"no_nodes,omitempty"`
}

// tenantListFields are the fields to request when listing tenants.
const tenantListFields = "$key,name,description,url,uuid,vnet,ui_address,oidc_application,is_snapshot,isolate,expose_cloud_snapshots,allow_branding,change_password,theme_access,help_url,note,creator,created"

// tenantGetFields are the fields to request when getting a single tenant.
const tenantGetFields = tenantListFields

// TenantNode represents a VergeOS tenant node.
// Tenant nodes are virtual nodes that run within a tenant environment.
type TenantNode struct {
	// Key is the unique identifier for the tenant node.
	Key FlexInt `json:"$key,omitempty"`
	// Tenant is the parent tenant ID (readonly).
	Tenant FlexInt `json:"tenant,omitempty"`
	// NodeID is the node identifier within the tenant (1-65535).
	NodeID int `json:"nodeid,omitempty"`
	// Name is the node name (unique within tenant).
	Name string `json:"name"`
	// Description is an optional description.
	Description string `json:"description,omitempty"`
	// Enabled indicates whether the node is enabled.
	Enabled bool `json:"enabled,omitempty"`
	// Machine is the underlying machine row ID (readonly).
	Machine FlexInt `json:"machine,omitempty"`
	// IsSnapshot indicates whether this is a snapshot.
	IsSnapshot bool `json:"is_snapshot,omitempty"`
	// CPUCores is the number of CPU cores allocated.
	CPUCores int `json:"cpu_cores,omitempty"`
	// RAM is the amount of RAM in MB.
	RAM int `json:"ram,omitempty"`
	// Cluster is the target cluster ID.
	Cluster *FlexInt `json:"cluster,omitempty"`
	// ClusterFailover is the failover cluster ID.
	ClusterFailover *FlexInt `json:"cluster_failover,omitempty"`
	// PreferredNode is the preferred host node ID.
	PreferredNode *FlexInt `json:"preferred_node,omitempty"`
	// HAGroup is the high-availability group name.
	HAGroup string `json:"ha_group,omitempty"`
	// OnPowerLoss determines behavior on power loss (power_on, last_state, leave_off).
	OnPowerLoss string `json:"on_power_loss,omitempty"`
	// Creator is the username that created this node (readonly).
	Creator string `json:"creator,omitempty"`
	// Created is the creation timestamp (Unix epoch, readonly).
	Created int64 `json:"created,omitempty"`
	// Modified is the last modification timestamp (Unix epoch, readonly).
	Modified int64 `json:"modified,omitempty"`
}

// TenantNodeCreateRequest is the request body for creating a tenant node.
type TenantNodeCreateRequest struct {
	// Tenant is the parent tenant ID (required).
	Tenant int `json:"tenant"`
	// Name is the node name (optional, auto-generated if not provided).
	Name string `json:"name,omitempty"`
	// Description is an optional description.
	Description string `json:"description,omitempty"`
	// Enabled indicates whether the node is enabled.
	Enabled *bool `json:"enabled,omitempty"`
	// CPUCores is the number of CPU cores (required).
	CPUCores int `json:"cpu_cores"`
	// RAM is the amount of RAM in MB (required, minimum 2048).
	RAM int `json:"ram"`
	// Cluster is the target cluster ID.
	Cluster *int `json:"cluster,omitempty"`
	// ClusterFailover is the failover cluster ID.
	ClusterFailover *int `json:"cluster_failover,omitempty"`
	// PreferredNode is the preferred host node ID.
	PreferredNode *int `json:"preferred_node,omitempty"`
	// HAGroup is the high-availability group name.
	HAGroup *string `json:"ha_group,omitempty"`
	// OnPowerLoss determines behavior on power loss.
	OnPowerLoss *string `json:"on_power_loss,omitempty"`
}

// TenantNodeUpdateRequest is the request body for updating a tenant node.
type TenantNodeUpdateRequest struct {
	// Name is the node name.
	Name *string `json:"name,omitempty"`
	// Description is the node description.
	Description *string `json:"description,omitempty"`
	// Enabled indicates whether the node is enabled.
	Enabled *bool `json:"enabled,omitempty"`
	// CPUCores is the number of CPU cores.
	CPUCores *int `json:"cpu_cores,omitempty"`
	// RAM is the amount of RAM in MB.
	RAM *int `json:"ram,omitempty"`
	// Cluster is the target cluster ID.
	Cluster *int `json:"cluster,omitempty"`
	// ClusterFailover is the failover cluster ID.
	ClusterFailover *int `json:"cluster_failover,omitempty"`
	// PreferredNode is the preferred host node ID.
	PreferredNode *int `json:"preferred_node,omitempty"`
	// HAGroup is the high-availability group name.
	HAGroup *string `json:"ha_group,omitempty"`
	// OnPowerLoss determines behavior on power loss.
	OnPowerLoss *string `json:"on_power_loss,omitempty"`
}

// tenantNodeListFields are the fields to request when listing tenant nodes.
const tenantNodeListFields = "$key,tenant,nodeid,name,description,enabled,machine,is_snapshot,cpu_cores,ram,cluster,cluster_failover,preferred_node,ha_group,on_power_loss,creator,created,modified"

// tenantNodeGetFields are the fields to request when getting a single tenant node.
const tenantNodeGetFields = tenantNodeListFields

// TenantStorage represents a VergeOS tenant storage allocation.
// Each tenant can have storage allocated from different storage tiers.
type TenantStorage struct {
	// Key is the unique identifier for the storage allocation.
	Key FlexInt `json:"$key,omitempty"`
	// Tenant is the parent tenant ID.
	Tenant FlexInt `json:"tenant,omitempty"`
	// Tier is the storage tier ID (readonly after creation).
	Tier FlexInt `json:"tier,omitempty"`
	// Provisioned is the provisioned storage in bytes.
	Provisioned int64 `json:"provisioned,omitempty"`
	// Used is the used storage in bytes (readonly).
	Used int64 `json:"used,omitempty"`
	// Allocated is the allocated storage in bytes (readonly).
	Allocated int64 `json:"allocated,omitempty"`
	// UsedPct is the percentage of provisioned storage used (readonly).
	UsedPct int `json:"used_pct,omitempty"`
	// LastUpdate is the last update timestamp (Unix epoch, readonly).
	LastUpdate int64 `json:"last_update,omitempty"`
}

// TenantStorageCreateRequest is the request body for creating a tenant storage allocation.
type TenantStorageCreateRequest struct {
	// Tenant is the parent tenant ID (required).
	Tenant int `json:"tenant"`
	// Tier is the storage tier ID (required).
	Tier int `json:"tier"`
	// Provisioned is the provisioned storage in bytes (required).
	Provisioned int64 `json:"provisioned"`
}

// TenantStorageUpdateRequest is the request body for updating a tenant storage allocation.
type TenantStorageUpdateRequest struct {
	// Provisioned is the provisioned storage in bytes.
	Provisioned *int64 `json:"provisioned,omitempty"`
}

// tenantStorageListFields are the fields to request when listing tenant storage.
const tenantStorageListFields = "$key,tenant,tier,provisioned,used,allocated,used_pct,last_update"

// tenantStorageGetFields are the fields to request when getting a single storage allocation.
const tenantStorageGetFields = tenantStorageListFields
