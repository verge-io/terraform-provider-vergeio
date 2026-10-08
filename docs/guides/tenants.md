---
page_title: "Tenants"
description: |-
  Create a VergeOS tenant on the parent, then manage the inside of that tenant from a second Terraform configuration.
---

# Tenants

A VergeOS tenant is a full VergeOS instance carved from the parent: its own UI, API, nodes, storage, and networks. `vergeio_tenant`, `vergeio_tenant_node`, `vergeio_tenant_storage`, `vergeio_tenant_external_ip`, `vergeio_tenant_network_block`, and `vergeio_tenant_layer2_network` declare that bundle on the parent. `vergeio_tenants` reads it back.

`isolate` on `vergeio_tenant` turns network isolation on or off. `true` calls isolate on. `false` calls isolate off. Omit it to leave the current isolation unchanged. A change made in the parent UI is drift, and the next apply restores the configured value.

## Two configurations

Terraform configures a provider before it applies resources. The tenant UI address does not exist until `vergeio_tenant` is created, so the same configuration cannot point a provider at that address.

This configuration fails. The host is unknown until apply:

```terraform
resource "vergeio_tenant" "customer" {
  name                = "customer-a"
  password_wo         = var.tenant_password
  password_wo_version = 1
  powerstate          = true
}

provider "vergeio" {
  alias    = "inside"
  host     = vergeio_tenant.customer.ui_address
  username = "admin"
  password = var.tenant_password
}

resource "vergeio_vm" "app" {
  provider  = vergeio.inside
  name      = "app"
  cpu_cores = 2
  ram       = 2048
}
```

Terraform reports:

```
Error: Invalid provider configuration

The configuration for provider["registry.terraform.io/verge-io/vergeio"].inside
depends on values that cannot be determined until apply.
```

Use two root modules. The parent module creates the tenant and outputs `ui_address`. The tenant module reads that output with `terraform_remote_state` and configures its own provider. Apply the parent module twice before you apply the tenant module. Terraform creates the tenant before `vergeio_tenant_node`, so the first parent apply leaves the tenant offline even when that node is in the same configuration. Refresh stores the offline `powerstate`. The second parent apply powers the tenant on. `vergeio_tenant_external_ip` assigns the UI address from the parent network. The tenant is created before that address, so the first apply leaves `ui_address` empty. The next plan refreshes `vergeio_tenant` and stores the address once VergeOS has made that first IP the UI address. Apply the tenant module after the tenant is online. The UI does not answer before then. A `powerstate` of true with no node in the configuration also waits for a later apply, on create and on update.

The parent module passes `var.tenant_password` to `vergeio_tenant.password_wo`. Terraform stores `password_wo_version`, not the password. Increment that version to change the password. `password` still works through v3.x and is stored in state. It is deprecated and will be removed in v4. Leave the password out of the parent outputs. A `password` set on the parent provider is stored, because provider arguments cannot be write-only. `VERGEOS_PASSWORD` keeps that parent credential out of state.

```terraform
provider "vergeio" {
  host     = "parent.example.com"
  username = "admin"
  password = var.parent_password
}

variable "parent_password" {
  type      = string
  sensitive = true
}

variable "tenant_password" {
  type      = string
  sensitive = true
}

resource "vergeio_tenant" "customer" {
  name                = "customer-a"
  description         = "Customer A virtual data center"
  password_wo         = var.tenant_password
  password_wo_version = 1
  powerstate          = true
}

resource "vergeio_tenant_node" "node" {
  tenant_id = vergeio_tenant.customer.id
  name      = "node1"
  cpu_cores = 4
  ram       = 8192
  enabled   = true
}

resource "vergeio_tenant_storage" "tier" {
  tenant_id   = vergeio_tenant.customer.id
  tier        = 1
  provisioned = 107374182400
}

variable "parent_external_network_id" {
  type        = string
  description = "Key of the parent external network that hands the tenant its UI address and routed blocks."
}

# The first assigned IP becomes the tenant UI address.
# Set vergeio_tenant.ui_address_id to this resource's id on a later change
# when a different assigned IP should be the UI address.
resource "vergeio_tenant_external_ip" "ui" {
  tenant_id             = vergeio_tenant.customer.id
  network_id            = var.parent_external_network_id
  ip                    = "203.0.113.50"
  hostname              = "customer-a"
  description           = "Tenant UI address"
  apply_parent_firewall = true
}

resource "vergeio_tenant_network_block" "routed" {
  tenant_id             = vergeio_tenant.customer.id
  network_id            = var.parent_external_network_id
  cidr                  = "198.51.100.0/28"
  description           = "Customer A routed addresses"
  apply_parent_firewall = true
}

variable "parent_layer2_network_id" {
  type        = string
  description = "Key of the parent layer 2 network bridged into the tenant. Requires VergeOS 26.0 or later."
}

resource "vergeio_tenant_layer2_network" "external" {
  tenant_id  = vergeio_tenant.customer.id
  network_id = var.parent_layer2_network_id
  enabled    = true
}

output "tenant_ui_address" {
  value = vergeio_tenant.customer.ui_address
}

output "tenant_id" {
  value = vergeio_tenant.customer.id
}
```

The tenant module does not store the admin password or a long-lived API key. The bootstrap provider reads `VERGEOS_USERNAME` and `VERGEOS_PASSWORD`. The bootstrap provider looks up the tenant admin with `vergeio_users`. `ephemeral.vergeio_api_key` mints a key for that user, and the default provider uses the token. Terraform and OpenTofu do not store ephemeral results. Close deletes the key. `ttl_seconds` expires it if Close does not run. `name` is exclusive: Open deletes an existing key of that name for that user before creating a new one. Do not reuse a long-lived key name.

```terraform
# Bootstrap credentials come from the environment, not this configuration:
#   VERGEOS_USERNAME and VERGEOS_PASSWORD sign in as the tenant admin.
# Those values are not written to state. Provider arguments cannot be
# write-only, so a password or api_key set in a provider block is stored.
# The ephemeral token is not stored either. Close deletes the key.

data "terraform_remote_state" "parent" {
  backend = "local"
  config = {
    path = "../parent/terraform.tfstate"
  }
}

provider "vergeio" {
  alias    = "bootstrap"
  host     = data.terraform_remote_state.parent.outputs.tenant_ui_address
  insecure = true
}

data "vergeio_users" "admin" {
  provider    = vergeio.bootstrap
  filter_name = "admin"
}

# name is exclusive. Open deletes an existing key with this name for
# user_id, then creates a new one. Do not reuse a long-lived key name.
ephemeral "vergeio_api_key" "run" {
  provider    = vergeio.bootstrap
  user_id     = data.vergeio_users.admin.users[0].id
  name        = "terraform-tenant"
  ttl_seconds = 3600
}

provider "vergeio" {
  host     = data.terraform_remote_state.parent.outputs.tenant_ui_address
  api_key  = ephemeral.vergeio_api_key.run.token
  insecure = true
}

resource "vergeio_vm" "app" {
  name      = "app"
  os_family = "linux"
  cpu_cores = 2
  ram       = 2048
}
```

`password_wo` and `ephemeral.vergeio_api_key` need a CLI that supports them. See [Terraform and OpenTofu](#terraform-and-opentofu).

## Terraform and OpenTofu

Write-only attributes (`password_wo`, `console_pass_wo`, and cloud-init `contents_wo`) require Terraform 1.11 or OpenTofu 1.11. Ephemeral resources require Terraform 1.10 or OpenTofu 1.11. OpenTofu 1.10 and earlier reject both ephemeral blocks and write-only attributes. Terraform 1.10 has ephemeral resources and rejects write-only attributes.

Once the CLI is new enough, storage matches on both. A write-only value is null in state. Changing that value does not plan an update until the version attribute changes. Ephemeral results are not in state or in the plan. They can be used in provider configuration and in write-only attributes. Provider schema attributes cannot be write-only on either CLI. A `password` or `api_key` set in the provider block is stored.

## Addresses handed down from the parent

`vergeio_tenant_external_ip` declares one virtual IP on a parent network and sets the tenant as its owner. That is the address the parent UI assigns by hand today. The first assigned IP becomes the tenant UI address. A plan that runs after the address exists refreshes `vergeio_tenant` and can use `ui_address` in that same plan. `network_id` is the parent network key, the same value as `vergeio_network.id`.

`ui_address_id` on `vergeio_tenant` chooses which assigned external IP is the tenant UI. Set it to the id of a `vergeio_tenant_external_ip` on a later change, after that address exists. The address resource takes `tenant_id` from the tenant, so the same apply cannot also feed that address id back into the tenant. Omit `ui_address_id` and the first assigned IP stays the UI address. `ui_address` remains the IP string. A change made in the parent UI is drift, and the next apply restores the configured value.

Creating or deleting the address leaves `need_fw_apply` set on that parent network until its rules are applied. `apply_parent_firewall` applies them in the same call. `parent_firewall_pending` reports the flag afterward. A stopped parent network loads staged rules when it starts, and it may refuse a refresh while it is stopped.

`vergeio_tenant_network_block` assigns one routed CIDR from a parent network to the tenant (`vnet_cidrs`). The tenant can build its own network on that range. `network_id` is the parent network key, the same value as `vergeio_network.id`. Changing `tenant_id`, `network_id`, `cidr`, or `description` replaces the block. There is no update API for the row.

Creating or deleting the block leaves `need_fw_apply` set on that parent network until its rules are applied. `apply_parent_firewall` applies them in the same call and waits until the flag clears. `parent_firewall_pending` reports the flag afterward.

VergeOS refuses to delete the block while a network inside the tenant is still built on it. Terraform returns that error and leaves the block in state. Remove the tenant network, then destroy the block.

## Recipes

A tenant recipe is a catalog entry built from a tenant snapshot. It asks a set of questions and creates a configured tenant. `vergeio_tenant_recipes` lists those recipes and their questions. `filter_name` is an exact recipe name. `catalog_id` or `catalog_name` limits the list to one catalog. `vergeio_catalogs` returns the catalog key.

`vergeio_tenant_recipe_instance` deploys one tenant from a recipe. `recipe_id` is `recipes[0].id`. `answers` is a map of question name to string. A bool answer is `true`, `false`, `yes`, `no`, `on`, `off`, `1`, or `0`. A disk size is bytes, so 50 GB is `53687091200`. A network answer is a network name, a vnet key, or `__new_internal__`.

`tenant_id` is the key of the tenant VergeOS created. That is the same key `vergeio_tenant` stores in `id`, as a number. Pass `tostring(vergeio_tenant_recipe_instance.customer.tenant_id)` to `vergeio_tenant_node`, `vergeio_tenant_storage`, `vergeio_tenant_external_ip`, `vergeio_tenant_network_block`, `vergeio_tenant_layer2_network`, or `vergeio_tenant_snapshot`. The recipe instance owns the tenant: destroy powers it off, waits for its network to stop, deletes the tenant, then deletes the recipe instance.

```terraform
data "vergeio_catalogs" "tenants" {
  filter_name = "Tenants"
}

data "vergeio_tenant_recipes" "trial" {
  catalog_id  = data.vergeio_catalogs.tenants.catalogs[0].id
  filter_name = "30-Day Trial (POC)"
}

# answers values are strings. A disksize question is bytes: 50 GB is
# 53687091200. 50 is fifty bytes and is rejected. A bool question accepts
# true, false, yes, no, on, off, 1, or 0. enabled is rejected.
# tenant_id is the new tenant's key. Child resources take that key as a string.
resource "vergeio_tenant_recipe_instance" "customer" {
  name      = "customer-a"
  recipe_id = data.vergeio_tenant_recipes.trial.recipes[0].id

  answers = {
    YB_USER_NAME              = "admin"
    YB_EXPOSE_CLOUD_SNAPSHOTS = "true"
    YB_DRIVE_OS_SIZE          = "53687091200"
    YB_NIC_ETH0               = "Internal"
  }
}

resource "vergeio_tenant_node" "node" {
  tenant_id = tostring(vergeio_tenant_recipe_instance.customer.tenant_id)
  name      = "node1"
  cpu_cores = 4
  ram       = 8192
}
```

## Snapshots

`vergeio_tenant_snapshot` keeps one snapshot of the tenant. Create takes it. A later change to `description` updates that text. A later change to `expires` or `never_expires` updates the expiration. Destroy deletes the snapshot. `name` can be set at creation. VergeOS will not rename the snapshot, so a new name replaces it. Omit `name` and VergeOS assigns one. `type` is `full`, `partial_include`, or `partial_exclude`. Omit it and VergeOS uses `full`. Changing `type` or `tenant_id` replaces the snapshot.

The action `vergeio_tenant_snapshot` takes a snapshot from `terraform apply -invoke` or an `action_trigger` and does not keep it in state. Use the action for a one-off recovery point before a change. Use the resource when Terraform should retain the snapshot.

`vergeio_tenant_snapshots` lists the snapshots VergeOS has for one tenant, including ones this configuration did not create. `expires` is null when a snapshot does not expire. The Provider and Local labels in the tenant UI belong to cloud snapshots. This list reports the coverage type on the tenant snapshot row, which is not that label.

## Layer 2 networks

`vergeio_tenant_layer2_network` bridges a tenant onto a parent network (`tenant_layer2_vnets`). It requires VergeOS 26.0 or later. VergeOS creates three things inside the tenant: a NIC on the tenant node connected to that network, a physical network whose name is "Physical" followed by the parent network name, and an external network named after the parent network. `network_id` is the parent network key, the same value as `vergeio_network.id`. Changing `tenant_id` or `network_id` replaces the assignment. `enabled` updates in place and defaults to true.

Destroy disables the assignment, then deletes it. VergeOS rejects a delete that skips the disable. Networks created inside the tenant remain after the host-side delete. They belong to the tenant-side configuration. Remove them from inside the tenant. Leaving those components in place can block a later recreation of the same assignment.

An assignment made in the VergeOS UI, or with another client, can still leave `need_fw_apply` set. `vergeio_network_rules` on the parent configuration applies `need_fw_apply` for rule edits. It does not apply a flag left by an assignment made outside Terraform.
