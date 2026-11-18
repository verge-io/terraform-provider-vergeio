variable "system_key" {
  type        = string
  description = "Inventory key of the system this state configures. The root module sets the provider host for that system. This module does not connect to any other system."

  validation {
    condition     = length(var.system_key) > 0 && length(regexall("[/\\s]", var.system_key)) == 0
    error_message = "system_key must not be empty and must not contain spaces or slashes."
  }
}

variable "waves" {
  type        = set(string)
  description = "Waves this state applies. A tenant is created only when its wave is in this set. Start with the canary wave. Add the next wave after that plan shows only the additions you expect. Removing a wave plans a destroy of that wave's tenants and networks."

  validation {
    condition     = length(var.waves) > 0
    error_message = "waves must contain at least one wave."
  }

  validation {
    condition = alltrue([
      for wave in var.waves : length(wave) > 0 && length(regexall("[/\\s]", wave)) == 0
    ])
    error_message = "Each wave name must not be empty and must not contain spaces or slashes."
  }
}

variable "networks" {
  type = map(object({
    description         = optional(string)
    enabled             = optional(bool, true)
    powerstate          = optional(bool, false)
    dhcp_enabled        = optional(bool, true)
    dynamic_dhcp        = optional(bool, true)
    dhcp_sequential     = optional(bool, true)
    dns_servers         = optional(list(string), [])
    external_network_id = optional(number)
    firewall_rules = optional(list(object({
      name              = string
      description       = optional(string)
      direction         = optional(string)
      action            = optional(string)
      protocol          = optional(string)
      source_ip         = optional(string)
      source_ports      = optional(string)
      destination_ip    = optional(string)
      destination_ports = optional(string)
      target_ip         = optional(string)
      target_ports      = optional(string)
      enabled           = optional(bool)
    })), [])
  }))
  description = "Internal network and firewall policy applied to every tenant. The map key is the for_each key and must be known at plan time. Each tenant supplies the CIDR. powerstate defaults to false so a new network stays off until set. external_network_id is a network key the caller manages. Omit it to leave the network without an outbound gateway. This module does not look up an existing vnet."

  validation {
    condition     = length(var.networks) > 0
    error_message = "networks must contain the policy applied to every tenant."
  }

  validation {
    condition = alltrue([
      for key, _ in var.networks : length(key) > 0 && length(regexall("/", key)) == 0
    ])
    error_message = "Each network key must not be empty and must not contain a slash. The key is part of the for_each address."
  }

  validation {
    condition = alltrue([
      for _, network in var.networks : alltrue([
        for server in network.dns_servers : !strcontains(server, ",") && can(cidrhost("${server}/32", 0))
      ])
    ])
    error_message = "Each dns_servers entry must be an IP address."
  }

  validation {
    condition = alltrue([
      for _, network in var.networks :
      network.external_network_id == null || network.external_network_id == floor(network.external_network_id)
    ])
    error_message = "external_network_id must be a whole number when set."
  }

  validation {
    condition = alltrue([
      for _, network in var.networks :
      length(network.firewall_rules) == length(distinct([for rule in network.firewall_rules : rule.name]))
    ])
    error_message = "Firewall rule names must be unique on a network."
  }

  validation {
    condition = alltrue([
      for _, network in var.networks : alltrue([
        for rule in network.firewall_rules :
        length(rule.name) > 0 &&
        contains(["accept", "drop", "reject", "translate", "route"], coalesce(rule.action, "accept")) &&
        contains(["incoming", "outgoing"], coalesce(rule.direction, "incoming")) &&
        contains(["tcp", "udp", "tcpudp", "icmp", "any", "2", "47", "50", "51", "89"], coalesce(rule.protocol, "any"))
      ])
    ])
    error_message = "Each firewall rule needs a name. action is accept, drop, reject, translate, or route. direction is incoming or outgoing. protocol is tcp, udp, tcpudp, icmp, any, 2, 47, 50, 51, or 89."
  }
}

variable "tenants" {
  type = map(object({
    wave            = string
    name            = string
    description     = optional(string)
    change_password = optional(bool)
    powerstate      = optional(bool, false)
    isolate         = optional(bool)
    nodes = list(object({
      name           = string
      cpu_cores      = number
      ram            = number
      description    = optional(string)
      enabled        = optional(bool, true)
      on_power_loss  = optional(string)
      cluster        = optional(number)
      preferred_node = optional(number)
    }))
    storage = list(object({
      tier        = number
      provisioned = number
    }))
    network_blocks = optional(list(object({
      network_id            = string
      cidr                  = string
      description           = optional(string)
      apply_parent_firewall = optional(bool, true)
    })), [])
    external_ips = optional(list(object({
      network_id            = string
      ip                    = string
      hostname              = optional(string)
      description           = optional(string)
      apply_parent_firewall = optional(bool, true)
    })), [])
    admin_user = object({
      name            = string
      displayname     = optional(string)
      email           = optional(string)
      enabled         = optional(bool, true)
      type            = optional(string, "normal")
      change_password = optional(bool, true)
    })
    networks = map(object({
      cidr       = string
      ipaddress  = optional(string)
      dhcp_start = optional(string)
      dhcp_stop  = optional(string)
      domain     = optional(string)
      dns = optional(object({
        zone_domain   = string
        view_name     = optional(string, "internal")
        match_clients = optional(string)
        recursion     = optional(bool)
        email         = optional(string)
        records = optional(list(object({
          type          = string
          value         = string
          host          = optional(string)
          ttl           = optional(string)
          description   = optional(string)
          mx_preference = optional(number)
          port          = optional(number)
          weight        = optional(number)
        })), [])
      }))
    }))
  }))
  description = "Tenant inventory, keyed by a name known at plan time. wave selects the rollout set. powerstate defaults to false, so a new tenant stays off until set. networks is keyed by the networks policy. cidr is the network address. ipaddress is a bare IP. Omit ipaddress to use the first host of cidr. network_blocks and external_ips need a parent network id the caller manages. Omit both to assign neither. This module does not read an existing tenant, vnet, or address."

  validation {
    condition     = length(var.tenants) > 0
    error_message = "tenants must contain the inventory. A wave selects from this map."
  }

  validation {
    condition = alltrue([
      for key, _ in var.tenants : length(key) > 0 && length(regexall("/", key)) == 0
    ])
    error_message = "Each tenant key must not be empty and must not contain a slash. The key is the for_each address."
  }

  validation {
    condition = alltrue([
      for _, tenant in var.tenants :
      length(tenant.wave) > 0 && length(regexall("[/\\s]", tenant.wave)) == 0
    ])
    error_message = "Each tenant wave must not be empty and must not contain spaces or slashes."
  }

  validation {
    condition = length(var.tenants) == length(distinct([for _, tenant in var.tenants : tenant.name])) && alltrue([
      for _, tenant in var.tenants : length(tenant.name) > 0 && length(regexall("[/\\s]", tenant.name)) == 0
    ])
    error_message = "Tenant names must be unique, and each name must not be empty or contain spaces or slashes."
  }

  validation {
    condition = length(var.tenants) == length(distinct([for _, tenant in var.tenants : tenant.admin_user.name])) && alltrue([
      for _, tenant in var.tenants :
      length(tenant.admin_user.name) > 0 && length(regexall("[/\\s]", tenant.admin_user.name)) == 0 && contains(["normal", "api", "vdi"], tenant.admin_user.type)
    ])
    error_message = "Tenant admin user names must be unique. Each name must not be empty or contain spaces or slashes, and type must be normal, api, or vdi."
  }

  validation {
    condition = alltrue([
      for _, tenant in var.tenants : length(tenant.nodes) > 0 && length(tenant.nodes) == length(distinct([for node in tenant.nodes : node.name]))
    ])
    error_message = "Each tenant needs at least one node, and node names must be unique on that tenant."
  }

  validation {
    condition = alltrue([
      for _, tenant in var.tenants : alltrue([
        for node in tenant.nodes :
        length(node.name) > 0 && length(regexall("[/\\s]", node.name)) == 0 && node.cpu_cores >= 1 && node.cpu_cores == floor(node.cpu_cores) && node.ram >= 2048 && node.ram == floor(node.ram)
      ])
    ])
    error_message = "Each node needs a name without spaces or slashes, a whole cpu_cores of at least 1, and a whole ram of at least 2048 megabytes."
  }

  validation {
    condition = alltrue([
      for _, tenant in var.tenants : alltrue([
        for node in tenant.nodes :
        node.on_power_loss == null || contains(["power_on", "last_state", "leave_off"], node.on_power_loss)
      ])
    ])
    error_message = "node on_power_loss must be power_on, last_state, or leave_off."
  }

  validation {
    condition = alltrue([
      for _, tenant in var.tenants : length(tenant.storage) > 0 && length(tenant.storage) == length(distinct([for item in tenant.storage : item.tier]))
    ])
    error_message = "Each tenant needs at least one storage allocation, and storage tiers must be unique on that tenant."
  }

  validation {
    condition = alltrue([
      for _, tenant in var.tenants : alltrue([
        for item in tenant.storage :
        item.tier == floor(item.tier) && item.provisioned > 0 && item.provisioned == floor(item.provisioned) && item.provisioned % 1073741824 == 0
      ])
    ])
    error_message = "Each storage tier must be a whole number, and provisioned must be a positive multiple of 1073741824 (1 GiB)."
  }

  validation {
    condition = alltrue([
      for _, tenant in var.tenants : alltrue([
        for network_key, assignment in tenant.networks :
        length(network_key) > 0 && length(regexall("/", network_key)) == 0 && can(cidrhost(assignment.cidr, 1)) && try(cidrhost(assignment.cidr, 0) == split("/", assignment.cidr)[0], false)
      ])
    ])
    error_message = "Each tenant network key must not contain a slash. cidr must be a network address that has a host, such as 10.83.10.0/24."
  }

  validation {
    condition = alltrue([
      for _, tenant in var.tenants : alltrue([
        for _, assignment in tenant.networks :
        assignment.ipaddress == null || can(cidrhost("${assignment.ipaddress}/32", 0))
      ])
    ])
    error_message = "Tenant network ipaddress must be a bare IP, such as 10.83.10.1."
  }

  validation {
    condition = alltrue([
      for _, tenant in var.tenants : alltrue([
        for _, assignment in tenant.networks :
        (assignment.dhcp_start == null || can(cidrhost("${assignment.dhcp_start}/32", 0))) && (assignment.dhcp_stop == null || can(cidrhost("${assignment.dhcp_stop}/32", 0)))
      ])
    ])
    error_message = "dhcp_start and dhcp_stop must be IP addresses when set."
  }

  validation {
    condition = alltrue([
      for _, tenant in var.tenants :
      length(tenant.network_blocks) == length(distinct([for block in tenant.network_blocks : block.cidr]))
    ])
    error_message = "Network block CIDRs must be unique on a tenant."
  }

  validation {
    condition = alltrue([
      for _, tenant in var.tenants : alltrue([
        for block in tenant.network_blocks :
        length(block.network_id) > 0 && can(cidrhost(block.cidr, 0)) && try(cidrhost(block.cidr, 0) == split("/", block.cidr)[0], false)
      ])
    ])
    error_message = "Each network block needs a network_id and a network address in CIDR notation, such as 198.51.100.0/28."
  }

  validation {
    condition = alltrue([
      for _, tenant in var.tenants :
      length(tenant.external_ips) == length(distinct([for addr in tenant.external_ips : addr.ip]))
    ])
    error_message = "External IP addresses must be unique on a tenant."
  }

  validation {
    condition = alltrue([
      for _, tenant in var.tenants : alltrue([
        for addr in tenant.external_ips :
        length(addr.network_id) > 0 && can(cidrhost("${addr.ip}/32", 0))
      ])
    ])
    error_message = "Each external IP needs a network_id and an IP address."
  }

  validation {
    condition = alltrue([
      for _, tenant in var.tenants : alltrue([
        for _, assignment in tenant.networks :
        assignment.dns == null || (
          length(assignment.dns.zone_domain) > 0 &&
          length(assignment.dns.records) == length(distinct([
            for record in assignment.dns.records : "${coalesce(record.host, "")}/${record.type}/${record.value}"
          ])) &&
          alltrue([
            for record in assignment.dns.records :
            contains(["A", "AAAA", "CNAME", "MX", "NS", "PTR", "SRV", "TXT", "CAA"], record.type) && length(record.value) > 0
          ])
        )
      ])
    ])
    error_message = "DNS zone_domain must not be empty. Record types must be A, AAAA, CNAME, MX, NS, PTR, SRV, TXT, or CAA. host, type, and value must be unique in the zone."
  }
}

variable "tenant_passwords" {
  type        = map(string)
  description = "Built-in tenant admin passwords, keyed by tenant inventory key. Sent as vergeio_tenant.password_wo and not stored. Include a later wave before that wave is applied. VergeOS requires an uppercase letter, a lowercase letter, and a digit."
  sensitive   = true

  validation {
    condition = alltrue([
      for _, password in var.tenant_passwords :
      length(password) >= 8 && can(regex("[A-Z]", password)) && can(regex("[a-z]", password)) && can(regex("[0-9]", password))
    ])
    error_message = "Each tenant password must be at least 8 characters and include an uppercase letter, a lowercase letter, and a digit."
  }
}

variable "tenant_password_versions" {
  type        = map(number)
  description = "tenant_password_wo_version keyed by tenant inventory key. Omit a key to use 1. Increment the number to change that tenant's password."
  default     = {}

  validation {
    condition = alltrue([
      for _, version in var.tenant_password_versions : version >= 1 && version == floor(version)
    ])
    error_message = "Each tenant password version must be a whole number of at least 1."
  }
}

variable "admin_passwords" {
  type        = map(string)
  description = "Parent-system admin user passwords, keyed by tenant inventory key. Sent as vergeio_user.password_wo and not stored. VergeOS requires an uppercase letter, a lowercase letter, and a digit."
  sensitive   = true

  validation {
    condition = alltrue([
      for _, password in var.admin_passwords :
      length(password) >= 8 && can(regex("[A-Z]", password)) && can(regex("[a-z]", password)) && can(regex("[0-9]", password))
    ])
    error_message = "Each admin password must be at least 8 characters and include an uppercase letter, a lowercase letter, and a digit."
  }
}

variable "admin_password_versions" {
  type        = map(number)
  description = "admin_password_wo_version keyed by tenant inventory key. Omit a key to use 1. Increment the number to change that user's password."
  default     = {}

  validation {
    condition = alltrue([
      for _, version in var.admin_password_versions : version >= 1 && version == floor(version)
    ])
    error_message = "Each admin password version must be a whole number of at least 1."
  }
}

variable "tenant_admin_group" {
  type = object({
    name        = string
    description = optional(string)
    enabled     = optional(bool, true)
    permissions = optional(list(object({
      table     = string
      object_id = optional(number)
      list      = optional(bool)
      read      = optional(bool)
      create    = optional(bool)
      modify    = optional(bool)
      delete    = optional(bool)
    })), [])
  })
  description = "Group this module creates. Each selected tenant's admin user joins it. name is not an existing group. A membership in a group that already exists is a vergeio_member next to the module."

  validation {
    condition     = length(var.tenant_admin_group.name) > 0 && length(regexall("[/\\s]", var.tenant_admin_group.name)) == 0
    error_message = "tenant_admin_group.name must not be empty and must not contain spaces or slashes."
  }

  validation {
    condition = length(var.tenant_admin_group.permissions) == length(distinct([
      for perm in var.tenant_admin_group.permissions : "${perm.table}/${perm.object_id == null ? "table" : tostring(perm.object_id)}"
    ]))
    error_message = "tenant_admin_group can grant a table, or one object in that table, once."
  }

  validation {
    condition = alltrue([
      for perm in var.tenant_admin_group.permissions :
      length(perm.table) > 0 && (perm.read != true || perm.list == null || perm.list == true)
    ])
    error_message = "Each tenant admin permission needs a table. When read is true, list must be true or omitted."
  }
}

variable "baseline_profile" {
  type        = string
  description = "Profile passed to modules/baseline. baseline writes the controls you pass. strict also requires every setting control when baseline_apply_settings is true, a certificate, a group, and a user in a group."
  default     = "baseline"

  validation {
    condition     = contains(["baseline", "strict"], var.baseline_profile)
    error_message = "baseline_profile must be baseline or strict."
  }
}

variable "baseline_apply_settings" {
  type        = bool
  description = "Write vergeio_setting for each baseline_settings control. Set false to leave every system setting unchanged. Destroy of a setting the baseline writes restores that key's default_value."
  default     = true
}

variable "baseline_settings" {
  type = map(object({
    key   = string
    value = string
  }))
  description = "System settings passed to modules/baseline, keyed by control id. key is the string VergeOS stores. The provider does not publish the stored names. Read each key on this system. for_each uses the control id, which must be known at plan time."
  default     = {}
}

variable "baseline_api_key_max_lifetime_seconds" {
  type        = number
  description = "Passed to modules/baseline. When set, each baseline API key expires must fall within this many seconds of the plan time."
  default     = null
}

variable "baseline_certificates" {
  type = list(object({
    name                    = string
    type                    = string
    domain_name             = optional(string)
    domain_list             = optional(string)
    description             = optional(string)
    public_certificate      = optional(string)
    private_key_wo_version  = optional(number)
    chain                   = optional(string)
    acme_server             = optional(string)
    eab_kid                 = optional(string)
    eab_hmac_key_wo_version = optional(number)
    key_type                = optional(string)
    rsa_key_size            = optional(string)
    contact                 = optional(number)
    agree_tos               = optional(bool)
  }))
  description = "TLS certificates passed to modules/baseline. name is the for_each key. A manual private key goes in baseline_certificate_private_keys."
  default     = []
}

variable "baseline_certificate_private_keys" {
  type        = map(string)
  description = "PEM private keys for manual baseline certificates, keyed by certificate name. Sent as private_key_wo and not stored."
  sensitive   = true
  default     = {}
}

variable "baseline_certificate_eab_hmac_keys" {
  type        = map(string)
  description = "External account binding HMAC keys for baseline ACME certificates, keyed by certificate name. Sent as eab_hmac_key_wo and not stored."
  sensitive   = true
  default     = {}
}

variable "baseline_groups" {
  type = list(object({
    name        = string
    description = optional(string)
    enabled     = optional(bool, true)
    permissions = optional(list(object({
      table     = string
      object_id = optional(number)
      list      = optional(bool)
      read      = optional(bool)
      create    = optional(bool)
      modify    = optional(bool)
      delete    = optional(bool)
    })), [])
  }))
  description = "Groups passed to modules/baseline. This module creates them. The tenant admin group is tenant_admin_group, not an entry here."
  default     = []
}

variable "baseline_users" {
  type = list(object({
    name            = string
    displayname     = optional(string)
    email           = optional(string)
    enabled         = optional(bool, true)
    type            = optional(string, "normal")
    change_password = optional(bool, true)
    groups          = optional(list(string), [])
    api_keys = optional(list(object({
      name          = string
      description   = optional(string)
      expires       = number
      ip_allow_list = optional(string)
      ip_deny_list  = optional(string)
    })), [])
  }))
  description = "Users passed to modules/baseline. name is the for_each key and must be known at plan time. Passwords go in baseline_passwords."
  default     = []
}

variable "baseline_passwords" {
  type        = map(string)
  description = "Baseline user passwords, keyed by user name. Sent as password_wo and not stored. VergeOS requires an uppercase letter, a lowercase letter, and a digit."
  sensitive   = true
  default     = {}
}

variable "baseline_password_versions" {
  type        = map(number)
  description = "password_wo_version for baseline users, keyed by user name. Omit a name to use 1."
  default     = {}
}
