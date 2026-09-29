package user_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
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

// testAccCheckUserLogin posts {"login","password"} to /api/sys/tokens with no
// Authorization header. 200 with a $key means that password is the stored
// credential. 401 means it is not.
//
// GET /api/v4 with basic auth is not used. On the lab, that check still
// returned 200 for the password from the previous step after a rotation,
// while the new password also returned 200. The token call does not send
// the candidate as basic auth, so it cannot reuse that accepted pair.
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
	endpoint := strings.TrimRight(vergeio.EnsureHTTPSPrefix(host), "/") + "/api/sys/tokens"
	payload, err := json.Marshal(map[string]string{
		"login":    username,
		"password": password,
	})
	if err != nil {
		return false, 0, err
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return false, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-JSON-Non-Compact", "1")

	client := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // lab matches the provider insecure flag
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return false, 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return false, resp.StatusCode, err
	}

	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return true, resp.StatusCode, nil
	case http.StatusOK:
		var parsed struct {
			Key any `json:"$key"`
		}
		if err := json.Unmarshal(respBody, &parsed); err != nil {
			return false, resp.StatusCode, fmt.Errorf("login as %s: status 200 without a token", username)
		}
		token, ok := tokenKey(parsed.Key)
		if !ok {
			return false, resp.StatusCode, fmt.Errorf("login as %s: status 200 without a token", username)
		}
		testAccDeleteLoginToken(client, endpoint, token)
		return false, resp.StatusCode, nil
	default:
		return false, resp.StatusCode, fmt.Errorf("login as %s: unexpected status %d", username, resp.StatusCode)
	}
}

func tokenKey(v any) (string, bool) {
	switch k := v.(type) {
	case string:
		return k, k != ""
	case float64:
		return fmt.Sprintf("%.0f", k), true
	default:
		return "", false
	}
}

func testAccDeleteLoginToken(client *http.Client, tokensEndpoint, token string) {
	if token == "" {
		return
	}
	req, err := http.NewRequest(http.MethodDelete, tokensEndpoint+"/"+token, nil)
	if err != nil {
		return
	}
	req.Header.Set("x-yottabyte-token", token)
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
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
