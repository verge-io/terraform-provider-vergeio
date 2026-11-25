mock_provider "vergeio" {}

variables {
  name            = "edge"
  source_type     = "image"
  media_source_id = 25
  boot_disk_size  = 40
  cidr            = "192.168.100.0/24"
  vergeos_host    = "vergeos.example.com"
}

run "image" {
  command = plan

  assert {
    condition     = vergeio_network.cluster.network == "192.168.100.0/24" && vergeio_network.cluster.ipaddress == "192.168.100.1" && vergeio_network.cluster.type == "internal" && vergeio_network.cluster.dhcp_enabled == true && vergeio_network.cluster.dhcp_start == "192.168.100.150" && vergeio_network.cluster.dhcp_stop == "192.168.100.189" && vergeio_network.cluster.powerstate == true && vergeio_network.cluster.vnet_default_gateway == null
    error_message = "The cluster network was not planned with DHCP hosts 150-189 and no uplink."
  }

  assert {
    condition     = vergeio_vm.node["cp-1"].name == "edge-cp-1" && vergeio_vm.node["cp-1"].cpu_cores == 2 && vergeio_vm.node["cp-1"].ram == 2048 && vergeio_vm.node["cp-1"].guest_agent == true && vergeio_vm.node["cp-1"].powerstate == false && vergeio_vm.node["worker-1"].name == "edge-worker-1" && length(vergeio_vm.node) == 2 && length(vergeio_vm_recipe_instance.node) == 0
    error_message = "The control plane and worker were not planned from the image."
  }

  assert {
    condition     = vergeio_vm_nic.node["cp-1"].ipaddress == "192.168.100.10" && vergeio_vm_nic.node["worker-1"].ipaddress == "192.168.100.40" && vergeio_vm_nic.node["cp-1"].interface == "virtio" && vergeio_vm.node["cp-1"].boot_disk.media == "import" && vergeio_vm.node["cp-1"].boot_disk.source == 25 && vergeio_vm.node["cp-1"].cloudinit_datasource == "nocloud"
    error_message = "Node NICs were not reserved at hosts 10 and 40, or the boot disk was not an image import."
  }

  assert {
    condition     = vergeio_vm.pool.name == "edge-pool" && vergeio_vm.pool.powerstate == false && vergeio_vm.pool.shutdown_on_destroy == "kill" && length(vergeio_vm.template) == 0
    error_message = "The block pool VM was not planned, or a template was planned."
  }

  assert {
    condition     = vergeio_user.api.name == "edge-csi" && vergeio_user.api.type == "api" && vergeio_api_key.csi.store_token == true && vergeio_api_key.csi.name == "edge-csi" && length(vergeio_permission.api) == 12 && vergeio_permission.api["vms"].table == "vms" && vergeio_permission.api["vm_services"].table == "vm_services" && vergeio_permission.api["machine_status"].table == "machine_status"
    error_message = "The API user, stored token, or table grants were not planned."
  }

  assert {
    condition     = vergeio_network_rule.cluster.source_ip == "192.168.100.0/24" && vergeio_network_rule.cluster.orderid == 1 && vergeio_network_rule.cluster.apply == false && vergeio_network_rule.api.orderid == 2 && vergeio_network_rule.api.apply == false && vergeio_network_rule.api.destination_ports == "6443" && vergeio_network_rule.rke2.orderid == 3 && vergeio_network_rule.rke2.apply == false && vergeio_network_rule.rke2.destination_ports == "9345" && vergeio_network_rule.https.orderid == 4 && vergeio_network_rule.https.apply == true && vergeio_network_rule.https.direction == "outgoing" && vergeio_network_rule.https.destination_ports == "443" && length(vergeio_network_rule.ssh) == 0
    error_message = "Firewall rules were not planned as a 1 through 4 chain, with outgoing TCP 443 applying the network."
  }

  assert {
    condition     = output.endpoint == "https://192.168.100.10:6443" && output.vergeos_host == "https://vergeos.example.com" && output.vergeos_hostname == "vergeos.example.com" && output.vergeos_verify_ssl == "false" && output.vergeos_insecure == "true" && output.load_balancer_ips[0] == "192.168.100.200" && output.load_balancer_ips[1] == "192.168.100.201"
    error_message = "Helm and kubernetes outputs were not planned from the first control plane and the published chart defaults."
  }

  assert {
    condition = (
      contains(tls_cert_request.server.dns_names, "edge-cp-1") &&
      contains(tls_cert_request.server.dns_names, "kubernetes") &&
      contains(tls_cert_request.server.dns_names, "kubernetes.default") &&
      contains(tls_cert_request.server.dns_names, "kubernetes.default.svc") &&
      contains(tls_cert_request.server.dns_names, "kubernetes.default.svc.cluster.local") &&
      contains(tls_cert_request.server.dns_names, "localhost") &&
      contains(tls_cert_request.server.ip_addresses, "192.168.100.10") &&
      contains(tls_cert_request.server.ip_addresses, "192.168.100.200") &&
      contains(tls_cert_request.server.ip_addresses, "192.168.100.201") &&
      contains(tls_cert_request.server.ip_addresses, "10.96.0.1") &&
      contains(tls_cert_request.server.ip_addresses, "127.0.0.1")
    )
    error_message = "The API server certificate does not name the control plane, the load balancer addresses, the in-cluster API names, or localhost."
  }
}

run "certificates" {
  command = apply

  override_resource {
    target = vergeio_network.cluster
    values = {
      id = "6"
    }
  }

  override_resource {
    target = vergeio_user.api
    values = {
      id = "9"
    }
  }

  override_resource {
    target = vergeio_api_key.csi
    values = {
      id    = "5"
      token = "stored-token"
    }
  }

  assert {
    condition     = strcontains(output.kubeconfig, "https://192.168.100.10:6443") && strcontains(output.kubeconfig, "kubernetes-admin") && strcontains(output.cluster_ca_certificate, "BEGIN CERTIFICATE") && output.client_key != "" && output.vergeos_api_key == "stored-token" && vergeio_vm_nic.node["cp-1"].vnet == 6 && vergeio_api_key.csi.user_id == 9
    error_message = "The admin kubeconfig was not built from the generated CA, or the NIC and API key did not use the numeric keys."
  }
}

run "ssh_on" {
  command = apply

  variables {
    ssh_source_cidr = "192.0.2.0/24"
  }

  assert {
    condition     = vergeio_network_rule.cluster.orderid == 1 && vergeio_network_rule.api.orderid == 2 && vergeio_network_rule.rke2.orderid == 3 && vergeio_network_rule.https.orderid == 4 && vergeio_network_rule.https.apply == false && length(vergeio_network_rule.ssh) == 1 && vergeio_network_rule.ssh[0].orderid == 5 && vergeio_network_rule.ssh[0].apply == true
    error_message = "Turning SSH on moved an earlier rule or did not append SSH at orderid 5."
  }
}

run "ssh_off" {
  command = apply

  variables {
    ssh_source_cidr = null
  }

  assert {
    condition     = vergeio_network_rule.cluster.orderid == 1 && vergeio_network_rule.api.orderid == 2 && vergeio_network_rule.rke2.orderid == 3 && vergeio_network_rule.https.orderid == 4 && vergeio_network_rule.https.apply == true && length(vergeio_network_rule.ssh) == 0
    error_message = "Turning SSH off moved an earlier rule or left the SSH rule in place."
  }
}

run "uplink_ssh_and_template" {
  command = plan

  variables {
    external_network_id     = 3
    api_source_cidr         = "192.0.2.0/24"
    ssh_source_cidr         = "192.0.2.0/24"
    create_template         = true
    vergeos_verify_ssl      = true
    vergeos_host            = "https://10.8.8.26:443/api"
    control_plane_count     = 1
    worker_count            = 0
    network_powerstate      = true
    control_plane_cpu_cores = 4
    control_plane_ram       = 4096
  }

  assert {
    condition     = vergeio_network.cluster.vnet_default_gateway == 3 && vergeio_network_rule.api.source_ip == "192.0.2.0/24" && vergeio_network_rule.cluster.orderid == 1 && vergeio_network_rule.api.orderid == 2 && vergeio_network_rule.rke2.orderid == 3 && vergeio_network_rule.https.orderid == 4 && vergeio_network_rule.https.apply == false && vergeio_network_rule.ssh[0].orderid == 5 && vergeio_network_rule.ssh[0].destination_ports == "22" && vergeio_network_rule.ssh[0].apply == true && length(vergeio_vm.node) == 1 && length(vergeio_vm.template) == 1 && vergeio_vm.template[0].name == "edge-template" && vergeio_vm.template[0].guest_agent == true && vergeio_vm.template[0].ram == 4096 && vergeio_vm_nic.template[0].ipaddress == "192.168.100.2" && output.template_vm_name == "edge-template" && output.vergeos_host == "https://10.8.8.26:443" && output.vergeos_hostname == "10.8.8.26" && output.vergeos_verify_ssl == "true" && output.vergeos_insecure == "false"
    error_message = "The uplink, SSH rule, template, or TLS verification flag was not planned."
  }
}

run "recipe" {
  command = plan

  variables {
    source_type     = "recipe"
    media_source_id = null
    boot_disk_size  = null
    recipe_id       = "0123456789abcdef0123456789abcdef01234567"
    create_template = true
    worker_count    = 1
    recipe_answers = {
      HOSTNAME = "edge"
    }
  }

  assert {
    condition     = length(vergeio_vm.node) == 0 && length(vergeio_vm_nic.node) == 0 && vergeio_vm_recipe_instance.node["cp-1"].recipe_id == "0123456789abcdef0123456789abcdef01234567" && vergeio_vm_recipe_instance.node["worker-1"].name == "edge-worker-1" && length(vergeio_vm_recipe_instance.template) == 1 && vergeio_vm_recipe_instance.template[0].name == "edge-template" && vergeio_vm.pool.name == "edge-pool" && output.endpoint == "https://192.168.100.10:6443"
    error_message = "Recipe nodes were not planned, or image node VMs were planned."
  }
}
