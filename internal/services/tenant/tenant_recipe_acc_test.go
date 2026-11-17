// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tenant_test

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/verge-io/govergeos"

	"terraform-provider-vergeio/internal/acctest"
)

// TestAccTenantRecipeInstance deploys a tenant from a catalog recipe, reads
// it back, imports it, and destroys the tenant and the recipe instance.
// Names start with tf-acc-. Sweep removes a leftover recipe, catalog, and
// instance. Destroy of the instance tenant removes its nodes highest
// nodeid first, the same order as the recipe snapshot copy.
//
// VergeOS builds a recipe from a source tenant that was configured inside
// the tenant. That tenant is dedicated to recipe use and is powered off
// when the recipe is created. Creating a tenant from the host and powering
// it on does not do the inside configuration, and no host-side field
// records it, so this test does not provision a source tenant.
//
// TF_ACC_VERGEIO_TENANT_RECIPE_ID is a published recipe hex key, the same
// value as vergeio_tenant_recipes recipes[].id. When it is unset,
// TF_ACC_VERGEIO_TENANT_RECIPE_SOURCE_TENANT_ID is the key of an existing
// tenant that is already configured and powered off. The test posts a
// tf-acc- recipe from that tenant and deletes the recipe, its catalog, and
// the snapshot tenant VergeOS created. It does not power, edit, or delete
// the source tenant. The test skips when neither variable is set.
//
// TF_ACC_VERGEIO_TENANT_RECIPE_ANSWERS is optional HCL inside the answers
// map. Set it when the published recipe has required questions, for example
// YB_USER_NAME = "admin". When the test creates the recipe, that HCL
// replaces the answers the fixture would send.
func TestAccTenantRecipeInstance(t *testing.T) {
	acctest.PreCheck(t)
	acctest.SkipWithoutTenantRecipeLab(t)
	recipeID := acctest.PublishedTenantRecipeID()
	answers := os.Getenv("TF_ACC_VERGEIO_TENANT_RECIPE_ANSWERS")
	if recipeID == "" {
		fixture := acctest.NewTenantRecipeFixture(t)
		recipeID = fixture.RecipeID
		if answers == "" {
			answers = fixture.AnswersHCL
		}
	}
	name := acctest.Name("tenant-recipe")
	if err := acctest.RequirePrefix(name); err != nil {
		t.Fatal(err)
	}
	config := testAccTenantRecipeInstanceConfig(name, recipeID, answers)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckTenantRecipeInstanceDestroy,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_tenant_recipe_instance.test", "name", name),
					resource.TestCheckResourceAttr("vergeio_tenant_recipe_instance.test", "recipe_id", recipeID),
					resource.TestCheckResourceAttrSet("vergeio_tenant_recipe_instance.test", "id"),
					resource.TestCheckResourceAttrSet("vergeio_tenant_recipe_instance.test", "tenant_id"),
					testAccTenantRecipeListed(recipeID),
					testAccTenantFromRecipeExists(name),
				),
			},
			{
				ResourceName:            "vergeio_tenant_recipe_instance.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"answers"},
			},
		},
	})
}

func testAccTenantRecipeInstanceConfig(name, recipeID, answers string) string {
	body := ""
	if answers != "" {
		if strings.Contains(answers, "\n") {
			body = "\n  answers = {\n" + answers + "\n  }\n"
		} else {
			body = "\n  answers = {\n    " + answers + "\n  }\n"
		}
	}
	return acctest.Config(fmt.Sprintf(`
data "vergeio_tenant_recipes" "all" {}

resource "vergeio_tenant_recipe_instance" "test" {
  name      = %q
  recipe_id = %q
%s}
`, name, recipeID, body))
}

func testAccTenantRecipeListed(recipeID string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources["data.vergeio_tenant_recipes.all"]
		if !ok {
			return fmt.Errorf("tenant recipes data source missing from state")
		}
		count, err := strconv.Atoi(rs.Primary.Attributes["recipes.#"])
		if err != nil {
			return err
		}
		for i := 0; i < count; i++ {
			if rs.Primary.Attributes[fmt.Sprintf("recipes.%d.id", i)] == recipeID {
				return nil
			}
		}
		return fmt.Errorf("recipe %s was not in vergeio_tenant_recipes", recipeID)
	}
}

func testAccTenantFromRecipeExists(name string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources["vergeio_tenant_recipe_instance.test"]
		if !ok {
			return fmt.Errorf("recipe instance missing from state")
		}
		tenantID, err := strconv.Atoi(rs.Primary.Attributes["tenant_id"])
		if err != nil || tenantID <= 0 {
			return fmt.Errorf("tenant_id = %q", rs.Primary.Attributes["tenant_id"])
		}
		client, err := acctest.SDKClient()
		if err != nil {
			return err
		}
		tenant, err := client.Tenants.Get(context.Background(), tenantID)
		if err != nil {
			return err
		}
		if tenant.Name != name {
			return fmt.Errorf("tenant %d name = %q, want %q", tenantID, tenant.Name, name)
		}
		return nil
	}
}

func testAccCheckTenantRecipeInstanceDestroy(s *terraform.State) error {
	client, err := acctest.SDKClient()
	if err != nil {
		return err
	}
	ctx := context.Background()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "vergeio_tenant_recipe_instance" {
			continue
		}
		id, err := strconv.Atoi(rs.Primary.ID)
		if err != nil {
			return err
		}
		_, err = client.TenantRecipeInstances.Get(ctx, id)
		if err == nil {
			return fmt.Errorf("tenant recipe instance %d still exists", id)
		}
		if !vergeos.IsNotFoundError(err) {
			return err
		}
		tenantID, convErr := strconv.Atoi(rs.Primary.Attributes["tenant_id"])
		if convErr != nil || tenantID <= 0 {
			continue
		}
		_, err = client.Tenants.Get(ctx, tenantID)
		if err == nil {
			return fmt.Errorf("tenant %d from recipe instance %d still exists", tenantID, id)
		}
		if !vergeos.IsNotFoundError(err) {
			return err
		}
	}
	return nil
}
