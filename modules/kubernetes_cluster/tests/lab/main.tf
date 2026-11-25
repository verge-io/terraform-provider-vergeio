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

module "cluster" {
  source = "../.."

  name               = "tf-acc-k8s"
  source_type        = "image"
  boot_disk_size     = 8
  cidr               = "192.168.100.0/24"
  vergeos_host       = "https://tf-acc-k8s.example"
  powerstate         = false
  network_powerstate = false

  control_plane_count     = 1
  worker_count            = 1
  control_plane_cpu_cores = 1
  control_plane_ram       = 1024
  worker_cpu_cores        = 1
  worker_ram              = 1024
}

output "network_id" {
  value = module.cluster.network_id
}

output "endpoint" {
  value = module.cluster.endpoint
}

output "control_plane_ids" {
  value = module.cluster.control_plane_ids
}

output "worker_ids" {
  value = module.cluster.worker_ids
}

output "pool_vm_id" {
  value = module.cluster.pool_vm_id
}

output "node_ips" {
  value = module.cluster.node_ips
}

output "vergeos_api_key" {
  value     = module.cluster.vergeos_api_key
  sensitive = true
}
