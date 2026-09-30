// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tags_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"terraform-provider-vergeio/internal/acctest"
)

// TestAccTagCategoryAndTag creates a category and a tag, updates them, and
// imports both. It is skipped unless TF_ACC=1.
//
// The first configuration sets taggable_vms and taggable_vnets and leaves
// every other taggable_* flag unset. An omitted flag must not be sent as
// false: that turns tagging off for that object type.
//
// The update renames the category while the tag's category id stays the
// same, and also renames the tag. VergeOS then returns the new category
// name on the tag. category_name is planned unknown on that apply so the
// refreshed name is not an inconsistent result.
//
// Destroying the category deletes every tag in it and every assignment of
// those tags. VergeOS does not ask for confirmation. The published example
// sets prevent_destroy so an ordinary destroy cannot do that by accident.
// This test does not, because the lab objects have to be removed. Terraform
// deletes the tag first; the category delete still cascades if a tag remains.
func TestAccTagCategoryAndTag(t *testing.T) {
	categoryName := acctest.Name("tagcat")
	tagName := acctest.Name("tag")
	basic := testAccTagConfig(categoryName, tagName, "classification", "workloads", `
  single_tag_selection = true
  taggable_vms         = true
  taggable_vnets       = true
`)
	updated := testAccTagConfig(categoryName+"-v2", tagName+"-v2", "updated classification", "updated workloads", `
  single_tag_selection = false
  taggable_vms         = false
  taggable_vnets       = true
  taggable_nodes       = true
`)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckTagDestroy,
		Steps: []resource.TestStep{
			{
				Config: basic,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_tag_category.test", "name", categoryName),
					resource.TestCheckResourceAttr("vergeio_tag_category.test", "description", "classification"),
					resource.TestCheckResourceAttr("vergeio_tag_category.test", "single_tag_selection", "true"),
					resource.TestCheckResourceAttr("vergeio_tag_category.test", "taggable_vms", "true"),
					resource.TestCheckResourceAttr("vergeio_tag_category.test", "taggable_vnets", "true"),
					resource.TestCheckResourceAttrSet("vergeio_tag_category.test", "id"),
					resource.TestCheckResourceAttr("vergeio_tag.test", "name", tagName),
					resource.TestCheckResourceAttr("vergeio_tag.test", "description", "workloads"),
					resource.TestCheckResourceAttrSet("vergeio_tag.test", "id"),
					resource.TestCheckResourceAttrPair("vergeio_tag.test", "category_name", "vergeio_tag_category.test", "name"),
					testAccCheckTagCategory("vergeio_tag.test", "vergeio_tag_category.test"),
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
					resource.TestCheckResourceAttr("vergeio_tag_category.test", "name", categoryName+"-v2"),
					resource.TestCheckResourceAttr("vergeio_tag_category.test", "description", "updated classification"),
					resource.TestCheckResourceAttr("vergeio_tag_category.test", "single_tag_selection", "false"),
					resource.TestCheckResourceAttr("vergeio_tag_category.test", "taggable_vms", "false"),
					resource.TestCheckResourceAttr("vergeio_tag_category.test", "taggable_vnets", "true"),
					resource.TestCheckResourceAttr("vergeio_tag_category.test", "taggable_nodes", "true"),
					resource.TestCheckResourceAttr("vergeio_tag.test", "name", tagName+"-v2"),
					resource.TestCheckResourceAttr("vergeio_tag.test", "description", "updated workloads"),
					resource.TestCheckResourceAttrPair("vergeio_tag.test", "category_name", "vergeio_tag_category.test", "name"),
					testAccCheckTagCategory("vergeio_tag.test", "vergeio_tag_category.test"),
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
				ResourceName:      "vergeio_tag_category.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				ResourceName:      "vergeio_tag.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccCheckTagCategory(tagName, categoryName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		tag, ok := s.RootModule().Resources[tagName]
		if !ok {
			return fmt.Errorf("resource not found: %s", tagName)
		}
		category, ok := s.RootModule().Resources[categoryName]
		if !ok {
			return fmt.Errorf("resource not found: %s", categoryName)
		}
		if tag.Primary.ID == "" || category.Primary.ID == "" {
			return fmt.Errorf("tag or category id was not saved")
		}
		if tag.Primary.Attributes["category"] != category.Primary.ID {
			return fmt.Errorf("tag category = %s, want %s", tag.Primary.Attributes["category"], category.Primary.ID)
		}
		return nil
	}
}

func testAccCheckTagDestroy(s *terraform.State) error {
	client, err := acctest.SDKClient()
	if err != nil {
		return err
	}
	// Check the tag first. A category delete cascades to its tags, so a tag
	// that is already gone is still a successful destroy.
	if err := acctest.CheckDeleted(s, "vergeio_tag", func(ctx context.Context, id int) error {
		_, err := client.Tags.Get(ctx, id)
		return err
	}); err != nil {
		return err
	}
	return acctest.CheckDeleted(s, "vergeio_tag_category", func(ctx context.Context, id int) error {
		_, err := client.TagCategories.Get(ctx, id)
		return err
	})
}

func testAccTagConfig(categoryName, tagName, catDesc, tagDesc, flags string) string {
	if err := acctest.RequirePrefix(categoryName); err != nil {
		panic(err)
	}
	if err := acctest.RequirePrefix(tagName); err != nil {
		panic(err)
	}
	return acctest.Config(fmt.Sprintf(`
resource "vergeio_tag_category" "test" {
  name        = %q
  description = %q
%s
}

resource "vergeio_tag" "test" {
  category    = tonumber(vergeio_tag_category.test.id)
  name        = %q
  description = %q
}
`, categoryName, catDesc, flags, tagName, tagDesc))
}
