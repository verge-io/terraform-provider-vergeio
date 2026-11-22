locals {
  indexes = [0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19]

  vergeos_raw = (
    startswith(var.vergeos_host, "https://") || startswith(var.vergeos_host, "http://")
  ) ? var.vergeos_host : "https://${var.vergeos_host}"
  vergeos_scheme    = startswith(local.vergeos_raw, "http://") ? "http" : "https"
  vergeos_authority = split("/", trimprefix(trimprefix(local.vergeos_raw, "https://"), "http://"))[0]
  vergeos_host      = "${local.vergeos_scheme}://${trimsuffix(local.vergeos_authority, "/")}"
  vergeos_hostname  = split(":", local.vergeos_authority)[0]

  router_ip          = cidrhost(var.cidr, 1)
  template_ip        = cidrhost(var.cidr, 2)
  endpoint_ip        = cidrhost(var.cidr, 10)
  endpoint           = "https://${local.endpoint_ip}:6443"
  dhcp_start         = cidrhost(var.cidr, 150)
  dhcp_stop          = cidrhost(var.cidr, 189)
  load_balancer_ips  = [for index in slice(local.indexes, 0, var.load_balancer_pool_size) : cidrhost(var.cidr, 200 + index)]
  vergeos_verify_ssl = var.vergeos_verify_ssl ? "true" : "false"
  vergeos_insecure   = var.vergeos_verify_ssl ? "false" : "true"

  control_planes = {
    for index in slice(local.indexes, 0, var.control_plane_count) : "cp-${index + 1}" => {
      name = "${var.name}-cp-${index + 1}"
      ip   = cidrhost(var.cidr, 10 + index)
      role = "control-plane"
    }
  }
  workers = {
    for index in slice(local.indexes, 0, var.worker_count) : "worker-${index + 1}" => {
      name = "${var.name}-worker-${index + 1}"
      ip   = cidrhost(var.cidr, 40 + index)
      role = "worker"
    }
  }
  nodes = merge(local.control_planes, local.workers)

  recipe_id_valid = var.recipe_id != null && can(regex("(?i)^[0-9a-f]{40}$", var.recipe_id))
  recipe_id       = local.recipe_id_valid ? var.recipe_id : "0000000000000000000000000000000000000000"

  permission_tables = toset([
    "vms",
    "machine_drives",
    "machine_nics",
    "cloudinit_files",
    "machines",
    "machine_status",
    "vnets",
    "vnet_rules",
    "vnet_addresses",
    "volumes",
    "volume_nfs_shares",
    "vm_services",
  ])

  firewall_rules = [
    for rule in [
      {
        key               = "cluster"
        include           = true
        name              = "${var.name}-allow-cluster"
        protocol          = "any"
        direction         = "incoming"
        destination_ports = ""
        source_ip         = var.cidr
      },
      {
        key               = "api"
        include           = true
        name              = "${var.name}-allow-api"
        protocol          = "tcp"
        direction         = "incoming"
        destination_ports = "6443"
        source_ip         = var.api_source_cidr == null ? "" : var.api_source_cidr
      },
      {
        key               = "rke2"
        include           = true
        name              = "${var.name}-allow-rke2"
        protocol          = "tcp"
        direction         = "incoming"
        destination_ports = "9345"
        source_ip         = var.api_source_cidr == null ? "" : var.api_source_cidr
      },
      {
        key               = "https"
        include           = true
        name              = "${var.name}-allow-https-out"
        protocol          = "tcp"
        direction         = "outgoing"
        destination_ports = "443"
        source_ip         = ""
      },
      {
        key               = "ssh"
        include           = var.ssh_source_cidr != null
        name              = "${var.name}-allow-ssh"
        protocol          = "tcp"
        direction         = "incoming"
        destination_ports = "22"
        source_ip         = var.ssh_source_cidr == null ? "" : var.ssh_source_cidr
      },
    ] : rule if rule.include
  ]
  firewall = {
    for index, rule in local.firewall_rules : rule.key => {
      name              = rule.name
      orderid           = index + 1
      protocol          = rule.protocol
      direction         = rule.direction
      destination_ports = rule.destination_ports
      source_ip         = rule.source_ip
    }
  }
  firewall_last = length(local.firewall_rules)

  service_cluster_ip = cidrhost(var.service_cidr, 1)
  server_dns_names = distinct(concat(
    [for node in local.control_planes : node.name],
    [
      "kubernetes",
      "kubernetes.default",
      "kubernetes.default.svc",
      "kubernetes.default.svc.${var.cluster_domain}",
      "localhost",
    ],
  ))
  server_ip_addresses = distinct(concat(
    [for node in local.control_planes : node.ip],
    local.load_balancer_ips,
    [local.service_cluster_ip, "127.0.0.1"],
  ))
}

resource "tls_private_key" "ca" {
  algorithm = "RSA"
  rsa_bits  = 2048
}

resource "tls_self_signed_cert" "ca" {
  private_key_pem = tls_private_key.ca.private_key_pem

  subject {
    common_name = "${var.name}-ca"
  }

  validity_period_hours = 87600
  is_ca_certificate     = true

  allowed_uses = [
    "cert_signing",
    "crl_signing",
    "digital_signature",
    "key_encipherment",
  ]
}

resource "tls_private_key" "server" {
  algorithm = "RSA"
  rsa_bits  = 2048
}

resource "tls_cert_request" "server" {
  private_key_pem = tls_private_key.server.private_key_pem

  subject {
    common_name = "kubernetes"
  }

  dns_names    = local.server_dns_names
  ip_addresses = local.server_ip_addresses
}

resource "tls_locally_signed_cert" "server" {
  cert_request_pem   = tls_cert_request.server.cert_request_pem
  ca_private_key_pem = tls_private_key.ca.private_key_pem
  ca_cert_pem        = tls_self_signed_cert.ca.cert_pem

  validity_period_hours = 87600

  allowed_uses = [
    "digital_signature",
    "key_encipherment",
    "server_auth",
  ]
}

resource "tls_private_key" "admin" {
  algorithm = "RSA"
  rsa_bits  = 2048
}

resource "tls_cert_request" "admin" {
  private_key_pem = tls_private_key.admin.private_key_pem

  subject {
    common_name  = "kubernetes-admin"
    organization = "system:masters"
  }
}

resource "tls_locally_signed_cert" "admin" {
  cert_request_pem   = tls_cert_request.admin.cert_request_pem
  ca_private_key_pem = tls_private_key.ca.private_key_pem
  ca_cert_pem        = tls_self_signed_cert.ca.cert_pem

  validity_period_hours = 87600

  allowed_uses = [
    "digital_signature",
    "key_encipherment",
    "client_auth",
  ]
}

locals {
  kubeconfig = templatefile("${path.module}/templates/kubeconfig.tftpl", {
    name       = var.name
    endpoint   = local.endpoint
    ca_b64     = base64encode(tls_self_signed_cert.ca.cert_pem)
    client_b64 = base64encode(tls_locally_signed_cert.admin.cert_pem)
    key_b64    = base64encode(tls_private_key.admin.private_key_pem)
  })

  ca_file = {
    path        = "/etc/kubernetes/vergeio/ca.crt"
    permissions = "0644"
    content     = tls_self_signed_cert.ca.cert_pem
  }
  control_plane_files = [
    local.ca_file,
    {
      path        = "/etc/kubernetes/vergeio/ca.key"
      permissions = "0600"
      content     = tls_private_key.ca.private_key_pem
    },
    {
      path        = "/etc/kubernetes/vergeio/server.crt"
      permissions = "0644"
      content     = tls_locally_signed_cert.server.cert_pem
    },
    {
      path        = "/etc/kubernetes/vergeio/server.key"
      permissions = "0600"
      content     = tls_private_key.server.private_key_pem
    },
    {
      path        = "/etc/kubernetes/vergeio/admin.crt"
      permissions = "0644"
      content     = tls_locally_signed_cert.admin.cert_pem
    },
    {
      path        = "/etc/kubernetes/vergeio/admin.key"
      permissions = "0600"
      content     = tls_private_key.admin.private_key_pem
    },
    {
      path        = "/etc/kubernetes/vergeio/kubeconfig"
      permissions = "0600"
      content     = local.kubeconfig
    },
  ]

  node_user_data = {
    for key, node in local.nodes : key => "#cloud-config\n${yamlencode({
      hostname    = node.name
      write_files = node.role == "control-plane" ? local.control_plane_files : [local.ca_file]
    })}"
  }
  template_user_data = "#cloud-config\n${yamlencode({
    hostname = "${var.name}-template"
  })}"
}

resource "vergeio_network" "cluster" {
  name                 = var.name
  description          = var.description
  enabled              = true
  type                 = "internal"
  network              = var.cidr
  ipaddress            = local.router_ip
  dhcp_enabled         = true
  dynamic_dhcp         = true
  dhcp_sequential      = true
  dhcp_start           = local.dhcp_start
  dhcp_stop            = local.dhcp_stop
  dnslist              = length(var.dns_servers) == 0 ? null : join(",", var.dns_servers)
  domain               = var.domain
  vnet_default_gateway = var.external_network_id
  powerstate           = var.network_powerstate
}

resource "vergeio_network_rule" "staged" {
  for_each = {
    for key, rule in local.firewall : key => rule if rule.orderid != local.firewall_last
  }

  vnet              = vergeio_network.cluster.id
  name              = each.value.name
  description       = each.value.name
  direction         = each.value.direction
  action            = "accept"
  protocol          = each.value.protocol
  source_ip         = each.value.source_ip != "" ? each.value.source_ip : null
  destination_ports = each.value.destination_ports != "" ? each.value.destination_ports : null
  enabled           = true
  orderid           = each.value.orderid
  apply             = false
}

resource "vergeio_network_rule" "apply" {
  for_each = {
    for key, rule in local.firewall : key => rule if rule.orderid == local.firewall_last
  }

  vnet              = vergeio_network.cluster.id
  name              = each.value.name
  description       = each.value.name
  direction         = each.value.direction
  action            = "accept"
  protocol          = each.value.protocol
  source_ip         = each.value.source_ip != "" ? each.value.source_ip : null
  destination_ports = each.value.destination_ports != "" ? each.value.destination_ports : null
  enabled           = true
  orderid           = each.value.orderid
  apply             = true

  depends_on = [vergeio_network_rule.staged]
}

resource "vergeio_vm" "node" {
  for_each = var.source_type == "image" ? local.nodes : {}

  name                 = each.value.name
  description          = each.value.name
  enabled              = true
  os_family            = "linux"
  cpu_cores            = each.value.role == "control-plane" ? var.control_plane_cpu_cores : var.worker_cpu_cores
  ram                  = each.value.role == "control-plane" ? var.control_plane_ram : var.worker_ram
  cluster              = var.cluster
  powerstate           = var.powerstate
  guest_agent          = true
  cloudinit_datasource = "nocloud"

  cloudinit_files = [
    {
      name                = "user-data"
      contents_wo         = local.node_user_data[each.key]
      contents_wo_version = parseint(substr(sha256(local.node_user_data[each.key]), 0, 7), 16)
    },
    {
      name                = "meta-data"
      contents_wo         = "instance-id: ${each.value.name}\nlocal-hostname: ${each.value.name}\n"
      contents_wo_version = 1
    },
  ]

  boot_disk {
    name   = each.value.name
    size   = var.boot_disk_size
    media  = var.media_source_id == null ? "disk" : "import"
    source = var.media_source_id
  }
}

resource "vergeio_vm_nic" "node" {
  for_each = vergeio_vm.node

  vm_id            = each.value.id
  name             = "lan"
  interface        = "virtio"
  vnet             = tonumber(vergeio_network.cluster.id)
  assign_ipaddress = true
  ipaddress        = local.nodes[each.key].ip
}

resource "vergeio_vm_recipe_instance" "node" {
  for_each = var.source_type == "recipe" ? local.nodes : {}

  name        = each.value.name
  recipe_id   = local.recipe_id
  answers     = var.recipe_answers
  auto_update = var.recipe_auto_update
}

resource "vergeio_vm" "pool" {
  name        = "${var.name}-pool"
  description = "${var.name}-pool"
  enabled     = true
  os_family   = "linux"
  cpu_cores   = 1
  ram         = 1024
  powerstate  = false

  shutdown_on_destroy = "kill"

  boot_disk {
    name  = "${var.name}-pool"
    size  = 8
    media = "disk"
  }
}

resource "vergeio_vm" "template" {
  count = var.create_template && var.source_type == "image" ? 1 : 0

  name                 = "${var.name}-template"
  description          = "${var.name}-template"
  enabled              = true
  os_family            = "linux"
  cpu_cores            = var.control_plane_cpu_cores
  ram                  = var.control_plane_ram
  cluster              = var.cluster
  powerstate           = false
  guest_agent          = true
  cloudinit_datasource = "nocloud"

  cloudinit_files = [
    {
      name                = "user-data"
      contents_wo         = local.template_user_data
      contents_wo_version = parseint(substr(sha256(local.template_user_data), 0, 7), 16)
    },
    {
      name                = "meta-data"
      contents_wo         = "instance-id: ${var.name}-template\nlocal-hostname: ${var.name}-template\n"
      contents_wo_version = 1
    },
  ]

  boot_disk {
    name   = "${var.name}-template"
    size   = var.boot_disk_size
    media  = var.media_source_id == null ? "disk" : "import"
    source = var.media_source_id
  }
}

resource "vergeio_vm_nic" "template" {
  count = length(vergeio_vm.template)

  vm_id            = vergeio_vm.template[0].id
  name             = "lan"
  interface        = "virtio"
  vnet             = tonumber(vergeio_network.cluster.id)
  assign_ipaddress = true
  ipaddress        = local.template_ip
}

resource "vergeio_vm_recipe_instance" "template" {
  count = var.create_template && var.source_type == "recipe" ? 1 : 0

  name        = "${var.name}-template"
  recipe_id   = local.recipe_id
  answers     = var.recipe_answers
  auto_update = var.recipe_auto_update
}

resource "random_password" "api_user" {
  length           = 24
  special          = true
  override_special = "!@#%*-_"
  min_lower        = 1
  min_upper        = 1
  min_numeric      = 1
  min_special      = 1
}

resource "vergeio_user" "api" {
  name                = "${var.name}-csi"
  displayname         = "${var.name}-csi"
  email               = "${var.name}@cluster.invalid"
  enabled             = true
  type                = "api"
  password_wo         = random_password.api_user.result
  password_wo_version = 1
  change_password     = false
}

resource "vergeio_api_key" "csi" {
  user_id     = tonumber(vergeio_user.api.id)
  name        = "${var.name}-csi"
  description = "${var.name} CSI driver and cloud controller"
  store_token = true
}

resource "vergeio_permission" "api" {
  for_each = local.permission_tables

  user_id = vergeio_user.api.id
  table   = each.value
  list    = true
  read    = true
  create  = true
  modify  = true
  delete  = true
}

resource "terraform_data" "check" {
  lifecycle {
    precondition {
      condition     = var.source_type != "image" || var.media_source_id != null || var.boot_disk_size != null
      error_message = "An image node needs media_source_id or boot_disk_size."
    }

    precondition {
      condition     = var.source_type != "recipe" || local.recipe_id_valid
      error_message = "recipe_id is required when source_type is recipe and must be a 40-character hexadecimal recipe key."
    }

    precondition {
      condition     = var.source_type != "image" || var.recipe_id == null
      error_message = "recipe_id is used when source_type is recipe."
    }

    precondition {
      condition     = var.source_type != "recipe" || var.media_source_id == null
      error_message = "media_source_id is used when source_type is image."
    }

    precondition {
      condition     = var.source_type != "recipe" || var.boot_disk_size == null
      error_message = "boot_disk_size is used when source_type is image."
    }
  }
}
