terraform {
	required_providers {
		vergeio = {
			source  = "vergeio/cloud/vergeio"
		}
	}
}

resource "vergeio_user" "testuser" {
	auth_source = 1
	name  = "testuser"
	displayname = "testuser"
	email = "testuser@verge.io"
	enabled = false
	type = "normal"
	password = "changeme123"
	change_password = true
}
