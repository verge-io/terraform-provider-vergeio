package vergeos

// NASService represents a VergeOS NAS service (VM providing NAS functionality).
// A NAS service is a specialized VM that hosts volumes, shares, and sync operations.
type NASService struct {
	// Key is the unique identifier for this NAS service.
	Key FlexInt `json:"$key,omitempty"`
	// VM is the underlying VM ID for this NAS service.
	VM FlexInt `json:"vm,omitempty"`
	// Name is the service name (derived from VM name).
	Name string `json:"name,omitempty"`
	// Enabled indicates whether the service is enabled.
	Enabled bool `json:"enabled"`
	// Created is the creation timestamp (Unix epoch).
	Created int64 `json:"created,omitempty"`
	// Modified is the last modified timestamp (Unix epoch).
	Modified int64 `json:"modified,omitempty"`

	// Service configuration
	// MaxImports is the maximum number of concurrent imports (1-200, default 4).
	MaxImports int `json:"max_imports,omitempty"`
	// MaxSyncs is the maximum number of concurrent syncs (0-200, default 0 = disabled).
	MaxSyncs int `json:"max_syncs,omitempty"`
	// DisableSwap disables swap usage for this NAS service.
	DisableSwap bool `json:"disable_swap,omitempty"`
	// ReadAheadKBDefault is the default read-ahead buffer size (0, 64, 128, 256, 512, 1024, 2048, 4096 KB).
	ReadAheadKBDefault string `json:"read_ahead_kb_default,omitempty"`

	// Related row references (read-only)
	// CIFS is the CIFS settings row reference.
	CIFS FlexInt `json:"cifs,omitempty"`
	// NFS is the NFS settings row reference.
	NFS FlexInt `json:"nfs,omitempty"`
	// Antivirus is the antivirus settings row reference.
	Antivirus FlexInt `json:"antivirus,omitempty"`
}

// NASServiceCreateRequest is the request body for creating a NAS service.
// Note: NAS services are typically created by creating a NAS VM, but this
// endpoint allows direct service creation if the VM already exists.
type NASServiceCreateRequest struct {
	// VM is the underlying VM ID (required).
	VM int `json:"vm"`
	// MaxImports is the maximum number of concurrent imports (1-200).
	MaxImports *int `json:"max_imports,omitempty"`
	// MaxSyncs is the maximum number of concurrent syncs (0-200).
	MaxSyncs *int `json:"max_syncs,omitempty"`
	// DisableSwap disables swap usage for this NAS service.
	DisableSwap *bool `json:"disable_swap,omitempty"`
	// ReadAheadKBDefault is the default read-ahead buffer size.
	ReadAheadKBDefault *string `json:"read_ahead_kb_default,omitempty"`
}

// NASServiceUpdateRequest is the request body for updating a NAS service.
type NASServiceUpdateRequest struct {
	// MaxImports is the maximum number of concurrent imports (1-200).
	MaxImports *int `json:"max_imports,omitempty"`
	// MaxSyncs is the maximum number of concurrent syncs (0-200).
	MaxSyncs *int `json:"max_syncs,omitempty"`
	// DisableSwap disables swap usage for this NAS service.
	DisableSwap *bool `json:"disable_swap,omitempty"`
	// ReadAheadKBDefault is the default read-ahead buffer size.
	ReadAheadKBDefault *string `json:"read_ahead_kb_default,omitempty"`
}

// nasServiceListFields are the fields to request when listing NAS services.
const nasServiceListFields = "$key,vm,name,enabled,created,modified,max_imports,max_syncs,disable_swap,read_ahead_kb_default,cifs,nfs,antivirus"

// nasServiceGetFields are the fields to request when getting a single NAS service.
const nasServiceGetFields = nasServiceListFields

// NASServiceUser represents a user account for a NAS service.
// NAS service users can access CIFS shares and have optional home directories.
type NASServiceUser struct {
	// ID is the unique SHA1 hash identifier for this user.
	ID string `json:"id,omitempty"`
	// Key is the same as ID for vm_service_users.
	Key string `json:"$key,omitempty"`
	// Service is the NAS service ID this user belongs to.
	Service FlexInt `json:"service,omitempty"`
	// Name is the username (1-32 chars, alphanumeric with underscore/hyphen).
	Name string `json:"name"`
	// DisplayName is the display name for this user.
	DisplayName string `json:"displayname,omitempty"`
	// Description is an optional description of the user.
	Description string `json:"description,omitempty"`
	// Enabled indicates whether the user account is enabled.
	Enabled bool `json:"enabled"`
	// Created is the creation timestamp (Unix epoch).
	Created int64 `json:"created,omitempty"`

	// Home directory configuration
	// HomeShare is the CIFS share ID to use as home directory.
	HomeShare FlexInt `json:"home_share,omitempty"`
	// HomeDrive is the Windows drive letter for home directory (A-Z).
	HomeDrive string `json:"home_drive,omitempty"`
}

// NASServiceUserCreateRequest is the request body for creating a NAS service user.
type NASServiceUserCreateRequest struct {
	// Service is the NAS service ID (required).
	Service int `json:"service"`
	// Name is the username (required, 1-32 chars).
	Name string `json:"name"`
	// Password is the user password (required).
	Password string `json:"password"`
	// DisplayName is the display name for this user.
	DisplayName string `json:"displayname,omitempty"`
	// Description is an optional description of the user.
	Description string `json:"description,omitempty"`
	// Enabled indicates whether the user account is enabled.
	Enabled *bool `json:"enabled,omitempty"`
	// HomeShare is the CIFS share ID to use as home directory.
	HomeShare *int `json:"home_share,omitempty"`
	// HomeDrive is the Windows drive letter for home directory (A-Z).
	HomeDrive *string `json:"home_drive,omitempty"`
}

// NASServiceUserUpdateRequest is the request body for updating a NAS service user.
type NASServiceUserUpdateRequest struct {
	// Password is the new user password.
	Password *string `json:"password,omitempty"`
	// DisplayName is the display name for this user.
	DisplayName *string `json:"displayname,omitempty"`
	// Description is an optional description of the user.
	Description *string `json:"description,omitempty"`
	// Enabled indicates whether the user account is enabled.
	Enabled *bool `json:"enabled,omitempty"`
	// HomeShare is the CIFS share ID to use as home directory.
	HomeShare *int `json:"home_share,omitempty"`
	// HomeDrive is the Windows drive letter for home directory (A-Z).
	HomeDrive *string `json:"home_drive,omitempty"`
}

// nasServiceUserListFields are the fields to request when listing NAS service users.
const nasServiceUserListFields = "$key,id,service,name,displayname,description,enabled,created,home_share,home_drive"

// nasServiceUserGetFields are the fields to request when getting a single NAS service user.
const nasServiceUserGetFields = nasServiceUserListFields
