package vergeos

// VMNIC represents a network interface attached to a VM.
type VMNIC struct {
	// ID is the unique identifier for the NIC.
	ID FlexInt `json:"$key,omitempty"`
	// Machine is the machine reference ID.
	Machine int `json:"machine,omitempty"`
	// OrderID is the NIC order (0-30).
	OrderID int `json:"orderid,omitempty"`
	// Name is the NIC name.
	Name string `json:"name"`
	// Description is an optional description.
	Description string `json:"description,omitempty"`
	// Interface is the interface type (virtio, e1000, e1000e, rtl8139, pcnet, igb, vmxnet3, direct).
	Interface string `json:"interface,omitempty"`
	// Driver is the network driver.
	Driver string `json:"driver,omitempty"`
	// Model is the NIC model.
	Model string `json:"model,omitempty"`
	// Vendor is the NIC vendor.
	Vendor string `json:"vendor,omitempty"`
	// Port is the port number.
	Port int `json:"port,omitempty"`
	// Enabled indicates whether the NIC is enabled.
	Enabled bool `json:"enabled"`
	// DisableMQ disables multiqueue for this NIC.
	DisableMQ bool `json:"disable_mq,omitempty"`
	// VNET is the virtual network ID.
	VNET FlexInt `json:"vnet,omitempty"`
	// MAC is the MAC address.
	MAC string `json:"macaddress,omitempty"`
	// IPAddress is the assigned IP address.
	IPAddress string `json:"ipaddress,omitempty"`
	// Asset is the asset tag (used for recipe/snapshot identification).
	Asset string `json:"asset,omitempty"`
	// Device is the device name (read-only).
	Device string `json:"device,omitempty"`
	// PowerState is the NIC power state ("up" or "down").
	PowerState string `json:"powerState,omitempty"`
}

// VMNICCreateRequest is the request body for creating a NIC.
type VMNICCreateRequest struct {
	// Machine is the VM's machine ID.
	Machine int `json:"machine"`
	// OrderID is the NIC order (0-30, auto-assigned if not specified).
	OrderID *int `json:"orderid,omitempty"`
	// Name is the NIC name.
	Name string `json:"name"`
	// Description is an optional description.
	Description string `json:"description,omitempty"`
	// Interface is the interface type (virtio, e1000, e1000e, rtl8139, pcnet, igb, vmxnet3, direct).
	Interface string `json:"interface,omitempty"`
	// Driver is the network driver.
	Driver string `json:"driver,omitempty"`
	// Model is the NIC model.
	Model string `json:"model,omitempty"`
	// Vendor is the NIC vendor.
	Vendor string `json:"vendor,omitempty"`
	// Port is the port number.
	Port *int `json:"port,omitempty"`
	// Enabled indicates whether the NIC is enabled.
	Enabled *bool `json:"enabled,omitempty"`
	// DisableMQ disables multiqueue for this NIC.
	DisableMQ *bool `json:"disable_mq,omitempty"`
	// VNET is the virtual network ID.
	VNET int `json:"vnet,omitempty"`
	// MAC is the MAC address (optional, auto-generated if not specified).
	MAC string `json:"macaddress,omitempty"`
	// AssignIPAddress indicates whether to assign an IP address.
	AssignIPAddress bool `json:"-"` // Handled separately via vnet_addresses
	// Asset is the asset tag (used for recipe/snapshot identification).
	Asset string `json:"asset,omitempty"`
}

// VMNICUpdateRequest is the request body for updating a NIC.
type VMNICUpdateRequest struct {
	// OrderID is the NIC order (0-30).
	OrderID *int `json:"orderid,omitempty"`
	// Name is the NIC name.
	Name *string `json:"name,omitempty"`
	// Description is the description.
	Description *string `json:"description,omitempty"`
	// Interface is the interface type (virtio, e1000, e1000e, rtl8139, pcnet, igb, vmxnet3, direct).
	Interface *string `json:"interface,omitempty"`
	// Driver is the network driver.
	Driver *string `json:"driver,omitempty"`
	// Model is the NIC model.
	Model *string `json:"model,omitempty"`
	// Vendor is the NIC vendor.
	Vendor *string `json:"vendor,omitempty"`
	// Port is the port number.
	Port *int `json:"port,omitempty"`
	// Enabled indicates whether the NIC is enabled.
	Enabled *bool `json:"enabled,omitempty"`
	// DisableMQ disables multiqueue for this NIC.
	DisableMQ *bool `json:"disable_mq,omitempty"`
	// VNET is the virtual network ID.
	VNET *int `json:"vnet,omitempty"`
	// Asset is the asset tag.
	Asset *string `json:"asset,omitempty"`
}

// vnetAddressRequest is the request body for assigning an IP address to a NIC.
type vnetAddressRequest struct {
	VNET int    `json:"vnet"`
	MAC  string `json:"macaddress"`
	Type string `json:"type"`
}

// nicListFields are the fields to request when listing NICs.
const nicListFields = "$key,machine,orderid,name,description,interface,driver,model,vendor,port,enabled,disable_mq,vnet,macaddress,asset,device"

// nicGetFields are the fields to request when getting a single NIC (includes power state).
const nicGetFields = nicListFields + ",ipaddress,status#status as powerState"
