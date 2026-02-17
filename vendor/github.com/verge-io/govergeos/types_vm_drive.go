package vergeos

// VMDrive represents a virtual disk attached to a VM.
type VMDrive struct {
	// ID is the unique identifier for the drive.
	ID FlexInt `json:"$key,omitempty"`
	// Machine is the machine reference ID.
	Machine int `json:"machine,omitempty"`
	// OrderID is the boot order ID.
	OrderID int `json:"orderid,omitempty"`
	// Name is the drive name.
	Name string `json:"name"`
	// Description is an optional description.
	Description string `json:"description,omitempty"`
	// Interface is the disk interface. Valid values:
	//   - virtio-scsi (default, recommended)
	//   - virtio (legacy paravirtual)
	//   - ide, ahci (SATA), nvme
	//   - lsi53c895a, megasas, megasas-gen2, mptsas1068 (SCSI)
	//   - cifs, nfs, vsan (pass-through)
	//   - usb, pflash, direct, tpm_state
	Interface string `json:"interface,omitempty"`
	// Media is the media type (disk, cdrom, efidisk, import, 9p, dir, clone, nonpersistent, etc.).
	Media string `json:"media,omitempty"`
	// File is the file ID (for cdrom/import).
	File FlexInt `json:"media_source,omitempty"`
	// SizeGB is the disk size in GB.
	SizeGB int64 `json:"-"`
	// SizeBytes is the disk size in bytes.
	SizeBytes int64 `json:"disksize,omitempty"`
	// UsedBytes is the actual used space in bytes (read-only).
	UsedBytes int64 `json:"used_bytes,omitempty"`
	// PreferredTier is the preferred storage tier (1-5).
	PreferredTier string `json:"preferred_tier,omitempty"`
	// Enabled indicates whether the drive is enabled.
	Enabled bool `json:"enabled"`
	// ReadOnly indicates whether the drive is read-only.
	ReadOnly bool `json:"readonly,omitempty"`
	// Optimize is the optimization setting (general, large).
	Optimize string `json:"optimize,omitempty"`
	// Serial is the drive serial number.
	Serial string `json:"serial,omitempty"`
	// Fsync is the strict fsync setting ("" = system default, "0" = off, "1" = on).
	Fsync string `json:"fsync,omitempty"`
	// Discard enables TRIM/discard support (default true).
	Discard bool `json:"discard,omitempty"`
	// Advanced contains advanced properties (newline-delimited key=value pairs).
	Advanced string `json:"advanced,omitempty"`
	// Asset is the asset tag (used for recipe/snapshot identification).
	Asset string `json:"asset,omitempty"`
	// PreserveDriveFormat indicates whether to preserve the drive format.
	PreserveDriveFormat bool `json:"preserve_drive_format,omitempty"`
	// PowerState is the drive power state ("online" or "offline").
	PowerState string `json:"powerState,omitempty"`
	// Status is the drive status (e.g., "importing").
	Status string `json:"status,omitempty"`
}

// VMDriveCreateRequest is the request body for creating a drive.
type VMDriveCreateRequest struct {
	// Machine is the VM's machine ID.
	Machine int `json:"machine"`
	// OrderID is the boot order ID (auto-assigned if not specified).
	OrderID *int `json:"orderid,omitempty"`
	// Name is the drive name.
	Name string `json:"name"`
	// Description is an optional description.
	Description string `json:"description,omitempty"`
	// Interface is the disk interface. Valid values: virtio-scsi (default), virtio, ide, ahci, nvme, usb, etc.
	Interface string `json:"interface,omitempty"`
	// Media is the media type (disk, cdrom, efidisk, import, etc.).
	Media string `json:"media,omitempty"`
	// File is the file ID (for cdrom/import).
	File int `json:"media_source,omitempty"`
	// SizeGB is the disk size in GB.
	SizeGB int64 `json:"-"`
	// SizeBytes is the disk size in bytes (set from SizeGB).
	SizeBytes int64 `json:"disksize,omitempty"`
	// PreferredTier is the preferred storage tier (1-5).
	PreferredTier string `json:"preferred_tier,omitempty"`
	// Enabled indicates whether the drive is enabled.
	Enabled *bool `json:"enabled,omitempty"`
	// ReadOnly indicates whether the drive is read-only.
	ReadOnly *bool `json:"readonly,omitempty"`
	// Optimize is the optimization setting (general, large).
	Optimize string `json:"optimize,omitempty"`
	// Serial is the drive serial number.
	Serial string `json:"serial,omitempty"`
	// Fsync is the strict fsync setting ("" = system default, "0" = off, "1" = on).
	Fsync string `json:"fsync,omitempty"`
	// Discard enables TRIM/discard support.
	Discard *bool `json:"discard,omitempty"`
	// Advanced contains advanced properties (newline-delimited key=value pairs).
	Advanced string `json:"advanced,omitempty"`
	// Asset is the asset tag.
	Asset string `json:"asset,omitempty"`
	// PreserveDriveFormat indicates whether to preserve the drive format.
	PreserveDriveFormat *bool `json:"preserve_drive_format,omitempty"`
}

// VMDriveUpdateRequest is the request body for updating a drive.
type VMDriveUpdateRequest struct {
	// OrderID is the boot order ID.
	OrderID *int `json:"orderid,omitempty"`
	// Name is the drive name.
	Name *string `json:"name,omitempty"`
	// Description is the description.
	Description *string `json:"description,omitempty"`
	// Interface is the disk interface. Valid values: virtio-scsi (default), virtio, ide, ahci, nvme, usb, etc.
	Interface *string `json:"interface,omitempty"`
	// SizeGB is the disk size in GB (can only increase).
	SizeGB *int64 `json:"-"`
	// SizeBytes is the disk size in bytes.
	SizeBytes *int64 `json:"disksize,omitempty"`
	// PreferredTier is the preferred storage tier (1-5).
	PreferredTier *string `json:"preferred_tier,omitempty"`
	// Enabled indicates whether the drive is enabled.
	Enabled *bool `json:"enabled,omitempty"`
	// ReadOnly indicates whether the drive is read-only.
	ReadOnly *bool `json:"readonly,omitempty"`
	// Optimize is the optimization setting (general, large).
	Optimize *string `json:"optimize,omitempty"`
	// Serial is the drive serial number.
	Serial *string `json:"serial,omitempty"`
	// Fsync is the strict fsync setting ("" = system default, "0" = off, "1" = on).
	Fsync *string `json:"fsync,omitempty"`
	// Discard enables TRIM/discard support.
	Discard *bool `json:"discard,omitempty"`
	// Advanced contains advanced properties (newline-delimited key=value pairs).
	Advanced *string `json:"advanced,omitempty"`
	// Asset is the asset tag.
	Asset *string `json:"asset,omitempty"`
	// PreserveDriveFormat indicates whether to preserve the drive format.
	PreserveDriveFormat *bool `json:"preserve_drive_format,omitempty"`
}

// driveListFields are the fields to request when listing drives.
const driveListFields = "$key,machine,orderid,name,description,disksize,used_bytes,interface,media,media_source,preferred_tier,enabled,readonly,optimize,serial,fsync,discard,advanced,preserve_drive_format,asset"

// driveGetFields are the fields to request when getting a single drive (includes power state and status).
const driveGetFields = driveListFields + ",status#status as powerState,status#status_info as status"

// Conversion constants for disk size
const (
	// bytesPerGB is the number of bytes in a gigabyte.
	bytesPerGB = 1024 * 1024 * 1024
)
