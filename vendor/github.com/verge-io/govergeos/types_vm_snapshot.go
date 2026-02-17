package vergeos

// VMSnapshot represents a snapshot of a virtual machine in VergeOS.
// VM Snapshots are point-in-time copies of a VM's state that can be restored.
type VMSnapshot struct {
	// Key is the unique identifier for the snapshot.
	Key FlexInt `json:"$key,omitempty"`
	// Machine is the VM ID this snapshot belongs to.
	Machine FlexInt `json:"machine,omitempty"`
	// SnapMachine is the internal snapshot machine reference.
	SnapMachine FlexInt `json:"snap_machine,omitempty"`
	// Name is the snapshot name.
	Name string `json:"name"`
	// Description is an optional description of the snapshot.
	Description string `json:"description,omitempty"`
	// Created is the creation timestamp (Unix epoch).
	Created int64 `json:"created,omitempty"`
	// Expires is the expiration timestamp (Unix epoch). 0 means never expires.
	Expires int64 `json:"expires,omitempty"`
	// ExpiresType is the expiration type ("never" or "date").
	ExpiresType string `json:"expires_type,omitempty"`
	// CreatedManually indicates whether the snapshot was created manually.
	CreatedManually bool `json:"created_manually,omitempty"`
	// Quiesce indicates whether the snapshot should quiesce the filesystem.
	Quiesce bool `json:"quiesce,omitempty"`
	// Quiesced indicates whether the snapshot was quiesced.
	Quiesced bool `json:"quiesced,omitempty"`
	// QueueDelete indicates whether the snapshot is queued for deletion.
	QueueDelete bool `json:"queue_delete,omitempty"`
	// SnapshotPeriod is the snapshot profile period ID (if scheduled).
	SnapshotPeriod FlexInt `json:"snapshot_period,omitempty"`
	// MachineDisplay is the display name of the parent machine.
	MachineDisplay string `json:"machine_display,omitempty"`
	// SnapshotProfile is the snapshot profile name (from join).
	SnapshotProfile string `json:"snapshot_profile,omitempty"`
}

// VMSnapshotCreateRequest is the request body for creating a VM snapshot.
type VMSnapshotCreateRequest struct {
	// Machine is the VM ID to snapshot (required).
	Machine int `json:"machine"`
	// Name is the snapshot name (required).
	Name string `json:"name"`
	// Description is an optional description.
	Description string `json:"description,omitempty"`
	// ExpiresType is the expiration type ("never" or "date"). Defaults to "date".
	ExpiresType string `json:"expires_type,omitempty"`
	// Expires is the expiration timestamp (Unix epoch). Defaults to 3 days from creation.
	Expires *int64 `json:"expires,omitempty"`
	// Quiesce indicates whether to quiesce the filesystem (requires guest agent).
	Quiesce *bool `json:"quiesce,omitempty"`
}

// VMSnapshotUpdateRequest is the request body for updating a VM snapshot.
type VMSnapshotUpdateRequest struct {
	// Name is the snapshot name.
	Name *string `json:"name,omitempty"`
	// Description is the snapshot description.
	Description *string `json:"description,omitempty"`
	// ExpiresType is the expiration type ("never" or "date").
	ExpiresType *string `json:"expires_type,omitempty"`
	// Expires is the expiration timestamp (Unix epoch).
	Expires *int64 `json:"expires,omitempty"`
}

// VMSnapshotRestoreOptions contains options for restoring a VM snapshot.
type VMSnapshotRestoreOptions struct {
	// PowerOn indicates whether to power on the VM after restore.
	PowerOn bool
}

// Field list constants for VM snapshot resources.
const (
	vmSnapshotListFields = "$key,machine,snap_machine,name,description,created,expires,expires_type,created_manually,quiesce,quiesced,queue_delete,snapshot_period,machine#$display as machine_display,machine#snapshot_profile#$display as snapshot_profile"
	vmSnapshotGetFields  = vmSnapshotListFields
)
