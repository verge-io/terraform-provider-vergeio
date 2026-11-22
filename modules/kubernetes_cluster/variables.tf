variable "name" {
  type        = string
  description = "Cluster name. The network, pool VM, API user, and firewall rules use it as a prefix. Node VMs are named name-cp-N and name-worker-N."

  validation {
    condition     = can(regex("^[a-z0-9](?:[a-z0-9-]{0,30}[a-z0-9])?$", var.name))
    error_message = "name must be 1 to 32 characters of lowercase letters, digits, and hyphens, and must not start or end with a hyphen."
  }
}

variable "description" {
  type        = string
  description = "Network description. Omit to leave an existing description unchanged."
  default     = null
}

variable "source_type" {
  type        = string
  description = "image creates each node with vergeio_vm from a media image or a blank disk. recipe creates each node with vergeio_vm_recipe_instance. A recipe instance has no cloud-init files, so the certificates this module generates are not written to those VMs."

  validation {
    condition     = contains(["image", "recipe"], var.source_type)
    error_message = "source_type must be image or recipe."
  }
}

variable "media_source_id" {
  type        = number
  description = "Media image key for an image node. vergeio_mediasources.mediasources[].id. Written to boot_disk.source with media import. Omit it and set boot_disk_size to create a blank disk."
  default     = null

  validation {
    condition     = var.media_source_id == null || (var.media_source_id > 0 && var.media_source_id == floor(var.media_source_id))
    error_message = "media_source_id must be a positive whole number."
  }
}

variable "boot_disk_size" {
  type        = number
  description = "Boot disk size in GB for an image node. Omit it when media_source_id supplies the disk."
  default     = null

  validation {
    condition     = var.boot_disk_size == null || (var.boot_disk_size > 0 && var.boot_disk_size == floor(var.boot_disk_size))
    error_message = "boot_disk_size must be a positive whole number."
  }
}

variable "recipe_id" {
  type        = string
  description = "Recipe key for a recipe node. 40 hexadecimal characters. vergeio_vm_recipes.recipes[].id. Question names are not set by this module. Put hostname, disk, and network answers in recipe_answers."
  default     = null

  validation {
    condition     = var.recipe_id == null || can(regex("(?i)^[0-9a-f]{40}$", var.recipe_id))
    error_message = "recipe_id must be a 40-character hexadecimal recipe key."
  }
}

variable "recipe_answers" {
  type        = map(string)
  description = "Recipe question names and string answers, applied to every recipe node and to the template when create_template is true. The same map is used for every node. A disksize answer is bytes. A network answer is a network name, a vnet key, or __new_internal__."
  default     = {}
  sensitive   = true
}

variable "recipe_auto_update" {
  type        = bool
  description = "Update recipe nodes when the recipe is updated. Omit to leave the platform default. Changing it replaces the VMs."
  default     = null
}

variable "cidr" {
  type        = string
  description = "CIDR of the cluster network. The address is the network address, such as 192.168.100.0/24. The network must contain host 215. A /24 or larger does. Host 1 is the router. Host 2 is the template. Hosts 10 and up are control planes. Hosts 40 and up are workers. Hosts 150 through 189 are the DHCP pool. Hosts 200 and up are the load balancer pool."

  validation {
    condition     = can(cidrhost(var.cidr, 0)) && try(cidrhost(var.cidr, 0) == split("/", var.cidr)[0], false) && can(cidrhost(var.cidr, 215))
    error_message = "cidr must be a network address in CIDR notation that contains at least 216 hosts, such as 192.168.100.0/24."
  }
}

variable "external_network_id" {
  type        = number
  description = "External network key written to vnet_default_gateway. Omit it for a cluster network with no uplink."
  default     = null

  validation {
    condition     = var.external_network_id == null || (var.external_network_id > 0 && var.external_network_id == floor(var.external_network_id))
    error_message = "external_network_id must be a positive whole number."
  }
}

variable "network_powerstate" {
  type        = bool
  description = "Whether the cluster network router is powered on. True applies firewall rules after they are written. A stopped network loads staged rules when it starts."
  default     = true
}

variable "dns_servers" {
  type        = list(string)
  description = "DNS servers handed to DHCP clients. Written to dnslist as a comma-separated list. Omit to leave the current list unchanged."
  default     = []

  validation {
    condition     = alltrue([for server in var.dns_servers : can(cidrhost("${server}/32", 0))])
    error_message = "dns_servers entries must be IP addresses."
  }
}

variable "domain" {
  type        = string
  description = "DNS domain handed to DHCP clients. Omit to leave the current domain unchanged."
  default     = null

  validation {
    condition     = var.domain == null || length(var.domain) > 0
    error_message = "domain must not be empty."
  }
}

variable "control_plane_count" {
  type        = number
  description = "Number of control plane VMs. The first is https://<host 10>:6443."
  default     = 1

  validation {
    condition     = var.control_plane_count >= 1 && var.control_plane_count <= 20 && var.control_plane_count == floor(var.control_plane_count)
    error_message = "control_plane_count must be a whole number from 1 to 20."
  }
}

variable "worker_count" {
  type        = number
  description = "Number of worker VMs."
  default     = 1

  validation {
    condition     = var.worker_count >= 0 && var.worker_count <= 20 && var.worker_count == floor(var.worker_count)
    error_message = "worker_count must be a whole number from 0 to 20."
  }
}

variable "control_plane_cpu_cores" {
  type        = number
  description = "CPU cores for each control plane VM and for the template VM."
  default     = 2

  validation {
    condition     = var.control_plane_cpu_cores >= 1 && var.control_plane_cpu_cores == floor(var.control_plane_cpu_cores)
    error_message = "control_plane_cpu_cores must be a whole number of at least 1."
  }
}

variable "control_plane_ram" {
  type        = number
  description = "RAM in megabytes for each control plane VM and for the template VM."
  default     = 2048

  validation {
    condition     = var.control_plane_ram >= 1 && var.control_plane_ram == floor(var.control_plane_ram)
    error_message = "control_plane_ram must be a whole number of at least 1."
  }
}

variable "worker_cpu_cores" {
  type        = number
  description = "CPU cores for each worker VM."
  default     = 2

  validation {
    condition     = var.worker_cpu_cores >= 1 && var.worker_cpu_cores == floor(var.worker_cpu_cores)
    error_message = "worker_cpu_cores must be a whole number of at least 1."
  }
}

variable "worker_ram" {
  type        = number
  description = "RAM in megabytes for each worker VM."
  default     = 2048

  validation {
    condition     = var.worker_ram >= 1 && var.worker_ram == floor(var.worker_ram)
    error_message = "worker_ram must be a whole number of at least 1."
  }
}

variable "cluster" {
  type        = number
  description = "Compute cluster key for image VMs. Omit to leave the system default."
  default     = null

  validation {
    condition     = var.cluster == null || (var.cluster > 0 && var.cluster == floor(var.cluster))
    error_message = "cluster must be a positive whole number."
  }
}

variable "powerstate" {
  type        = bool
  description = "Whether image node VMs are powered on. False leaves a new VM stopped. The pool VM and the template stay stopped."
  default     = false
}

variable "create_template" {
  type        = bool
  description = "Create a stopped VM named name-template for the VergeOS Rancher node driver to clone. An image template is attached to this network at host 2 and has the guest agent enabled. A recipe template uses recipe_answers and has no NIC from this module."
  default     = false
}

variable "load_balancer_pool_size" {
  type        = number
  description = "Number of addresses reserved for the cloud controller load balancer pool, starting at host 200. These addresses are not assigned to nodes and are not in the DHCP pool."
  default     = 2

  validation {
    condition     = var.load_balancer_pool_size >= 1 && var.load_balancer_pool_size <= 16 && var.load_balancer_pool_size == floor(var.load_balancer_pool_size)
    error_message = "load_balancer_pool_size must be a whole number from 1 to 16."
  }
}

variable "api_source_cidr" {
  type        = string
  description = "Source CIDR for the TCP 6443 and TCP 9345 accept rules. Omit it to leave the source unset."
  default     = null

  validation {
    condition     = var.api_source_cidr == null || can(cidrhost(var.api_source_cidr, 0))
    error_message = "api_source_cidr must be a CIDR, such as 192.0.2.0/24."
  }
}

variable "ssh_source_cidr" {
  type        = string
  description = "When set, add an incoming TCP 22 accept rule from this CIDR."
  default     = null

  validation {
    condition     = var.ssh_source_cidr == null || can(cidrhost(var.ssh_source_cidr, 0))
    error_message = "ssh_source_cidr must be a CIDR, such as 192.0.2.0/24."
  }
}

variable "vergeos_host" {
  type        = string
  description = "VergeOS API URL for the CSI driver and cloud controller, such as https://vergeos.example.com. A host without a scheme is prefixed with https://. A path is removed. A port is kept on vergeos_host and removed from vergeos_hostname. IPv6 addresses are not parsed."

  validation {
    condition     = length(var.vergeos_host) > 0 && !strcontains(var.vergeos_host, " ")
    error_message = "vergeos_host must not be empty."
  }
}

variable "vergeos_verify_ssl" {
  type        = bool
  description = "Verify the VergeOS API certificate. False matches the published vergeos-csi 0.2.0 and vergeos-cloud-controller-manager 0.2.0 chart default. The charts read this as the string true or false."
  default     = false
}
