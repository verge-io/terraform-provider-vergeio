terraform {
  required_version = ">= 1.11.0"

  required_providers {
    helm = {
      source  = "hashicorp/helm"
      version = "~> 3.0"
    }
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "~> 2.36"
    }
    vergeio = {
      source  = "verge-io/vergeio"
      version = "~> 3.0"
    }
  }
}

provider "vergeio" {}

provider "kubernetes" {
  host                   = module.cluster.endpoint
  cluster_ca_certificate = module.cluster.cluster_ca_certificate
  client_certificate     = module.cluster.client_certificate
  client_key             = module.cluster.client_key
}

provider "helm" {
  kubernetes = {
    host                   = module.cluster.endpoint
    cluster_ca_certificate = module.cluster.cluster_ca_certificate
    client_certificate     = module.cluster.client_certificate
    client_key             = module.cluster.client_key
  }
}

variable "name" {
  type        = string
  description = "Cluster name."
}

variable "cidr" {
  type        = string
  description = "Cluster network CIDR, such as 192.168.100.0/24."
}

variable "vergeos_host" {
  type        = string
  description = "VergeOS API URL, such as https://vergeos.example.com."
}

variable "media_source_id" {
  type        = number
  description = "Golden image key. vergeio_mediasources.mediasources[].id."
}

variable "external_network_id" {
  type        = number
  description = "Uplink network key. Omit for a network with no gateway."
  default     = null
}

module "cluster" {
  source = "../../../../modules/kubernetes_cluster"

  name                = var.name
  source_type         = "image"
  media_source_id     = var.media_source_id
  boot_disk_size      = 40
  cidr                = var.cidr
  vergeos_host        = var.vergeos_host
  external_network_id = var.external_network_id
  powerstate          = true

  control_plane_count = 1
  worker_count        = 1
}

resource "helm_release" "csi" {
  name       = "vergeos-csi"
  repository = "https://verge-io.github.io/helm-charts"
  chart      = "vergeos-csi"
  version    = "0.2.0"
  namespace  = "kube-system"

  values = [
    yamlencode({
      vergeos = {
        host      = module.cluster.vergeos_host
        verifySSL = module.cluster.vergeos_verify_ssl
      }
      block = {
        poolVmId = module.cluster.pool_vm_id
      }
    }),
  ]

  set_sensitive = [
    {
      name  = "vergeos.apiKey"
      value = module.cluster.vergeos_api_key
    },
  ]
}

resource "helm_release" "cloud_controller" {
  name       = "vergeos-cloud-controller-manager"
  repository = "https://verge-io.github.io/helm-charts"
  chart      = "vergeos-cloud-controller-manager"
  version    = "0.2.0"
  namespace  = "kube-system"

  values = [
    yamlencode({
      vergeos = {
        host      = module.cluster.vergeos_host
        verifySSL = module.cluster.vergeos_verify_ssl
      }
      loadBalancer = {
        enabled   = true
        networkID = tonumber(module.cluster.network_id)
        ipPool    = module.cluster.load_balancer_ips
      }
    }),
  ]

  set_sensitive = [
    {
      name  = "vergeos.apiKey"
      value = module.cluster.vergeos_api_key
    },
  ]
}

output "endpoint" {
  description = "Kubernetes API address."
  value       = module.cluster.endpoint
}

output "kubeconfig" {
  description = "Admin kubeconfig for the endpoint."
  value       = module.cluster.kubeconfig
  sensitive   = true
}
