
# Get all groups
data "vergeio_groups" "all" {
  filter_name = "Administrators (default)"
}

# Create a user
resource "vergeio_user" "testuser" {
  name            = "testuser1"
  displayname     = "testuser1"
  email           = "testuser@verge.io"
  enabled         = true
  type            = "normal"
  password        = "changeme123"
  change_password = true
}

# Add user to group
resource "vergeio_member" "membership" {
  group  = data.vergeio_groups.all.groups[0].id
  member = format("users/%s", vergeio_user.testuser.id)
}
