package vergeos

import "encoding/json"

// VM represents a VergeOS virtual machine.
type VM struct {
	// ID is the unique identifier for the VM.
	ID FlexInt `json:"$key,omitempty"`
	// UUID is the universally unique identifier for the VM.
	UUID string `json:"uuid,omitempty"`
	// Machine is the machine reference ID.
	Machine int `json:"machine,omitempty"`
	// Name is the VM name.
	Name string `json:"name"`
	// Description is an optional description of the VM.
	Description string `json:"description,omitempty"`
	// Enabled indicates whether the VM is enabled.
	Enabled bool `json:"enabled"`
	// Created is the creation timestamp (Unix epoch).
	Created int64 `json:"created,omitempty"`
	// Modified is the last modified timestamp (Unix epoch).
	Modified int64 `json:"modified,omitempty"`
	// IsSnapshot indicates whether this VM is a snapshot.
	IsSnapshot bool `json:"is_snapshot,omitempty"`
	// Owner is the owner reference path (e.g., "vm_recipes/...", "vms/...").
	Owner string `json:"owner,omitempty"`
	// OwnerUser is the owner user ID (nullable).
	OwnerUser *int `json:"owner_user,omitempty"`
	// Creator is the username that created this VM.
	Creator string `json:"creator,omitempty"`

	// Cluster configuration
	// Cluster is the cluster the VM belongs to.
	Cluster FlexInt `json:"cluster,omitempty"`
	// ClusterFailover is the failover cluster ID (0 = none).
	ClusterFailover FlexInt `json:"cluster_failover,omitempty"`

	// Hardware configuration
	// CPUCores is the number of CPU cores.
	CPUCores int `json:"cpu_cores"`
	// CPUType is the CPU type (e.g., "host", "qemu64").
	CPUType string `json:"cpu_type,omitempty"`
	// RAM is the amount of RAM in MB.
	RAM int `json:"ram"`
	// MachineType is the machine type (e.g., "pc-i440fx-10.0", "pc-q35-10.0").
	MachineType string `json:"machine_type,omitempty"`
	// IOMMU indicates whether IOMMU is enabled.
	IOMMU bool `json:"iommu,omitempty"`
	// USBLegacy indicates whether to use a legacy USB controller.
	USBLegacy bool `json:"usb_legacy,omitempty"`

	// Boot configuration
	// UEFI indicates whether UEFI boot is enabled.
	UEFI bool `json:"uefi,omitempty"`
	// SecureBoot indicates whether Secure Boot is enabled.
	SecureBoot bool `json:"secure_boot,omitempty"`
	// BootOrder specifies the boot device order.
	BootOrder string `json:"boot_order,omitempty"`
	// BootDelay is the boot delay in seconds.
	BootDelay int `json:"boot_delay,omitempty"`

	// Display and console
	// Console is the console type.
	Console string `json:"console,omitempty"`
	// Display is the display type.
	Display string `json:"display,omitempty"`
	// Video is the video adapter type.
	Video string `json:"video,omitempty"`
	// Sound is the sound device type.
	Sound string `json:"sound,omitempty"`
	// SerialPort indicates whether the serial port is enabled.
	SerialPort bool `json:"serial_port,omitempty"`
	// ConsolePassEnabled indicates whether console password is enabled.
	ConsolePassEnabled bool `json:"console_pass_enabled,omitempty"`
	// ConsolePass is the console password.
	ConsolePass string `json:"console_pass,omitempty"`

	// OS configuration
	// OSFamily is the operating system family.
	OSFamily string `json:"os_family,omitempty"`
	// OSDescription is the operating system description.
	OSDescription string `json:"os_description,omitempty"`
	// RTCBase is the RTC base setting.
	RTCBase string `json:"rtc_base,omitempty"`

	// Features
	// AllowHotplug indicates whether hotplug is allowed.
	AllowHotplug bool `json:"allow_hotplug,omitempty"`
	// DisablePowercycle indicates whether power cycling is disabled.
	DisablePowercycle bool `json:"disable_powercycle,omitempty"`
	// USBTablet indicates whether USB tablet is enabled.
	USBTablet bool `json:"usb_tablet,omitempty"`
	// GuestAgent indicates whether the guest agent is enabled.
	GuestAgent bool `json:"guest_agent,omitempty"`
	// NestedVirtualization indicates whether nested virtualization is enabled.
	NestedVirtualization bool `json:"nested_virtualization,omitempty"`
	// DisableHypervisor indicates whether the hypervisor is disabled.
	DisableHypervisor bool `json:"disable_hypervisor,omitempty"`
	// AllowExport indicates whether this VM can be exported to NAS.
	AllowExport bool `json:"allow_export,omitempty"`

	// Cloud-Init configuration
	// CloudInitDataSource is the cloud-init data source.
	CloudInitDataSource string `json:"cloudinit_datasource,omitempty"`
	// CloudInitFiles are the cloud-init files.
	CloudInitFiles []CloudInitFileRef `json:"cloudinit_files,omitempty"`

	// Scheduling and availability
	// PreferredNode is the preferred node for the VM (0 = none).
	PreferredNode FlexInt `json:"preferred_node,omitempty"`
	// HAGroup is the HA group the VM belongs to.
	HAGroup string `json:"ha_group,omitempty"`
	// SnapshotProfile is the snapshot profile ID (0 = none).
	SnapshotProfile FlexInt `json:"snapshot_profile,omitempty"`
	// OnPowerLoss is the behavior when power is restored.
	// Valid values: "power_on", "last_state", "leave_off"
	OnPowerLoss string `json:"on_power_loss,omitempty"`
	// MigrationMethod is the migration method.
	// Valid values: "auto", "live"
	MigrationMethod string `json:"migration_method,omitempty"`
	// PowerCycleTimeout is the migration power-cycle timeout (0 = use system setting).
	PowerCycleTimeout int `json:"power_cycle_timeout,omitempty"`

	// State
	// PowerState indicates whether the VM is running (true) or stopped (false).
	PowerState bool `json:"powerstate,omitempty"`
	// NeedRestart indicates whether the VM needs to be restarted.
	NeedRestart bool `json:"need_restart,omitempty"`

	// Origin tracking
	// CreatedFrom indicates how the VM was created.
	// Valid values: "import", "import_vmx", "import_ovf", "import_vmware", "import_shared", "clone", "recipe", "custom", "terraform"
	CreatedFrom string `json:"created_from,omitempty"`
	// Imported indicates whether this VM was imported.
	Imported bool `json:"imported,omitempty"`

	// Advanced configuration
	// Advanced is the advanced configuration (key=value pairs, newline separated).
	Advanced string `json:"advanced,omitempty"`
	// Note is a free-form note about the VM.
	Note string `json:"note,omitempty"`
	// Meta contains metadata (raw JSON, typically recipe data).
	Meta json.RawMessage `json:"meta,omitempty"`
	// PasteKeyConfig is the paste key mapping configuration ID (nullable).
	PasteKeyConfig *int `json:"paste_key_config,omitempty"`
}

// CloudInitFileRef is a reference to a cloud-init file within a VM.
type CloudInitFileRef struct {
	// Name is the cloud-init file name.
	Name string `json:"name"`
	// Contents is the cloud-init file contents.
	Contents string `json:"contents,omitempty"`
}

// VMCreateRequest is the request body for creating a VM.
type VMCreateRequest struct {
	// Name is the VM name (required).
	Name string `json:"name"`
	// Description is an optional description of the VM.
	Description string `json:"description,omitempty"`
	// Enabled indicates whether the VM is enabled.
	Enabled *bool `json:"enabled,omitempty"`
	// OwnerUser is the owner user ID.
	OwnerUser *int `json:"owner_user,omitempty"`

	// Cluster configuration
	// Cluster is the cluster to create the VM in.
	Cluster *int `json:"cluster,omitempty"`
	// ClusterFailover is the failover cluster ID.
	ClusterFailover *int `json:"cluster_failover,omitempty"`

	// Hardware configuration
	// CPUCores is the number of CPU cores (required).
	CPUCores int `json:"cpu_cores"`
	// CPUType is the CPU type.
	CPUType string `json:"cpu_type,omitempty"`
	// RAM is the amount of RAM in MB (required).
	RAM int `json:"ram"`
	// MachineType is the machine type.
	MachineType string `json:"machine_type,omitempty"`
	// IOMMU indicates whether IOMMU is enabled.
	IOMMU *bool `json:"iommu,omitempty"`
	// USBLegacy indicates whether to use a legacy USB controller.
	USBLegacy *bool `json:"usb_legacy,omitempty"`

	// Boot configuration
	// UEFI indicates whether UEFI boot is enabled.
	UEFI *bool `json:"uefi,omitempty"`
	// SecureBoot indicates whether Secure Boot is enabled.
	SecureBoot *bool `json:"secure_boot,omitempty"`
	// BootOrder specifies the boot device order.
	BootOrder string `json:"boot_order,omitempty"`
	// BootDelay is the boot delay in seconds.
	BootDelay *int `json:"boot_delay,omitempty"`

	// Display and console
	// Console is the console type.
	Console string `json:"console,omitempty"`
	// Display is the display type.
	Display string `json:"display,omitempty"`
	// Video is the video adapter type.
	Video string `json:"video,omitempty"`
	// Sound is the sound device type.
	Sound string `json:"sound,omitempty"`
	// SerialPort indicates whether the serial port is enabled.
	SerialPort *bool `json:"serial_port,omitempty"`
	// ConsolePassEnabled indicates whether console password is enabled.
	ConsolePassEnabled *bool `json:"console_pass_enabled,omitempty"`
	// ConsolePass is the console password.
	ConsolePass string `json:"console_pass,omitempty"`

	// OS configuration
	// OSFamily is the operating system family.
	OSFamily string `json:"os_family,omitempty"`
	// OSDescription is the operating system description.
	OSDescription string `json:"os_description,omitempty"`
	// RTCBase is the RTC base setting.
	RTCBase string `json:"rtc_base,omitempty"`

	// Features
	// AllowHotplug indicates whether hotplug is allowed.
	AllowHotplug *bool `json:"allow_hotplug,omitempty"`
	// DisablePowercycle indicates whether power cycling is disabled.
	DisablePowercycle *bool `json:"disable_powercycle,omitempty"`
	// USBTablet indicates whether USB tablet is enabled.
	USBTablet *bool `json:"usb_tablet,omitempty"`
	// GuestAgent indicates whether the guest agent is enabled.
	GuestAgent *bool `json:"guest_agent,omitempty"`
	// NestedVirtualization indicates whether nested virtualization is enabled.
	NestedVirtualization *bool `json:"nested_virtualization,omitempty"`
	// DisableHypervisor indicates whether the hypervisor is disabled.
	DisableHypervisor *bool `json:"disable_hypervisor,omitempty"`
	// AllowExport indicates whether this VM can be exported to NAS.
	AllowExport *bool `json:"allow_export,omitempty"`

	// Cloud-Init configuration
	// CloudInitDataSource is the cloud-init data source.
	CloudInitDataSource string `json:"cloudinit_datasource,omitempty"`
	// CloudInitFiles are the cloud-init files.
	CloudInitFiles []CloudInitFileRef `json:"cloudinit_files,omitempty"`

	// Scheduling and availability
	// PreferredNode is the preferred node for the VM.
	PreferredNode *int `json:"preferred_node,omitempty"`
	// HAGroup is the HA group the VM belongs to.
	HAGroup string `json:"ha_group,omitempty"`
	// SnapshotProfile is the snapshot profile ID.
	SnapshotProfile *int `json:"snapshot_profile,omitempty"`
	// OnPowerLoss is the behavior when power is restored.
	OnPowerLoss string `json:"on_power_loss,omitempty"`
	// MigrationMethod is the migration method (auto/live).
	MigrationMethod string `json:"migration_method,omitempty"`
	// PowerCycleTimeout is the migration power-cycle timeout.
	PowerCycleTimeout *int `json:"power_cycle_timeout,omitempty"`

	// Origin tracking
	// CreatedFrom indicates how the VM was created.
	CreatedFrom string `json:"created_from,omitempty"`

	// Advanced configuration
	// Advanced is the advanced configuration.
	Advanced string `json:"advanced,omitempty"`
	// Note is a free-form note about the VM.
	Note string `json:"note,omitempty"`
	// PasteKeyConfig is the paste key mapping configuration ID.
	PasteKeyConfig *int `json:"paste_key_config,omitempty"`
}

// VMUpdateRequest is the request body for updating a VM.
type VMUpdateRequest struct {
	// Name is the VM name.
	Name *string `json:"name,omitempty"`
	// Description is the VM description.
	Description *string `json:"description,omitempty"`
	// Enabled indicates whether the VM is enabled.
	Enabled *bool `json:"enabled,omitempty"`
	// OwnerUser is the owner user ID.
	OwnerUser *int `json:"owner_user,omitempty"`

	// Cluster configuration
	// Cluster is the cluster the VM belongs to.
	Cluster *int `json:"cluster,omitempty"`
	// ClusterFailover is the failover cluster ID.
	ClusterFailover *int `json:"cluster_failover,omitempty"`

	// Hardware configuration
	// CPUCores is the number of CPU cores.
	CPUCores *int `json:"cpu_cores,omitempty"`
	// CPUType is the CPU type.
	CPUType *string `json:"cpu_type,omitempty"`
	// RAM is the amount of RAM in MB.
	RAM *int `json:"ram,omitempty"`
	// MachineType is the machine type.
	MachineType *string `json:"machine_type,omitempty"`
	// IOMMU indicates whether IOMMU is enabled.
	IOMMU *bool `json:"iommu,omitempty"`
	// USBLegacy indicates whether to use a legacy USB controller.
	USBLegacy *bool `json:"usb_legacy,omitempty"`

	// Boot configuration
	// UEFI indicates whether UEFI boot is enabled.
	UEFI *bool `json:"uefi,omitempty"`
	// SecureBoot indicates whether Secure Boot is enabled.
	SecureBoot *bool `json:"secure_boot,omitempty"`
	// BootOrder specifies the boot device order.
	BootOrder *string `json:"boot_order,omitempty"`
	// BootDelay is the boot delay in seconds.
	BootDelay *int `json:"boot_delay,omitempty"`

	// Display and console
	// Console is the console type.
	Console *string `json:"console,omitempty"`
	// Display is the display type.
	Display *string `json:"display,omitempty"`
	// Video is the video adapter type.
	Video *string `json:"video,omitempty"`
	// Sound is the sound device type.
	Sound *string `json:"sound,omitempty"`
	// SerialPort indicates whether the serial port is enabled.
	SerialPort *bool `json:"serial_port,omitempty"`
	// ConsolePassEnabled indicates whether console password is enabled.
	ConsolePassEnabled *bool `json:"console_pass_enabled,omitempty"`
	// ConsolePass is the console password.
	ConsolePass *string `json:"console_pass,omitempty"`

	// OS configuration
	// OSFamily is the operating system family.
	OSFamily *string `json:"os_family,omitempty"`
	// OSDescription is the operating system description.
	OSDescription *string `json:"os_description,omitempty"`
	// RTCBase is the RTC base setting.
	RTCBase *string `json:"rtc_base,omitempty"`

	// Features
	// AllowHotplug indicates whether hotplug is allowed.
	AllowHotplug *bool `json:"allow_hotplug,omitempty"`
	// DisablePowercycle indicates whether power cycling is disabled.
	DisablePowercycle *bool `json:"disable_powercycle,omitempty"`
	// USBTablet indicates whether USB tablet is enabled.
	USBTablet *bool `json:"usb_tablet,omitempty"`
	// GuestAgent indicates whether the guest agent is enabled.
	GuestAgent *bool `json:"guest_agent,omitempty"`
	// NestedVirtualization indicates whether nested virtualization is enabled.
	NestedVirtualization *bool `json:"nested_virtualization,omitempty"`
	// DisableHypervisor indicates whether the hypervisor is disabled.
	DisableHypervisor *bool `json:"disable_hypervisor,omitempty"`
	// AllowExport indicates whether this VM can be exported to NAS.
	AllowExport *bool `json:"allow_export,omitempty"`

	// Cloud-Init configuration
	// CloudInitDataSource is the cloud-init data source.
	CloudInitDataSource *string `json:"cloudinit_datasource,omitempty"`

	// Scheduling and availability
	// PreferredNode is the preferred node for the VM.
	PreferredNode *int `json:"preferred_node,omitempty"`
	// HAGroup is the HA group the VM belongs to.
	HAGroup *string `json:"ha_group,omitempty"`
	// SnapshotProfile is the snapshot profile ID.
	SnapshotProfile *int `json:"snapshot_profile,omitempty"`
	// OnPowerLoss is the behavior when power is restored.
	OnPowerLoss *string `json:"on_power_loss,omitempty"`
	// MigrationMethod is the migration method (auto/live).
	MigrationMethod *string `json:"migration_method,omitempty"`
	// PowerCycleTimeout is the migration power-cycle timeout.
	PowerCycleTimeout *int `json:"power_cycle_timeout,omitempty"`

	// Advanced configuration
	// Advanced is the advanced configuration.
	Advanced *string `json:"advanced,omitempty"`
	// Note is a free-form note about the VM.
	Note *string `json:"note,omitempty"`
	// PasteKeyConfig is the paste key mapping configuration ID.
	PasteKeyConfig *int `json:"paste_key_config,omitempty"`
}

// vmAction represents a VM action request.
type vmAction struct {
	VM     int            `json:"vm"`
	Action string         `json:"action"`
	Params vmActionParams `json:"params"`
}

// vmActionParams contains parameters for VM actions.
type vmActionParams struct {
	Device string `json:"device,omitempty"`
	Unplug bool   `json:"unplug,omitempty"`
}

// vmListFields are the fields to request when listing VMs.
const vmListFields = "$key,uuid,machine,name,description,enabled,created,modified,is_snapshot,owner,owner_user,creator,cluster,cluster_failover,cpu_cores,cpu_type,ram,machine_type,iommu,usb_legacy,uefi,secure_boot,boot_order,boot_delay,console,video,sound,serial_port,os_family,os_description,rtc_base,allow_hotplug,guest_agent,nested_virtualization,disable_hypervisor,allow_export,cloudinit_datasource,preferred_node,ha_group,snapshot_profile,on_power_loss,migration_method,power_cycle_timeout,need_restart,created_from,imported,machine#status#running as powerstate"

// vmGetFields are the fields to request when getting a single VM.
const vmGetFields = vmListFields + ",console_pass_enabled,console_pass,usb_tablet,disable_powercycle,advanced,note,meta,paste_key_config"
