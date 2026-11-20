// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package identity_test

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/verge-io/govergeos"

	"terraform-provider-vergeio/internal/acctest"
)

func TestAccAPIKeyEphemeral(t *testing.T) {
	keyName := acctest.Name("eph-key")
	config := testAccAPIKeyEphemeralConfig(keyName)
	closed := testAccCheckEphemeralAPIKeyClosed("data.vergeio_users.acc", keyName)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.RequireEphemeral(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.vergeio_version.as_user", "version"),
					closed,
				),
			},
			{
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
				Check: closed,
			},
		},
	})
}

func testAccAPIKeyEphemeralConfig(keyName string) string {
	if err := acctest.RequirePrefix(keyName); err != nil {
		panic(err)
	}
	return acctest.Config(fmt.Sprintf(`
data "vergeio_users" "acc" {
  filter_name = %q
}

ephemeral "vergeio_api_key" "run" {
  user_id     = data.vergeio_users.acc.users[0].id
  name        = %q
  description = "acceptance"
  ttl_seconds = 600
}

provider "vergeio" {
  alias    = "as_user"
  host     = %q
  insecure = true
  api_key  = ephemeral.vergeio_api_key.run.token
}

data "vergeio_version" "as_user" {
  provider = vergeio.as_user
}
`, os.Getenv("TF_ACC_VERGEIO_USERNAME"), keyName, os.Getenv("TF_ACC_VERGEIO_HOST")))
}

func testAccCheckEphemeralAPIKeyClosed(userResource, keyName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[userResource]
		if !ok {
			return fmt.Errorf("resource not found: %s", userResource)
		}
		raw := rs.Primary.Attributes["users.0.id"]
		id, err := strconv.Atoi(raw)
		if err != nil {
			return fmt.Errorf("user id %q: %w", raw, err)
		}
		client, err := acctest.SDKClient()
		if err != nil {
			return err
		}
		keys, err := client.UserAPIKeys.ListByUser(context.Background(), id)
		if err != nil {
			if vergeos.IsNotFoundError(err) {
				return nil
			}
			return err
		}
		for _, key := range keys {
			if key.Name == keyName {
				return fmt.Errorf("ephemeral API key %q (id %d) still exists after apply", keyName, key.Key.Int())
			}
		}
		return nil
	}
}
