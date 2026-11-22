// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package identity_test

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/verge-io/govergeos"

	"terraform-provider-vergeio/internal/acctest"
)

// TestAccAPIKey_KeyReuseDoesNotAdoptForeign covers #326. The same user with
// a different created time is another key.
func TestAccAPIKey_KeyReuseDoesNotAdoptForeign(t *testing.T) {
	acctest.PreCheck(t)
	userName := acctest.Name("api-user-kr326")
	name := acctest.Name("api-key-kr326")
	foreignName := acctest.Name("api-key-kr326-f")
	config := testAccAPIKeyOnly(userName, name, "mine")

	var userID int
	var managedKey int
	var managedCreated string
	var foreignKey int
	var keyReused bool

	t.Cleanup(func() {
		client, err := acctest.SDKClient()
		if err != nil || foreignKey <= 0 {
			return
		}
		_ = client.UserAPIKeys.Delete(context.Background(), foreignKey)
	})

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAPIKeyKeyReuseDestroy,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_api_key.test", "name", name),
					resource.TestCheckResourceAttrSet("vergeio_api_key.test", "id"),
					resource.TestCheckResourceAttrSet("vergeio_api_key.test", "created"),
					func(s *terraform.State) error {
						rs := s.RootModule().Resources["vergeio_api_key.test"]
						id, err := acctest.ResourceID(s, "vergeio_api_key.test")
						if err != nil {
							return err
						}
						owner, err := acctest.ResourceID(s, "vergeio_user.test")
						if err != nil {
							return err
						}
						managedKey = id
						userID = owner
						managedCreated = rs.Primary.Attributes["created"]
						if managedCreated == "" || managedCreated == "0" {
							return fmt.Errorf("created is %q; the ownership check cannot tell rows apart", managedCreated)
						}
						return nil
					},
				),
			},
			{
				PreConfig: func() {
					client, err := acctest.SDKClient()
					if err != nil {
						t.Fatalf("sdk client: %v", err)
					}
					ctx := context.Background()
					if err := client.UserAPIKeys.Delete(ctx, managedKey); err != nil && !vergeos.IsNotFoundError(err) {
						t.Fatalf("delete managed API key %d: %v", managedKey, err)
					}
					acctest.WaitUntilMissing(t, func() error {
						_, err := client.UserAPIKeys.Get(ctx, managedKey)
						return err
					}, fmt.Sprintf("API key %d", managedKey))
					created, _, err := client.UserAPIKeys.Create(ctx, &vergeos.UserAPIKeyCreateRequest{
						User:        userID,
						Name:        foreignName,
						Description: "belongs to someone else",
					})
					if err != nil {
						t.Fatalf("create foreign API key: %v", err)
					}
					foreignKey = created.Key.Int()
					keyReused = foreignKey == managedKey
					t.Logf("managed API key key=%d created=%s; foreign key=%d created=%d reused=%v",
						managedKey, managedCreated, foreignKey, created.Created, keyReused)
				},
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vergeio_api_key.test", plancheck.ResourceActionCreate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_api_key.test", "name", name),
					resource.TestCheckResourceAttrSet("vergeio_api_key.test", "created"),
					func(s *terraform.State) error {
						if err := acctest.ForeignUntouched(s, "vergeio_api_key.test", foreignKey, keyReused, func(ctx context.Context) (string, int, error) {
							client, err := acctest.SDKClient()
							if err != nil {
								return "", 0, err
							}
							got, err := client.UserAPIKeys.Get(ctx, foreignKey)
							if err != nil {
								return "", 0, err
							}
							return got.Name, got.Key.Int(), nil
						}, foreignName); err != nil {
							return err
						}
						rs := s.RootModule().Resources["vergeio_api_key.test"]
						if rs.Primary.Attributes["created"] == "" || rs.Primary.Attributes["created"] == managedCreated {
							return fmt.Errorf("managed API key kept created %s after outside delete", managedCreated)
						}
						client, err := acctest.SDKClient()
						if err != nil {
							return err
						}
						if err := client.UserAPIKeys.Delete(context.Background(), foreignKey); err != nil && !vergeos.IsNotFoundError(err) {
							return fmt.Errorf("delete foreign API key %d before user destroy: %w", foreignKey, err)
						}
						foreignKey = 0
						return nil
					},
				),
			},
		},
	})
}

func testAccCheckAPIKeyKeyReuseDestroy(s *terraform.State) error {
	client, err := acctest.SDKClient()
	if err != nil {
		return err
	}
	if err := acctest.CheckDeletedMatching(s, "vergeio_api_key", func(ctx context.Context, id int, attrs map[string]string) error {
		got, err := client.UserAPIKeys.Get(ctx, id)
		if err != nil {
			return err
		}
		if created := attrs["created"]; created != "" && created != "0" && strconv.FormatInt(got.Created, 10) != created {
			return &vergeos.NotFoundError{Resource: "UserAPIKey", ID: id}
		}
		return nil
	}); err != nil {
		return err
	}
	return acctest.CheckDeleted(s, "vergeio_user", func(ctx context.Context, id int) error {
		_, err := client.Users.Get(ctx, id)
		return err
	})
}

func testAccAPIKeyOnly(userName, keyName, description string) string {
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
  email               = %q
  enabled             = false
  type                = "normal"
  password_wo         = "TerraformTest123!"
  password_wo_version = 1
}

resource "vergeio_api_key" "test" {
  user_id     = tonumber(vergeio_user.test.id)
  name        = %q
  description = %q
}
`, userName, userName+"@example.com", keyName, description))
}
