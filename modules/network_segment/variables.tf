variable "name" {
  type        = string
  description = "Network name. Must be unique."

  validation {
    condition     = length(var.name) > 0
    error_message = "name must not be empty."
  }
}

variable "description" {
  type        = string
  description = "Network description. Omit to leave an existing description unchanged."
  default     = null
}

variable "cidr" {
  type        = string
  description = "CIDR of this internal network, written to vergeio_network.network. Use the network address, such as 192.168.16.0/24."

  validation {
    condition     = can(cidrhost(var.cidr, 0)) && try(cidrhost(var.cidr, 0) == split("/", var.cidr)[0], false)
    error_message = "cidr must be a network address in CIDR notation, such as 192.168.16.0/24."
  }
}

variable "ipaddress" {
  type        = string
  description = "Address of this network, such as 192.168.16.1/24. Omit to use the first host of cidr, for example 192.168.16.1/24 from 192.168.16.0/24."
  default     = null
}

variable "enabled" {
  type        = bool
  description = "Whether the network is enabled."
  default     = true
}

variable "powerstate" {
  type        = bool
  description = "Whether the network router is powered on. Omit to leave the current power unchanged."
  default     = null
}

variable "dhcp_enabled" {
  type        = bool
  description = "Hand addresses to clients from dhcp_start through dhcp_stop."
  default     = true
}

variable "dynamic_dhcp" {
  type        = bool
  description = "Enable the dynamic DHCP pool. Used when dhcp_enabled is true."
  default     = true
}

variable "dhcp_sequential" {
  type        = bool
  description = "Allocate the dynamic pool in order. Used when dynamic_dhcp is true."
  default     = true
}

variable "dhcp_start" {
  type        = string
  description = "First address of the DHCP pool. Required when dhcp_enabled is true."
  default     = null

  validation {
    condition     = var.dhcp_start == null || can(cidrhost("${var.dhcp_start}/32", 0))
    error_message = "dhcp_start must be an IP address."
  }
}

variable "dhcp_stop" {
  type        = string
  description = "Last address of the DHCP pool. Required when dhcp_enabled is true."
  default     = null

  validation {
    condition     = var.dhcp_stop == null || can(cidrhost("${var.dhcp_stop}/32", 0))
    error_message = "dhcp_stop must be an IP address."
  }
}

variable "dns_servers" {
  type        = list(string)
  description = "DNS servers handed to DHCP clients. Written to dnslist as a comma-separated list."
  default     = []

  validation {
    condition = alltrue([
      for server in var.dns_servers :
      !strcontains(server, ",") && can(cidrhost("${server}/32", 0))
    ])
    error_message = "Each dns_servers entry must be an IP address."
  }
}

variable "domain" {
  type        = string
  description = "DNS domain handed to DHCP clients."
  default     = null
}

variable "dns" {
  type = object({
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
  })
  description = "DNS view, master zone, and records served by this network. Omit to skip those resources. DHCP still hands out dns_servers."
  default     = null

  validation {
    condition     = var.dns == null || length(var.dns.zone_domain) > 0
    error_message = "dns.zone_domain must not be empty."
  }

  validation {
    condition = var.dns == null || alltrue([
      for record in var.dns.records :
      contains(["A", "AAAA", "CNAME", "MX", "NS", "PTR", "SRV", "TXT", "CAA"], record.type) && length(record.value) > 0
    ])
    error_message = "Each DNS record type must be A, AAAA, CNAME, MX, NS, PTR, SRV, TXT, or CAA, and value must not be empty."
  }

  validation {
    condition = var.dns == null || length(var.dns.records) == length(distinct([
      for record in var.dns.records : "${coalesce(record.host, "")}/${record.type}/${record.value}"
    ]))
    error_message = "DNS record host, type, and value must be unique within the zone."
  }
}

variable "external_network_id" {
  type        = number
  description = "Key of an external network, written to vnet_default_gateway. Omit to leave the internal network without an outbound gateway. The provider schema has no separate NAT resource."
  default     = null

  validation {
    condition     = var.external_network_id == null || var.external_network_id == floor(var.external_network_id)
    error_message = "external_network_id must be a whole number."
  }
}

variable "firewall_rules" {
  type = list(object({
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
  }))
  description = "Non-system firewall policy, in order. vergeio_network_rules owns every non-system rule on this network. An empty list leaves only system rules."
  default     = []

  validation {
    condition     = length(var.firewall_rules) == length(distinct([for rule in var.firewall_rules : rule.name]))
    error_message = "Firewall rule names must be unique on the network."
  }

  validation {
    condition = alltrue([
      for rule in var.firewall_rules :
      length(rule.name) > 0 &&
      contains(["accept", "drop", "reject", "translate", "route"], coalesce(rule.action, "accept")) &&
      contains(["incoming", "outgoing"], coalesce(rule.direction, "incoming")) &&
      contains(["tcp", "udp", "tcpudp", "icmp", "any", "2", "47", "50", "51", "89"], coalesce(rule.protocol, "any"))
    ])
    error_message = "Each firewall rule needs a name. action is accept, drop, reject, translate, or route. direction is incoming or outgoing. protocol is tcp, udp, tcpudp, icmp, any, 2, 47, 50, 51, or 89."
  }
}
