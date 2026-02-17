package vergeos

// StorageTier represents a system-wide storage tier in VergeOS.
// Storage tiers are at the system level (not cluster-specific).
// Use ClusterTier for cluster-specific tier information.
type StorageTier struct {
	// Key is the unique identifier for the tier (the tier number 0-5).
	Key int `json:"$key,omitempty"`
	// Tier is the tier number (0-5).
	Tier int `json:"tier,omitempty"`
	// Description is the tier description.
	Description string `json:"description,omitempty"`
	// Capacity is the total capacity in bytes.
	Capacity uint64 `json:"capacity,omitempty"`
	// Used is the used space in bytes.
	Used uint64 `json:"used,omitempty"`
	// Allocated is the allocated space in bytes.
	Allocated uint64 `json:"allocated,omitempty"`
	// UsedInflated is the used space before deduplication in bytes.
	UsedInflated uint64 `json:"used_inflated,omitempty"`
	// DedupeRatio is the deduplication ratio (multiply by 0.01 for percentage).
	DedupeRatio uint32 `json:"dedupe_ratio,omitempty"`
	// UsedPct is the used space percentage (0-100).
	UsedPct uint32 `json:"used_pct,omitempty"`
	// Modified is the last modified timestamp (Unix epoch).
	Modified int64 `json:"modified,omitempty"`
	// Stats contains the tier statistics (I/O operations, bytes, etc.).
	Stats *StorageTierStats `json:"stats,omitempty"`
}

// StorageTierStats contains I/O statistics for a storage tier.
type StorageTierStats struct {
	// Reads is the total read operations.
	Reads uint64 `json:"reads,omitempty"`
	// Writes is the total write operations.
	Writes uint64 `json:"writes,omitempty"`
	// ReadBytes is the total bytes read.
	ReadBytes uint64 `json:"read_bytes,omitempty"`
	// WriteBytes is the total bytes written.
	WriteBytes uint64 `json:"write_bytes,omitempty"`
	// Rops is the read operations per second.
	Rops uint64 `json:"rops,omitempty"`
	// Wops is the write operations per second.
	Wops uint64 `json:"wops,omitempty"`
	// Rbps is the read bytes per second.
	Rbps uint64 `json:"rbps,omitempty"`
	// Wbps is the write bytes per second.
	Wbps uint64 `json:"wbps,omitempty"`
}

// ClusterTier represents a cluster-specific storage tier in VergeOS.
// Cluster tiers contain status information about the VSAN tier for a specific cluster.
type ClusterTier struct {
	// Key is the unique identifier for the cluster tier record.
	Key FlexInt `json:"$key,omitempty"`
	// Cluster is the parent cluster ID.
	Cluster int `json:"cluster,omitempty"`
	// Tier is the tier number (0-5).
	Tier int `json:"tier,omitempty"`
	// Description is the tier description.
	Description string `json:"description,omitempty"`
	// CostPerGB is the cost per GB for billing.
	CostPerGB float64 `json:"cost_per_gb,omitempty"`
	// PricePerGB is the price per GB for billing.
	PricePerGB float64 `json:"price_per_gb,omitempty"`
	// Status contains the tier status information.
	Status *ClusterTierStatus `json:"status,omitempty"`
	// Stats contains the tier statistics.
	Stats *ClusterTierStats `json:"stats,omitempty"`
}

// ClusterTierStatus contains status information for a cluster tier.
type ClusterTierStatus struct {
	// Tier is the parent cluster tier reference.
	Tier int `json:"tier,omitempty"`
	// Status is the tier status (online, offline, repairing, initializing, verifying, noredundant, outofspace).
	Status string `json:"status,omitempty"`
	// State is the tier state (online, offline, warning, error).
	State string `json:"state,omitempty"`
	// Capacity is the total capacity in bytes.
	Capacity uint64 `json:"capacity,omitempty"`
	// Used is the used space in bytes.
	Used uint64 `json:"used,omitempty"`
	// UsedPct is the used space percentage (0-100).
	UsedPct uint32 `json:"used_pct,omitempty"`
	// Allocated is the allocated space in bytes.
	Allocated uint64 `json:"allocated,omitempty"`
	// Redundant indicates whether the tier has redundancy.
	Redundant bool `json:"redundant,omitempty"`
	// Encrypted indicates whether the tier is encrypted.
	Encrypted bool `json:"encrypted,omitempty"`
	// Working indicates whether the tier is actively working (walking/repairing).
	Working bool `json:"working,omitempty"`
	// LastWalkTimeMs is the last walk time in milliseconds.
	LastWalkTimeMs uint64 `json:"last_walk_time_ms,omitempty"`
	// LastFullwalkTimeMs is the last full walk time in milliseconds.
	LastFullwalkTimeMs uint64 `json:"last_fullwalk_time_ms,omitempty"`
	// Transaction is the current transaction number.
	Transaction uint64 `json:"transaction,omitempty"`
	// Repairs is the number of repairs performed.
	Repairs uint64 `json:"repairs,omitempty"`
	// BadDrives is the number of unavailable drives.
	BadDrives float64 `json:"bad_drives,omitempty"`
	// Fullwalk indicates whether a full walk is in progress.
	Fullwalk bool `json:"fullwalk,omitempty"`
	// Progress is the walk progress (0.0-1.0).
	Progress float64 `json:"progress,omitempty"`
	// IndexUnique is the number of unique index entries.
	IndexUnique uint64 `json:"index_unique,omitempty"`
	// StateTimestamp is the timestamp when the state last changed.
	StateTimestamp uint64 `json:"state_timestamp,omitempty"`
	// CurSpaceThrottleMs is the current space throttle in milliseconds.
	CurSpaceThrottleMs float64 `json:"cur_space_throttle_ms,omitempty"`
	// TransactionStartStamp is the transaction start timestamp.
	TransactionStartStamp uint32 `json:"transaction_start_stamp,omitempty"`
}

// ClusterTierStats contains I/O statistics for a cluster tier.
type ClusterTierStats struct {
	// Reads is the total read operations.
	Reads uint64 `json:"reads,omitempty"`
	// Writes is the total write operations.
	Writes uint64 `json:"writes,omitempty"`
	// ReadBytes is the total bytes read.
	ReadBytes uint64 `json:"read_bytes,omitempty"`
	// WriteBytes is the total bytes written.
	WriteBytes uint64 `json:"write_bytes,omitempty"`
	// Rops is the read operations per second.
	Rops uint64 `json:"rops,omitempty"`
	// Wops is the write operations per second.
	Wops uint64 `json:"wops,omitempty"`
	// Rbps is the read bytes per second.
	Rbps uint64 `json:"rbps,omitempty"`
	// Wbps is the write bytes per second.
	Wbps uint64 `json:"wbps,omitempty"`
}

// MachineDrivePhys represents physical drive information for a machine drive.
// This contains hardware-level information like temperature, wear level, and VSAN status.
type MachineDrivePhys struct {
	// Key is the unique identifier for this physical drive record.
	Key FlexInt `json:"$key,omitempty"`
	// ParentDrive is the parent machine drive ID.
	ParentDrive int `json:"parent_drive,omitempty"`
	// Path is the local drive path.
	Path string `json:"path,omitempty"`
	// AllPaths contains all drive paths.
	AllPaths string `json:"all_paths,omitempty"`
	// Modified is the last modified timestamp.
	Modified int64 `json:"modified,omitempty"`
	// Size is the disk size in bytes.
	Size uint64 `json:"size,omitempty"`
	// Model is the drive model.
	Model string `json:"model,omitempty"`
	// Fw is the firmware version.
	Fw string `json:"fw,omitempty"`
	// Serial is the serial number.
	Serial string `json:"serial,omitempty"`

	// Temperature metrics
	// Temp is the current temperature in Celsius.
	Temp uint16 `json:"temp,omitempty"`
	// TempWarn indicates whether the temperature is in warning state.
	TempWarn bool `json:"temp_warn,omitempty"`

	// SMART metrics
	// Encrypted indicates whether the drive is encrypted.
	Encrypted bool `json:"encrypted,omitempty"`
	// ReallocSectors is the number of reallocated sectors.
	ReallocSectors uint32 `json:"realloc_sectors,omitempty"`
	// ReallocSectorsWarn indicates a reallocated sectors warning.
	ReallocSectorsWarn bool `json:"realloc_sectors_warn,omitempty"`
	// WearLevel is the wear level percentage (for SSDs).
	WearLevel uint32 `json:"wear_level,omitempty"`
	// WearLevelWarn indicates a wear level warning.
	WearLevelWarn bool `json:"wear_level_warn,omitempty"`
	// Hours is the total power-on hours.
	Hours uint32 `json:"hours,omitempty"`
	// HoursWarn indicates a power-on hours warning.
	HoursWarn bool `json:"hours_warn,omitempty"`
	// CurrentPendingSector is the current pending sector count.
	CurrentPendingSector float64 `json:"current_pending_sector,omitempty"`
	// CurrentPendingSectorWarn indicates a pending sector warning.
	CurrentPendingSectorWarn bool `json:"current_pending_sector_warn,omitempty"`
	// OfflineUncorrectable is the offline uncorrectable sector count.
	OfflineUncorrectable float64 `json:"offline_uncorrectable,omitempty"`
	// OfflineUncorrectableWarn indicates an offline uncorrectable warning.
	OfflineUncorrectableWarn bool `json:"offline_uncorrectable_warn,omitempty"`
	// Smart indicates whether SMART status is available.
	Smart bool `json:"smart,omitempty"`

	// VSAN metrics
	// VSANDriveID is the VSAN drive ID (-1 if not in VSAN).
	VSANDriveID int8 `json:"vsan_driveid,omitempty"`
	// VSANTier is the VSAN tier number (-1 if not in VSAN).
	VSANTier int8 `json:"vsan_tier,omitempty"`
	// VSANUsed is the VSAN used space in bytes.
	VSANUsed uint64 `json:"vsan_used,omitempty"`
	// VSANMax is the VSAN maximum space in bytes.
	VSANMax uint64 `json:"vsan_max,omitempty"`
	// VSANReadErrors is the VSAN read error count.
	VSANReadErrors uint64 `json:"vsan_read_errors,omitempty"`
	// VSANWriteErrors is the VSAN write error count.
	VSANWriteErrors uint64 `json:"vsan_write_errors,omitempty"`
	// VSANAvgLatency is the VSAN average latency in microseconds.
	VSANAvgLatency uint32 `json:"vsan_avg_latency,omitempty"`
	// VSANMaxLatency is the VSAN maximum latency in microseconds.
	VSANMaxLatency uint32 `json:"vsan_max_latency,omitempty"`
	// VSANRepairing is the number of blocks being repaired.
	VSANRepairing uint64 `json:"vsan_repairing,omitempty"`
	// VSANRepairEstimate is the estimated repair time in seconds.
	VSANRepairEstimate uint64 `json:"vsan_repair_estimate,omitempty"`
	// VSANLastError is the last VSAN error message.
	VSANLastError string `json:"vsan_last_error,omitempty"`
	// VSANThrottle is the VSAN write throttle in bytes per second.
	VSANThrottle uint64 `json:"vsan_throttle,omitempty"`
	// VSANPath is the VSAN drive path.
	VSANPath string `json:"vsan_path,omitempty"`
	// VSANOnlineSince is the timestamp when the drive came online.
	VSANOnlineSince uint32 `json:"vsan_online_since,omitempty"`

	// Physical drive configuration
	// Spare indicates whether this is a VSAN spare drive.
	Spare bool `json:"spare,omitempty"`
	// Swap indicates whether this drive has a swap partition.
	Swap bool `json:"swap,omitempty"`
	// SwapSize is the swap partition size in bytes.
	SwapSize uint64 `json:"swap_size,omitempty"`
	// Boot indicates whether this drive has a boot partition.
	Boot bool `json:"boot,omitempty"`
	// BootSize is the boot partition size in bytes.
	BootSize uint64 `json:"boot_size,omitempty"`
	// YBPart indicates whether this drive has a VergeOS partition.
	YBPart bool `json:"ybpart,omitempty"`
	// YBPartSize is the VergeOS partition size in bytes.
	YBPartSize uint64 `json:"ybpart_size,omitempty"`
	// EncryptionKey indicates whether the drive holds the encryption key.
	EncryptionKey bool `json:"encryption_key,omitempty"`

	// Location information
	// EnclosureSlot is the enclosure slot number.
	EnclosureSlot string `json:"enclosure_slot,omitempty"`
	// LocateStatus is the locate LED status (unsupported, on, off).
	LocateStatus string `json:"locate_status,omitempty"`
	// Location is the physical location description.
	Location string `json:"location,omitempty"`
	// Bus is the bus type.
	Bus string `json:"bus,omitempty"`
}

// ClusterStatsHistory represents a historical cluster statistics record.
// These records are stored at regular intervals for monitoring.
type ClusterStatsHistory struct {
	// Key is the unique identifier for this record.
	Key FlexInt `json:"$key,omitempty"`
	// Cluster is the parent cluster ID.
	Cluster int `json:"cluster,omitempty"`
	// Timestamp is the record timestamp (Unix epoch).
	Timestamp uint32 `json:"timestamp,omitempty"`

	// Node metrics
	// TotalNodes is the total number of nodes in the cluster.
	TotalNodes uint32 `json:"total_nodes,omitempty"`
	// OnlineNodes is the number of online nodes.
	OnlineNodes uint32 `json:"online_nodes,omitempty"`
	// RunningMachines is the number of running machines.
	RunningMachines uint32 `json:"running_machines,omitempty"`

	// RAM metrics (in MB)
	// TotalRAM is the total RAM in MB.
	TotalRAM uint32 `json:"total_ram,omitempty"`
	// OnlineRAM is the online RAM in MB.
	OnlineRAM uint32 `json:"online_ram,omitempty"`
	// UsedRAM is the used RAM in MB.
	UsedRAM uint32 `json:"used_ram,omitempty"`

	// Core metrics
	// TotalCores is the total CPU cores.
	TotalCores uint32 `json:"total_cores,omitempty"`
	// OnlineCores is the online CPU cores.
	OnlineCores uint32 `json:"online_cores,omitempty"`
	// UsedCores is the used CPU cores.
	UsedCores uint32 `json:"used_cores,omitempty"`

	// Physical resource metrics
	// PhysRAMUsed is the physical RAM used in bytes.
	PhysRAMUsed uint64 `json:"phys_ram_used,omitempty"`
	// PhysVRAMUsed is the physical VRAM used in bytes.
	PhysVRAMUsed uint64 `json:"phys_vram_used,omitempty"`
	// PhysTotalCPU is the physical CPU usage percentage.
	PhysTotalCPU uint8 `json:"phys_total_cpu,omitempty"`

	// GPU metrics
	// GPUsTotal is the total physical GPUs.
	GPUsTotal uint16 `json:"gpus_total,omitempty"`
	// GPUs is the available GPUs.
	GPUs uint16 `json:"gpus,omitempty"`
	// GPUsIdle is the idle GPUs.
	GPUsIdle uint16 `json:"gpus_idle,omitempty"`
	// VGPUsTotal is the total vGPUs.
	VGPUsTotal uint16 `json:"vgpus_total,omitempty"`
	// VGPUs is the available vGPUs.
	VGPUs uint16 `json:"vgpus,omitempty"`
	// VGPUsIdle is the idle vGPUs.
	VGPUsIdle uint16 `json:"vgpus_idle,omitempty"`
}

// Field list constants for VSAN resources
const (
	// storageTierListFields lists all fields for storage tier queries.
	storageTierListFields = "$key,tier,description,capacity,used,allocated,used_inflated,dedupe_ratio,used_pct,modified,stats[reads,writes,read_bytes,write_bytes,rops,wops,rbps,wbps]"

	// clusterTierListFields lists all fields for cluster tier queries.
	clusterTierListFields = "$key,cluster,tier,description,cost_per_gb,price_per_gb,status[tier,status,state,capacity,used,used_pct,allocated,redundant,encrypted,working,last_walk_time_ms,last_fullwalk_time_ms,transaction,repairs,bad_drives,fullwalk,progress,index_unique,state_timestamp,cur_space_throttle_ms,transaction_start_stamp],stats[reads,writes,read_bytes,write_bytes,rops,wops,rbps,wbps]"

	// machineDrivePhysListFields lists all fields for machine drive physical status queries.
	machineDrivePhysListFields = "$key,parent_drive,path,all_paths,modified,size,model,fw,serial,temp,temp_warn,encrypted,realloc_sectors,realloc_sectors_warn,wear_level,wear_level_warn,hours,hours_warn,current_pending_sector,current_pending_sector_warn,offline_uncorrectable,offline_uncorrectable_warn,smart,vsan_driveid,vsan_tier,vsan_used,vsan_max,vsan_read_errors,vsan_write_errors,vsan_avg_latency,vsan_max_latency,vsan_repairing,vsan_repair_estimate,vsan_last_error,vsan_throttle,vsan_path,vsan_online_since,spare,swap,swap_size,boot,boot_size,ybpart,ybpart_size,encryption_key,enclosure_slot,locate_status,location,bus"

	// clusterStatsHistoryFields lists all fields for cluster stats history queries.
	clusterStatsHistoryFields = "$key,cluster,timestamp,total_nodes,online_nodes,running_machines,total_ram,online_ram,used_ram,total_cores,online_cores,used_cores,phys_ram_used,phys_vram_used,phys_total_cpu,gpus_total,gpus,gpus_idle,vgpus_total,vgpus,vgpus_idle"
)
