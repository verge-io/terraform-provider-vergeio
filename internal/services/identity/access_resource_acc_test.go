// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package identity_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"terraform-provider-vergeio/internal/acctest"
)

func TestAccGroupPermissionMember(t *testing.T) {
	groupName := acctest.Name("group")
	userName := acctest.Name("user")
	basic := testAccAccessConfig(groupName, userName, "operators", true, false)
	updated := testAccAccessConfig(groupName, userName, "updated operators", false, true)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAccessDestroy,
		Steps: []resource.TestStep{
			{
				Config: basic,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_group.test", "name", groupName),
					resource.TestCheckResourceAttr("vergeio_group.test", "description", "operators"),
					resource.TestCheckResourceAttr("vergeio_group.test", "enabled", "true"),
					resource.TestCheckResourceAttrSet("vergeio_group.test", "id"),
					resource.TestCheckResourceAttr("vergeio_user.test", "name", userName),
					testAccCheckMemberPointsAt("vergeio_member.test", "vergeio_group.test", "vergeio_user.test"),
					resource.TestCheckResourceAttr("vergeio_permission.table", "table", "vms"),
					resource.TestCheckResourceAttr("vergeio_permission.table", "list", "true"),
					resource.TestCheckResourceAttr("vergeio_permission.table", "read", "true"),
					resource.TestCheckResourceAttr("vergeio_permission.table", "modify", "false"),
					resource.TestCheckNoResourceAttr("vergeio_permission.table", "object_id"),
					resource.TestCheckResourceAttr("vergeio_permission.object", "table", "groups"),
					resource.TestCheckResourceAttr("vergeio_permission.object", "read", "true"),
					resource.TestCheckResourceAttr("vergeio_permission.object", "modify", "false"),
					testAccCheckPermissionObject("vergeio_permission.object", "vergeio_group.test"),
					resource.TestCheckResourceAttr("data.vergeio_groups.test", "groups.0.name", groupName),
					resource.TestCheckResourceAttr("data.vergeio_groups.test", "groups.0.enabled", "true"),
					resource.TestCheckResourceAttr("data.vergeio_users.test", "users.0.name", userName),
					resource.TestCheckResourceAttr("data.vergeio_users.test", "users.0.enabled", "true"),
				),
			},
			{
				Config: basic,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			{
				Config: updated,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_group.test", "description", "updated operators"),
					resource.TestCheckResourceAttr("vergeio_group.test", "enabled", "false"),
					resource.TestCheckResourceAttr("vergeio_permission.table", "modify", "true"),
					resource.TestCheckResourceAttr("vergeio_permission.object", "modify", "true"),
					testAccCheckMemberPointsAt("vergeio_member.test", "vergeio_group.test", "vergeio_user.test"),
					resource.TestCheckResourceAttr("data.vergeio_groups.test", "groups.0.description", "updated operators"),
					resource.TestCheckResourceAttr("data.vergeio_groups.test", "groups.0.enabled", "false"),
				),
			},
			{
				Config: updated,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			{
				ResourceName:      "vergeio_member.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				ResourceName:      "vergeio_group.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				ResourceName:      "vergeio_permission.table",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				ResourceName:      "vergeio_permission.object",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccCheckMemberPointsAt(memberName, groupName, userName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		member, ok := s.RootModule().Resources[memberName]
		if !ok {
			return fmt.Errorf("resource not found: %s", memberName)
		}
		group, ok := s.RootModule().Resources[groupName]
		if !ok {
			return fmt.Errorf("resource not found: %s", groupName)
		}
		user, ok := s.RootModule().Resources[userName]
		if !ok {
			return fmt.Errorf("resource not found: %s", userName)
		}
		if member.Primary.ID == "" {
			return fmt.Errorf("member id was not saved")
		}
		if member.Primary.Attributes["group"] != group.Primary.ID {
			return fmt.Errorf("member group = %s, want %s", member.Primary.Attributes["group"], group.Primary.ID)
		}
		wantMember := "users/" + user.Primary.ID
		if member.Primary.Attributes["member"] != wantMember {
			return fmt.Errorf("member = %s, want %s", member.Primary.Attributes["member"], wantMember)
		}
		return nil
	}
}

func testAccCheckPermissionObject(permissionName, groupName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		permission, ok := s.RootModule().Resources[permissionName]
		if !ok {
			return fmt.Errorf("resource not found: %s", permissionName)
		}
		group, ok := s.RootModule().Resources[groupName]
		if !ok {
			return fmt.Errorf("resource not found: %s", groupName)
		}
		user, ok := s.RootModule().Resources["vergeio_user.test"]
		if !ok {
			return fmt.Errorf("resource not found: vergeio_user.test")
		}
		if permission.Primary.Attributes["object_id"] != group.Primary.ID {
			return fmt.Errorf("object_id = %s, want group %s", permission.Primary.Attributes["object_id"], group.Primary.ID)
		}
		if permission.Primary.Attributes["user_id"] != user.Primary.ID {
			return fmt.Errorf("user_id = %s, want %s", permission.Primary.Attributes["user_id"], user.Primary.ID)
		}
		return nil
	}
}

func testAccCheckAccessDestroy(s *terraform.State) error {
	client, err := acctest.SDKClient()
	if err != nil {
		return err
	}
	checks := []struct {
		resourceType string
		get          func(context.Context, int) error
	}{
		{"vergeio_permission", func(ctx context.Context, id int) error {
			_, err := client.Permissions.Get(ctx, id)
			return err
		}},
		{"vergeio_member", func(ctx context.Context, id int) error {
			_, err := client.Members.Get(ctx, id)
			return err
		}},
		{"vergeio_group", func(ctx context.Context, id int) error {
			_, err := client.Groups.Get(ctx, id)
			return err
		}},
		{"vergeio_user", func(ctx context.Context, id int) error {
			_, err := client.Users.Get(ctx, id)
			return err
		}},
	}
	for _, check := range checks {
		if err := acctest.CheckDeleted(s, check.resourceType, check.get); err != nil {
			return err
		}
	}
	return nil
}

func testAccAccessConfig(groupName, userName, description string, enabled, modify bool) string {
	if err := acctest.RequirePrefix(groupName); err != nil {
		panic(err)
	}
	if err := acctest.RequirePrefix(userName); err != nil {
		panic(err)
	}
	return acctest.Config(fmt.Sprintf(`
resource "vergeio_group" "test" {
  name        = %q
  description = %q
  enabled     = %t
}

resource "vergeio_user" "test" {
  name        = %q
  enabled     = true
  displayname = %q
  password    = "TerraformTest123!"
}

resource "vergeio_member" "test" {
  group  = tonumber(vergeio_group.test.id)
  member = format("users/%%s", vergeio_user.test.id)
}

resource "vergeio_permission" "table" {
  group_id = vergeio_group.test.id
  table    = "vms"
  list     = true
  read     = true
  create   = false
  modify   = %t
  delete   = false
}

resource "vergeio_permission" "object" {
  user_id   = vergeio_user.test.id
  table     = "groups"
  object_id = tonumber(vergeio_group.test.id)
  list      = true
  read      = true
  create    = false
  modify    = %t
  delete    = false
}

data "vergeio_groups" "test" {
  filter_name = vergeio_group.test.name
}

data "vergeio_users" "test" {
  filter_name = vergeio_user.test.name
}
`, groupName, description, enabled, userName, userName, modify, modify))
}
