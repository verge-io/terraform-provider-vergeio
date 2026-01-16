# Pull Request: Cloud-Init Enhancement and Bug Fixes

## Summary

This PR introduces managed Cloud-Init fields for simplified VM provisioning and fixes three critical bugs that prevent Cloud-Init from working correctly with the VergeOS Terraform Provider.

## Changes Overview

| File | Lines Changed | Description |
|------|---------------|-------------|
| `internal/provider/vm/vm_resource.go` | +71 | Add managed Cloud-Init fields and fix password handling |
| `internal/provider/vm/vm_api.go` | +32 | Fix cloud-init files not persisting + preserve contents from state |
| `internal/provider/vm/nic_api.go` | +7 | Fix NICs created as disabled by default |

---

## Bug Fixes

### Bug #1: NICs Created with `enabled: false`

**Symptom:** VMs are created but have no network connectivity. NICs are visible in VergeOS but marked as disabled.

**Root Cause:** In `nic_api.go`, the `createNIC` function uses `data.Enabled.ValueBool()` which returns `false` when the attribute is null/unknown (not explicitly set in Terraform configuration).

**Fix:** Default the `enabled` field to `true` when not explicitly set to `false`.

```go
// Before
Enabled: data.Enabled.ValueBool(),

// After
enabled := true
if !data.Enabled.IsNull() && !data.Enabled.IsUnknown() {
    enabled = data.Enabled.ValueBool()
}
// ... then use 'enabled' variable
```

**Design Decision: Why Default to `enabled: true`?**

We considered whether there might be legitimate reasons to default NICs to disabled:

| Potential Reason | Assessment |
|-----------------|------------|
| **Security/Staging** - Create VMs in offline state for review before connecting | Valid use case, but should be opt-in via explicit `enabled = false` |
| **Template/Clone workflow** - Templates shouldn't respond on network | Valid, but templates are typically powered off anyway |
| **Consistency with VergeOS UI** | The VergeOS UI creates NICs as enabled by default |

**Conclusion:** The current behavior (defaulting to disabled) was unintentional, caused by Go's zero-value for bool being `false`. Other major cloud providers (AWS, Azure, GCP) all create network interfaces as enabled/attached by default.

**The fix respects explicit user intent:**
- `enabled` not specified → defaults to `true` (expected behavior)
- `enabled = true` → explicitly enabled
- `enabled = false` → explicitly disabled (for staging/template workflows)

```hcl
# For users who need disabled NICs, they can still do:
vergeio_nic {
  name    = "nic_0"
  vnet    = 17
  enabled = false  # Explicitly disable for staging
}
```

---

### Bug #2: Cloud-Init Files Not Returned from API

**Symptom:** After VM creation, `terraform show` displays `cloudinit_files = null` even though the files were successfully created in VergeOS.

**Root Cause:** The `readVM` function in `vm_api.go` requests specific fields from the API but does not include `cloudinit_files` in the fields list. After creation, when the VM state is refreshed, the cloud-init files are not returned.

**Fix:** Add `cloudinit_files` to the fields parameter in the `readVM` API request.

```go
// Before
), &vergeio.Options{Fields: "id,machine,name,...,cloudinit_datasource,ha_group,..."})

// After  
), &vergeio.Options{Fields: "id,machine,name,...,cloudinit_datasource,cloudinit_files,ha_group,..."})
```

---

### Bug #3: Password Authentication Not Working

**Symptom:** When specifying a password in Terraform configuration, the VM is created but SSH password authentication fails. Only SSH key authentication works.

**Root Cause:** The cloud-init generation code uses the `passwd` field, which expects a **pre-hashed password** (SHA-512 format). Passing a plaintext password results in cloud-init setting an invalid password hash.

**Fix:** Use `plain_text_passwd` instead of `passwd`. Cloud-init's `plain_text_passwd` field accepts plaintext and handles the hashing automatically.

```go
// Before
userData += fmt.Sprintf("    passwd: %s\n    lock_passwd: false\n", data.Password.ValueString())

// After
userData += fmt.Sprintf("    plain_text_passwd: %s\n    lock_passwd: false\n", data.Password.ValueString())
```

---

### Bug #4: Cloud-Init File Contents Not Preserved from API

**Symptom:** After VM creation with `cloudinit_files`, Terraform shows errors like:
```
.cloudinit_files[0].contents: was cty.StringVal("..."), but now cty.StringVal("")
```

**Root Cause:** The VergeOS API returns cloud-init file **names** but not their **contents**. When the provider reads the VM state, it receives empty contents and tries to update the state, causing an inconsistency error.

**Fix:** When reading cloud-init files from the API, preserve the contents from the existing Terraform state if the API returns empty contents.

```go
// Build a map of existing file contents from state (API doesn't return contents)
existingContents := make(map[string]string)
if !data.CloudInitFiles.IsNull() && !data.CloudInitFiles.IsUnknown() {
    var existingFiles []CloudInitFile
    data.CloudInitFiles.ElementsAs(ctx, &existingFiles, false)
    for _, f := range existingFiles {
        existingContents[f.Name.ValueString()] = f.Contents.ValueString()
    }
}

// Use API contents if available, otherwise preserve from state
contents := cloudInitFileAPI.Contents
if contents == "" {
    if existing, ok := existingContents[fileName]; ok {
        contents = existing
    }
}
```

---

## New Feature: Managed Cloud-Init Fields

### Description

Added four new optional fields to the `vergeio_vm` resource that automatically generate cloud-init `user-data` and `meta-data` files:

| Field | Description |
|-------|-------------|
| `username` | Creates a user with sudo privileges |
| `password` | Sets the user's password (enables SSH password auth) |
| `ssh_key` | Adds an SSH public key for key-based authentication |
| `hostname` | Sets the VM hostname via cloud-init meta-data |

### Behavior

When `username` is provided, the provider automatically:

1. Generates `/user-data` with user configuration
2. Generates `/meta-data` with hostname and instance-id
3. Sets `cloudinit_datasource` to `nocloud` if not already set
4. Appends files to any existing `cloudinit_files` blocks

### Usage Example

```hcl
resource "vergeio_vm" "example" {
  name         = "my-server"
  cpu_cores    = 2
  ram          = 4096
  machine_type = "q35"
  uefi         = true
  powerstate   = true
  guest_agent  = true

  # NEW: Managed Cloud-Init fields
  username = "ubuntu"
  password = "MySecurePassword"
  ssh_key  = "ssh-rsa AAAAB3NzaC1yc2E... user@host"
  hostname = "my-server"

  vergeio_drive {
    name         = "OS Disk"
    disksize     = 20
    media        = "import"
    media_source = 82  # Ubuntu cloud image ID
    interface    = "virtio-scsi"
  }

  vergeio_nic {
    name = "nic_0"
    vnet = 17
  }
}
```

### Generated Cloud-Init Content

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

All changes were thoroughly tested with the following scenarios:

### Test 1: NIC Enabled by Default
- Created VM without explicitly setting `enabled` on NIC
- **Result:** NIC is enabled, VM gets DHCP address ✅

### Test 2: Cloud-Init Files Persist in State
- Created VM with managed fields
- Ran `terraform show`
- **Result:** `cloudinit_files` shows user-data and meta-data ✅

### Test 3: SSH Key Authentication
- Created VM with `ssh_key` field
- SSH'd to VM using private key
- **Result:** Authentication successful ✅

### Test 4: Password Authentication
- Created VM with `password` field
- SSH'd to VM from different machine using password
- **Result:** Authentication successful ✅

### Test 5: Custom Cloud-Init with Package Installation
- Created VM with explicit `cloudinit_files` block including packages
- **Result:** Packages (htop, btop) installed successfully ✅

### Test 6: Multiple VMs and Concurrent Deployment
- Deployed 3 Ubuntu VMs simultaneously using `count` (the "Ubuntu Fleet")
- **Result:** All 3 VMs created in ~22 seconds, fully configured with Cloud-Init and guest agents ✅

---

## Backwards Compatibility

All changes are fully backwards compatible:

- **NICs:** Now default to `enabled: true` (expected behavior for most users)
- **Cloud-Init:** Existing `cloudinit_files` blocks continue to work unchanged
- **Managed Fields:** The new `username`, `password`, `ssh_key`, and `hostname` fields are optional
- **State:** Existing state files will work; cloud-init files will be populated on next refresh

---

## Schema Changes

### New Attributes for `vergeio_vm`

```hcl
username = optional(string)  # Username for Cloud-Init
password = optional(string)  # Password for Cloud-Init (sensitive)
ssh_key  = optional(string)  # SSH public key for Cloud-Init
hostname = optional(string)  # Hostname for Cloud-Init
```

### Modified Attributes

- `cloudinit_files`: Added `Computed: true` to allow provider-generated files

---

## Files Modified

### `internal/provider/vm/nic_api.go`

```diff
+ // Default enabled to true if not explicitly set
+ enabled := true
+ if !data.Enabled.IsNull() && !data.Enabled.IsUnknown() {
+     enabled = data.Enabled.ValueBool()
+ }
  
  apiData := nicAPIResourceModel{
      // ...
-     Enabled:     data.Enabled.ValueBool(),
+     Enabled:     enabled,
      // ...
  }
```

### `internal/provider/vm/vm_api.go`

```diff
  ), &vergeio.Options{Fields: "id,machine,name,...,cloudinit_datasource,
+ cloudinit_files,
  ha_group,..."})
```

### `internal/provider/vm/vm_resource.go`

```diff
+ // New model fields
+ Username types.String `tfsdk:"username"`
+ Password types.String `tfsdk:"password"`
+ SSHKey   types.String `tfsdk:"ssh_key"`
+ Hostname types.String `tfsdk:"hostname"`

+ // New schema attributes
+ "username": schema.StringAttribute{
+     MarkdownDescription: "Username for Cloud-Init",
+     Optional:            true,
+ },
+ // ... (password, ssh_key, hostname)

+ // Cloud-Init generation logic in Create()
+ if !data.Username.IsNull() && data.Username.ValueString() != "" {
+     // Generate user-data and meta-data
+     userData := fmt.Sprintf("#cloud-config\nusers:\n  - name: %s\n...", username)
+     // ... (SSH key, password handling)
+     // Append to cloudinit_files
+ }
```

---

## Related Issues

- Resolves: NICs created as disabled by default
- Resolves: Cloud-init files not persisting in Terraform state
- Resolves: Password authentication failing with plaintext passwords
- Implements: Simplified Cloud-Init configuration via managed fields

---

## Checklist

- [x] Code compiles without errors
- [x] All existing tests pass
- [x] New functionality tested manually
- [x] Backwards compatible with existing configurations
- [x] Documentation updated (PROVIDER-FIXES.md)
- [x] Schema changes are additive only (no breaking changes)
