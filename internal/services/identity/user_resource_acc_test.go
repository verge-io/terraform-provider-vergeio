package identity_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"

	"terraform-provider-vergeio/internal/acctest"
	"terraform-provider-vergeio/internal/client"
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

func TestAccUserPassword(t *testing.T) {
	userName := acctest.Name("user")
	original := "TerraformTest123!"
	rotated := "TerraformTest456!"
	basic := testAccUserPasswordConfig(userName, original)
	updated := testAccUserPasswordConfig(userName, rotated)
	var userID string

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
					resource.TestCheckResourceAttr("vergeio_user.test", "change_password", "false"),
					resource.TestCheckResourceAttrSet("vergeio_user.test", "id"),
					func(s *terraform.State) error {
						rs, ok := s.RootModule().Resources["vergeio_user.test"]
						if !ok || rs.Primary.ID == "" {
							return fmt.Errorf("user id was not saved")
						}
						userID = rs.Primary.ID
						return nil
					},
					testAccCheckUserLogin(userName, original, false),
					testAccCheckUserLogin(userName, rotated, true),
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
					resource.TestCheckResourceAttr("vergeio_user.test", "change_password", "false"),
					func(s *terraform.State) error {
						rs, ok := s.RootModule().Resources["vergeio_user.test"]
						if !ok {
							return fmt.Errorf("resource not found: vergeio_user.test")
						}
						if rs.Primary.ID != userID {
							return fmt.Errorf("user id changed from %s to %s", userID, rs.Primary.ID)
						}
						return nil
					},
					testAccCheckUserLogin(userName, rotated, false),
					testAccCheckUserLogin(userName, original, true),
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
		},
	})
}

// TestAccUserUpdateKeepsDependents covers #193: in-place user updates must
// keep user.id known so vergeio_member and vergeio_permission that reference
// it are not force-replaced.
func TestAccUserUpdateKeepsDependents(t *testing.T) {
	userName := acctest.Name("user")
	groupName := acctest.Name("group")
	var userID, memberID, permissionID string

	basic := testAccUserDependentsConfig(userName, groupName, "one@example.invalid", true)
	emailUpdated := testAccUserDependentsConfig(userName, groupName, "two@example.invalid", true)
	disabled := testAccUserDependentsConfig(userName, groupName, "two@example.invalid", false)
	reenabled := testAccUserDependentsConfig(userName, groupName, "two@example.invalid", true)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAccessDestroy,
		Steps: []resource.TestStep{
			{
				Config: basic,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckUserExists("vergeio_user.test"),
					resource.TestCheckResourceAttr("vergeio_user.test", "email", "one@example.invalid"),
					resource.TestCheckResourceAttr("vergeio_user.test", "enabled", "true"),
					testAccCheckMemberPointsAt("vergeio_member.test", "vergeio_group.test", "vergeio_user.test"),
					resource.TestCheckResourceAttr("vergeio_permission.test", "table", "vms"),
					resource.TestCheckResourceAttrPair("vergeio_permission.test", "user_id", "vergeio_user.test", "id"),
					func(s *terraform.State) error {
						user, ok := s.RootModule().Resources["vergeio_user.test"]
						if !ok || user.Primary.ID == "" {
							return fmt.Errorf("user id was not saved")
						}
						member, ok := s.RootModule().Resources["vergeio_member.test"]
						if !ok || member.Primary.ID == "" {
							return fmt.Errorf("member id was not saved")
						}
						permission, ok := s.RootModule().Resources["vergeio_permission.test"]
						if !ok || permission.Primary.ID == "" {
							return fmt.Errorf("permission id was not saved")
						}
						userID = user.Primary.ID
						memberID = member.Primary.ID
						permissionID = permission.Primary.ID
						return nil
					},
				),
			},
			{
				Config: emailUpdated,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vergeio_user.test", plancheck.ResourceActionUpdate),
						plancheck.ExpectKnownValue("vergeio_user.test", tfjsonpath.New("id"), knownvalue.NotNull()),
						expectNoReplaceOrDestroy("vergeio_member.test"),
						expectNoReplaceOrDestroy("vergeio_permission.test"),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_user.test", "email", "two@example.invalid"),
					testAccCheckIDsUnchanged(&userID, &memberID, &permissionID),
					testAccCheckMemberPointsAt("vergeio_member.test", "vergeio_group.test", "vergeio_user.test"),
					resource.TestCheckResourceAttrPair("vergeio_permission.test", "user_id", "vergeio_user.test", "id"),
				),
			},
			{
				Config: disabled,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vergeio_user.test", plancheck.ResourceActionUpdate),
						plancheck.ExpectKnownValue("vergeio_user.test", tfjsonpath.New("id"), knownvalue.NotNull()),
						expectNoReplaceOrDestroy("vergeio_member.test"),
						expectNoReplaceOrDestroy("vergeio_permission.test"),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_user.test", "enabled", "false"),
					testAccCheckIDsUnchanged(&userID, &memberID, &permissionID),
					testAccCheckMemberPointsAt("vergeio_member.test", "vergeio_group.test", "vergeio_user.test"),
				),
			},
			{
				Config: reenabled,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vergeio_user.test", plancheck.ResourceActionUpdate),
						plancheck.ExpectKnownValue("vergeio_user.test", tfjsonpath.New("id"), knownvalue.NotNull()),
						expectNoReplaceOrDestroy("vergeio_member.test"),
						expectNoReplaceOrDestroy("vergeio_permission.test"),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_user.test", "enabled", "true"),
					testAccCheckIDsUnchanged(&userID, &memberID, &permissionID),
				),
			},
			{
				Config: reenabled,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			{
				ResourceName:            "vergeio_user.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"password"},
			},
			{
				ResourceName:      "vergeio_member.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				ResourceName:      "vergeio_permission.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				// After import, the first plan must not force-replace dependents
				// solely because user.id was planned unknown (#193).
				Config: reenabled,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						expectNoReplaceOrDestroy("vergeio_member.test"),
						expectNoReplaceOrDestroy("vergeio_permission.test"),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckIDsUnchanged(&userID, &memberID, &permissionID),
					testAccCheckMemberPointsAt("vergeio_member.test", "vergeio_group.test", "vergeio_user.test"),
				),
			},
		},
	})
}

func testAccCheckIDsUnchanged(userID, memberID, permissionID *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		user := s.RootModule().Resources["vergeio_user.test"]
		member := s.RootModule().Resources["vergeio_member.test"]
		permission := s.RootModule().Resources["vergeio_permission.test"]
		if user == nil || member == nil || permission == nil {
			return fmt.Errorf("user/member/permission missing from state")
		}
		if user.Primary.ID != *userID {
			return fmt.Errorf("user id changed from %s to %s", *userID, user.Primary.ID)
		}
		if member.Primary.ID != *memberID {
			return fmt.Errorf("member id changed from %s to %s (dependent was replaced)", *memberID, member.Primary.ID)
		}
		if permission.Primary.ID != *permissionID {
			return fmt.Errorf("permission id changed from %s to %s (dependent was replaced)", *permissionID, permission.Primary.ID)
		}
		return nil
	}
}

// expectNoReplaceOrDestroy fails when the named resource is planned for
// replace or destroy. Absence from ResourceChanges is treated as a no-op.
type expectNoReplaceOrDestroyCheck struct {
	address string
}

func expectNoReplaceOrDestroy(address string) plancheck.PlanCheck {
	return expectNoReplaceOrDestroyCheck{address: address}
}

func (e expectNoReplaceOrDestroyCheck) CheckPlan(ctx context.Context, req plancheck.CheckPlanRequest, resp *plancheck.CheckPlanResponse) {
	if req.Plan == nil {
		resp.Error = fmt.Errorf("plan is nil")
		return
	}
	for _, rc := range req.Plan.ResourceChanges {
		if rc.Address != e.address {
			continue
		}
		if rc.Change.Actions.Replace() || rc.Change.Actions.Delete() {
			resp.Error = fmt.Errorf("%s - expected no replace/destroy, got actions %v", rc.Address, rc.Change.Actions)
			return
		}
		return
	}
}

func testAccUserDependentsConfig(userName, groupName, email string, enabled bool) string {
	if err := acctest.RequirePrefix(userName); err != nil {
		panic(err)
	}
	if err := acctest.RequirePrefix(groupName); err != nil {
		panic(err)
	}
	return acctest.Config(fmt.Sprintf(`
resource "vergeio_user" "test" {
  name            = %q
  password        = "TerraformTest123!"
  email           = %q
  enabled         = %t
  displayname     = %q
  change_password = false
}

resource "vergeio_group" "test" {
  name = %q
}

resource "vergeio_member" "test" {
  group  = tonumber(vergeio_group.test.id)
  member = format("users/%%s", vergeio_user.test.id)
}

resource "vergeio_permission" "test" {
  table   = "vms"
  user_id = vergeio_user.test.id
  list    = true
  read    = true
}
`, userName, email, enabled, userName, groupName))
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

// testAccCheckUserLogin posts {"login","password"} to /api/sys/tokens with no
// Authorization header. A 2xx response with a $key means that password is the
// stored credential. VergeOS returns 201 Created for a new token. Any other
// status means the password was rejected.
//
// GET /api/v4 with basic auth is not used. On the lab, that check still
// returned 200 for the password from the previous step after a rotation,
// while the new password also returned 200. The token call does not send
// the candidate as basic auth, so it cannot reuse that accepted pair.
func testAccCheckUserLogin(username, password string, wantRejected bool) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rejected, status, err := vergeio.UserLoginRejected(os.Getenv("TF_ACC_VERGEIO_HOST"), username, password)
		if err != nil {
			return err
		}
		if rejected != wantRejected {
			return fmt.Errorf("login as %s status %d: rejected=%v, want rejected=%v", username, status, rejected, wantRejected)
		}
		return nil
	}
}

func testAccUserPasswordConfig(userName, password string) string {
	if err := acctest.RequirePrefix(userName); err != nil {
		panic(err)
	}
	return acctest.Config(fmt.Sprintf(`
resource "vergeio_user" "test" {
  name     = %q
  password = %q
}
`, userName, password))
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
