package vergeos

// Cluster represents a VergeOS cluster.
type Cluster struct {
	// Key is the unique identifier for the cluster (row key).
	Key FlexInt `json:"$key,omitempty"`
	// ID is the 40-character unique cluster identifier string.
	ID string `json:"id,omitempty"`
	// System is the parent system reference.
	// Returns "self" for the local system, or potentially a row key for tenant hierarchies.
	// Note: Schema defines as type "row" but API returns string "self".
	System string `json:"system,omitempty"`
	// Name is the cluster name.
	Name string `json:"name"`
	// Description is the cluster description.
	Description string `json:"description,omitempty"`
	// Enabled indicates whether the cluster is enabled.
	Enabled bool `json:"enabled"`
	// Created is the creation timestamp.
	Created int64 `json:"created,omitempty"`

	// Storage indicates whether this is a storage cluster.
	Storage bool `json:"storage,omitempty"`
	// Compute indicates whether this is a compute cluster.
	Compute bool `json:"compute,omitempty"`

	// KVMNested enables nested virtualization.
	KVMNested bool `json:"kvm_nested,omitempty"`
	// AllowNestedVirtMigration allows live migration of nested virtualization VMs.
	AllowNestedVirtMigration bool `json:"allow_nested_virt_migration,omitempty"`
	// AllowVGPUMigration allows live migration of vGPU VMs (experimental).
	AllowVGPUMigration bool `json:"allow_vgpu_migration,omitempty"`
	// EnableSplitLockDetection enables split lock detection (may impact VM performance).
	EnableSplitLockDetection bool `json:"enable_split_lock_detection,omitempty"`

	// RecommendedCPUType is the detected recommended CPU type for the cluster.
	RecommendedCPUType string `json:"recommended_cpu_type,omitempty"`
	// DefaultCPU is the default CPU type for VMs in this cluster.
	DefaultCPU string `json:"default_cpu,omitempty"`

	// DisableCPUSecurityMitigations disables CPU security mitigations.
	DisableCPUSecurityMitigations bool `json:"disable_cpu_security_mitigations,omitempty"`
	// SpecStoreBypassDisable disables speculative store bypass.
	SpecStoreBypassDisable bool `json:"spec_store_bypass_disable,omitempty"`
	// DisableSMT disables hyper-threading.
	DisableSMT bool `json:"disable_smt,omitempty"`
	// DisableSleep disables CPU sleep states.
	DisableSleep bool `json:"disable_sleep,omitempty"`

	// EnableNVMEPowerManagement enables NVMe power management.
	EnableNVMEPowerManagement bool `json:"enable_nvme_power_management,omitempty"`

	// X86EnergyPerfPolicy is the energy-performance policy.
	// Valid values: "balance-performance", "balance-power", "normal", "performance", "power"
	X86EnergyPerfPolicy string `json:"x86_energy_perf_policy,omitempty"`
	// ScalingGovernor is the CPU scaling governor.
	// Valid values: "ondemand", "performance", "powersave"
	ScalingGovernor string `json:"scaling_governor,omitempty"`

	// RAMPerUnit is the RAM per billing unit in MB.
	RAMPerUnit int `json:"ram_per_unit,omitempty"`
	// CoresPerUnit is the cores per billing unit.
	CoresPerUnit int `json:"cores_per_unit,omitempty"`
	// CostPerUnit is the cost per unit.
	CostPerUnit float64 `json:"cost_per_unit,omitempty"`
	// PricePerUnit is the price per unit.
	PricePerUnit float64 `json:"price_per_unit,omitempty"`

	// MaxRAMPerVM is the maximum RAM per VM in MB.
	MaxRAMPerVM int `json:"max_ram_per_vm,omitempty"`
	// MaxCoresPerVM is the maximum cores per VM.
	MaxCoresPerVM int `json:"max_cores_per_vm,omitempty"`

	// StorageCacheSize is the storage cache per node in MB.
	StorageCacheSize int `json:"storage_cachesize,omitempty"`
	// StorageBufferSize is the storage buffer per node in MB.
	StorageBufferSize int `json:"storage_buffersize,omitempty"`
	// StorageHugepages indicates whether to allocate hugepages for storage.
	StorageHugepages bool `json:"storage_hugepages,omitempty"`

	// TargetRAMPct is the target maximum RAM percentage (0-100).
	TargetRAMPct float64 `json:"target_ram_pct,omitempty"`
	// RAMOvercommitPct is the percentage of reserve RAM to use for machines (0-100).
	RAMOvercommitPct float64 `json:"ram_overcommit_pct,omitempty"`

	// SwapTier is the tier used for swap (-1 to 5, -1 = disabled).
	SwapTier int `json:"swap_tier,omitempty"`
	// SwapPerDrive is the swap space per drive in MB.
	SwapPerDrive int `json:"swap_per_drive,omitempty"`

	// LogFilter is the system log filter expression.
	LogFilter string `json:"log_filter,omitempty"`

	// MaxCoreTemp is the maximum core temperature in Celsius (0 = disabled).
	MaxCoreTemp int `json:"max_core_temp,omitempty"`
	// MaxCoreTempWarnPerc is the warning threshold percentage of max core temp.
	MaxCoreTempWarnPerc int `json:"max_core_temp_warn_perc,omitempty"`
	// CriticalCoreTemp is the critical core temperature in Celsius (0 = disabled).
	CriticalCoreTemp int `json:"critical_core_temp,omitempty"`

	// Status contains cluster status information.
	Status *ClusterStatus `json:"status,omitempty"`
}

// ClusterStatus contains cluster status information.
type ClusterStatus struct {
	// Cluster is the parent cluster reference.
	Cluster int `json:"cluster,omitempty"`
	// Status is the cluster status.
	// Valid values: "online", "shutdown", "offline", "maintenance", "reduced", "noredundant", "error", "updating", "insufficient"
	Status string `json:"status,omitempty"`
	// StatusInfo provides additional status information.
	StatusInfo string `json:"status_info,omitempty"`
	// State is the cluster state (online, offline, warning, error).
	State string `json:"state,omitempty"`
	// LastUpdate is the last update timestamp.
	LastUpdate int64 `json:"last_update,omitempty"`

	// TotalNodes is the total number of nodes.
	TotalNodes int `json:"total_nodes,omitempty"`
	// OnlineNodes is the number of online nodes.
	OnlineNodes int `json:"online_nodes,omitempty"`
	// RunningMachines is the number of running machines.
	RunningMachines int `json:"running_machines,omitempty"`

	// TotalRAM is the total RAM in MB.
	TotalRAM int64 `json:"total_ram,omitempty"`
	// OnlineRAM is the online RAM in MB.
	OnlineRAM int64 `json:"online_ram,omitempty"`
	// UsedRAM is the used RAM in MB.
	UsedRAM int64 `json:"used_ram,omitempty"`

	// TotalCores is the total CPU cores.
	TotalCores int `json:"total_cores,omitempty"`
	// OnlineCores is the online CPU cores.
	OnlineCores int `json:"online_cores,omitempty"`
	// UsedCores is the used CPU cores.
	UsedCores int `json:"used_cores,omitempty"`

	// PhysRAMUsed is the physical RAM used in bytes.
	PhysRAMUsed int64 `json:"phys_ram_used,omitempty"`
	// PhysVRAMUsed is the physical virtual RAM used in bytes.
	PhysVRAMUsed int64 `json:"phys_vram_used,omitempty"`
	// PhysTotalCPU is the physical total CPU usage percentage.
	PhysTotalCPU int `json:"phys_total_cpu,omitempty"`
}

// Node represents a VergeOS node.
type Node struct {
	// ID is the unique identifier for the node (same as $key).
	ID int `json:"id,omitempty"`
	// Name is the node hostname.
	Name string `json:"name"`
	// Description is the node description.
	Description string `json:"description,omitempty"`
	// Physical indicates whether this is a physical node.
	Physical bool `json:"physical,omitempty"`
	// Cluster is the cluster ID the node belongs to.
	Cluster int `json:"cluster,omitempty"`
	// Machine is the machine ID.
	Machine int `json:"machine,omitempty"`

	// Hardware information
	// Model is the hardware model.
	Model string `json:"model,omitempty"`
	// AssetTag is the hardware asset tag.
	AssetTag string `json:"asset_tag,omitempty"`
	// CPU is the CPU description.
	CPU string `json:"cpu,omitempty"`
	// CPUSpeed is the CPU speed.
	CPUSpeed string `json:"cpu_speed,omitempty"`
	// Cores is the total CPU cores.
	Cores int `json:"cores,omitempty"`
	// IOMMU indicates whether IOMMU (VT-d) is supported.
	IOMMU bool `json:"iommu,omitempty"`

	// Memory configuration
	// RAM is the total RAM in MB.
	RAM int `json:"ram,omitempty"`
	// Overcommit is the overcommit RAM in MB.
	Overcommit int `json:"overcommit,omitempty"`
	// VMRAM is the VM RAM limit in MB.
	VMRAM int `json:"vm_ram,omitempty"`
	// FailoverRAM is the failover RAM reserve in MB.
	FailoverRAM int `json:"failover_ram,omitempty"`

	// Software versions
	// YBVersion is the VergeOS version.
	YBVersion string `json:"yb_version,omitempty"`
	// OSVersion is the OS version.
	OSVersion string `json:"os_version,omitempty"`
	// KernelVersion is the kernel version.
	KernelVersion string `json:"kernel_version,omitempty"`
	// AppserverVersion is the appserver version.
	AppserverVersion string `json:"appserver_version,omitempty"`
	// VSANVersion is the vSAN version.
	VSANVersion string `json:"vsan_version,omitempty"`
	// QEMUVersion is the QEMU version.
	QEMUVersion string `json:"qemu_version,omitempty"`

	// vSAN configuration
	// VSANNodeID is the vSAN node ID (-1 if not in vSAN).
	VSANNodeID int `json:"vsan_nodeid,omitempty"`
	// VSANConnected indicates whether the node is connected to vSAN.
	VSANConnected bool `json:"vsan_connected,omitempty"`

	// Maintenance
	// Maintenance indicates whether the node is in maintenance mode.
	Maintenance bool `json:"maintenance,omitempty"`
	// NeedRestart indicates whether the node needs to be rebooted.
	NeedRestart bool `json:"need_restart,omitempty"`
	// RestartReason explains why a restart is needed.
	RestartReason string `json:"restart_reason,omitempty"`

	// IPMI configuration
	// IPMIAddress is the IPMI management IP address.
	IPMIAddress string `json:"ipmi_address,omitempty"`
	// IPMIUser is the IPMI username.
	IPMIUser string `json:"ipmi_user,omitempty"`
	// IPMIStatus is the IPMI connection status (offline, ready, connecting, error).
	IPMIStatus string `json:"ipmi_status,omitempty"`
	// IPMIStatusInfo provides additional IPMI status details.
	IPMIStatusInfo string `json:"ipmi_status_info,omitempty"`

	// Temperature thresholds
	// MaxCoreTemp is the maximum core temperature in Celsius (0 = disabled, -1 = inherit from cluster).
	MaxCoreTemp int `json:"max_core_temp,omitempty"`
	// MaxCoreTempWarnPerc is the warning threshold percentage.
	MaxCoreTempWarnPerc int `json:"max_core_temp_warn_perc,omitempty"`
	// CriticalCoreTemp is the critical core temperature in Celsius.
	CriticalCoreTemp int `json:"critical_core_temp,omitempty"`

	// Networking
	// PXEVnet is the PXE boot network ID (nullable).
	PXEVnet *int `json:"pxe_vnet,omitempty"`
	// CaptureLogs indicates whether to capture system logs.
	CaptureLogs bool `json:"capture_logs,omitempty"`
	// LLDP indicates whether to send/receive LLDP advertisements.
	LLDP bool `json:"lldp,omitempty"`

	// Notes
	// Note is a free-form note about the node.
	Note string `json:"note,omitempty"`
}

// NodeStatus contains node status information.
type NodeStatus struct {
	// CPUTemp is the CPU temperature.
	CPUTemp float64 `json:"cpu_temp,omitempty"`
	// CPUUsage is the CPU usage percentage.
	CPUUsage float64 `json:"cpu_usage,omitempty"`
	// MemoryTotal is the total memory in bytes.
	MemoryTotal int64 `json:"memory_total,omitempty"`
	// MemoryUsed is the used memory in bytes.
	MemoryUsed int64 `json:"memory_used,omitempty"`
	// MemoryPct is the memory usage percentage.
	MemoryPct float64 `json:"memory_pct,omitempty"`
}

// Group represents a VergeOS group.
type Group struct {
	// ID is the unique identifier for the group.
	ID FlexInt `json:"$key,omitempty"`
	// Name is the group name.
	Name string `json:"name"`
	// Description is the group description.
	Description string `json:"description,omitempty"`
	// Enabled indicates whether the group is enabled.
	Enabled bool `json:"enabled"`
	// Type is the group type.
	Type string `json:"type,omitempty"`
	// Email is the group email address.
	Email string `json:"email,omitempty"`
	// SystemGroup indicates whether this is a system-managed group.
	SystemGroup bool `json:"system_group,omitempty"`
	// AuthSource is the authentication source ID.
	AuthSource int `json:"auth_source,omitempty"`
	// Created is the creation timestamp (Unix epoch).
	Created int64 `json:"created,omitempty"`
	// Creator is the username that created this group.
	Creator string `json:"creator,omitempty"`
}

// GroupCreateRequest is the request body for creating a group.
type GroupCreateRequest struct {
	// Name is the group name (required).
	Name string `json:"name"`
	// Description is the group description.
	Description string `json:"description,omitempty"`
	// Enabled indicates whether the group is enabled.
	Enabled *bool `json:"enabled,omitempty"`
	// Email is the group email address.
	Email string `json:"email,omitempty"`
	// AuthSource is the authentication source ID.
	AuthSource *int `json:"auth_source,omitempty"`
}

// GroupUpdateRequest is the request body for updating a group.
type GroupUpdateRequest struct {
	// Name is the group name.
	Name *string `json:"name,omitempty"`
	// Description is the group description.
	Description *string `json:"description,omitempty"`
	// Enabled indicates whether the group is enabled.
	Enabled *bool `json:"enabled,omitempty"`
	// Email is the group email address.
	Email *string `json:"email,omitempty"`
}

// File represents a file (ISO, image, etc.) in VergeOS.
type File struct {
	// ID is the unique identifier for the file.
	ID FlexInt `json:"$key,omitempty"`
	// Name is the file name.
	Name string `json:"name"`
	// Description is the description.
	Description string `json:"description,omitempty"`
	// Type is the file type (iso, img, qcow, qcow2, raw, vdi, vhd, vhdx, vmdk, ova, etc.).
	Type string `json:"type,omitempty"`
	// Owner is the owning machine drive ID.
	Owner FlexInt `json:"owner,omitempty"`
	// AllocatedBytes is the allocated size in bytes.
	AllocatedBytes int64 `json:"allocated_bytes,omitempty"`
	// UsedBytes is the actual size on disk in bytes.
	UsedBytes int64 `json:"used_bytes,omitempty"`
	// Filesize is the logical file size in bytes.
	Filesize int64 `json:"filesize,omitempty"`
	// Modified is the last modified timestamp (Unix epoch).
	Modified int64 `json:"modified,omitempty"`
	// PreferredTier is the preferred storage tier (1-5).
	PreferredTier string `json:"preferred_tier,omitempty"`
	// URL is the source URL if the file was imported from a URL.
	URL string `json:"url,omitempty"`
	// Creator is the username that created this file.
	Creator string `json:"creator,omitempty"`
}

// FileCreateRequest is the request body for creating a file entry.
// After creating the entry, use Upload() to upload the file content.
type FileCreateRequest struct {
	// Name is the file name (required).
	Name string `json:"name"`
	// Description is an optional description.
	Description string `json:"description,omitempty"`
	// AllocatedBytes is the expected file size in bytes (required for uploads).
	// Pass as string to match API expectations.
	AllocatedBytes string `json:"allocated_bytes,omitempty"`
	// PreferredTier is the preferred storage tier (1-5).
	PreferredTier string `json:"preferred_tier,omitempty"`
	// URL is a source URL to import the file from (optional).
	// If provided, VergeOS will download the file from this URL.
	URL string `json:"url,omitempty"`
}

// FileUpdateRequest is the request body for updating a file.
type FileUpdateRequest struct {
	// Name is the file name.
	Name *string `json:"name,omitempty"`
	// Description is the description.
	Description *string `json:"description,omitempty"`
	// PreferredTier is the preferred storage tier (1-5).
	PreferredTier *string `json:"preferred_tier,omitempty"`
}

// ResourceGroup represents a resource group in VergeOS.
type ResourceGroup struct {
	// ID is the unique identifier for the resource group (UUID string).
	ID string `json:"$key,omitempty"`
	// Name is the resource group name.
	Name string `json:"name"`
	// Description is the description.
	Description string `json:"description,omitempty"`
	// Type is the resource group type.
	Type string `json:"type,omitempty"`
	// Enabled indicates whether the resource group is enabled.
	Enabled bool `json:"enabled"`
}

// Setting represents a VergeOS system setting.
type Setting struct {
	// Key is the setting key/name (used as identifier).
	Key string `json:"key,omitempty"`
	// Value is the setting value.
	Value string `json:"value,omitempty"`
	// DefaultValue is the default value.
	DefaultValue string `json:"default_value,omitempty"`
	// Description is the setting description.
	Description string `json:"description,omitempty"`
}

// SystemInfo represents VergeOS system information.
type SystemInfo struct {
	// Name is the API name (e.g., "v4").
	Name string `json:"name,omitempty"`
	// Version is the VergeOS version.
	Version string `json:"version,omitempty"`
	// Hash is the build hash.
	Hash string `json:"hash,omitempty"`
}

// TableSchema represents the schema for an API resource.
type TableSchema struct {
	// Fields contains field definitions.
	Fields map[string]TableField `json:"fields,omitempty"`
}

// TableField represents a field definition in a table schema.
type TableField struct {
	// Type is the field type.
	Type string `json:"type,omitempty"`
	// List contains valid values and their descriptions.
	List map[string]string `json:"list,omitempty"`
	// Required indicates whether the field is required.
	Required bool `json:"required,omitempty"`
}

// ClusterCreateRequest is the request body for creating a cluster.
type ClusterCreateRequest struct {
	// Name is the cluster name (required).
	Name string `json:"name"`
	// Description is the cluster description.
	Description string `json:"description,omitempty"`
	// Enabled indicates whether the cluster is enabled (default: true).
	Enabled *bool `json:"enabled,omitempty"`
	// Compute indicates whether this is a compute cluster.
	Compute *bool `json:"compute,omitempty"`
	// KVMNested enables nested virtualization.
	KVMNested *bool `json:"kvm_nested,omitempty"`
	// AllowNestedVirtMigration allows live migration of nested virtualization VMs.
	AllowNestedVirtMigration *bool `json:"allow_nested_virt_migration,omitempty"`
	// AllowVGPUMigration allows live migration of vGPU VMs (experimental).
	AllowVGPUMigration *bool `json:"allow_vgpu_migration,omitempty"`
	// EnableSplitLockDetection enables split lock detection.
	EnableSplitLockDetection *bool `json:"enable_split_lock_detection,omitempty"`
	// DefaultCPU is the default CPU type for VMs.
	DefaultCPU *string `json:"default_cpu,omitempty"`
	// DisableCPUSecurityMitigations disables CPU security mitigations.
	DisableCPUSecurityMitigations *bool `json:"disable_cpu_security_mitigations,omitempty"`
	// SpecStoreBypassDisable disables speculative store bypass.
	SpecStoreBypassDisable *bool `json:"spec_store_bypass_disable,omitempty"`
	// DisableSMT disables hyper-threading.
	DisableSMT *bool `json:"disable_smt,omitempty"`
	// DisableSleep disables CPU sleep states.
	DisableSleep *bool `json:"disable_sleep,omitempty"`
	// EnableNVMEPowerManagement enables NVMe power management.
	EnableNVMEPowerManagement *bool `json:"enable_nvme_power_management,omitempty"`
	// X86EnergyPerfPolicy is the energy-performance policy.
	X86EnergyPerfPolicy *string `json:"x86_energy_perf_policy,omitempty"`
	// ScalingGovernor is the CPU scaling governor.
	ScalingGovernor *string `json:"scaling_governor,omitempty"`
	// RAMPerUnit is the RAM per billing unit in MB.
	RAMPerUnit *int `json:"ram_per_unit,omitempty"`
	// CoresPerUnit is the cores per billing unit.
	CoresPerUnit *int `json:"cores_per_unit,omitempty"`
	// CostPerUnit is the cost per unit.
	CostPerUnit *float64 `json:"cost_per_unit,omitempty"`
	// PricePerUnit is the price per unit.
	PricePerUnit *float64 `json:"price_per_unit,omitempty"`
	// MaxRAMPerVM is the maximum RAM per VM in MB.
	MaxRAMPerVM *int `json:"max_ram_per_vm,omitempty"`
	// MaxCoresPerVM is the maximum cores per VM.
	MaxCoresPerVM *int `json:"max_cores_per_vm,omitempty"`
	// StorageCacheSize is the storage cache per node in MB.
	StorageCacheSize *int `json:"storage_cachesize,omitempty"`
	// StorageBufferSize is the storage buffer per node in MB.
	StorageBufferSize *int `json:"storage_buffersize,omitempty"`
	// StorageHugepages indicates whether to allocate hugepages for storage.
	StorageHugepages *bool `json:"storage_hugepages,omitempty"`
	// TargetRAMPct is the target maximum RAM percentage (0-100).
	TargetRAMPct *float64 `json:"target_ram_pct,omitempty"`
	// RAMOvercommitPct is the percentage of reserve RAM to use for machines (0-100).
	RAMOvercommitPct *float64 `json:"ram_overcommit_pct,omitempty"`
	// SwapTier is the tier used for swap (-1 to 5, -1 = disabled).
	SwapTier *int `json:"swap_tier,omitempty"`
	// SwapPerDrive is the swap space per drive in MB.
	SwapPerDrive *int `json:"swap_per_drive,omitempty"`
	// LogFilter is the system log filter expression.
	LogFilter *string `json:"log_filter,omitempty"`
	// MaxCoreTemp is the maximum core temperature in Celsius (0 = disabled).
	MaxCoreTemp *int `json:"max_core_temp,omitempty"`
	// MaxCoreTempWarnPerc is the warning threshold percentage of max core temp.
	MaxCoreTempWarnPerc *int `json:"max_core_temp_warn_perc,omitempty"`
	// CriticalCoreTemp is the critical core temperature in Celsius (0 = disabled).
	CriticalCoreTemp *int `json:"critical_core_temp,omitempty"`
}

// ClusterUpdateRequest is the request body for updating a cluster.
type ClusterUpdateRequest struct {
	// Name is the cluster name.
	Name *string `json:"name,omitempty"`
	// Description is the cluster description.
	Description *string `json:"description,omitempty"`
	// Enabled indicates whether the cluster is enabled.
	Enabled *bool `json:"enabled,omitempty"`
	// Compute indicates whether this is a compute cluster.
	Compute *bool `json:"compute,omitempty"`
	// KVMNested enables nested virtualization.
	KVMNested *bool `json:"kvm_nested,omitempty"`
	// AllowNestedVirtMigration allows live migration of nested virtualization VMs.
	AllowNestedVirtMigration *bool `json:"allow_nested_virt_migration,omitempty"`
	// AllowVGPUMigration allows live migration of vGPU VMs (experimental).
	AllowVGPUMigration *bool `json:"allow_vgpu_migration,omitempty"`
	// EnableSplitLockDetection enables split lock detection.
	EnableSplitLockDetection *bool `json:"enable_split_lock_detection,omitempty"`
	// DefaultCPU is the default CPU type for VMs.
	DefaultCPU *string `json:"default_cpu,omitempty"`
	// DisableCPUSecurityMitigations disables CPU security mitigations.
	DisableCPUSecurityMitigations *bool `json:"disable_cpu_security_mitigations,omitempty"`
	// SpecStoreBypassDisable disables speculative store bypass.
	SpecStoreBypassDisable *bool `json:"spec_store_bypass_disable,omitempty"`
	// DisableSMT disables hyper-threading.
	DisableSMT *bool `json:"disable_smt,omitempty"`
	// DisableSleep disables CPU sleep states.
	DisableSleep *bool `json:"disable_sleep,omitempty"`
	// EnableNVMEPowerManagement enables NVMe power management.
	EnableNVMEPowerManagement *bool `json:"enable_nvme_power_management,omitempty"`
	// X86EnergyPerfPolicy is the energy-performance policy.
	X86EnergyPerfPolicy *string `json:"x86_energy_perf_policy,omitempty"`
	// ScalingGovernor is the CPU scaling governor.
	ScalingGovernor *string `json:"scaling_governor,omitempty"`
	// RAMPerUnit is the RAM per billing unit in MB.
	RAMPerUnit *int `json:"ram_per_unit,omitempty"`
	// CoresPerUnit is the cores per billing unit.
	CoresPerUnit *int `json:"cores_per_unit,omitempty"`
	// CostPerUnit is the cost per unit.
	CostPerUnit *float64 `json:"cost_per_unit,omitempty"`
	// PricePerUnit is the price per unit.
	PricePerUnit *float64 `json:"price_per_unit,omitempty"`
	// MaxRAMPerVM is the maximum RAM per VM in MB.
	MaxRAMPerVM *int `json:"max_ram_per_vm,omitempty"`
	// MaxCoresPerVM is the maximum cores per VM.
	MaxCoresPerVM *int `json:"max_cores_per_vm,omitempty"`
	// StorageCacheSize is the storage cache per node in MB.
	StorageCacheSize *int `json:"storage_cachesize,omitempty"`
	// StorageBufferSize is the storage buffer per node in MB.
	StorageBufferSize *int `json:"storage_buffersize,omitempty"`
	// StorageHugepages indicates whether to allocate hugepages for storage.
	StorageHugepages *bool `json:"storage_hugepages,omitempty"`
	// TargetRAMPct is the target maximum RAM percentage (0-100).
	TargetRAMPct *float64 `json:"target_ram_pct,omitempty"`
	// RAMOvercommitPct is the percentage of reserve RAM to use for machines (0-100).
	RAMOvercommitPct *float64 `json:"ram_overcommit_pct,omitempty"`
	// SwapTier is the tier used for swap (-1 to 5, -1 = disabled).
	SwapTier *int `json:"swap_tier,omitempty"`
	// SwapPerDrive is the swap space per drive in MB.
	SwapPerDrive *int `json:"swap_per_drive,omitempty"`
	// LogFilter is the system log filter expression.
	LogFilter *string `json:"log_filter,omitempty"`
	// MaxCoreTemp is the maximum core temperature in Celsius (0 = disabled).
	MaxCoreTemp *int `json:"max_core_temp,omitempty"`
	// MaxCoreTempWarnPerc is the warning threshold percentage of max core temp.
	MaxCoreTempWarnPerc *int `json:"max_core_temp_warn_perc,omitempty"`
	// CriticalCoreTemp is the critical core temperature in Celsius (0 = disabled).
	CriticalCoreTemp *int `json:"critical_core_temp,omitempty"`
}

// Field list constants for read-only resources
const (
	clusterListFields       = "$key,id,system,name,description,enabled,created,storage,compute,kvm_nested,allow_nested_virt_migration,allow_vgpu_migration,enable_split_lock_detection,recommended_cpu_type,default_cpu,disable_cpu_security_mitigations,spec_store_bypass_disable,disable_smt,disable_sleep,enable_nvme_power_management,x86_energy_perf_policy,scaling_governor,ram_per_unit,cores_per_unit,cost_per_unit,price_per_unit,max_ram_per_vm,max_cores_per_vm,storage_cachesize,storage_buffersize,storage_hugepages,target_ram_pct,ram_overcommit_pct,swap_tier,swap_per_drive,log_filter,max_core_temp,max_core_temp_warn_perc,critical_core_temp"
	clusterStatusFields     = "cluster,status,status_info,state,last_update,total_nodes,online_nodes,running_machines,total_ram,online_ram,used_ram,total_cores,online_cores,used_cores,phys_ram_used,phys_vram_used,phys_total_cpu"
	nodeListFields          = "id,name,description,physical,cluster,machine,model,asset_tag,cpu,cpu_speed,cores,iommu,ram,overcommit,vm_ram,failover_ram,yb_version,os_version,kernel_version,appserver_version,vsan_version,qemu_version,vsan_nodeid,vsan_connected,maintenance,need_restart,restart_reason,ipmi_address,ipmi_user,ipmi_status,ipmi_status_info,max_core_temp,max_core_temp_warn_perc,critical_core_temp,pxe_vnet,capture_logs,lldp,note"
	groupListFields         = "$key,name,description,enabled,type,email,system_group,auth_source,created,creator"
	fileListFields          = "$key,name,description,type,owner,allocated_bytes,used_bytes,filesize,modified,preferred_tier,url,creator"
	resourceGroupListFields = "$key,name,description,type,enabled"
	settingListFields       = "key,value,default_value,description"
)
