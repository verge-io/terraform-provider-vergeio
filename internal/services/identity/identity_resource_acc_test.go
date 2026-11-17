// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package identity_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"terraform-provider-vergeio/internal/acctest"
)

func TestAccAPIKeyResource(t *testing.T) {
	userName := acctest.Name("api-user")
	keyName := acctest.Name("api-key")
	basic := testAccAPIKeyConfig(userName, keyName, "runner", "192.0.2.0/24", "", 1893456000)
	updated := testAccAPIKeyConfig(userName, keyName, "updated runner", "192.0.2.10/32", "198.51.100.10", 1893456000)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAPIKeyDestroy,
		Steps: []resource.TestStep{
			{
				Config: basic,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_api_key.test", "name", keyName),
					resource.TestCheckResourceAttr("vergeio_api_key.test", "description", "runner"),
					resource.TestCheckResourceAttr("vergeio_api_key.test", "ip_allow_list", "192.0.2.0/24"),
					resource.TestCheckResourceAttr("vergeio_api_key.test", "expires", "1893456000"),
					resource.TestCheckResourceAttrSet("vergeio_api_key.test", "id"),
					resource.TestCheckNoResourceAttr("vergeio_api_key.test", "token"),
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
					resource.TestCheckResourceAttr("vergeio_api_key.test", "description", "updated runner"),
					resource.TestCheckResourceAttr("vergeio_api_key.test", "ip_allow_list", "192.0.2.10/32"),
					resource.TestCheckResourceAttr("vergeio_api_key.test", "ip_deny_list", "198.51.100.10"),
					resource.TestCheckResourceAttr("vergeio_api_key.test", "expires", "1893456000"),
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
				ResourceName:      "vergeio_api_key.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccCheckAPIKeyDestroy(s *terraform.State) error {
	client, err := acctest.SDKClient()
	if err != nil {
		return err
	}
	if err := acctest.CheckDeleted(s, "vergeio_api_key", func(ctx context.Context, id int) error {
		_, err := client.UserAPIKeys.Get(ctx, id)
		return err
	}); err != nil {
		return err
	}
	return acctest.CheckDeleted(s, "vergeio_user", func(ctx context.Context, id int) error {
		_, err := client.Users.Get(ctx, id)
		return err
	})
}

func testAccAPIKeyConfig(userName, keyName, description, allow, deny string, expires int64) string {
	if err := acctest.RequirePrefix(userName); err != nil {
		panic(err)
	}
	if err := acctest.RequirePrefix(keyName); err != nil {
		panic(err)
	}
	return acctest.Config(fmt.Sprintf(`
resource "vergeio_user" "test" {
  name                = %q
  displayname         = "API key acceptance user"
  email               = "api-key@example.com"
  enabled             = true
  type                = "normal"
  password_wo         = "TerraformTest123!"
  password_wo_version = 1
}

resource "vergeio_api_key" "test" {
  user_id       = tonumber(vergeio_user.test.id)
  name          = %q
  description   = %q
  ip_allow_list = %q
  ip_deny_list  = %q
  expires       = %d
}
`, userName, keyName, description, allow, deny, expires))
}

func TestAccAuthSourceResource(t *testing.T) {
	name := acctest.Name("auth-source")
	secret := "TerraformAuthSecret123!"
	basic := testAccAuthSourceConfig(name, secret, "openid profile email", 1)
	updated := testAccAuthSourceConfig(name, secret, "openid profile email groups", 1)
	rotated := testAccAuthSourceConfig(name, "TerraformAuthSecret456!", "openid profile email groups", 2)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAuthSourceDestroy,
		Steps: []resource.TestStep{
			{
				Config: basic,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_auth_source.test", "name", name),
					resource.TestCheckResourceAttr("vergeio_auth_source.test", "driver", "openid"),
					resource.TestCheckResourceAttr("vergeio_auth_source.test", "client_secret_wo_version", "1"),
					resource.TestCheckResourceAttr("vergeio_auth_source.test", "button_fa_icon", "bi-key"),
					resource.TestCheckResourceAttrSet("vergeio_auth_source.test", "id"),
					testAccCheckStateOmits("vergeio_auth_source.test", secret),
					testAccCheckStateOmits("vergeio_auth_source.test", "client_secret"),
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
					resource.TestCheckResourceAttr("vergeio_auth_source.test", "client_secret_wo_version", "1"),
					testAccCheckStateOmits("vergeio_auth_source.test", secret),
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
				Config: rotated,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_auth_source.test", "client_secret_wo_version", "2"),
					testAccCheckStateOmits("vergeio_auth_source.test", secret),
					testAccCheckStateOmits("vergeio_auth_source.test", "TerraformAuthSecret456!"),
				),
			},
			{
				ResourceName:      "vergeio_auth_source.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"settings",
					"client_secret_wo_version",
				},
			},
		},
	})
}

func testAccCheckAuthSourceDestroy(s *terraform.State) error {
	client, err := acctest.SDKClient()
	if err != nil {
		return err
	}
	return acctest.CheckDeleted(s, "vergeio_auth_source", func(ctx context.Context, id int) error {
		_, err := client.AuthSources.Get(ctx, id)
		return err
	})
}

func testAccAuthSourceConfig(name, secret, scope string, version int) string {
	if err := acctest.RequirePrefix(name); err != nil {
		panic(err)
	}
	return acctest.Config(fmt.Sprintf(`
resource "vergeio_auth_source" "test" {
  name   = %q
  driver = "openid"
  settings = jsonencode({
    client_id              = "tf-acc-client"
    authorization_endpoint = "https://example.com/auth"
    token_endpoint         = "https://example.com/token"
    userinfo_endpoint      = "https://example.com/userinfo"
    scope                  = %q
  })
  client_secret_wo         = %q
  client_secret_wo_version = %d
  button_fa_icon           = "bi-key"
}
`, name, scope, secret, version))
}

func testAccCheckStateOmits(resourceName, secret string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("resource not found: %s", resourceName)
		}
		for attr, value := range rs.Primary.Attributes {
			if strings.Contains(value, secret) {
				return fmt.Errorf("%s.%s contains %q", resourceName, attr, secret)
			}
		}
		return nil
	}
}
