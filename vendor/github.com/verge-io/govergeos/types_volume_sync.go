package vergeos

// VolumeSync represents a VergeOS volume sync job for replicating data between volumes.
type VolumeSync struct {
	// ID is the unique SHA1 hash identifier for this sync job.
	ID string `json:"id,omitempty"`
	// Key is the same as ID for volume_syncs.
	Key string `json:"$key,omitempty"`
	// Service is the NAS service ID this sync belongs to.
	Service FlexInt `json:"service,omitempty"`
	// Name is the sync job name.
	Name string `json:"name"`
	// Description is an optional description of the sync job.
	Description string `json:"description,omitempty"`
	// Enabled indicates whether the sync job is enabled.
	Enabled bool `json:"enabled"`
	// Created is the creation timestamp (Unix epoch).
	Created int64 `json:"created,omitempty"`
	// Modified is the last modified timestamp (Unix epoch).
	Modified int64 `json:"modified,omitempty"`

	// Source configuration
	// SourceVolume is the source volume ID.
	SourceVolume FlexInt `json:"source_volume,omitempty"`
	// SourcePath is the starting directory path in the source volume.
	SourcePath string `json:"source_path,omitempty"`

	// Destination configuration
	// DestinationVolume is the destination volume ID.
	DestinationVolume FlexInt `json:"destination_volume,omitempty"`
	// DestinationPath is the starting directory path in the destination volume.
	DestinationPath string `json:"destination_path,omitempty"`

	// Filter configuration
	// Include is a newline-delimited list of include patterns.
	Include string `json:"include,omitempty"`
	// Exclude is a newline-delimited list of exclude patterns.
	Exclude string `json:"exclude,omitempty"`

	// Sync behavior options
	// FSFreeze freezes the filesystem during snapshot.
	FSFreeze bool `json:"fsfreeze,omitempty"`
	// PreserveACLs preserves file ACLs during sync.
	PreserveACLs bool `json:"preserve_ACLs,omitempty"`
	// CopySymlinks copies symbolic links as links (vs following them).
	CopySymlinks bool `json:"copy_symlinks,omitempty"`
	// PreserveXattrs preserves extended attributes.
	PreserveXattrs bool `json:"preserve_xattrs,omitempty"`
	// PreservePermissions preserves file permissions.
	PreservePermissions bool `json:"preserve_permissions,omitempty"`
	// PreserveModTime preserves modification times.
	PreserveModTime bool `json:"preserve_mod_time,omitempty"`
	// PreserveGroups preserves group ownership.
	PreserveGroups bool `json:"preserve_groups,omitempty"`
	// PreserveOwner preserves user ownership.
	PreserveOwner bool `json:"preserve_owner,omitempty"`
	// PreserveDeviceFiles preserves device files (requires super-user).
	PreserveDeviceFiles bool `json:"preserve_device_files,omitempty"`

	// Scheduling
	// StartTimeProfile is the snapshot profile ID for scheduling.
	StartTimeProfile FlexInt `json:"start_time_profile,omitempty"`
	// RunTime is the maximum run time in seconds.
	RunTime int `json:"run_time,omitempty"`
	// RunAsUser is the user to run the sync as.
	RunAsUser string `json:"run_as_user,omitempty"`

	// Delete behavior
	// DestinationDelete controls deletion of files not in source.
	// Values: never, delete, delete-before, delete-during, delete-delay, delete-after
	DestinationDelete string `json:"destination_delete,omitempty"`

	// Performance tuning
	// ErrorsMax is the maximum error count before abort (default 1000).
	ErrorsMax int64 `json:"errors_max,omitempty"`
	// PreferredTier is the preferred storage tier (1-5).
	PreferredTier string `json:"preferred_tier,omitempty"`
	// Workers is the number of sync workers (1-128, default 4).
	Workers int `json:"workers,omitempty"`

	// Advanced options
	// OmitDirTimes omits directory modification times.
	OmitDirTimes bool `json:"omit_dir_times,omitempty"`
	// OmitLinkTimes omits symlink modification times.
	OmitLinkTimes bool `json:"omit_link_times,omitempty"`
	// Inplace updates files in-place.
	Inplace bool `json:"inplace,omitempty"`
	// CIFSACL preserves CIFS ACLs (default true).
	CIFSACL bool `json:"cifsacl,omitempty"`
	// SyncMethod is the sync method (rsync or ysync).
	SyncMethod string `json:"sync_method,omitempty"`
	// YSyncExtended contains extended ysync properties.
	YSyncExtended string `json:"ysync_extended,omitempty"`

	// Type is the sync type (volsync or vmimport).
	Type string `json:"type,omitempty"`

	// Progress tracking (read-only)
	// Progress is the current progress row reference.
	Progress FlexInt `json:"progress,omitempty"`
}

// VolumeSyncCreateRequest is the request body for creating a volume sync job.
type VolumeSyncCreateRequest struct {
	// Service is the NAS service ID (required).
	Service int `json:"service"`
	// Name is the sync job name (required, 1-128 chars).
	Name string `json:"name"`
	// Description is an optional description of the sync job.
	Description string `json:"description,omitempty"`
	// Enabled indicates whether the sync job is enabled.
	Enabled *bool `json:"enabled,omitempty"`

	// Source configuration
	// SourceVolume is the source volume ID (required).
	SourceVolume int `json:"source_volume"`
	// SourcePath is the starting directory path in the source volume.
	SourcePath *string `json:"source_path,omitempty"`

	// Destination configuration
	// DestinationVolume is the destination volume ID (required).
	DestinationVolume int `json:"destination_volume"`
	// DestinationPath is the starting directory path in the destination volume.
	DestinationPath *string `json:"destination_path,omitempty"`

	// Filter configuration
	// Include is a newline-delimited list of include patterns.
	Include *string `json:"include,omitempty"`
	// Exclude is a newline-delimited list of exclude patterns.
	Exclude *string `json:"exclude,omitempty"`

	// Sync behavior options
	// FSFreeze freezes the filesystem during snapshot.
	FSFreeze *bool `json:"fsfreeze,omitempty"`
	// PreserveACLs preserves file ACLs during sync.
	PreserveACLs *bool `json:"preserve_ACLs,omitempty"`
	// CopySymlinks copies symbolic links as links.
	CopySymlinks *bool `json:"copy_symlinks,omitempty"`
	// PreserveXattrs preserves extended attributes.
	PreserveXattrs *bool `json:"preserve_xattrs,omitempty"`
	// PreservePermissions preserves file permissions.
	PreservePermissions *bool `json:"preserve_permissions,omitempty"`
	// PreserveModTime preserves modification times.
	PreserveModTime *bool `json:"preserve_mod_time,omitempty"`
	// PreserveGroups preserves group ownership.
	PreserveGroups *bool `json:"preserve_groups,omitempty"`
	// PreserveOwner preserves user ownership.
	PreserveOwner *bool `json:"preserve_owner,omitempty"`
	// PreserveDeviceFiles preserves device files.
	PreserveDeviceFiles *bool `json:"preserve_device_files,omitempty"`

	// Scheduling
	// StartTimeProfile is the snapshot profile ID for scheduling.
	StartTimeProfile *int `json:"start_time_profile,omitempty"`
	// RunTime is the maximum run time in seconds.
	RunTime *int `json:"run_time,omitempty"`
	// RunAsUser is the user to run the sync as.
	RunAsUser *string `json:"run_as_user,omitempty"`

	// Delete behavior
	// DestinationDelete controls deletion of files not in source.
	DestinationDelete *string `json:"destination_delete,omitempty"`

	// Performance tuning
	// ErrorsMax is the maximum error count before abort.
	ErrorsMax *int64 `json:"errors_max,omitempty"`
	// PreferredTier is the preferred storage tier (1-5).
	PreferredTier *string `json:"preferred_tier,omitempty"`
	// Workers is the number of sync workers (1-128).
	Workers *int `json:"workers,omitempty"`

	// Advanced options
	// OmitDirTimes omits directory modification times.
	OmitDirTimes *bool `json:"omit_dir_times,omitempty"`
	// OmitLinkTimes omits symlink modification times.
	OmitLinkTimes *bool `json:"omit_link_times,omitempty"`
	// Inplace updates files in-place.
	Inplace *bool `json:"inplace,omitempty"`
	// CIFSACL preserves CIFS ACLs.
	CIFSACL *bool `json:"cifsacl,omitempty"`
	// SyncMethod is the sync method (rsync or ysync).
	SyncMethod *string `json:"sync_method,omitempty"`
	// YSyncExtended contains extended ysync properties.
	YSyncExtended *string `json:"ysync_extended,omitempty"`
}

// VolumeSyncUpdateRequest is the request body for updating a volume sync job.
type VolumeSyncUpdateRequest struct {
	// Name is the sync job name.
	Name *string `json:"name,omitempty"`
	// Description is an optional description of the sync job.
	Description *string `json:"description,omitempty"`
	// Enabled indicates whether the sync job is enabled.
	Enabled *bool `json:"enabled,omitempty"`

	// Source configuration
	// SourceVolume is the source volume ID.
	SourceVolume *int `json:"source_volume,omitempty"`
	// SourcePath is the starting directory path in the source volume.
	SourcePath *string `json:"source_path,omitempty"`

	// Destination configuration
	// DestinationVolume is the destination volume ID.
	DestinationVolume *int `json:"destination_volume,omitempty"`
	// DestinationPath is the starting directory path in the destination volume.
	DestinationPath *string `json:"destination_path,omitempty"`

	// Filter configuration
	// Include is a newline-delimited list of include patterns.
	Include *string `json:"include,omitempty"`
	// Exclude is a newline-delimited list of exclude patterns.
	Exclude *string `json:"exclude,omitempty"`

	// Sync behavior options
	// FSFreeze freezes the filesystem during snapshot.
	FSFreeze *bool `json:"fsfreeze,omitempty"`
	// PreserveACLs preserves file ACLs during sync.
	PreserveACLs *bool `json:"preserve_ACLs,omitempty"`
	// CopySymlinks copies symbolic links as links.
	CopySymlinks *bool `json:"copy_symlinks,omitempty"`
	// PreserveXattrs preserves extended attributes.
	PreserveXattrs *bool `json:"preserve_xattrs,omitempty"`
	// PreservePermissions preserves file permissions.
	PreservePermissions *bool `json:"preserve_permissions,omitempty"`
	// PreserveModTime preserves modification times.
	PreserveModTime *bool `json:"preserve_mod_time,omitempty"`
	// PreserveGroups preserves group ownership.
	PreserveGroups *bool `json:"preserve_groups,omitempty"`
	// PreserveOwner preserves user ownership.
	PreserveOwner *bool `json:"preserve_owner,omitempty"`
	// PreserveDeviceFiles preserves device files.
	PreserveDeviceFiles *bool `json:"preserve_device_files,omitempty"`

	// Scheduling
	// StartTimeProfile is the snapshot profile ID for scheduling.
	StartTimeProfile *int `json:"start_time_profile,omitempty"`
	// RunTime is the maximum run time in seconds.
	RunTime *int `json:"run_time,omitempty"`
	// RunAsUser is the user to run the sync as.
	RunAsUser *string `json:"run_as_user,omitempty"`

	// Delete behavior
	// DestinationDelete controls deletion of files not in source.
	DestinationDelete *string `json:"destination_delete,omitempty"`

	// Performance tuning
	// ErrorsMax is the maximum error count before abort.
	ErrorsMax *int64 `json:"errors_max,omitempty"`
	// PreferredTier is the preferred storage tier (1-5).
	PreferredTier *string `json:"preferred_tier,omitempty"`
	// Workers is the number of sync workers (1-128).
	Workers *int `json:"workers,omitempty"`

	// Advanced options
	// OmitDirTimes omits directory modification times.
	OmitDirTimes *bool `json:"omit_dir_times,omitempty"`
	// OmitLinkTimes omits symlink modification times.
	OmitLinkTimes *bool `json:"omit_link_times,omitempty"`
	// Inplace updates files in-place.
	Inplace *bool `json:"inplace,omitempty"`
	// CIFSACL preserves CIFS ACLs.
	CIFSACL *bool `json:"cifsacl,omitempty"`
	// SyncMethod is the sync method (rsync or ysync).
	SyncMethod *string `json:"sync_method,omitempty"`
	// YSyncExtended contains extended ysync properties.
	YSyncExtended *string `json:"ysync_extended,omitempty"`
}

// volumeSyncListFields are the fields to request when listing volume syncs.
const volumeSyncListFields = "$key,id,service,name,description,enabled,created,modified,source_volume,source_path,destination_volume,destination_path,type,sync_method,workers,destination_delete,start_time_profile,run_time"

// volumeSyncGetFields are the fields to request when getting a single volume sync.
const volumeSyncGetFields = volumeSyncListFields + ",include,exclude,fsfreeze,preserve_ACLs,copy_symlinks,preserve_xattrs,preserve_permissions,preserve_mod_time,preserve_groups,preserve_owner,preserve_device_files,run_as_user,errors_max,preferred_tier,omit_dir_times,omit_link_times,inplace,cifsacl,ysync_extended,progress"
