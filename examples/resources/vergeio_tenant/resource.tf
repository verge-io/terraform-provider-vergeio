# Create a tenant on the parent VergeOS system.
# powerstate false leaves it stopped. ui_address is filled when VergeOS
# assigns the tenant UI address.
# ui_address_id chooses which assigned external IP is that address.
# Omit it and the first vergeio_tenant_external_ip becomes ui_address.
# Set it on a later change, after that address exists. A parent UI move
# is drift, and the next apply restores the configured value.
# isolate false leaves the tenant on the shared parent network.
# true enables network isolation. Omit isolate to leave the current
# setting unchanged.

resource "vergeio_tenant" "example" {
  name        = "customer-a"
  description = "Customer A virtual data center"
  password    = "change-me"
  powerstate  = false
  isolate     = false
}
