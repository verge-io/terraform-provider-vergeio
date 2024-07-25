terraform {
	required_providers {
		vergeio = {
			source  = "vergeio/cloud/vergeio"
		}
	}
}

data "vergeio_groups" "beagle" { 
	filter_name = "beagle groop"
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

resource "vergeio_member" "membership" {
	group = data.vergeio_groups.beagle.groups[0].id
	member = format("users/%s", vergeio_user.testuser.id)
}
