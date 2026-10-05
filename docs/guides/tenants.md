---
page_title: "Tenants"
description: |-
  Create a VergeOS tenant on the parent, then manage the inside of that tenant from a second Terraform configuration.
---

# Tenants

A VergeOS tenant is a full VergeOS instance carved from the parent: its own UI, API, nodes, storage, and networks. `vergeio_tenant`, `vergeio_tenant_node`, `vergeio_tenant_storage`, and `vergeio_tenant_external_ip` declare that bundle on the parent. `vergeio_tenants` reads it back.

## Two configurations

Terraform configures a provider before it applies resources. The tenant UI address does not exist until `vergeio_tenant` is created, so the same configuration cannot point a provider at that address.

This configuration fails. The host is unknown until apply:

```terraform
resource "vergeio_tenant" "customer" {
  name       = "customer-a"
  password   = var.tenant_password
  powerstate = true
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

The parent module:

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
  name        = "customer-a"
  description = "Customer A virtual data center"
  password    = var.tenant_password
  powerstate  = true
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
  description = "Key of the parent external network that hands the tenant its UI address."
}

resource "vergeio_tenant_external_ip" "ui" {
  tenant_id             = vergeio_tenant.customer.id
  network_id            = var.parent_external_network_id
  ip                    = "203.0.113.50"
  hostname              = "customer-a"
  description           = "Tenant UI address"
  apply_parent_firewall = true
}

output "tenant_ui_address" {
  value = vergeio_tenant.customer.ui_address
}

output "tenant_id" {
  value = vergeio_tenant.customer.id
}
```

The tenant module. `var.tenant_password` is the same value passed to `vergeio_tenant.password` in the parent module. VergeOS creates the tenant admin user (`admin`) with that password. Keep the password in both modules' inputs. Leave it out of the parent outputs.

```terraform
variable "tenant_password" {
  type      = string
  sensitive = true
}

data "terraform_remote_state" "parent" {
  backend = "local"
  config = {
    path = "../parent/terraform.tfstate"
  }
}

provider "vergeio" {
  host     = data.terraform_remote_state.parent.outputs.tenant_ui_address
  username = "admin"
  password = var.tenant_password
  insecure = true
}

resource "vergeio_vm" "app" {
  name      = "app"
  os_family = "linux"
  cpu_cores = 2
  ram       = 2048
}
```

After the tenant exists, create an API key in the tenant UI and switch the tenant module to that key. The parent module cannot mint the key: a key is issued by the tenant API, which is this second configuration.

```terraform
provider "vergeio" {
  host    = data.terraform_remote_state.parent.outputs.tenant_ui_address
  api_key = var.tenant_api_key
}
```

## Addresses handed down from the parent

`vergeio_tenant_external_ip` declares one virtual IP on a parent network and sets the tenant as its owner. That is the address the parent UI assigns by hand today. The first assigned IP becomes the tenant UI address. A plan that runs after the address exists refreshes `vergeio_tenant` and can use `ui_address` in that same plan. `network_id` is the parent network key, the same value as `vergeio_network.id`.

Creating or deleting the address leaves `need_fw_apply` set on that parent network until its rules are applied. `apply_parent_firewall` applies them in the same call. `parent_firewall_pending` reports the flag afterward. A stopped parent network loads staged rules when it starts, and it may refuse a refresh while it is stopped.

`vergeio_tenant_network_block` is not a resource in this provider. The govergeos module this provider builds against can assign a network block on `vnet_cidrs`, and that call also reports `need_fw_apply` on the parent network. This provider does not call it. A layer 2 network handed to a tenant is not a resource either.

Assigning a network block in the VergeOS UI, or with another client, can leave `need_fw_apply` set on the parent external network. `vergeio_network_rules` on the parent configuration applies `need_fw_apply` for rule edits. It does not apply a flag left by an address assignment made outside Terraform.
