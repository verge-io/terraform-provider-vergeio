variable "tenant_password" {
  type      = string
  sensitive = true
}

data "terraform_remote_state" "parent" {
  backend = "local"
  config = {
    path = "../parent/terraform.tfstate"
  }
}

provider "vergeio" {
  host     = data.terraform_remote_state.parent.outputs.tenant_ui_address
  username = "admin"
  password = var.tenant_password
  insecure = true
}

resource "vergeio_vm" "app" {
  name      = "app"
  os_family = "linux"
  cpu_cores = 2
  ram       = 2048
}
