# Create a tenant on the parent VergeOS system.
# powerstate false leaves it stopped. ui_address is filled when VergeOS
# assigns the tenant UI address.

resource "vergeio_tenant" "example" {
  name        = "customer-a"
  description = "Customer A virtual data center"
  password    = "change-me"
  powerstate  = false
}
