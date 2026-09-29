terraform {
  required_providers {
    vergeio = {
      source = "verge-io/vergeio"
    }
  }
}

# api_key is sent as a bearer token. username and password are not required.
# When both are set, api_key is used.
provider "vergeio" {
  host    = "vergeos.example.com"
  api_key = var.vergeos_api_key
}

variable "vergeos_api_key" {
  type      = string
  sensitive = true
}
