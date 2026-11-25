variable "name" {
  type        = string
  description = "Name prefix for the fabric networks and node VMs. Must be unique on the system. Generated names append -core-1, -controller-1, -scale-1, -storage-1, and -compute-1."

  validation {
    condition     = can(regex("^[A-Za-z0-9]([A-Za-z0-9-]{0,38}[A-Za-z0-9])?$", var.name))
    error_message = "name must start and end with a letter or digit, may contain hyphens, and must be 40 characters or fewer."
  }
}

variable "shape" {
  type        = string
  description = "Cluster shape. hci is two controllers. scale_out adds scale-out nodes. hybrid adds compute nodes. uci adds storage nodes and compute nodes."
  default     = "hci"

  validation {
    condition     = contains(["hci", "scale_out", "hybrid", "uci"], var.shape)
    error_message = "shape must be hci, scale_out, hybrid, or uci."
  }
}

variable "vergeos_version" {
  type        = string
  description = "VergeOS version recorded on each node and in the install seed, for example 26.0.2. The provider cannot download an ISO. Pass media_source_id of the ISO already uploaded for this version."

  validation {
    condition     = can(regex("^[0-9]+\\.[0-9]+\\.[0-9]+([.+-][0-9A-Za-z]+)*$", var.vergeos_version))
    error_message = "vergeos_version must look like 26.0.2."
  }
}

variable "media_source_id" {
  type        = number
  description = "Install ISO key from vergeio_mediasources. Attached as a CD-ROM and booted first. Omit it to create the nodes with no ISO. The lab test omits it."
  default     = null

  validation {
    condition     = var.media_source_id == null || (var.media_source_id >= 1 && var.media_source_id == floor(var.media_source_id))
    error_message = "media_source_id must be a media key of at least 1."
  }
}

variable "external_network_id" {
  type        = number
  description = "Network key for each node's external NIC. This module does not look up or change that network, and the NIC does not request an address on it."

  validation {
    condition     = var.external_network_id >= 1 && var.external_network_id == floor(var.external_network_id)
    error_message = "external_network_id must be a network key of at least 1."
  }
}

variable "external_ip" {
  type        = string
  description = "Bare IP the controller seed writes as the nested system's external address. No prefix length."

  validation {
    condition     = !strcontains(var.external_ip, "/") && can(cidrhost("${var.external_ip}/32", 0))
    error_message = "external_ip must be a bare IP address, with no prefix length."
  }
}

variable "external_prefix" {
  type        = number
  description = "Prefix length for external_ip, written into the seed as external_ip/external_prefix."

  validation {
    condition     = var.external_prefix >= 1 && var.external_prefix <= 32 && var.external_prefix == floor(var.external_prefix)
    error_message = "external_prefix must be a whole number from 1 to 32."
  }
}

variable "external_gateway" {
  type        = string
  description = "Bare gateway address written into the controller seed."

  validation {
    condition     = !strcontains(var.external_gateway, "/") && can(cidrhost("${var.external_gateway}/32", 0))
    error_message = "external_gateway must be a bare IP address, with no prefix length."
  }
}

variable "external_dns" {
  type        = string
  description = "DNS servers written into the controller seed. One bare IP, or several separated by commas."

  validation {
    condition = length(var.external_dns) > 0 && alltrue([
      for ip in split(",", var.external_dns) : length(ip) > 0 && !strcontains(ip, "/") && can(cidrhost("${ip}/32", 0))
    ])
    error_message = "external_dns must be bare IP addresses separated by commas, with no prefix length."
  }
}

variable "core_fabric_1_cidr" {
  type        = string
  description = "CIDR of the first core fabric network. Must be a /24. The router address is host 254. Node N uses host N. The default matches the install seed."
  default     = "172.16.1.0/24"

  validation {
    condition     = endswith(var.core_fabric_1_cidr, "/24") && can(cidrhost(var.core_fabric_1_cidr, 254)) && try(cidrhost(var.core_fabric_1_cidr, 0) == split("/", var.core_fabric_1_cidr)[0], false)
    error_message = "core_fabric_1_cidr must be a /24 network address, for example 172.16.1.0/24."
  }
}

variable "core_fabric_2_cidr" {
  type        = string
  description = "CIDR of the second core fabric network. Must be a /24, and must differ from core_fabric_1_cidr. Node N uses host N."
  default     = "172.16.2.0/24"

  validation {
    condition     = endswith(var.core_fabric_2_cidr, "/24") && can(cidrhost(var.core_fabric_2_cidr, 254)) && try(cidrhost(var.core_fabric_2_cidr, 0) == split("/", var.core_fabric_2_cidr)[0], false)
    error_message = "core_fabric_2_cidr must be a /24 network address, for example 172.16.2.0/24."
  }
}

variable "admin_username" {
  type        = string
  description = "Admin user the install seed creates on the nested system."
  default     = "admin"

  validation {
    condition     = can(regex("^[A-Za-z][A-Za-z0-9_-]{0,31}$", var.admin_username))
    error_message = "admin_username must start with a letter and contain only letters, digits, underscores, and hyphens."
  }
}

variable "admin_password" {
  type        = string
  description = "Admin password written into the install seed. It is stored in state, because cloud-init contents_wo cannot be planned. It needs an uppercase letter, a lowercase letter, a digit, and 8 characters."
  sensitive   = true

  validation {
    condition = (
      length(var.admin_password) >= 8 &&
      can(regex("[A-Z]", var.admin_password)) &&
      can(regex("[a-z]", var.admin_password)) &&
      can(regex("[0-9]", var.admin_password)) &&
      can(regex("^[A-Za-z0-9!@#%^*_-]+$", var.admin_password))
    )
    error_message = "admin_password must be at least 8 characters and include an uppercase letter, a lowercase letter, and a digit. Use letters, digits, and !@#%^*_-."
  }
}

variable "admin_email" {
  type        = string
  description = "Admin email written into the controller seed."

  validation {
    condition     = can(regex("^[^@'\\s]+@[^@'\\s]+$", var.admin_email))
    error_message = "admin_email must be an email address without spaces or quotes."
  }
}

variable "timezone" {
  type        = string
  description = "Timezone written into the controller seed."
  default     = "America/Detroit"

  validation {
    condition     = can(regex("^[A-Za-z0-9_+/-]+$", var.timezone))
    error_message = "timezone must contain only letters, digits, underscores, plus signs, hyphens, and slashes."
  }
}

variable "cpu_cores" {
  type        = number
  description = "CPU cores for each controller and scale-out node."
  default     = 8

  validation {
    condition     = var.cpu_cores >= 1 && var.cpu_cores == floor(var.cpu_cores)
    error_message = "cpu_cores must be a whole number of at least 1."
  }
}

variable "ram" {
  type        = number
  description = "RAM in megabytes for each controller and scale-out node."
  default     = 16384

  validation {
    condition     = var.ram >= 1 && var.ram == floor(var.ram)
    error_message = "ram must be a whole number of at least 1."
  }
}

variable "storage_cpu_cores" {
  type        = number
  description = "CPU cores for each storage node."
  default     = 8

  validation {
    condition     = var.storage_cpu_cores >= 1 && var.storage_cpu_cores == floor(var.storage_cpu_cores)
    error_message = "storage_cpu_cores must be a whole number of at least 1."
  }
}

variable "storage_ram" {
  type        = number
  description = "RAM in megabytes for each storage node."
  default     = 16384

  validation {
    condition     = var.storage_ram >= 1 && var.storage_ram == floor(var.storage_ram)
    error_message = "storage_ram must be a whole number of at least 1."
  }
}

variable "compute_cpu_cores" {
  type        = number
  description = "CPU cores for each compute node."
  default     = 8

  validation {
    condition     = var.compute_cpu_cores >= 1 && var.compute_cpu_cores == floor(var.compute_cpu_cores)
    error_message = "compute_cpu_cores must be a whole number of at least 1."
  }
}

variable "compute_ram" {
  type        = number
  description = "RAM in megabytes for each compute node."
  default     = 16384

  validation {
    condition     = var.compute_ram >= 1 && var.compute_ram == floor(var.compute_ram)
    error_message = "compute_ram must be a whole number of at least 1."
  }
}

variable "tier0_disk_count" {
  type        = number
  description = "Metadata disks on each controller, scale-out node, and storage node. The install seed maps them to vSAN tier 0."
  default     = 2

  validation {
    condition     = var.tier0_disk_count >= 1 && var.tier0_disk_count <= 8 && var.tier0_disk_count == floor(var.tier0_disk_count)
    error_message = "tier0_disk_count must be a whole number from 1 to 8."
  }
}

variable "tier0_disk_size_gb" {
  type        = number
  description = "Size in GB of each tier 0 disk."
  default     = 100

  validation {
    condition     = var.tier0_disk_size_gb > 0
    error_message = "tier0_disk_size_gb must be greater than 0."
  }
}

variable "tier1_disk_count" {
  type        = number
  description = "Workload disks on each controller and scale-out node, except uci controllers, which have none. The install seed maps them to vSAN tier 1."
  default     = 2

  validation {
    condition     = var.tier1_disk_count >= 1 && var.tier1_disk_count <= 8 && var.tier1_disk_count == floor(var.tier1_disk_count)
    error_message = "tier1_disk_count must be a whole number from 1 to 8."
  }
}

variable "tier1_disk_size_gb" {
  type        = number
  description = "Size in GB of each tier 1 disk on controllers and scale-out nodes."
  default     = 1000

  validation {
    condition     = var.tier1_disk_size_gb > 0
    error_message = "tier1_disk_size_gb must be greater than 0."
  }
}

variable "storage_tier1_disk_count" {
  type        = number
  description = "Workload disks on each storage node. Used when shape is uci."
  default     = 2

  validation {
    condition     = var.shape != "uci" || (var.storage_tier1_disk_count >= 1 && var.storage_tier1_disk_count <= 16 && var.storage_tier1_disk_count == floor(var.storage_tier1_disk_count))
    error_message = "storage_tier1_disk_count must be a whole number from 1 to 16 when shape is uci."
  }
}

variable "storage_tier1_disk_size_gb" {
  type        = number
  description = "Size in GB of each storage node tier 1 disk."
  default     = 1000

  validation {
    condition     = var.storage_tier1_disk_size_gb > 0
    error_message = "storage_tier1_disk_size_gb must be greater than 0."
  }
}

variable "compute_disk_size_gb" {
  type        = number
  description = "Size in GB of the single disk on each compute node."
  default     = 100

  validation {
    condition     = var.compute_disk_size_gb > 0
    error_message = "compute_disk_size_gb must be greater than 0."
  }
}

variable "scale_out_count" {
  type        = number
  description = "Scale-out nodes created when shape is scale_out. The count must be known at plan time."
  default     = 2

  validation {
    condition     = var.shape != "scale_out" || (var.scale_out_count >= 1 && var.scale_out_count <= 10 && var.scale_out_count == floor(var.scale_out_count))
    error_message = "scale_out_count must be a whole number from 1 to 10 when shape is scale_out."
  }
}

variable "storage_count" {
  type        = number
  description = "Storage nodes created when shape is uci. The count must be known at plan time."
  default     = 2

  validation {
    condition     = var.shape != "uci" || (var.storage_count >= 1 && var.storage_count <= 10 && var.storage_count == floor(var.storage_count))
    error_message = "storage_count must be a whole number from 1 to 10 when shape is uci."
  }
}

variable "compute_count" {
  type        = number
  description = "Compute nodes created when shape is hybrid or uci. The count must be known at plan time."
  default     = 2

  validation {
    condition     = !contains(["hybrid", "uci"], var.shape) || (var.compute_count >= 1 && var.compute_count <= 10 && var.compute_count == floor(var.compute_count))
    error_message = "compute_count must be a whole number from 1 to 10 when shape is hybrid or uci."
  }
}

variable "powerstate" {
  type        = bool
  description = "Power the node VMs on. Leave this false. Powering on from this resource detaches cloud-init about a second later, before the installer can read the seed. Power the nodes on from the UI after apply, controller 1 first."
  default     = false
}

variable "network_powerstate" {
  type        = bool
  description = "Power the core fabric networks on. Leave this false until the nodes are ready to boot."
  default     = false
}

variable "wait_for_guest_ip_timeout" {
  type        = number
  description = "Seconds vergeio_vm waits for a guest-agent address. Only valid when powerstate is true. The wait is not an install-complete check, and it continues when no address appears. Leave it unset."
  default     = null

  validation {
    condition     = var.wait_for_guest_ip_timeout == null || (var.wait_for_guest_ip_timeout >= 1 && var.wait_for_guest_ip_timeout == floor(var.wait_for_guest_ip_timeout))
    error_message = "wait_for_guest_ip_timeout must be a whole number of at least 1, or null."
  }
}

variable "cluster_id" {
  type        = number
  description = "Host cluster key for the node VMs. Omit to leave the system default. This module does not read vergeio_clusters."
  default     = null

  validation {
    condition     = var.cluster_id == null || (var.cluster_id >= 1 && var.cluster_id == floor(var.cluster_id))
    error_message = "cluster_id must be a cluster key of at least 1."
  }
}
