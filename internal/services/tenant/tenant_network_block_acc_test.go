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

// TestAccTenantNetworkBlock assigns one routed CIDR to a tenant.
// The parent network is internal so a lab can create it without a physical
// NIC. The provider still posts a vnet_cidrs row owned by the tenant, the
// same call used for a block on a parent external network, and reads
// need_fw_apply afterward. The apply step waits until that flag clears.
// VergeOS accepts the refresh before need_fw_apply drops.
func TestAccTenantNetworkBlock(t *testing.T) {
	acctest.PreCheck(t)
	tenantName := acctest.Name("tenant-block")
	networkName := acctest.Name("tenant-block-net")
	basic := testAccTenantNetworkBlockConfig(tenantName, networkName, "tenant block", false)
	renamed := testAccTenantNetworkBlockConfig(tenantName, networkName, "tenant block 2", false)
	applied := testAccTenantNetworkBlockConfig(tenantName, networkName, "tenant block 2", true)
	staged := testAccTenantNetworkBlockConfig(tenantName, networkName, "tenant block 2", false)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckTenantNetworkBlockDestroy,
		Steps: []resource.TestStep{
			{
				Config: basic,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_tenant_network_block.test", "cidr", "203.0.113.64/28"),
					resource.TestCheckResourceAttr("vergeio_tenant_network_block.test", "description", "tenant block"),
					resource.TestCheckResourceAttr("vergeio_tenant_network_block.test", "apply_parent_firewall", "false"),
					resource.TestCheckResourceAttr("vergeio_tenant_network_block.test", "parent_firewall_pending", "true"),
					resource.TestCheckResourceAttr("vergeio_tenant_network_block.test", "parent_firewall_applied", "false"),
					resource.TestCheckResourceAttrSet("vergeio_tenant_network_block.test", "id"),
					resource.TestCheckResourceAttrPair("vergeio_tenant_network_block.test", "tenant_id", "vergeio_tenant.test", "id"),
					resource.TestCheckResourceAttrPair("vergeio_tenant_network_block.test", "network_id", "vergeio_network.parent", "id"),
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
				Config: renamed,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vergeio_tenant_network_block.test", plancheck.ResourceActionReplace),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_tenant_network_block.test", "description", "tenant block 2"),
					resource.TestCheckResourceAttr("vergeio_tenant_network_block.test", "cidr", "203.0.113.64/28"),
				),
			},
			{
				Config: applied,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_tenant_network_block.test", "apply_parent_firewall", "true"),
					resource.TestCheckResourceAttr("vergeio_tenant_network_block.test", "parent_firewall_applied", "true"),
					resource.TestCheckResourceAttr("vergeio_tenant_network_block.test", "parent_firewall_pending", "false"),
				),
			},
			{
				Config: staged,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_tenant_network_block.test", "apply_parent_firewall", "false"),
					resource.TestCheckResourceAttr("vergeio_tenant_network_block.test", "parent_firewall_applied", "false"),
				),
			},
			{
				ResourceName:      "vergeio_tenant_network_block.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccCheckTenantNetworkBlockDestroy(s *terraform.State) error {
	client, err := acctest.SDKClient()
	if err != nil {
		return err
	}
	if err := acctest.CheckDeletedMatching(s, "vergeio_tenant_network_block", func(ctx context.Context, id int, attrs map[string]string) error {
		got, err := client.TenantNetworkBlocks.Get(ctx, id)
		if err != nil {
			return err
		}
		if !accAttrMatchesInt(attrs, "tenant_id", got.TenantKey()) {
			return &vergeos.NotFoundError{Resource: "TenantNetworkBlock", ID: id}
		}
		if cidr := attrs["cidr"]; cidr != "" && got.CIDR != cidr {
			return &vergeos.NotFoundError{Resource: "TenantNetworkBlock", ID: id}
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

func testAccTenantNetworkBlockConfig(tenantName, networkName, description string, apply bool) string {
	if err := acctest.RequirePrefix(tenantName); err != nil {
		panic(err)
	}
	if err := acctest.RequirePrefix(networkName); err != nil {
		panic(err)
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

resource "vergeio_tenant" "test" {
  name       = %q
  password   = "Tf-acc-tenant-password1"
  powerstate = false
}

resource "vergeio_tenant_network_block" "test" {
  tenant_id             = vergeio_tenant.test.id
  network_id            = vergeio_network.parent.id
  cidr                  = "203.0.113.64/28"
  description           = %q
  apply_parent_firewall = %t
}
`, networkName, tenantName, description, apply))
}
