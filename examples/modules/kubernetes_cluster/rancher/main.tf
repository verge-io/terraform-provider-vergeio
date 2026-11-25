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
  alias       = "management"
  config_path = var.rancher_kubeconfig_path
}

provider "helm" {
  alias = "management"
  kubernetes = {
    config_path = var.rancher_kubeconfig_path
  }
}

provider "kubernetes" {
  alias       = "downstream"
  config_path = var.downstream_kubeconfig_path
}

provider "helm" {
  alias = "downstream"
  kubernetes = {
    config_path = var.downstream_kubeconfig_path
  }
}

variable "name" {
  type        = string
  description = "Cluster name. The template VM is name-template and the network uses this name."
}

variable "cidr" {
  type        = string
  description = "Cluster network CIDR. DHCP is enabled for nodes the Rancher driver clones."
}

variable "vergeos_host" {
  type        = string
  description = "VergeOS API URL, such as https://vergeos.example.com."
}

variable "media_source_id" {
  type        = number
  description = "Ubuntu 24.04 image key used for the template VM. The image needs cloud-init. The guest agent is enabled on the template."
}

variable "rancher_kubeconfig_path" {
  type        = string
  description = "Kubeconfig path for the Rancher management cluster. The node driver chart installs into cattle-system."
}

variable "downstream_kubeconfig_path" {
  type        = string
  description = "Kubeconfig path for the Rancher downstream cluster. Rancher generates that cluster's CA. This module's kubeconfig is the certificate written to image control planes, not the Rancher CA."
}

module "cluster" {
  source = "../../../../modules/kubernetes_cluster"

  name            = var.name
  source_type     = "image"
  media_source_id = var.media_source_id
  boot_disk_size  = 40
  cidr            = var.cidr
  vergeos_host    = var.vergeos_host
  powerstate      = false
  create_template = true

  control_plane_count = 1
  worker_count        = 1
  control_plane_ram   = 4096
  worker_ram          = 4096
}

resource "helm_release" "node_driver" {
  provider   = helm.management
  name       = "vergeos-node-driver"
  repository = "https://verge-io.github.io/helm-charts"
  chart      = "vergeos-node-driver"
  version    = "0.2.0"
  namespace  = "cattle-system"

  set_list = [
    {
      name  = "vergeosHosts"
      value = [module.cluster.vergeos_hostname]
    },
  ]
}

resource "helm_release" "csi" {
  provider   = helm.downstream
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
  provider   = helm.downstream
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

output "rancher_cloud_credential" {
  description = "Fields the VergeOS Rancher UI stores on a cloud credential. insecure is the string true or false."
  value = {
    host     = module.cluster.vergeos_host
    apiKey   = module.cluster.vergeos_api_key
    insecure = module.cluster.vergeos_insecure
  }
  sensitive = true
}

output "rancher_machine_config" {
  description = "Fields the VergeOS Rancher machine form stores. cpuCores, ram, diskSize, sshUser, and sshPort are the UI defaults. ram 4096 is the UI default and the documented minimum for an RKE2 node."
  value = {
    templateVm = module.cluster.template_vm_name
    network    = module.cluster.network_name
    cpuCores   = "2"
    ram        = "4096"
    diskSize   = "0"
    sshUser    = "ubuntu"
    sshPort    = "22"
  }
}
