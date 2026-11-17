# console_pass_wo and contents_wo are not stored. Increment the matching
# version to send the value again. A console password change updates the
# VM in place. It does not replace the VM.

resource "vergeio_vm" "web-server" {
  name                    = "my-web-server"
  os_family               = "linux"
  cpu_cores               = 2
  ram                     = 2048
  console_pass_enabled    = true
  console_pass_wo         = "console-secret"
  console_pass_wo_version = 1
  cloudinit_datasource    = "nocloud"

  cloudinit_files = [
    {
      name                = "user-data"
      contents_wo         = <<-EOT
        #cloud-config
        password: secret
      EOT
      contents_wo_version = 1
    }
  ]
}
