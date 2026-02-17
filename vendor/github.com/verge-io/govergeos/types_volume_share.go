package vergeos

// VolumeCIFSShare represents a CIFS (SMB) share on a NAS volume.
// Note: Like volumes, CIFS shares use SHA1 hash strings as their key.
type VolumeCIFSShare struct {
	// Key is the unique SHA1 hash key (same as ID for shares).
	Key string `json:"$key,omitempty"`
	// ID is the unique SHA1 hash identifier (same as Key).
	ID string `json:"id,omitempty"`
	// Name is the share name (required, unique within volume).
	Name string `json:"name"`
	// Volume is the parent volume SHA1 key.
	Volume string `json:"volume,omitempty"`
	// Description is an optional description of the share.
	Description string `json:"description,omitempty"`
	// Enabled indicates whether the share is enabled.
	Enabled bool `json:"enabled"`
	// Created is the creation timestamp (Unix epoch).
	Created int64 `json:"created,omitempty"`
	// Modified is the last modified timestamp (Unix epoch).
	Modified int64 `json:"modified,omitempty"`

	// Share configuration
	// SharePath is the path within the volume to share (empty = entire volume).
	SharePath string `json:"share_path,omitempty"`
	// Comment is a short comment about the share (max 64 chars).
	Comment string `json:"comment,omitempty"`

	// Access control - user/group lists (newline-delimited)
	// ValidUsers is the list of users who can connect (one per line).
	ValidUsers string `json:"valid_users,omitempty"`
	// ValidGroups is the list of groups who can connect (one per line).
	ValidGroups string `json:"valid_groups,omitempty"`
	// AdminUsers is the list of users with admin access (one per line).
	AdminUsers string `json:"admin_users,omitempty"`
	// AdminGroups is the list of groups with admin access (one per line).
	AdminGroups string `json:"admin_groups,omitempty"`

	// Host-based access control (newline-delimited)
	// HostAllow is the list of allowed hosts (one per line).
	HostAllow string `json:"host_allow,omitempty"`
	// HostDeny is the list of denied hosts (one per line).
	HostDeny string `json:"host_deny,omitempty"`

	// User/group mapping
	// ForceUser forces all operations as this user.
	ForceUser string `json:"force_user,omitempty"`
	// ForceGroup sets the default primary group for all users.
	ForceGroup string `json:"force_group,omitempty"`

	// Share options
	// Browseable indicates whether the share is visible in network browsing.
	Browseable bool `json:"browseable,omitempty"`
	// ReadOnly indicates whether the share is read-only.
	ReadOnly bool `json:"read_only,omitempty"`
	// GuestOK indicates whether guest access is allowed.
	GuestOK bool `json:"guest_ok,omitempty"`
	// GuestOnly indicates whether only guest connections are permitted.
	GuestOnly bool `json:"guest_only,omitempty"`

	// Advanced configuration
	// Advanced contains additional smb.conf share section options.
	Advanced string `json:"advanced,omitempty"`
	// VFSShadowCopy2 enables Windows "Previous Versions" via mounted snapshots.
	VFSShadowCopy2 bool `json:"vfs_shadow_copy2,omitempty"`

	// Status is the share status reference (read-only).
	Status FlexInt `json:"status,omitempty"`
}

// VolumeCIFSShareCreateRequest is the request body for creating a CIFS share.
type VolumeCIFSShareCreateRequest struct {
	// Name is the share name (required).
	Name string `json:"name"`
	// Volume is the parent volume SHA1 key (required).
	Volume string `json:"volume"`
	// Description is an optional description.
	Description string `json:"description,omitempty"`
	// Enabled indicates whether the share is enabled (default: true).
	Enabled *bool `json:"enabled,omitempty"`

	// Share configuration
	// SharePath is the path within the volume to share.
	SharePath *string `json:"share_path,omitempty"`
	// Comment is a short comment about the share.
	Comment *string `json:"comment,omitempty"`

	// Access control
	// ValidUsers is the list of users who can connect (newline-delimited).
	ValidUsers *string `json:"valid_users,omitempty"`
	// ValidGroups is the list of groups who can connect (newline-delimited).
	ValidGroups *string `json:"valid_groups,omitempty"`
	// AdminUsers is the list of admin users (newline-delimited).
	AdminUsers *string `json:"admin_users,omitempty"`
	// AdminGroups is the list of admin groups (newline-delimited).
	AdminGroups *string `json:"admin_groups,omitempty"`
	// HostAllow is the list of allowed hosts (newline-delimited).
	HostAllow *string `json:"host_allow,omitempty"`
	// HostDeny is the list of denied hosts (newline-delimited).
	HostDeny *string `json:"host_deny,omitempty"`

	// User/group mapping
	// ForceUser forces all operations as this user.
	ForceUser *string `json:"force_user,omitempty"`
	// ForceGroup sets the default primary group.
	ForceGroup *string `json:"force_group,omitempty"`

	// Share options
	// Browseable indicates whether the share is visible.
	Browseable *bool `json:"browseable,omitempty"`
	// ReadOnly indicates whether the share is read-only.
	ReadOnly *bool `json:"read_only,omitempty"`
	// GuestOK indicates whether guest access is allowed.
	GuestOK *bool `json:"guest_ok,omitempty"`
	// GuestOnly indicates whether only guest connections are permitted.
	GuestOnly *bool `json:"guest_only,omitempty"`

	// Advanced configuration
	// Advanced contains additional smb.conf options.
	Advanced *string `json:"advanced,omitempty"`
	// VFSShadowCopy2 enables Windows "Previous Versions".
	VFSShadowCopy2 *bool `json:"vfs_shadow_copy2,omitempty"`
}

// VolumeCIFSShareUpdateRequest is the request body for updating a CIFS share.
type VolumeCIFSShareUpdateRequest struct {
	// Name is the share name.
	Name *string `json:"name,omitempty"`
	// Description is the share description.
	Description *string `json:"description,omitempty"`
	// Enabled indicates whether the share is enabled.
	Enabled *bool `json:"enabled,omitempty"`

	// Share configuration
	// SharePath is the path within the volume.
	SharePath *string `json:"share_path,omitempty"`
	// Comment is a short comment.
	Comment *string `json:"comment,omitempty"`

	// Access control
	// ValidUsers is the list of users who can connect.
	ValidUsers *string `json:"valid_users,omitempty"`
	// ValidGroups is the list of groups who can connect.
	ValidGroups *string `json:"valid_groups,omitempty"`
	// AdminUsers is the list of admin users.
	AdminUsers *string `json:"admin_users,omitempty"`
	// AdminGroups is the list of admin groups.
	AdminGroups *string `json:"admin_groups,omitempty"`
	// HostAllow is the list of allowed hosts.
	HostAllow *string `json:"host_allow,omitempty"`
	// HostDeny is the list of denied hosts.
	HostDeny *string `json:"host_deny,omitempty"`

	// User/group mapping
	// ForceUser forces all operations as this user.
	ForceUser *string `json:"force_user,omitempty"`
	// ForceGroup sets the default primary group.
	ForceGroup *string `json:"force_group,omitempty"`

	// Share options
	// Browseable indicates whether the share is visible.
	Browseable *bool `json:"browseable,omitempty"`
	// ReadOnly indicates whether the share is read-only.
	ReadOnly *bool `json:"read_only,omitempty"`
	// GuestOK indicates whether guest access is allowed.
	GuestOK *bool `json:"guest_ok,omitempty"`
	// GuestOnly indicates whether only guest connections are permitted.
	GuestOnly *bool `json:"guest_only,omitempty"`

	// Advanced configuration
	// Advanced contains additional smb.conf options.
	Advanced *string `json:"advanced,omitempty"`
	// VFSShadowCopy2 enables Windows "Previous Versions".
	VFSShadowCopy2 *bool `json:"vfs_shadow_copy2,omitempty"`
}

// VolumeNFSShare represents an NFS share on a NAS volume.
// Note: Like volumes, NFS shares use SHA1 hash strings as their key.
type VolumeNFSShare struct {
	// Key is the unique SHA1 hash key (same as ID for shares).
	Key string `json:"$key,omitempty"`
	// ID is the unique SHA1 hash identifier (same as Key).
	ID string `json:"id,omitempty"`
	// Name is the share name (required, unique within volume).
	Name string `json:"name"`
	// Volume is the parent volume SHA1 key.
	Volume string `json:"volume,omitempty"`
	// Description is an optional description of the share.
	Description string `json:"description,omitempty"`
	// Enabled indicates whether the share is enabled.
	Enabled bool `json:"enabled"`
	// Created is the creation timestamp (Unix epoch).
	Created int64 `json:"created,omitempty"`
	// Modified is the last modified timestamp (Unix epoch).
	Modified int64 `json:"modified,omitempty"`

	// Share configuration
	// SharePath is the path within the volume to share (empty = entire volume).
	SharePath string `json:"share_path,omitempty"`

	// Host access control
	// AllowedHosts is a comma-delimited list of allowed hosts/networks.
	AllowedHosts string `json:"allowed_hosts,omitempty"`
	// AllowAll indicates whether everyone can access the share.
	AllowAll bool `json:"allow_all,omitempty"`

	// NFS options
	// FSID is the filesystem ID (must be unique; valid: number|root|uuid).
	FSID string `json:"fsid,omitempty"`
	// AnonUID is the anonymous user ID (default: 65534).
	AnonUID string `json:"anonuid,omitempty"`
	// AnonGID is the anonymous group ID (default: 65534).
	AnonGID string `json:"anongid,omitempty"`

	// Security and performance options
	// NoACL disables access control lists.
	NoACL bool `json:"no_acl,omitempty"`
	// Insecure removes restriction that requests originate on port < 1024.
	Insecure bool `json:"insecure,omitempty"`
	// Async improves performance but risks data loss on unclean restart.
	Async bool `json:"async,omitempty"`

	// User/group squashing
	// Squash controls user/group ID mapping: root_squash, all_squash, no_root_squash.
	Squash string `json:"squash,omitempty"`

	// Data access
	// DataAccess controls read/write permissions: ro, rw.
	DataAccess string `json:"data_access,omitempty"`

	// Status is the share status reference (read-only).
	Status FlexInt `json:"status,omitempty"`
}

// NFS squash options
const (
	NFSSquashRoot = "root_squash"    // Squash root user to anonymous
	NFSSquashAll  = "all_squash"     // Squash all users to anonymous
	NFSSquashNone = "no_root_squash" // No squashing (root has full access)
)

// NFS data access options
const (
	NFSAccessReadOnly  = "ro" // Read-only access
	NFSAccessReadWrite = "rw" // Read and write access
)

// VolumeNFSShareCreateRequest is the request body for creating an NFS share.
type VolumeNFSShareCreateRequest struct {
	// Name is the share name (required).
	Name string `json:"name"`
	// Volume is the parent volume SHA1 key (required).
	Volume string `json:"volume"`
	// Description is an optional description.
	Description string `json:"description,omitempty"`
	// Enabled indicates whether the share is enabled (default: true).
	Enabled *bool `json:"enabled,omitempty"`

	// Share configuration
	// SharePath is the path within the volume to share.
	SharePath *string `json:"share_path,omitempty"`

	// Host access control
	// AllowedHosts is a comma-delimited list of allowed hosts/networks.
	AllowedHosts *string `json:"allowed_hosts,omitempty"`
	// AllowAll indicates whether everyone can access the share.
	AllowAll *bool `json:"allow_all,omitempty"`

	// NFS options
	// FSID is the filesystem ID.
	FSID *string `json:"fsid,omitempty"`
	// AnonUID is the anonymous user ID.
	AnonUID *string `json:"anonuid,omitempty"`
	// AnonGID is the anonymous group ID.
	AnonGID *string `json:"anongid,omitempty"`

	// Security and performance options
	// NoACL disables access control lists.
	NoACL *bool `json:"no_acl,omitempty"`
	// Insecure removes the port < 1024 restriction.
	Insecure *bool `json:"insecure,omitempty"`
	// Async enables async mode for performance.
	Async *bool `json:"async,omitempty"`

	// User/group squashing
	// Squash controls user/group ID mapping (default: root_squash).
	Squash *string `json:"squash,omitempty"`

	// Data access
	// DataAccess controls read/write permissions (default: ro).
	DataAccess *string `json:"data_access,omitempty"`
}

// VolumeNFSShareUpdateRequest is the request body for updating an NFS share.
type VolumeNFSShareUpdateRequest struct {
	// Name is the share name.
	Name *string `json:"name,omitempty"`
	// Description is the share description.
	Description *string `json:"description,omitempty"`
	// Enabled indicates whether the share is enabled.
	Enabled *bool `json:"enabled,omitempty"`

	// Share configuration
	// SharePath is the path within the volume.
	SharePath *string `json:"share_path,omitempty"`

	// Host access control
	// AllowedHosts is a comma-delimited list of allowed hosts/networks.
	AllowedHosts *string `json:"allowed_hosts,omitempty"`
	// AllowAll indicates whether everyone can access the share.
	AllowAll *bool `json:"allow_all,omitempty"`

	// NFS options
	// FSID is the filesystem ID.
	FSID *string `json:"fsid,omitempty"`
	// AnonUID is the anonymous user ID.
	AnonUID *string `json:"anonuid,omitempty"`
	// AnonGID is the anonymous group ID.
	AnonGID *string `json:"anongid,omitempty"`

	// Security and performance options
	// NoACL disables access control lists.
	NoACL *bool `json:"no_acl,omitempty"`
	// Insecure removes the port < 1024 restriction.
	Insecure *bool `json:"insecure,omitempty"`
	// Async enables async mode for performance.
	Async *bool `json:"async,omitempty"`

	// User/group squashing
	// Squash controls user/group ID mapping.
	Squash *string `json:"squash,omitempty"`

	// Data access
	// DataAccess controls read/write permissions.
	DataAccess *string `json:"data_access,omitempty"`
}

// Field list constants for CIFS shares
const cifsShareListFields = "$key,id,name,volume,description,enabled,created,modified,share_path,comment,valid_users,valid_groups,admin_users,admin_groups,host_allow,host_deny,force_user,force_group,browseable,read_only,guest_ok,guest_only,vfs_shadow_copy2,status"
const cifsShareGetFields = cifsShareListFields + ",advanced"

// Field list constants for NFS shares
const nfsShareListFields = "$key,id,name,volume,description,enabled,created,modified,share_path,allowed_hosts,allow_all,fsid,anonuid,anongid,no_acl,insecure,async,squash,data_access,status"
const nfsShareGetFields = nfsShareListFields
