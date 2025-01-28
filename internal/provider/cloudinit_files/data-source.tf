# Copyright (c) HashiCorp, Inc.

terraform {
  required_providers {
    vergeio = {
      source = "borderssolutions/vergeio" #"vergeio" #"vergeio/cloud/vergeio"
      #source = "verge-io/vergeio"
      #version = "1.5.3"
    }
  }
}
provider "vergeio" {
  host     = "131.153.164.250"
  username = "admin"
  password = "P@ssw0rd!"
  insecure = true
}

data "vergeio_cloudinitfiles" "cloudinit" {
  #   name = "cloudinit"
}

output "cloudinit" {
  value = data.vergeio_cloudinitfiles.cloudinit
}
