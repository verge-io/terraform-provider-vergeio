---
page_title: "Adopt an existing VergeOS system"
description: |-
  List VMs, networks, tenants, users, groups, tags, and snapshot profiles that already exist, then import the ones you want to manage.
---

# Adopt an existing VergeOS system

`terraform query` lists objects that are already on a VergeOS system. Review that list, keep the ones this configuration should own, and import them. The identity `id` on each result is the same string `terraform import` and an OpenTofu `import` block already use.

`terraform query` needs Terraform 1.14 or later. OpenTofu uses the same provider configuration and the same `import` blocks. It does not run `terraform query`. Take the identity `id` from the VergeOS UI or API and write the import block by hand.

## What can be listed

| List type | Managed resource | Notes |
| --- | --- | --- |
| `vergeio_vm` | `vergeio_vm` | Snapshot VMs are omitted. |
| `vergeio_network` | `vergeio_network` | |
| `vergeio_tenant` | `vergeio_tenant` | Snapshot tenants are omitted. |
| `vergeio_user` | `vergeio_user` | |
| `vergeio_group` | `vergeio_group` | |
| `vergeio_tag` | `vergeio_tag` | |
| `vergeio_snapshot_profile` | `vergeio_snapshot_profile` | |

Drives, NICs, rules, members, and permissions are not list resources. Import those with the resource's import id after the object they belong to is in state.

## Filters

Every list accepts the same `config` arguments.

`name_pattern` is a name glob. `*` matches any run of characters and `?` matches one. A value with no glob is an exact name. A glob is applied to the names VergeOS returns. An exact name is also sent as `name eq`.

`tag` is a tag name, or `category/name` when that name exists in more than one category. Only objects assigned that tag are listed. VergeOS stores the assignment as `collection/key`. The collections for these lists are `vms`, `vnets`, `tenants`, `users`, `groups`, `tags`, and `snapshot_profiles`.

`tenant` is a tenant key or a tenant name. It is sent as `tenant eq` for `vergeio_vm` and `vergeio_network`. The other lists reject it. Objects inside a tenant are on that tenant's API. Point `host` at the tenant UI and query again.

`limit` on the list block caps how many results Terraform asks for. The CLI default is 100.

## Query

Put the provider in a normal `.tf` file. Put each query in a file whose name ends in `.tfquery.hcl`.

```terraform
terraform {
  required_providers {
    vergeio = {
      source = "vergeio/vergeio"
    }
  }
}

provider "vergeio" {
  # VERGEOS_HOST, and VERGEOS_API_KEY or VERGEOS_USERNAME and VERGEOS_PASSWORD.
}
```

```terraform
list "vergeio_vm" "web" {
  provider         = vergeio
  include_resource = true

  config {
    name_pattern = "web-*"
    tag          = "env/prod"
    tenant       = "customer-a"
  }
}

list "vergeio_network" "web" {
  provider = vergeio

  config {
    name_pattern = "web-*"
  }
}
```

`include_resource = true` asks the provider for the managed resource attributes. Leave it off when you only need names and ids.

List what is there:

```shell
terraform query
```

Generate import and resource blocks. The output file must not already exist:

```shell
terraform query -generate-config-out=generated.tf
```

## Review

Read `generated.tf` before you apply it. Generated configuration repeats what VergeOS returned, including values you may want to leave unset. Delete attributes this configuration should not own. A generated `vergeio_vm` does not include `vergeio_vm_drive` or `vergeio_vm_nic` resources. Add those yourself when you want Terraform to manage them.

`password`, `password_wo`, `console_pass`, and cloud-init file bodies are not recovered. A user or tenant that was created outside Terraform has no password in the generated file. Set `password_wo` and `password_wo_version` when you want this configuration to change that secret later.

The identity in each generated import block is the import id:

```terraform
import {
  to = vergeio_vm.web_1
  id = "15"
}

resource "vergeio_vm" "web_1" {
  name      = "web-1"
  cpu_cores = 2
  ram       = 2048
}
```

An OpenTofu configuration uses that same `import` block and resource. Copy them into the `.tf` files OpenTofu will apply.

## Import

Move the blocks you want to keep into the configuration Terraform or OpenTofu applies. Then plan.

```shell
terraform plan
```

The plan should show imports, and no other changes, once the resource arguments match VergeOS. Apply that plan.

```shell
terraform apply
```

Plan again. An empty plan means this configuration now matches the imported objects. A diff that remains is an argument the generated file does not match. Change the configuration or apply that diff on purpose. Do not apply a plan you have not read. Query lists every matching object, including ones this configuration should leave alone.
