package vergeos

// Volume represents a VergeOS NAS volume.
// Note: Unlike other resources, Volume uses a SHA1 hash string as its key ($key and id are the same).
type Volume struct {
	// Key is the unique SHA1 hash key (same as ID for volumes).
	Key string `json:"$key,omitempty"`
	// ID is the unique SHA1 hash identifier for the volume (same as Key).
	ID string `json:"id,omitempty"`
	// Name is the volume name (alphanumeric without spaces).
	Name string `json:"name"`
	// Description is an optional description of the volume.
	Description string `json:"description,omitempty"`
	// Enabled indicates whether the volume is enabled.
	Enabled bool `json:"enabled"`
	// Created is the creation timestamp (Unix epoch).
	Created int64 `json:"created,omitempty"`
	// Modified is the last modified timestamp (Unix epoch).
	Modified int64 `json:"modified,omitempty"`
	// Creator is the username that created this volume.
	Creator string `json:"creator,omitempty"`

	// Service is the NAS service ID this volume belongs to.
	Service FlexInt `json:"service,omitempty"`
	// Drive is the underlying machine drive ID.
	Drive FlexInt `json:"drive,omitempty"`
	// IsSnapshot indicates whether this volume is a snapshot.
	IsSnapshot bool `json:"is_snapshot,omitempty"`

	// Storage configuration
	// MaxSize is the maximum size in bytes.
	MaxSize int64 `json:"maxsize,omitempty"`
	// PreferredTier is the preferred storage tier (1-5).
	PreferredTier string `json:"preferred_tier,omitempty"`
	// SnapshotProfile is the snapshot profile ID (0 = none).
	SnapshotProfile FlexInt `json:"snapshot_profile,omitempty"`
	// FSType is the filesystem type (ext4, ybfs, cifs, nfs, fc_nimble, verge_vm_export).
	FSType string `json:"fs_type,omitempty"`
	// Discard indicates whether TRIM/discard is enabled.
	Discard bool `json:"discard,omitempty"`
	// ReadOnly indicates whether the volume is read-only.
	ReadOnly bool `json:"read_only,omitempty"`
	// Optimize is the optimization mode (general, large).
	Optimize string `json:"optimize,omitempty"`

	// Ownership
	// OwnerUser is the Unix user that owns the volume directory.
	OwnerUser string `json:"owner_user,omitempty"`
	// OwnerGroup is the Unix group that owns the volume directory.
	OwnerGroup string `json:"owner_group,omitempty"`

	// Snapshots
	// AutomountSnapshots indicates whether to automatically mount snapshots.
	AutomountSnapshots bool `json:"automount_snapshots,omitempty"`

	// Encryption
	// Encrypt indicates whether the volume is encrypted.
	Encrypt bool `json:"encrypt,omitempty"`

	// Remote volume configuration (for CIFS/NFS remote mounts)
	// RemoteTarget is the remote server and path (e.g., "//server/share" or "server:/path").
	RemoteTarget string `json:"remote_target,omitempty"`
	// CIFSUser is the username for CIFS authentication.
	CIFSUser string `json:"cifs_user,omitempty"`
	// CIFSProtocol is the SMB protocol version (1.0, 2.0, 2.1, 3.0).
	CIFSProtocol string `json:"cifs_protocol,omitempty"`
	// NFSProtocol is the NFS protocol version ("", 2, 3, 4).
	NFSProtocol string `json:"nfs_protocol,omitempty"`
	// MountOptions contains additional mount options.
	MountOptions string `json:"mount_options,omitempty"`
	// ReadAheadKB is the read-ahead buffer size (0, 64, 128, 256, 512, 1024, 2048, 4096).
	ReadAheadKB string `json:"read_ahead_kb,omitempty"`

	// Note is a free-form note about the volume.
	Note string `json:"note,omitempty"`
}

// VolumeCreateRequest is the request body for creating a volume.
type VolumeCreateRequest struct {
	// Name is the volume name (required, alphanumeric without spaces).
	Name string `json:"name"`
	// Service is the NAS service ID (required).
	Service int `json:"service"`
	// Description is an optional description of the volume.
	Description string `json:"description,omitempty"`
	// Enabled indicates whether the volume is enabled.
	Enabled *bool `json:"enabled,omitempty"`

	// Storage configuration
	// MaxSize is the maximum size in bytes (min 1MB).
	MaxSize *int64 `json:"maxsize,omitempty"`
	// PreferredTier is the preferred storage tier (1-5).
	PreferredTier *string `json:"preferred_tier,omitempty"`
	// SnapshotProfile is the snapshot profile ID.
	SnapshotProfile *int `json:"snapshot_profile,omitempty"`
	// Discard indicates whether TRIM/discard is enabled.
	Discard *bool `json:"discard,omitempty"`
	// ReadOnly indicates whether the volume is read-only.
	ReadOnly *bool `json:"read_only,omitempty"`
	// Optimize is the optimization mode (general, large).
	Optimize *string `json:"optimize,omitempty"`

	// Ownership
	// OwnerUser is the Unix user that owns the volume directory.
	OwnerUser *string `json:"owner_user,omitempty"`
	// OwnerGroup is the Unix group that owns the volume directory.
	OwnerGroup *string `json:"owner_group,omitempty"`

	// Snapshots
	// AutomountSnapshots indicates whether to automatically mount snapshots.
	AutomountSnapshots *bool `json:"automount_snapshots,omitempty"`

	// Encryption (only at creation time)
	// Encrypt indicates whether to encrypt the volume.
	Encrypt *bool `json:"encrypt,omitempty"`
	// EncryptionKey is the encryption key/passphrase.
	EncryptionKey *string `json:"encryption_key,omitempty"`

	// Remote volume configuration (for CIFS/NFS remote mounts)
	// RemoteTarget is the remote server and path.
	RemoteTarget *string `json:"remote_target,omitempty"`
	// CIFSUser is the username for CIFS authentication.
	CIFSUser *string `json:"cifs_user,omitempty"`
	// CIFSPassword is the password for CIFS authentication.
	CIFSPassword *string `json:"cifs_password,omitempty"`
	// CIFSProtocol is the SMB protocol version (1.0, 2.0, 2.1, 3.0).
	CIFSProtocol *string `json:"cifs_protocol,omitempty"`
	// NFSProtocol is the NFS protocol version ("", 2, 3, 4).
	NFSProtocol *string `json:"nfs_protocol,omitempty"`
	// MountOptions contains additional mount options.
	MountOptions *string `json:"mount_options,omitempty"`
	// ReadAheadKB is the read-ahead buffer size.
	ReadAheadKB *string `json:"read_ahead_kb,omitempty"`

	// Note is a free-form note about the volume.
	Note *string `json:"note,omitempty"`
}

// VolumeUpdateRequest is the request body for updating a volume.
type VolumeUpdateRequest struct {
	// Name is the volume name.
	Name *string `json:"name,omitempty"`
	// Description is the volume description.
	Description *string `json:"description,omitempty"`
	// Enabled indicates whether the volume is enabled.
	Enabled *bool `json:"enabled,omitempty"`

	// Storage configuration
	// MaxSize is the maximum size in bytes.
	MaxSize *int64 `json:"maxsize,omitempty"`
	// PreferredTier is the preferred storage tier (1-5).
	PreferredTier *string `json:"preferred_tier,omitempty"`
	// SnapshotProfile is the snapshot profile ID.
	SnapshotProfile *int `json:"snapshot_profile,omitempty"`
	// Discard indicates whether TRIM/discard is enabled.
	Discard *bool `json:"discard,omitempty"`
	// ReadOnly indicates whether the volume is read-only.
	ReadOnly *bool `json:"read_only,omitempty"`
	// Optimize is the optimization mode (general, large).
	Optimize *string `json:"optimize,omitempty"`

	// Ownership
	// OwnerUser is the Unix user that owns the volume directory.
	OwnerUser *string `json:"owner_user,omitempty"`
	// OwnerGroup is the Unix group that owns the volume directory.
	OwnerGroup *string `json:"owner_group,omitempty"`

	// Snapshots
	// AutomountSnapshots indicates whether to automatically mount snapshots.
	AutomountSnapshots *bool `json:"automount_snapshots,omitempty"`

	// Remote volume configuration (for CIFS/NFS remote mounts)
	// CIFSUser is the username for CIFS authentication.
	CIFSUser *string `json:"cifs_user,omitempty"`
	// CIFSPassword is the password for CIFS authentication.
	CIFSPassword *string `json:"cifs_password,omitempty"`
	// CIFSProtocol is the SMB protocol version.
	CIFSProtocol *string `json:"cifs_protocol,omitempty"`
	// NFSProtocol is the NFS protocol version.
	NFSProtocol *string `json:"nfs_protocol,omitempty"`
	// MountOptions contains additional mount options.
	MountOptions *string `json:"mount_options,omitempty"`
	// ReadAheadKB is the read-ahead buffer size.
	ReadAheadKB *string `json:"read_ahead_kb,omitempty"`

	// Note is a free-form note about the volume.
	Note *string `json:"note,omitempty"`
}

// volumeListFields are the fields to request when listing volumes.
const volumeListFields = "$key,id,name,description,enabled,created,modified,creator,service,drive,is_snapshot,maxsize,preferred_tier,snapshot_profile,fs_type,discard,read_only,optimize,owner_user,owner_group,automount_snapshots,encrypt,remote_target,cifs_user,cifs_protocol,nfs_protocol,mount_options,read_ahead_kb"

// volumeGetFields are the fields to request when getting a single volume.
const volumeGetFields = volumeListFields + ",note"
