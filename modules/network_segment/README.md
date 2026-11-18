# network_segment

Creates an internal network with DHCP, DNS, and a firewall policy.

`dns_servers` and `domain` are handed to DHCP clients (`dnslist` and `domain`). `dns` adds a DNS view, a master zone, and records on the same network. The view, the zone, and every record except one are staged with `apply` false. One record refreshes DNS after the others exist. Omit `ipaddress` to use the first host of `cidr`.

`firewall_rules` is the whole non-system policy, in order (`vergeio_network_rules`). Rules left off the list are deleted. System rules stay. `action` `translate` writes `target_ip` and `target_ports` for a port forward.

`external_network_id` is written to `vnet_default_gateway`, the external network used for outbound traffic. The provider schema has no NAT resource. That gateway is the outbound path.

```hcl
module "app" {
  source = "../../modules/network_segment"

  name                 = "app"
  cidr                 = "192.168.16.0/24"
  ipaddress            = "192.168.16.1/24"
  dhcp_start           = "192.168.16.10"
  dhcp_stop            = "192.168.16.200"
  dns_servers          = ["192.168.16.1"]
  domain               = "app.example.com"
  external_network_id  = data.vergeio_networks.external.networks[0].id

  dns = {
    zone_domain = "app.example.com"
    email       = "hostmaster@example.com"
    records = [{
      host  = "www"
      type  = "A"
      value = "192.168.16.10"
    }]
  }

  firewall_rules = [{
    name              = "allow-ssh"
    protocol          = "tcp"
    destination_ports = "22"
    source_ip         = "192.0.2.0/24"
  }]
}
```

`external_network_id` is a number, the same value as `vergeio_networks.networks[].id`. `vergeio_network.id` is a string. Pass `tonumber(vergeio_network.external.id)` when the key comes from that resource.

A runnable root module is in `examples/modules/network_segment`.

`terraform test` in this directory plans with a mocked provider. Lab apply tests are in `tests/lab`:

```bash
terraform test -test-directory=tests/lab
```
