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

// TestAccTag_KeyReuseDoesNotAdoptForeign covers #326. Ownership is the
// readonly created time, so a rename of the same row stays an update.
func TestAccTag_KeyReuseDoesNotAdoptForeign(t *testing.T) {
	acctest.PreCheck(t)
	categoryName := acctest.Name("tagcat-kr326")
	name := acctest.Name("tag-kr326")
	foreignName := acctest.Name("tag-kr326-f")
	config := testAccTagOnly(categoryName, name, "mine")

	var categoryKey int
	var managedKey int
	var managedCreated string
	var foreignKey int
	var keyReused bool

	t.Cleanup(func() {
		client, err := acctest.SDKClient()
		if err != nil || foreignKey <= 0 {
			return
		}
		_ = client.Tags.Delete(context.Background(), foreignKey)
	})

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckTagKeyReuseDestroy,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_tag.test", "name", name),
					resource.TestCheckResourceAttrSet("vergeio_tag.test", "id"),
					resource.TestCheckResourceAttrSet("vergeio_tag.test", "created"),
					func(s *terraform.State) error {
						rs := s.RootModule().Resources["vergeio_tag.test"]
						id, err := acctest.ResourceID(s, "vergeio_tag.test")
						if err != nil {
							return err
						}
						categoryID, err := acctest.ResourceID(s, "vergeio_tag_category.test")
						if err != nil {
							return err
						}
						managedKey = id
						categoryKey = categoryID
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
					if err := client.Tags.Delete(ctx, managedKey); err != nil && !vergeos.IsNotFoundError(err) {
						t.Fatalf("delete managed tag %d: %v", managedKey, err)
					}
					acctest.WaitUntilMissing(t, func() error {
						_, err := client.Tags.Get(ctx, managedKey)
						return err
					}, fmt.Sprintf("tag %d", managedKey))
					created, err := client.Tags.Create(ctx, &vergeos.TagCreateRequest{
						Category:    categoryKey,
						Name:        foreignName,
						Description: "belongs to someone else",
					})
					if err != nil {
						t.Fatalf("create foreign tag: %v", err)
					}
					foreignKey = created.Key.Int()
					keyReused = foreignKey == managedKey
					t.Logf("managed tag key=%d created=%s; foreign key=%d created=%d reused=%v",
						managedKey, managedCreated, foreignKey, created.Created, keyReused)
				},
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vergeio_tag.test", plancheck.ResourceActionCreate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_tag.test", "name", name),
					resource.TestCheckResourceAttrSet("vergeio_tag.test", "created"),
					func(s *terraform.State) error {
						if err := acctest.ForeignUntouched(s, "vergeio_tag.test", foreignKey, keyReused, func(ctx context.Context) (string, int, error) {
							client, err := acctest.SDKClient()
							if err != nil {
								return "", 0, err
							}
							got, err := client.Tags.Get(ctx, foreignKey)
							if err != nil {
								return "", 0, err
							}
							return got.Name, got.Key.Int(), nil
						}, foreignName); err != nil {
							return err
						}
						rs := s.RootModule().Resources["vergeio_tag.test"]
						if rs.Primary.Attributes["created"] == "" || rs.Primary.Attributes["created"] == managedCreated {
							return fmt.Errorf("managed tag kept created %s after outside delete", managedCreated)
						}
						return nil
					},
				),
			},
		},
	})
}

func testAccCheckTagKeyReuseDestroy(s *terraform.State) error {
	client, err := acctest.SDKClient()
	if err != nil {
		return err
	}
	if err := acctest.CheckDeletedMatching(s, "vergeio_tag", func(ctx context.Context, id int, attrs map[string]string) error {
		got, err := client.Tags.Get(ctx, id)
		if err != nil {
			return err
		}
		if created := attrs["created"]; created != "" && created != "0" && strconv.FormatInt(got.Created, 10) != created {
			return &vergeos.NotFoundError{Resource: "Tag", ID: id}
		}
		return nil
	}); err != nil {
		return err
	}
	return acctest.CheckDeleted(s, "vergeio_tag_category", func(ctx context.Context, id int) error {
		_, err := client.TagCategories.Get(ctx, id)
		return err
	})
}

// TestAccTag_KeyReuseDestroyRefreshOff covers #322. Destroy with refresh
// off must leave the tag that now holds the stored key.
func TestAccTag_KeyReuseDestroyRefreshOff(t *testing.T) {
	acctest.PreCheck(t)
	categoryName := acctest.Name("tagcat-kr322")
	name := acctest.Name("tag-kr322")
	foreignName := acctest.Name("tag-kr322-f")
	config := testAccTagOnly(categoryName, name, "mine")

	var categoryID int
	var managedKey int
	var foreignKey int
	var keyReused bool

	t.Cleanup(func() {
		client, err := acctest.SDKClient()
		if err != nil || foreignKey <= 0 {
			return
		}
		_ = client.Tags.Delete(context.Background(), foreignKey)
	})

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		AdditionalCLIOptions:     acctest.PlanWithoutRefresh(),
		CheckDestroy:             testAccCheckTagKeyReuseDestroy,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("vergeio_tag.test", "id"),
					resource.TestCheckResourceAttrSet("vergeio_tag.test", "created"),
					func(s *terraform.State) error {
						rs := s.RootModule().Resources["vergeio_tag.test"]
						id, err := acctest.ResourceID(s, "vergeio_tag.test")
						if err != nil {
							return err
						}
						category, err := acctest.ResourceID(s, "vergeio_tag_category.test")
						if err != nil {
							return err
						}
						managedKey = id
						categoryID = category
						if rs.Primary.Attributes["created"] == "" || rs.Primary.Attributes["created"] == "0" {
							return fmt.Errorf("created is %q", rs.Primary.Attributes["created"])
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
					if err := client.Tags.Delete(ctx, managedKey); err != nil && !vergeos.IsNotFoundError(err) {
						t.Fatalf("delete managed tag %d: %v", managedKey, err)
					}
					acctest.WaitUntilMissing(t, func() error {
						_, err := client.Tags.Get(ctx, managedKey)
						return err
					}, fmt.Sprintf("tag %d", managedKey))
					created, err := client.Tags.Create(ctx, &vergeos.TagCreateRequest{
						Category:    categoryID,
						Name:        foreignName,
						Description: "belongs to someone else",
					})
					if err != nil {
						t.Fatalf("create foreign tag: %v", err)
					}
					foreignKey = created.Key.Int()
					keyReused = foreignKey == managedKey
					t.Logf("managed tag key=%d; foreign key=%d created=%d reused=%v", managedKey, foreignKey, created.Created, keyReused)
				},
				Config: testAccTagCategoryKept(categoryName),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vergeio_tag.test", plancheck.ResourceActionDestroy),
					},
				},
				Check: func(*terraform.State) error {
					if err := acctest.ForeignPresent(foreignKey, foreignName, keyReused, func(ctx context.Context) (string, error) {
						client, err := acctest.SDKClient()
						if err != nil {
							return "", err
						}
						got, err := client.Tags.Get(ctx, foreignKey)
						if err != nil {
							return "", err
						}
						return got.Name, nil
					}); err != nil {
						return err
					}
					client, err := acctest.SDKClient()
					if err != nil {
						return err
					}
					if err := client.Tags.Delete(context.Background(), foreignKey); err != nil && !vergeos.IsNotFoundError(err) {
						return fmt.Errorf("delete foreign tag %d: %w", foreignKey, err)
					}
					foreignKey = 0
					return nil
				},
			},
		},
	})
}

func testAccTagCategoryKept(categoryName string) string {
	if err := acctest.RequirePrefix(categoryName); err != nil {
		panic(err)
	}
	return acctest.Config(fmt.Sprintf(`
resource "vergeio_tag_category" "test" {
  name         = %q
  taggable_vms = true
}
`, categoryName))
}

func testAccTagOnly(categoryName, name, description string) string {
	if err := acctest.RequirePrefix(categoryName); err != nil {
		panic(err)
	}
	if err := acctest.RequirePrefix(name); err != nil {
		panic(err)
	}
	return acctest.Config(fmt.Sprintf(`
resource "vergeio_tag_category" "test" {
  name         = %q
  taggable_vms = true
}

resource "vergeio_tag" "test" {
  category    = tonumber(vergeio_tag_category.test.id)
  name        = %q
  description = %q
}
`, categoryName, name, description))
}
