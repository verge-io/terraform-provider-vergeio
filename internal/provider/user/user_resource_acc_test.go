package user_test

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"terraform-provider-vergeio/internal/acctest"
	"terraform-provider-vergeio/internal/provider/vergeio"
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

// testAccCheckUserLogin checks basic-auth against the users API.
// VergeOS returns 401 when the password is wrong. A correct password is 200
// for an account that can list users and 403 when the password is accepted
// but the account is not allowed to list users.
func testAccCheckUserLogin(username, password string, wantRejected bool) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rejected, status, err := testAccUserLoginRejected(username, password)
		if err != nil {
			return err
		}
		if rejected != wantRejected {
			return fmt.Errorf("login as %s status %d: rejected=%v, want rejected=%v", username, status, rejected, wantRejected)
		}
		return nil
	}
}

func testAccUserLoginRejected(username, password string) (bool, int, error) {
	host := os.Getenv("TF_ACC_VERGEIO_HOST")
	endpoint := strings.TrimRight(vergeio.EnsureHTTPSPrefix(host), "/") + "/api/v4/users?fields=%24key&limit=1"
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return false, 0, err
	}
	req.SetBasicAuth(username, password)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // test lab uses the provider's insecure flag
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return false, 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))

	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return true, resp.StatusCode, nil
	case http.StatusOK, http.StatusForbidden:
		return false, resp.StatusCode, nil
	default:
		return false, resp.StatusCode, fmt.Errorf("login as %s: unexpected status %d", username, resp.StatusCode)
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
