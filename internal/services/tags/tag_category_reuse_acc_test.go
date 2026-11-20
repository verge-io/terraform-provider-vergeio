// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tags_test

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

// TestAccTagCategory_KeyReuseDoesNotAdoptForeign covers #291. Ownership is
// the readonly created time, so a rename of the same row stays an update.
func TestAccTagCategory_KeyReuseDoesNotAdoptForeign(t *testing.T) {
	acctest.PreCheck(t)
	name := acctest.Name("tagcat-kr291")
	foreignName := acctest.Name("tagcat-kr291-f")
	config := testAccTagCategoryOnly(name, "mine")

	var managedKey int
	var managedCreated string
	var foreignKey int
	var keyReused bool

	t.Cleanup(func() {
		client, err := acctest.SDKClient()
		if err != nil || foreignKey <= 0 {
			return
		}
		_ = client.TagCategories.Delete(context.Background(), foreignKey)
	})

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckTagCategoryKeyReuseDestroy,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_tag_category.test", "name", name),
					resource.TestCheckResourceAttrSet("vergeio_tag_category.test", "id"),
					resource.TestCheckResourceAttrSet("vergeio_tag_category.test", "created"),
					func(s *terraform.State) error {
						rs := s.RootModule().Resources["vergeio_tag_category.test"]
						id, err := acctest.ResourceID(s, "vergeio_tag_category.test")
						if err != nil {
							return err
						}
						managedKey = id
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
					if err := client.TagCategories.Delete(ctx, managedKey); err != nil && !vergeos.IsNotFoundError(err) {
						t.Fatalf("delete managed tag category %d: %v", managedKey, err)
					}
					acctest.WaitUntilMissing(t, func() error {
						_, err := client.TagCategories.Get(ctx, managedKey)
						return err
					}, fmt.Sprintf("tag category %d", managedKey))
					created, err := client.TagCategories.Create(ctx, &vergeos.TagCategoryCreateRequest{
						Name:        foreignName,
						Description: "belongs to someone else",
					})
					if err != nil {
						t.Fatalf("create foreign tag category: %v", err)
					}
					foreignKey = created.Key.Int()
					keyReused = foreignKey == managedKey
					t.Logf("managed tag category key=%d created=%s; foreign key=%d created=%d reused=%v",
						managedKey, managedCreated, foreignKey, created.Created, keyReused)
				},
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vergeio_tag_category.test", plancheck.ResourceActionCreate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_tag_category.test", "name", name),
					resource.TestCheckResourceAttrSet("vergeio_tag_category.test", "created"),
					func(s *terraform.State) error {
						if err := acctest.ForeignUntouched(s, "vergeio_tag_category.test", foreignKey, keyReused, func(ctx context.Context) (string, int, error) {
							client, err := acctest.SDKClient()
							if err != nil {
								return "", 0, err
							}
							got, err := client.TagCategories.Get(ctx, foreignKey)
							if err != nil {
								return "", 0, err
							}
							return got.Name, got.Key.Int(), nil
						}, foreignName); err != nil {
							return err
						}
						rs := s.RootModule().Resources["vergeio_tag_category.test"]
						if rs.Primary.Attributes["created"] == "" || rs.Primary.Attributes["created"] == managedCreated {
							return fmt.Errorf("managed category kept created %s after outside delete", managedCreated)
						}
						return nil
					},
				),
			},
		},
	})
}

func testAccCheckTagCategoryKeyReuseDestroy(s *terraform.State) error {
	client, err := acctest.SDKClient()
	if err != nil {
		return err
	}
	return acctest.CheckDeletedMatching(s, "vergeio_tag_category", func(ctx context.Context, id int, attrs map[string]string) error {
		got, err := client.TagCategories.Get(ctx, id)
		if err != nil {
			return err
		}
		if created := attrs["created"]; created != "" && created != "0" && strconv.FormatInt(got.Created, 10) != created {
			return &vergeos.NotFoundError{Resource: "TagCategory", ID: id}
		}
		return nil
	})
}

func testAccTagCategoryOnly(name, description string) string {
	if err := acctest.RequirePrefix(name); err != nil {
		panic(err)
	}
	return acctest.Config(fmt.Sprintf(`
resource "vergeio_tag_category" "test" {
  name        = %q
  description = %q
}
`, name, description))
}
