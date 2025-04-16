

terraform {
  required_providers {
    vergeio = {
      source  = "verge-io/vergeio"
      version = "1.5.3"
    }
  }
}
provider "vergeio" {
  host     = "192.168.1.1"
  username = "admin"
  password = "P@ssw0rd!"
  insecure = true
}




