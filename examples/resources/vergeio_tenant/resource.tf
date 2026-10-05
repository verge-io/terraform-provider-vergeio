# Create a tenant on the parent VergeOS system.
# powerstate false leaves it stopped. ui_address is filled when VergeOS
# assigns the tenant UI address.
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
