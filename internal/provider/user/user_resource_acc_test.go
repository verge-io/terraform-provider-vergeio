package user_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"terraform-provider-vergeio/internal/acctest"
)

func TestAccUserResource(t *testing.T) {
	userName := acctest.Name("user")
	basic := testAccUserResourceConfig(userName, "Terraform Acceptance Test User", "tf-test@example.com")
	updated := testAccUserResourceConfig(userName, "Updated Test User", "updated-tf-test@example.com")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckUserDestroy,
		Steps: []resource.TestStep{
			{
				Config: basic,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckUserExists("vergeio_user.test"),
					resource.TestCheckResourceAttr("vergeio_user.test", "name", userName),
					resource.TestCheckResourceAttr("vergeio_user.test", "enabled", "true"),
					resource.TestCheckResourceAttr("vergeio_user.test", "displayname", "Terraform Acceptance Test User"),
					resource.TestCheckResourceAttr("vergeio_user.test", "email", "tf-test@example.com"),
					resource.TestCheckResourceAttrSet("vergeio_user.test", "id"),
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
					testAccCheckUserExists("vergeio_user.test"),
					resource.TestCheckResourceAttr("vergeio_user.test", "name", userName),
					resource.TestCheckResourceAttr("vergeio_user.test", "enabled", "true"),
					resource.TestCheckResourceAttr("vergeio_user.test", "displayname", "Updated Test User"),
					resource.TestCheckResourceAttr("vergeio_user.test", "email", "updated-tf-test@example.com"),
				),
			},
			{
				ResourceName:            "vergeio_user.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"password"},
			},
		},
	})
}

func testAccCheckUserExists(resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("resource not found: %s", resourceName)
		}
		if rs.Primary.ID == "" {
			return fmt.Errorf("no ID is set")
		}
		return nil
	}
}

func testAccCheckUserDestroy(s *terraform.State) error {
	client, err := acctest.SDKClient()
	if err != nil {
		return err
	}
	return acctest.CheckDeleted(s, "vergeio_user", func(ctx context.Context, id int) error {
		_, err := client.Users.Get(ctx, id)
		return err
	})
}

func testAccUserResourceConfig(userName, displayName, email string) string {
	if err := acctest.RequirePrefix(userName); err != nil {
		panic(err)
	}
	return acctest.Config(fmt.Sprintf(`
resource "vergeio_user" "test" {
  name        = %q
  enabled     = true
  displayname = %q
  email       = %q
  password    = "TerraformTest123!"
}
`, userName, displayName, email))
}
