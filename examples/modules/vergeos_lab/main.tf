terraform {
  required_version = ">= 1.11.0"

  required_providers {
    vergeio = {
      source  = "verge-io/vergeio"
      version = "~> 3.0"
    }
  }
}

provider "vergeio" {}

variable "vergeos_version" {
  type        = string
  description = "VergeOS version to record on the nested cluster, for example 26.0.2."
  default     = "26.0.2"
}

variable "media_name" {
  type        = string
  description = "Name of the VergeOS install ISO already uploaded for vergeos_version."
  default     = "verge-io-install-26.0.2.iso"
}

variable "external_network_name" {
  type        = string
  description = "Name of the external network the node NICs attach to."
  default     = "External"
}

variable "admin_password" {
  type        = string
  description = "Admin password for the nested system. It needs an uppercase letter, a lowercase letter, and a digit."
  sensitive   = true
}

variable "admin_email" {
  type        = string
  description = "Admin email for the nested system."
  default     = "admin@example.com"
}

variable "external_ip" {
  type        = string
  description = "Bare IP for the nested system's UI."
  default     = "198.51.100.10"
}

variable "external_prefix" {
  type        = number
  description = "Prefix length for external_ip."
  default     = 24
}

variable "external_gateway" {
  type        = string
  description = "Bare gateway address for the nested system."
  default     = "198.51.100.1"
}

variable "external_dns" {
  type        = string
  description = "Bare DNS address for the nested system."
  default     = "198.51.100.1"
}

variable "shape" {
  type        = string
  description = "hci, scale_out, hybrid, or uci."
  default     = "hci"
}

data "vergeio_mediasources" "iso" {
  filter_name = var.media_name
}

data "vergeio_networks" "external" {
  filter_name = var.external_network_name
  filter_type = "external"
}

module "lab" {
  source = "../../../modules/vergeos_lab"

  name                = "training-1"
  shape               = var.shape
  vergeos_version     = var.vergeos_version
  media_source_id     = data.vergeio_mediasources.iso.mediasources[0].id
  admin_password      = var.admin_password
  admin_email         = var.admin_email
  external_network_id = data.vergeio_networks.external.networks[0].id
  external_ip         = var.external_ip
  external_prefix     = var.external_prefix
  external_gateway    = var.external_gateway
  external_dns        = var.external_dns
}

output "vm_ids" {
  description = "Node VM keys."
  value       = module.lab.vm_ids
}

output "ui_address" {
  description = "External address written into the controller seed."
  value       = module.lab.ui_address
}

output "vergeos_version" {
  description = "VergeOS version recorded on the nodes."
  value       = module.lab.vergeos_version
}
