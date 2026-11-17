# An A record whose value is the VM NIC address.
# vm_nic_id is the other form: omit value and set vm_nic_id to the NIC id.

resource "vergeio_network" "lan" {
  name    = "Example LAN"
  network = "192.0.2.0/24"
}

resource "vergeio_vm" "web" {
  name       = "web"
  powerstate = false

  boot_disk {
    name = "os"
    size = 5
  }
}

resource "vergeio_vm_nic" "web" {
  vm_id            = vergeio_vm.web.id
  name             = "web"
  interface        = "virtio"
  vnet             = tonumber(vergeio_network.lan.id)
  assign_ipaddress = true
  ipaddress        = "192.0.2.40"
}

resource "vergeio_network_dns_view" "internal" {
  network_id = vergeio_network.lan.id
  name       = "internal"
  apply      = false
}

resource "vergeio_network_dns_zone" "example" {
  view_id = vergeio_network_dns_view.internal.id
  domain  = "example.com"
  type    = "master"
  apply   = false
}

resource "vergeio_network_dns_record" "web" {
  zone_id = vergeio_network_dns_zone.example.id
  host    = "web"
  type    = "A"
  value   = vergeio_vm_nic.web.ipaddress

  depends_on = [
    vergeio_network_dns_view.internal,
    vergeio_network_dns_zone.example,
  ]
}
