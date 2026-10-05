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

// TestAccTenantExternalIP assigns one virtual IP to a tenant.
// The parent network is internal so a lab can create it without a physical
// NIC. The provider still sends a virtual address owned by the tenant, the
// same call used for a parent external IP, and reads need_fw_apply afterward.
func TestAccTenantExternalIP(t *testing.T) {
	acctest.PreCheck(t)
	tenantName := acctest.Name("tenant-ip")
	networkName := acctest.Name("tenant-ip-net")
	renamedHost := tenantName + "-2"
	basic := testAccTenantExternalIPConfig(tenantName, networkName, tenantName, "tenant ui", false)
	renamed := testAccTenantExternalIPConfig(tenantName, networkName, renamedHost, "tenant ui", false)
	applied := testAccTenantExternalIPConfig(tenantName, networkName, renamedHost, "tenant ui", true)
	staged := testAccTenantExternalIPConfig(tenantName, networkName, renamedHost, "tenant ui", false)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckTenantExternalIPDestroy,
		Steps: []resource.TestStep{
			{
				Config: basic,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_tenant_external_ip.test", "ip", "203.0.113.50"),
					resource.TestCheckResourceAttr("vergeio_tenant_external_ip.test", "hostname", tenantName),
					resource.TestCheckResourceAttr("vergeio_tenant_external_ip.test", "description", "tenant ui"),
					resource.TestCheckResourceAttr("vergeio_tenant_external_ip.test", "apply_parent_firewall", "false"),
					resource.TestCheckResourceAttr("vergeio_tenant_external_ip.test", "parent_firewall_pending", "true"),
					resource.TestCheckResourceAttr("vergeio_tenant_external_ip.test", "parent_firewall_applied", "false"),
					resource.TestCheckResourceAttrSet("vergeio_tenant_external_ip.test", "id"),
					resource.TestCheckResourceAttrPair("vergeio_tenant_external_ip.test", "tenant_id", "vergeio_tenant.test", "id"),
					resource.TestCheckResourceAttrPair("vergeio_tenant_external_ip.test", "network_id", "vergeio_network.parent", "id"),
				),
			},
			{
				Config: basic,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
				// The tenant is created before the address, so ui_address is
				// stored on the next refresh, once VergeOS has made this first
				// IP the UI address.
				Check: resource.TestCheckResourceAttr("vergeio_tenant.test", "ui_address", "203.0.113.50"),
			},
			{
				Config: renamed,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vergeio_tenant_external_ip.test", plancheck.ResourceActionReplace),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_tenant_external_ip.test", "hostname", renamedHost),
					resource.TestCheckResourceAttr("vergeio_tenant_external_ip.test", "ip", "203.0.113.50"),
				),
			},
			{
				Config: applied,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_tenant_external_ip.test", "apply_parent_firewall", "true"),
					resource.TestCheckResourceAttr("vergeio_tenant_external_ip.test", "parent_firewall_applied", "true"),
					resource.TestCheckResourceAttr("vergeio_tenant_external_ip.test", "parent_firewall_pending", "false"),
				),
			},
			{
				Config: staged,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_tenant_external_ip.test", "apply_parent_firewall", "false"),
					resource.TestCheckResourceAttr("vergeio_tenant_external_ip.test", "parent_firewall_applied", "false"),
				),
			},
			{
				ResourceName:      "vergeio_tenant_external_ip.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccCheckTenantExternalIPDestroy(s *terraform.State) error {
	client, err := acctest.SDKClient()
	if err != nil {
		return err
	}
	if err := acctest.CheckDeletedMatching(s, "vergeio_tenant_external_ip", func(ctx context.Context, id int, attrs map[string]string) error {
		got, err := client.TenantExternalIPs.Get(ctx, id)
		if err != nil {
			return err
		}
		if !accAttrMatchesInt(attrs, "tenant_id", got.TenantKey()) {
			return &vergeos.NotFoundError{Resource: "TenantExternalIP", ID: id}
		}
		if ip := attrs["ip"]; ip != "" && got.IP != ip {
			return &vergeos.NotFoundError{Resource: "TenantExternalIP", ID: id}
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

func testAccTenantExternalIPConfig(tenantName, networkName, hostname, description string, apply bool) string {
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

resource "vergeio_tenant_external_ip" "test" {
  tenant_id             = vergeio_tenant.test.id
  network_id            = vergeio_network.parent.id
  ip                    = "203.0.113.50"
  hostname              = %q
  description           = %q
  apply_parent_firewall = %t
}
`, networkName, tenantName, hostname, description, apply))
}
