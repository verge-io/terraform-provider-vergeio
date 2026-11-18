# fleet

Applies one baseline, and the same internal networks and firewall policy, to the tenants in an inventory. One state configures one VergeOS system. The provider has one host. This module uses the provider the root module configures for `system_key`. It does not open a connection to any other system.

`tenants` is the inventory. The map key is the for_each key and must be known at plan time. `waves` is the set this state applies. A tenant is created only when its wave is in that set.

Roll a change out one wave at a time. Start with the canary wave. `terraform plan -detailed-exitcode` exits 0 when the configuration still matches and exits 2 when the plan has a diff. Apply when that diff is the canary you expect. Stop when it is not. Then add the next wave to `waves` and plan again. Tenants already in the set stay. A destroy of one of them is a plan to stop for. Removing a wave plans the destroy of that wave's tenants and networks. This repository does not schedule the plan. Run it in the system that holds this state.

A second system is a second state. Use one workspace per system, with Terraform or OpenTofu, and pass that system's `system_key`. The root module sets the provider host from the inventory entry for that key. Credentials stay in the environment for that system. Terraform Stacks can use this module as one component and put each system in its own deployment. OpenTofu does not implement Stacks. This repository does not include a stack file. A stack was not applied to more than one system.

`networks` is the policy, expressed once. Each tenant supplies a CIDR for every key. The network is an internal `vergeio_network` on this system, named from the tenant name, a hyphen, and the policy key, with that tenant's CIDR and the shared firewall rules. Those names must be unique. A tenant named `west-net` with key `app` and a tenant named `west` with key `net-app` both produce `west-net-app`. `ipaddress` is a bare IP. Omit it to use the first host of the CIDR. `powerstate` defaults to false, so a new network stays off until set. `external_network_id` is optional and is a network key the caller manages. Omit it to leave the network without an outbound gateway. There is no NAT resource. This module does not look up an existing vnet.

These networks are on the parent system. They are not created inside the tenant. `tenant_ui_addresses` is empty until VergeOS assigns an external IP. A second configuration, with its host set to that address, manages the inside of the tenant. `network_blocks` and `external_ips` assign a parent network the caller already manages. Omit both to assign neither.

`baseline_profile` is `baseline` or `strict`, passed to `modules/baseline`. `baseline_settings` is keyed by control id. `key` is the string VergeOS stores. The provider does not publish the stored names for the baseline controls. Read each key on this system. A key VergeOS does not have fails apply. `baseline_apply_settings` false writes no settings. Destroy of a setting this module writes restores that key's `default_value`, which can differ from the value stored before this module ran.

The tenant admin group is a group this module creates. Each selected tenant's admin user joins it. A membership in a group that already exists, including Administrators, is a `vergeio_member` next to the module. This module does not create tag categories.

Passwords go in `tenant_passwords`, `admin_passwords`, and `baseline_passwords`. They are sent as write-only attributes and are not stored. A password needs an uppercase letter, a lowercase letter, a digit, and 8 characters. Include passwords for a later wave before that wave is added. `vergeio_site` records a pairing with a remote system. It does not apply this module there.

```hcl
module "fleet" {
  source = "../../modules/fleet"

  system_key = "lab"
  waves      = ["canary"]

  networks = {
    app = {
      powerstate = false
      firewall_rules = [{
        name              = "allow-ssh"
        protocol          = "tcp"
        destination_ports = "22"
      }]
    }
  }

  tenants = {
    canary = {
      wave = "canary"
      name = "customer-canary"
      nodes = [{
        name      = "canary-node1"
        cpu_cores = 2
        ram       = 2048
      }]
      storage = [{
        tier        = 1
        provisioned = 1073741824
      }]
      admin_user = {
        name = "canary-admin"
      }
      networks = {
        app = {
          cidr       = "10.83.10.0/24"
          dhcp_start = "10.83.10.10"
          dhcp_stop  = "10.83.10.50"
        }
      }
    }
  }

  tenant_passwords = {
    canary = var.canary_tenant_password
  }

  admin_passwords = {
    canary = var.canary_admin_password
  }

  tenant_admin_group = {
    name = "tenant-admins"
  }

  baseline_apply_settings = false
}
```

A runnable root module is in `examples/modules/fleet`. The inventory in that root lists more than one system. `system_key` selects one.

`terraform test` in this directory plans with a mocked provider. The lab apply test is a separate root in `tests/lab` and is not part of that run. It sets `baseline_apply_settings` to false, creates only objects named with a `tf-acc-` prefix, and does not read an existing vnet, tenant, user, group, or setting:

```bash
terraform -chdir=tests/lab init -backend=false
terraform -chdir=tests/lab test
```
