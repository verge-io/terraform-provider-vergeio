# VergeOS Terraform Provider - Bug Fixes and Enhancements

This document describes three fixes and one enhancement made to the VergeOS Terraform Provider to enable proper Cloud-Init functionality with the managed fields (`username`, `password`, `ssh_key`, `hostname`).

## Files Modified

| File | Changes |
|------|---------|
| `internal/provider/vm/nic_api.go` | +8 lines |
| `internal/provider/vm/vm_api.go` | +13 lines |
| `internal/provider/vm/vm_resource.go` | +74 lines |

---

## Fix 1: NICs Created with `enabled: false`

### Problem
When creating a VM with a NIC block that doesn't explicitly set `enabled = true`, the NIC is created with `enabled: false`, causing the VM to have no network connectivity.

### Root Cause
In `nic_api.go`, the `createNIC` function uses `data.Enabled.ValueBool()` which returns `false` for null/unknown values.

### Solution
Default the `enabled` field to `true` when not explicitly set.

### File: `internal/provider/vm/nic_api.go`

**Before (line 99-112):**
```go
func (nc *NICApi) createNIC(ctx context.Context, data *nicResourceModel) error {

	apiData := nicAPIResourceModel{
		Machine:     data.Machine.ValueInt32(),
		Name:        data.Name.ValueString(),
		Description: data.Description.ValueString(),
		Interface:   data.Interface.ValueString(),
		Driver:      data.Driver.ValueString(),
		Model:       data.Model.ValueString(),
		Vendor:      data.Vendor.ValueString(),
		Port:        data.Port.ValueInt32(),
		Enabled:     data.Enabled.ValueBool(),
		VNET:        data.VNET.ValueInt32(),
		MAC:         data.MAC.ValueString(),
		Asset:       data.Asset.ValueString(),
	}
```

**After:**
```go
func (nc *NICApi) createNIC(ctx context.Context, data *nicResourceModel) error {

	// Default enabled to true if not explicitly set
	enabled := true
	if !data.Enabled.IsNull() && !data.Enabled.IsUnknown() {
		enabled = data.Enabled.ValueBool()
	}

	apiData := nicAPIResourceModel{
		Machine:     data.Machine.ValueInt32(),
		Name:        data.Name.ValueString(),
		Description: data.Description.ValueString(),
		Interface:   data.Interface.ValueString(),
		Driver:      data.Driver.ValueString(),
		Model:       data.Model.ValueString(),
		Vendor:      data.Vendor.ValueString(),
		Port:        data.Port.ValueInt32(),
		Enabled:     enabled,
		VNET:        data.VNET.ValueInt32(),
		MAC:         data.MAC.ValueString(),
		Asset:       data.Asset.ValueString(),
	}
```

---

## Fix 2: Cloud-Init Files Not Returned from API

### Problem
After VM creation, the `cloudinit_files` attribute in Terraform state is set to `null`, even though the files were successfully created in VergeOS.

### Root Cause
The `readVM` function in `vm_api.go` requests specific fields from the API but does not include `cloudinit_files` in the fields list.

### Solution
Add `cloudinit_files` to the fields parameter in the API request.

### File: `internal/provider/vm/vm_api.go`

**Before (line 638):**
```go
), &vergeio.Options{Fields: "id,machine,name,cluster,description,enabled,machine_type,allow_hotplug,disable_powercycle,cpu_cores,cpu_type,ram,console,display,video,sound,os_family,os_description,rtc_base,boot_order,console_pass_enabled,console_pass,usb_tablet,uefi,secure_boot,serial_port,boot_delay,preferred_node,snapshot_profile,cloudinit_datasource,ha_group,guest_agent,advanced,nested_virtualization,disable_hypervisor,machine#status#running as powerstate"})
```

**After:**
```go
), &vergeio.Options{Fields: "id,machine,name,cluster,description,enabled,machine_type,allow_hotplug,disable_powercycle,cpu_cores,cpu_type,ram,console,display,video,sound,os_family,os_description,rtc_base,boot_order,console_pass_enabled,console_pass,usb_tablet,uefi,secure_boot,serial_port,boot_delay,preferred_node,snapshot_profile,cloudinit_datasource,cloudinit_files,ha_group,guest_agent,advanced,nested_virtualization,disable_hypervisor,machine#status#running as powerstate"})
```

---

## Fix 3: Password Authentication Not Working

### Problem
When specifying a `password` in the Terraform configuration, the VM is created but password authentication via SSH fails.

### Root Cause
The cloud-init generation code uses the `passwd` field, which expects a **pre-hashed password** (SHA-512). Passing a plaintext password causes cloud-init to set an invalid password hash.

### Solution
Use `plain_text_passwd` instead of `passwd`. Cloud-init's `plain_text_passwd` field accepts plaintext and handles the hashing automatically.

### File: `internal/provider/vm/vm_resource.go`

**Before (line 773-775):**
```go
if !data.Password.IsNull() && !data.Password.IsUnknown() && data.Password.ValueString() != "" {
    userData += fmt.Sprintf("    passwd: %s\n    lock_passwd: false\n", data.Password.ValueString())
    userData += "ssh_pwauth: true\n"
```

**After:**
```go
if !data.Password.IsNull() && !data.Password.IsUnknown() && data.Password.ValueString() != "" {
    userData += fmt.Sprintf("    plain_text_passwd: %s\n    lock_passwd: false\n", data.Password.ValueString())
    userData += "ssh_pwauth: true\n"
```

---

## Enhancement: Managed Cloud-Init Fields

### Description
Added four new optional fields to the `vergeio_vm` resource that automatically generate cloud-init `user-data` and `meta-data` files:

- `username` - Creates a user with sudo privileges
- `password` - Sets the user's password (enables password SSH auth)
- `ssh_key` - Adds an SSH public key for key-based authentication
- `hostname` - Sets the VM hostname

### Usage Example

```hcl
resource "vergeio_vm" "example" {
  name         = "my-server"
  cpu_cores    = 2
  ram          = 4096
  machine_type = "q35"
  uefi         = true
  powerstate   = true

  # Managed Cloud-Init fields - auto-generates user-data and meta-data
  username = "ubuntu"
  password = "MySecurePassword"
  ssh_key  = "ssh-rsa AAAAB3NzaC1yc2E... user@host"
  hostname = "my-server"

  vergeio_drive {
    name         = "OS Disk"
    disksize     = 20
    media        = "import"
    media_source = 82  # Cloud image ID
    interface    = "virtio-scsi"
  }

  vergeio_nic {
    name = "nic_0"
    vnet = 17
  }
}
```

### Generated Cloud-Init Files

**user-data:**
```yaml
#cloud-config
users:
  - name: ubuntu
    sudo: ALL=(ALL) NOPASSWD:ALL
    shell: /bin/bash
    ssh_authorized_keys:
      - ssh-rsa AAAAB3NzaC1yc2E... user@host
    plain_text_passwd: MySecurePassword
    lock_passwd: false
ssh_pwauth: true
```

**meta-data:**
```yaml
instance-id: my-server
local-hostname: my-server
```

---

## Testing

All fixes were verified by:

1. Creating VMs with `terraform apply`
2. Confirming NICs are enabled and VMs get DHCP addresses
3. SSH access with key authentication
4. SSH access with password authentication from a different machine
5. Verifying hostname is properly set

---

## Compatibility

These changes are backwards compatible:
- NICs default to `enabled: true` (expected behavior)
- Existing `cloudinit_files` blocks continue to work
- The new managed fields are optional

---

## Summary

| Issue | Root Cause | Fix |
|-------|-----------|-----|
| NICs disabled by default | `ValueBool()` returns `false` for null | Default to `true` when not set |
| Cloud-init files lost | `cloudinit_files` not in API request | Add field to API request |
| Password auth fails | Using `passwd` instead of `plain_text_passwd` | Use correct cloud-init field |
