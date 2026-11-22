output "name" {
  description = "Cluster name."
  value       = var.name
}

output "network_id" {
  description = "Cluster network key. The cloud controller loadBalancer.networkID is this key as a number."
  value       = vergeio_network.cluster.id
}

output "network_name" {
  description = "Cluster network name. The Rancher node driver network field is this name."
  value       = vergeio_network.cluster.name
}

output "endpoint" {
  description = "Kubernetes API address of the first control plane, https://<host 10>:6443. The kubernetes and helm providers use this as host."
  value       = local.endpoint
}

output "cluster_ca_certificate" {
  description = "PEM CA certificate for the kubernetes and helm provider cluster_ca_certificate argument. Image control planes also have this file at /etc/kubernetes/vergeio/ca.crt."
  value       = tls_self_signed_cert.ca.cert_pem
}

output "client_certificate" {
  description = "PEM client certificate for the kubernetes and helm provider client_certificate argument. The subject is kubernetes-admin in system:masters."
  value       = tls_locally_signed_cert.admin.cert_pem
}

output "client_key" {
  description = "PEM client key for the kubernetes and helm provider client_key argument."
  value       = tls_private_key.admin.private_key_pem
  sensitive   = true
}

output "kubeconfig" {
  description = "kubeconfig for the first control plane endpoint, using cluster_ca_certificate and the admin client certificate."
  value       = local.kubeconfig
  sensitive   = true
}

output "control_plane_ids" {
  description = "Control plane VM keys, keyed by cp-N. Recipe nodes use vergeio_vm_recipe_instance.vm_id."
  value = var.source_type == "image" ? {
    for key, vm in vergeio_vm.node : key => vm.id if local.nodes[key].role == "control-plane"
    } : {
    for key, vm in vergeio_vm_recipe_instance.node : key => tostring(vm.vm_id) if local.nodes[key].role == "control-plane"
  }
}

output "worker_ids" {
  description = "Worker VM keys, keyed by worker-N."
  value = var.source_type == "image" ? {
    for key, vm in vergeio_vm.node : key => vm.id if local.nodes[key].role == "worker"
    } : {
    for key, vm in vergeio_vm_recipe_instance.node : key => tostring(vm.vm_id) if local.nodes[key].role == "worker"
  }
}

output "node_ips" {
  description = "Static node addresses, keyed by cp-N and worker-N. Image NICs reserve these addresses. Recipe answers have to place the guests on them."
  value       = { for key, node in local.nodes : key => node.ip }
}

output "provider_ids" {
  description = "Cloud controller provider IDs, vergeos://<vm key>, keyed by cp-N and worker-N. The controller also matches a node to a VM by name."
  value = var.source_type == "image" ? {
    for key, vm in vergeio_vm.node : key => "vergeos://${vm.id}"
    } : {
    for key, vm in vergeio_vm_recipe_instance.node : key => "vergeos://${vm.vm_id}"
  }
}

output "pool_vm_id" {
  description = "Stopped VM key that holds idle CSI block drives. The vergeos-csi chart reads it as block.poolVmId."
  value       = vergeio_vm.pool.id
}

output "template_vm_name" {
  description = "Template VM name for the Rancher node driver templateVm field. Null when create_template is false."
  value       = var.create_template ? "${var.name}-template" : null
}

output "template_vm_id" {
  description = "Template VM key. Null when create_template is false."
  value = var.source_type == "image" ? one(vergeio_vm.template[*].id) : one([
    for vm in vergeio_vm_recipe_instance.template : tostring(vm.vm_id)
  ])
}

output "load_balancer_ips" {
  description = "Addresses for the cloud controller loadBalancer.ipPool. They are not DHCP addresses and they are not assigned to nodes."
  value       = local.load_balancer_ips
}

output "user_id" {
  description = "API user key. The user type is api."
  value       = vergeio_user.api.id
}

output "api_key_id" {
  description = "Long-lived API key id for the CSI driver and cloud controller."
  value       = vergeio_api_key.csi.id
}

output "vergeos_host" {
  description = "VergeOS API URL for the chart value vergeos.host, including the https:// scheme."
  value       = local.vergeos_host
}

output "vergeos_hostname" {
  description = "VergeOS hostname or address for the node driver chart value vergeosHosts. A port in vergeos_host is removed."
  value       = local.vergeos_hostname
}

output "vergeos_api_key" {
  description = "Bearer token for the chart value vergeos.apiKey. VergeOS returns it once, and vergeio_api_key stores it because store_token is true."
  value       = vergeio_api_key.csi.token
  sensitive   = true
}

output "vergeos_verify_ssl" {
  description = "String true or false for the chart value vergeos.verifySSL."
  value       = local.vergeos_verify_ssl
}

output "vergeos_insecure" {
  description = "String true or false for the Rancher cloud credential insecure field. True skips TLS verification."
  value       = local.vergeos_insecure
}
