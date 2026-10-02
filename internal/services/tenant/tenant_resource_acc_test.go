package tenant_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"terraform-provider-vergeio/internal/acctest"
)

func TestAccTenantResources(t *testing.T) {
	acctest.PreCheck(t)
	tenantName := acctest.Name("tenant")
	nodeName := acctest.Name("tenant-node")
	tier := accStorageTier(t)
	basic := testAccTenantConfig(tenantName, nodeName, tier, "Customer A", 8192, 1073741824)
	updated := testAccTenantConfig(tenantName, nodeName, tier, "Customer A updated", 16384, 2147483648)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckTenantDestroy,
		Steps: []resource.TestStep{
			{
				Config: basic,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_tenant.test", "name", tenantName),
					resource.TestCheckResourceAttr("vergeio_tenant.test", "powerstate", "false"),
					resource.TestCheckResourceAttrSet("vergeio_tenant.test", "id"),
					resource.TestCheckResourceAttr("vergeio_tenant_node.test", "name", nodeName),
					resource.TestCheckResourceAttr("vergeio_tenant_node.test", "cpu_cores", "2"),
					resource.TestCheckResourceAttr("vergeio_tenant_node.test", "ram", "8192"),
					resource.TestCheckResourceAttr("vergeio_tenant_storage.test", "tier", fmt.Sprintf("%d", tier)),
					resource.TestCheckResourceAttr("vergeio_tenant_storage.test", "provisioned", "1073741824"),
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
					resource.TestCheckResourceAttr("vergeio_tenant.test", "description", "Customer A updated"),
					resource.TestCheckResourceAttr("vergeio_tenant_node.test", "ram", "16384"),
					resource.TestCheckResourceAttr("vergeio_tenant_storage.test", "provisioned", "2147483648"),
				),
			},
			{
				ResourceName:            "vergeio_tenant.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"password", "preferred_node"},
			},
			{
				ResourceName:      "vergeio_tenant_node.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				ResourceName:      "vergeio_tenant_storage.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func accStorageTier(t *testing.T) int {
	t.Helper()
	client, err := acctest.SDKClient()
	if err != nil {
		t.Fatal(err)
	}
	tiers, err := client.StorageTiers.List(context.Background())
	if err != nil {
		t.Fatalf("list storage tiers: %v", err)
	}
	if len(tiers) == 0 {
		t.Fatal("no storage tiers on the lab")
	}
	return tiers[0].Key
}

func testAccCheckTenantDestroy(s *terraform.State) error {
	client, err := acctest.SDKClient()
	if err != nil {
		return err
	}
	if err := acctest.CheckDeleted(s, "vergeio_tenant_node", func(ctx context.Context, id int) error {
		_, err := client.TenantNodes.Get(ctx, id)
		return err
	}); err != nil {
		return err
	}
	if err := acctest.CheckDeleted(s, "vergeio_tenant_storage", func(ctx context.Context, id int) error {
		_, err := client.TenantStorage.Get(ctx, id)
		return err
	}); err != nil {
		return err
	}
	return acctest.CheckDeleted(s, "vergeio_tenant", func(ctx context.Context, id int) error {
		_, err := client.Tenants.Get(ctx, id)
		return err
	})
}

func testAccTenantConfig(tenantName, nodeName string, tier int, description string, ram int, provisioned int64) string {
	if err := acctest.RequirePrefix(tenantName); err != nil {
		panic(err)
	}
	if err := acctest.RequirePrefix(nodeName); err != nil {
		panic(err)
	}
	return acctest.Config(fmt.Sprintf(`
resource "vergeio_tenant" "test" {
  name        = %q
  description = %q
  password    = "Tf-acc-tenant-password1"
  powerstate  = false
}

resource "vergeio_tenant_node" "test" {
  tenant_id = vergeio_tenant.test.id
  name      = %q
  cpu_cores = 2
  ram       = %d
  enabled   = true
}

resource "vergeio_tenant_storage" "test" {
  tenant_id   = vergeio_tenant.test.id
  tier        = %d
  provisioned = %d
}
`, tenantName, description, nodeName, ram, tier, provisioned))
}

// TestAccTenantPowerSettleAndDestroy covers #196 and #195 together:
// power on waits for terminal online (no false clean plan), power off waits
// for terminal offline, and destroy of a powered-on tenant succeeds because
// tenant_node powers the tenant off before delete.
func TestAccTenantPowerSettleAndDestroy(t *testing.T) {
	acctest.PreCheck(t)
	tenantName := acctest.Name("tenant-pwr")
	nodeName := acctest.Name("tenant-node-pwr")
	tier := accStorageTier(t)
	offline := testAccTenantPowerConfig(tenantName, nodeName, tier, false)
	online := testAccTenantPowerConfig(tenantName, nodeName, tier, true)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckTenantDestroy,
		Steps: []resource.TestStep{
			{
				Config: offline,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_tenant.test", "name", tenantName),
					resource.TestCheckResourceAttr("vergeio_tenant.test", "powerstate", "false"),
					resource.TestCheckResourceAttrSet("vergeio_tenant_node.test", "id"),
					resource.TestCheckResourceAttrSet("vergeio_tenant_storage.test", "id"),
				),
			},
			{
				Config: online,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_tenant.test", "powerstate", "true"),
					resource.TestCheckResourceAttr("vergeio_tenant.test", "status", "online"),
				),
			},
			{
				Config: online,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			{
				Config: offline,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_tenant.test", "powerstate", "false"),
					resource.TestCheckResourceAttr("vergeio_tenant.test", "status", "offline"),
				),
			},
			{
				Config: online,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_tenant.test", "powerstate", "true"),
					resource.TestCheckResourceAttr("vergeio_tenant.test", "status", "online"),
				),
			},
			// Final step leaves powerstate=true so CheckDestroy exercises
			// node delete while the tenant is running (#195).
		},
	})
}

func testAccTenantPowerConfig(tenantName, nodeName string, tier int, power bool) string {
	if err := acctest.RequirePrefix(tenantName); err != nil {
		panic(err)
	}
	if err := acctest.RequirePrefix(nodeName); err != nil {
		panic(err)
	}
	return acctest.Config(fmt.Sprintf(`
resource "vergeio_tenant" "test" {
  name        = %q
  description = "acc power settle"
  password    = "Tf-acc-tenant-password1"
  powerstate  = %t
}

resource "vergeio_tenant_node" "test" {
  tenant_id = vergeio_tenant.test.id
  name      = %q
  cpu_cores = 2
  ram       = 2048
  enabled   = true
}

resource "vergeio_tenant_storage" "test" {
  tenant_id   = vergeio_tenant.test.id
  tier        = %d
  provisioned = 1073741824
}
`, tenantName, power, nodeName, tier))
}
