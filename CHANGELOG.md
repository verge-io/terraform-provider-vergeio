# Changelog

Notable changes to the VergeIO Terraform provider. Versions follow semantic versioning and every entry links to the pull request that made the change. Releases are published on the [Terraform Registry](https://registry.terraform.io/providers/verge-io/vergeio) and on [GitHub Releases](https://github.com/verge-io/terraform-provider-vergeio/releases).

## Unreleased

## v2.7.10 - 2026-10-08

### Fixed

- `LICENSE` is now the verbatim Apache 2.0 text from apache.org with only the copyright line filled in. The copy added in v2.7.7 carried three wording edits and a garbled appendix sentence, which made the OpenTofu registry scanner match the file as both Apache-2.0 and ECL-2.0 and flag v2.7.7 through v2.7.9 as `incompatible_license`. No code change. [#273](https://github.com/verge-io/terraform-provider-vergeio/pull/273)

## v2.7.9 - 2026-10-05

### Fixed

- `vergeio_vm`: an apply that sets `powerstate` now fails when the VM does not reach the requested state. The provider posts the `poweron` or `kill` action and polls the platform every 5 seconds, up to 25 seconds. Before this release it gave up silently after that window and returned success, so Terraform either recorded `powerstate = true` for a VM that never started or raised a framework "inconsistent result after apply" error that blamed the provider. The error now names the VM, the requested state, and the last observed state. On create the VM already exists on the platform when this error is raised but is not yet in Terraform state; the Power state section on the `vergeio_vm` page describes how to recover. [#248](https://github.com/verge-io/terraform-provider-vergeio/issues/248), [#249](https://github.com/verge-io/terraform-provider-vergeio/pull/249)

### Changed

- Removed comments that only restated the code and the scaffolding template notes left over from the provider template. No behavior change. [#246](https://github.com/verge-io/terraform-provider-vergeio/pull/246)

## v2.7.8 - 2026-08-12

### Added

- `vergeio_vm`: `on_power_loss` attribute sets the VM's power state after a host power loss. [#49](https://github.com/verge-io/terraform-provider-vergeio/pull/49)

## v2.7.7 - 2026-04-07

### Fixed

- `vergeio_vm`: disk imports with a fractional GB size were truncated; `disksize` is now a float. State written by earlier versions is upgraded in place. [#46](https://github.com/verge-io/terraform-provider-vergeio/pull/46)

### Changed

- Added the CLA and the full license text.

## v2.7.6 - 2026-03-12

### Changed

- `vergeio_vm`: cloud-init files are deleted after the VM powers on, so the VM does not depend on them on later boots. [#42](https://github.com/verge-io/terraform-provider-vergeio/pull/42)

## v2.7.5 - 2026-01-28

### Added

- `vergeio_tags` data source filters by category, which disambiguates tags that share a name across categories. [#40](https://github.com/verge-io/terraform-provider-vergeio/pull/40)

### Changed

- Documentation update. [#39](https://github.com/verge-io/terraform-provider-vergeio/pull/39)

## v2.7.4 - 2026-01-06

### Added

- Tag support: `vergeio_tag_member` resource and `vergeio_tags` data source. [#36](https://github.com/verge-io/terraform-provider-vergeio/pull/36)
- Machine types and other enumerations are read from the VergeOS system and cached, instead of being fixed in the provider. [#37](https://github.com/verge-io/terraform-provider-vergeio/pull/37)

## v2.7.3 - 2025-12-04

### Changed

- Disk import updates. [#29](https://github.com/verge-io/terraform-provider-vergeio/pull/29)
- Q35 and PC machine type plan modifier, so `q35` and `pc-q35-10.0` (and the PC equivalents) are treated as the same value. `readVM` updated to match. [#33](https://github.com/verge-io/terraform-provider-vergeio/pull/33)

## v2.7.2 - 2025-11-24

### Fixed

- `vergeio_vm`: machine type validation no longer relies on a static schema. [#31](https://github.com/verge-io/terraform-provider-vergeio/pull/31)

## v2.7.1 - 2025-07-30

### Fixed

- `vergeio_vm`: vGPU and NVIDIA device handling in the `vergeio_device` block, with documentation for VM devices.

## v2.7.0 - 2025-07-29

### Added

- `vergeio_vm`: `vergeio_device` block for USB, TPM, PCI and vGPU devices.

### Fixed

- Type fix in the device schema.

## v2.6.1 - 2025-06-30

### Fixed

- `vergeio_vm`: power state bug.

## v2.6.0 - 2025-06-12

### Changed

- `vergeio_vm`: `powerstate` is a boolean.

## v2.5.0 - 2025-04-29

### Added

- `vergeio_vm`: guest IP address reporting.

## v2.4.0 - 2025-04-22

### Added

- VLAN bonding on networks, with documentation.
- `vergeio_vm` resource enhancements, with documentation.

### Fixed

- Deleting a VM in the UI no longer breaks the next Terraform run.

## v2.3.0 - 2025-03-28

### Added

- `vergeio_vms` data source reports guest agent IP addresses, with a configurable wait.

## v2.2.2 - 2025-03-24

### Added

- `vergeio_vms` data source: media source on disks and MAC address on NICs.

### Fixed

- Index bug in the VM data source.
- Network documentation.

## v2.2.1 - 2025-03-19

### Changed

- Documentation update.

## v2.2.0 - 2025-03-17

### Added

- `vergeio_vms` data source includes drives and NICs.
- `vergeio_vm`: `advanced` attribute for key and value pairs.
- External VLAN support on networks.

### Fixed

- `vlan_default_gateway` handling on networks.

## v2.1.0 - 2025-02-04

### Added

- Automatic IP assignment on NICs.

## v2.0.1 - 2025-01-28

### Changed

- Registry index page.

## v2.0.0 - 2025-01-28

### Changed

- The provider was rebuilt on the Terraform Plugin Framework with protocol version 6. Configurations written for the 1.x provider need review; see the resource and data source pages under `docs/`.

Releases before 2.0.0 are listed on [GitHub Releases](https://github.com/verge-io/terraform-provider-vergeio/releases).
