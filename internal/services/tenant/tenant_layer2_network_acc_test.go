package tenant_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/verge-io/govergeos"

	"terraform-provider-vergeio/internal/acctest"
)

// TestAccTenantLayer2Network bridges a tenant onto a parent network.
// The parent networks are internal so a lab can create them without a
// physical NIC. VergeOS 26.0 or later is required. An older cluster, or a
// cluster that refuses a layer 2 assignment on an internal network, fails
// this test at create.
func TestAccTenantLayer2Network(t *testing.T) {
	acctest.PreCheck(t)
	tenantName := acctest.Name("tenant-l2")
	networkName := acctest.Name("tenant-l2-net")
	otherName := acctest.Name("tenant-l2-net2")
	enabled := testAccTenantLayer2NetworkConfig(tenantName, networkName, otherName, true, false)
	disabled := testAccTenantLayer2NetworkConfig(tenantName, networkName, otherName, false, false)
	replaced := testAccTenantLayer2NetworkConfig(tenantName, networkName, otherName, true, true)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckTenantLayer2NetworkDestroy,
		Steps: []resource.TestStep{
			{
				Config: enabled,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_tenant_layer2_network.test", "enabled", "true"),
					resource.TestCheckResourceAttrSet("vergeio_tenant_layer2_network.test", "id"),
					resource.TestCheckResourceAttrPair("vergeio_tenant_layer2_network.test", "tenant_id", "vergeio_tenant.test", "id"),
					resource.TestCheckResourceAttrPair("vergeio_tenant_layer2_network.test", "network_id", "vergeio_network.parent", "id"),
				),
			},
			{
				Config: enabled,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			{
				Config: disabled,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vergeio_tenant_layer2_network.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.TestCheckResourceAttr("vergeio_tenant_layer2_network.test", "enabled", "false"),
			},
			{
				Config: enabled,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vergeio_tenant_layer2_network.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.TestCheckResourceAttr("vergeio_tenant_layer2_network.test", "enabled", "true"),
			},
			{
				Config: replaced,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vergeio_tenant_layer2_network.test", plancheck.ResourceActionReplace),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_tenant_layer2_network.test", "enabled", "true"),
					resource.TestCheckResourceAttrPair("vergeio_tenant_layer2_network.test", "network_id", "vergeio_network.other", "id"),
				),
			},
			{
				ResourceName:      "vergeio_tenant_layer2_network.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccCheckTenantLayer2NetworkDestroy(s *terraform.State) error {
	client, err := acctest.SDKClient()
	if err != nil {
		return err
	}
	if err := acctest.CheckDeletedMatching(s, "vergeio_tenant_layer2_network", func(ctx context.Context, id int, attrs map[string]string) error {
		got, err := client.TenantLayer2Networks.Get(ctx, id)
		if err != nil {
			return err
		}
		if !accAttrMatchesInt(attrs, "tenant_id", got.Tenant.Int()) {
			return &vergeos.NotFoundError{Resource: "TenantLayer2Network", ID: id}
		}
		if !accAttrMatchesInt(attrs, "network_id", got.VNet.Int()) {
			return &vergeos.NotFoundError{Resource: "TenantLayer2Network", ID: id}
		}
		return nil
	}); err != nil {
		return err
	}
	if err := testAccCheckTenantDestroy(s); err != nil {
		return err
	}
	return acctest.CheckDeleted(s, "vergeio_network", func(ctx context.Context, id int) error {
		_, err := client.Networks.Get(ctx, id)
		return err
	})
}

func testAccTenantLayer2NetworkConfig(tenantName, networkName, otherName string, enabled, useOther bool) string {
	if err := acctest.RequirePrefix(tenantName); err != nil {
		panic(err)
	}
	if err := acctest.RequirePrefix(networkName); err != nil {
		panic(err)
	}
	if err := acctest.RequirePrefix(otherName); err != nil {
		panic(err)
	}
	networkRef := "vergeio_network.parent.id"
	if useOther {
		networkRef = "vergeio_network.other.id"
	}
	return acctest.Config(fmt.Sprintf(`
resource "vergeio_network" "parent" {
  name           = %q
  type           = "internal"
  network        = "203.0.113.0/24"
  ipaddress      = "203.0.113.1"
  powerstate     = true
  interface_vnet = 0
}

resource "vergeio_network" "other" {
  name           = %q
  type           = "internal"
  network        = "198.51.100.0/24"
  ipaddress      = "198.51.100.1"
  powerstate     = true
  interface_vnet = 0
}

resource "vergeio_tenant" "test" {
  name       = %q
  password   = "Tf-acc-tenant-password1"
  powerstate = false
}

resource "vergeio_tenant_layer2_network" "test" {
  tenant_id  = vergeio_tenant.test.id
  network_id = %s
  enabled    = %t
}
`, networkName, otherName, tenantName, networkRef, enabled))
}
