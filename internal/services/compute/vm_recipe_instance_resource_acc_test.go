// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package compute_test

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

func TestAccCatalogsDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: acctest.Config(`
data "vergeio_catalogs" "all" {}
`),
				Check: resource.TestCheckResourceAttrSet("data.vergeio_catalogs.all", "catalogs.#"),
			},
		},
	})
}

func TestAccVMRecipesDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: acctest.Config(`
data "vergeio_vm_recipes" "all" {}
`),
				Check: resource.TestCheckResourceAttrSet("data.vergeio_vm_recipes.all", "recipes.#"),
			},
		},
	})
}

// TestAccVMRecipeInstance deploys a downloaded recipe. It is skipped unless
// TF_ACC=1 and TF_ACC_VERGEIO_RECIPE_ID is set. TF_ACC_VERGEIO_RECIPE_ANSWERS
// is optional HCL inside the answers map, for example:
// HOSTNAME = "web" and YB_DRIVE_OS_SIZE = "53687091200".
func TestAccVMRecipeInstance(t *testing.T) {
	acctest.PreCheck(t)
	recipeID := os.Getenv("TF_ACC_VERGEIO_RECIPE_ID")
	if recipeID == "" {
		t.Skip("set TF_ACC_VERGEIO_RECIPE_ID to a downloaded recipe key to deploy a VM")
	}
	name := acctest.Name("recipe")
	if err := acctest.RequirePrefix(name); err != nil {
		t.Fatal(err)
	}
	answers := os.Getenv("TF_ACC_VERGEIO_RECIPE_ANSWERS")
	config := testAccVMRecipeInstanceConfig(name, recipeID, answers)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckRecipeInstanceDestroy,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_vm_recipe_instance.test", "name", name),
					resource.TestCheckResourceAttr("vergeio_vm_recipe_instance.test", "recipe_id", recipeID),
					resource.TestCheckResourceAttrSet("vergeio_vm_recipe_instance.test", "id"),
					resource.TestCheckResourceAttrSet("vergeio_vm_recipe_instance.test", "vm_id"),
				),
			},
			{
				ResourceName:            "vergeio_vm_recipe_instance.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"answers", "timeouts"},
			},
		},
	})
}

func testAccVMRecipeInstanceConfig(name, recipeID, answers string) string {
	body := ""
	if answers != "" {
		body = "\n  answers = {\n    " + answers + "\n  }\n"
	}
	return acctest.Config(fmt.Sprintf(`
resource "vergeio_vm_recipe_instance" "test" {
  name      = %q
  recipe_id = %q
%s}
`, name, recipeID, body))
}

func testAccCheckRecipeInstanceDestroy(s *terraform.State) error {
	client, err := acctest.SDKClient()
	if err != nil {
		return err
	}
	ctx := context.Background()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "vergeio_vm_recipe_instance" {
			continue
		}
		id, err := strconv.Atoi(rs.Primary.ID)
		if err != nil {
			return err
		}
		_, err = client.VMRecipeInstances.Get(ctx, id)
		if err == nil {
			return fmt.Errorf("recipe instance %d still exists", id)
		}
		if !vergeos.IsNotFoundError(err) {
			return err
		}
		vmID, convErr := strconv.Atoi(rs.Primary.Attributes["vm_id"])
		if convErr != nil || vmID <= 0 {
			continue
		}
		_, err = client.VMs.Get(ctx, vmID)
		if err == nil {
			return fmt.Errorf("vm %d from recipe instance %d still exists", vmID, id)
		}
		if !vergeos.IsNotFoundError(err) {
			return err
		}
	}
	return nil
}
