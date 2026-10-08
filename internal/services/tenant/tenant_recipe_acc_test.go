// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tenant_test

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/verge-io/govergeos"

	"terraform-provider-vergeio/internal/acctest"
)

// TestAccTenantRecipeInstance deploys a tenant from a catalog recipe, reads
// it back, imports it, and destroys the tenant and the recipe instance.
// Names start with tf-acc- so the tenant and tenant recipe instance sweeps
// can remove a leftover.
//
// It is skipped unless TF_ACC=1 and TF_ACC_VERGEIO_TENANT_RECIPE_ID is set
// to a tenant recipe key (40 hexadecimal characters, the id from
// vergeio_tenant_recipes). TF_ACC_VERGEIO_TENANT_RECIPE_ANSWERS is optional
// HCL inside the answers map, for example:
// YB_USER_NAME = "admin" and YB_EXPOSE_CLOUD_SNAPSHOTS = "true".
// Set it when the recipe has required questions.
func TestAccTenantRecipeInstance(t *testing.T) {
	acctest.PreCheck(t)
	recipeID := os.Getenv("TF_ACC_VERGEIO_TENANT_RECIPE_ID")
	if recipeID == "" {
		t.Skip("set TF_ACC_VERGEIO_TENANT_RECIPE_ID to a tenant recipe key (40 hexadecimal characters) to deploy a tenant")
	}
	name := acctest.Name("tenant-recipe")
	if err := acctest.RequirePrefix(name); err != nil {
		t.Fatal(err)
	}
	answers := os.Getenv("TF_ACC_VERGEIO_TENANT_RECIPE_ANSWERS")
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
		body = "\n  answers = {\n    " + answers + "\n  }\n"
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
