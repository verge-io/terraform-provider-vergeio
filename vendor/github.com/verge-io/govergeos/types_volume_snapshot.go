package vergeos

// VolumeSnapshot represents a snapshot of a VergeOS NAS volume.
type VolumeSnapshot struct {
	// Key is the unique identifier for this snapshot.
	Key FlexInt `json:"$key,omitempty"`
	// Volume is the parent volume ID.
	Volume FlexInt `json:"volume,omitempty"`
	// SnapVolume is the snapshot volume reference (read-only).
	SnapVolume FlexInt `json:"snap_volume,omitempty"`
	// Name is the snapshot name.
	Name string `json:"name"`
	// Description is an optional description of the snapshot.
	Description string `json:"description,omitempty"`
	// Created is the creation timestamp (Unix epoch).
	Created int64 `json:"created,omitempty"`

	// Expiration settings
	// ExpiresType is the expiration type (never or date).
	ExpiresType string `json:"expires_type,omitempty"`
	// Expires is the expiration timestamp (Unix epoch).
	Expires int64 `json:"expires,omitempty"`

	// Status flags
	// Enabled indicates whether the snapshot is enabled.
	Enabled bool `json:"enabled"`
	// CreatedManually indicates whether this snapshot was created manually (vs scheduled).
	CreatedManually bool `json:"created_manually,omitempty"`
	// Quiesce indicates whether I/O was frozen during snapshot creation.
	Quiesce bool `json:"quiesce,omitempty"`
}

// VolumeSnapshotCreateRequest is the request body for creating a volume snapshot.
type VolumeSnapshotCreateRequest struct {
	// Volume is the parent volume ID (required).
	Volume int `json:"volume"`
	// Name is the snapshot name (required, 1-128 chars).
	Name string `json:"name"`
	// Description is an optional description of the snapshot.
	Description string `json:"description,omitempty"`
	// Enabled indicates whether the snapshot is enabled.
	Enabled *bool `json:"enabled,omitempty"`

	// Expiration settings
	// ExpiresType is the expiration type (never or date).
	ExpiresType *string `json:"expires_type,omitempty"`
	// Expires is the expiration timestamp (Unix epoch). Default: now + 3 days.
	Expires *int64 `json:"expires,omitempty"`

	// Quiesce freezes I/O during snapshot creation for consistency.
	Quiesce *bool `json:"quiesce,omitempty"`
}

// VolumeSnapshotUpdateRequest is the request body for updating a volume snapshot.
type VolumeSnapshotUpdateRequest struct {
	// Name is the snapshot name.
	Name *string `json:"name,omitempty"`
	// Description is an optional description of the snapshot.
	Description *string `json:"description,omitempty"`
	// Enabled indicates whether the snapshot is enabled.
	Enabled *bool `json:"enabled,omitempty"`

	// Expiration settings
	// ExpiresType is the expiration type (never or date).
	ExpiresType *string `json:"expires_type,omitempty"`
	// Expires is the expiration timestamp (Unix epoch).
	Expires *int64 `json:"expires,omitempty"`
}

// volumeSnapshotListFields are the fields to request when listing volume snapshots.
const volumeSnapshotListFields = "$key,volume,snap_volume,name,description,created,expires_type,expires,enabled,created_manually,quiesce"

// volumeSnapshotGetFields are the fields to request when getting a single volume snapshot.
const volumeSnapshotGetFields = volumeSnapshotListFields
