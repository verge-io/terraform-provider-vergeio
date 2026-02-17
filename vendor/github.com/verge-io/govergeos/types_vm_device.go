package vergeos

// VMDevice represents a device attached to a VM.
type VMDevice struct {
	// ID is the unique identifier for the device.
	ID FlexInt `json:"$key,omitempty"`
	// Machine is the machine reference ID.
	Machine int `json:"machine,omitempty"`
	// MachineType is the machine type (vm, container, node, etc.) - read-only.
	MachineType string `json:"machine_type,omitempty"`
	// OrderID is the device order (0-64).
	OrderID int `json:"orderid,omitempty"`
	// Type is the device type (tpm, node_usb_devices, node_pci_devices, node_nvidia_vgpu_devices, etc.).
	Type string `json:"type"`
	// Name is the device name.
	Name string `json:"name"`
	// Description is an optional description.
	Description string `json:"description,omitempty"`
	// ResourceGroup is the resource group ID.
	ResourceGroup FlexInt `json:"resource_group,omitempty"`
	// UUID is the device UUID.
	UUID string `json:"uuid,omitempty"`
	// Enabled indicates whether the device is enabled.
	Enabled bool `json:"enabled"`
	// Optional allows the machine to start without this device if unavailable.
	Optional bool `json:"optional,omitempty"`
	// Asset is the asset tag (used for recipe/snapshot identification).
	Asset string `json:"asset,omitempty"`
	// Created is the creation timestamp (Unix epoch).
	Created int64 `json:"created,omitempty"`
	// Modified is the last modification timestamp (Unix epoch).
	Modified int64 `json:"modified,omitempty"`

	// Settings contains device-specific settings based on Type.
	// For USB devices: USBSettings
	// For TPM devices: TPMSettings
	// For vGPU devices: VGPUSettings
	USBSettings  *USBDeviceSettings  `json:"-"`
	TPMSettings  *TPMDeviceSettings  `json:"-"`
	VGPUSettings *VGPUDeviceSettings `json:"-"`
}

// USBDeviceSettings contains USB device-specific settings.
type USBDeviceSettings struct {
	// ID is the settings ID.
	ID int `json:"$key,omitempty"`
	// MachineDevice is the parent device ID.
	MachineDevice int `json:"machine_device,omitempty"`
	// GuestReset indicates whether guest reset is allowed.
	GuestReset bool `json:"guest_reset,omitempty"`
	// GuestResetsAll indicates whether guest can reset all devices.
	GuestResetsAll bool `json:"guest_resets_all,omitempty"`
}

// TPMDeviceSettings contains TPM device-specific settings.
type TPMDeviceSettings struct {
	// ID is the settings ID.
	ID int `json:"$key,omitempty"`
	// MachineDevice is the parent device ID.
	MachineDevice int `json:"machine_device,omitempty"`
	// Model is the TPM model.
	Model string `json:"model,omitempty"`
	// Version is the TPM version.
	Version string `json:"version,omitempty"`
}

// VGPUDeviceSettings contains NVIDIA vGPU device-specific settings.
type VGPUDeviceSettings struct {
	// ID is the settings ID.
	ID int `json:"$key,omitempty"`
	// MachineDevice is the parent device ID.
	MachineDevice int `json:"machine_device,omitempty"`
	// ProfileType is the vGPU profile type.
	ProfileType string `json:"profile_type,omitempty"`
	// FrameRateLimiter is the frame rate limiter setting.
	FrameRateLimiter int `json:"frame_rate_limiter,omitempty"`
	// DisableVNC indicates whether VNC is disabled.
	DisableVNC bool `json:"disable_vnc,omitempty"`
	// EnableUVM indicates whether UVM is enabled.
	EnableUVM bool `json:"enable_uvm,omitempty"`
	// EnableDebugging indicates whether debugging is enabled.
	EnableDebugging bool `json:"enable_debugging,omitempty"`
	// EnableProfiling indicates whether profiling is enabled.
	EnableProfiling bool `json:"enable_profiling,omitempty"`
}

// VMDeviceCreateRequest is the request body for creating a device.
type VMDeviceCreateRequest struct {
	// Machine is the VM's machine ID.
	Machine int `json:"machine"`
	// OrderID is the device order (0-64, auto-assigned if not specified).
	OrderID *int `json:"orderid,omitempty"`
	// Type is the device type (tpm, node_usb_devices, node_pci_devices, node_nvidia_vgpu_devices, etc.).
	Type string `json:"type"`
	// Name is the device name (auto-generated if blank).
	Name string `json:"name"`
	// Description is an optional description.
	Description string `json:"description,omitempty"`
	// ResourceGroup is the resource group ID.
	ResourceGroup int `json:"resource_group,omitempty"`
	// UUID is the device UUID (leave blank for system-generated).
	UUID string `json:"uuid,omitempty"`
	// Enabled indicates whether the device is enabled.
	Enabled *bool `json:"enabled,omitempty"`
	// Optional allows the machine to start without this device if unavailable.
	Optional *bool `json:"optional,omitempty"`
	// Asset is the asset tag.
	Asset string `json:"asset,omitempty"`
	// Count creates X copies of this device (1-16).
	Count *int `json:"count,omitempty"`

	// USB settings (only for USB devices)
	USBSettings *USBDeviceSettings `json:"-"`
	// TPM settings (only for TPM devices)
	TPMSettings *TPMDeviceSettings `json:"-"`
	// vGPU settings (only for vGPU devices)
	VGPUSettings *VGPUDeviceSettings `json:"-"`
}

// VMDeviceUpdateRequest is the request body for updating a device.
type VMDeviceUpdateRequest struct {
	// OrderID is the device order (0-64).
	OrderID *int `json:"orderid,omitempty"`
	// Name is the device name.
	Name *string `json:"name,omitempty"`
	// Description is the description.
	Description *string `json:"description,omitempty"`
	// ResourceGroup is the resource group ID.
	ResourceGroup *int `json:"resource_group,omitempty"`
	// UUID is the device UUID.
	UUID *string `json:"uuid,omitempty"`
	// Enabled indicates whether the device is enabled.
	Enabled *bool `json:"enabled,omitempty"`
	// Optional allows the machine to start without this device if unavailable.
	Optional *bool `json:"optional,omitempty"`
	// Asset is the asset tag.
	Asset *string `json:"asset,omitempty"`

	// USB settings (only for USB devices)
	USBSettings *USBDeviceSettings `json:"-"`
	// TPM settings (only for TPM devices)
	TPMSettings *TPMDeviceSettings `json:"-"`
	// vGPU settings (only for vGPU devices)
	VGPUSettings *VGPUDeviceSettings `json:"-"`
}

// Device type constants
const (
	DeviceTypeUSB  = "node_usb_devices"
	DeviceTypeTPM  = "tpm"
	DeviceTypePCI  = "node_pci_devices"
	DeviceTypeVGPU = "node_nvidia_vgpu_devices"
)

// deviceListFields are the fields to request when listing devices.
const deviceListFields = "$key,machine,machine_type,orderid,type,name,description,resource_group,uuid,enabled,optional,asset,created,modified"
