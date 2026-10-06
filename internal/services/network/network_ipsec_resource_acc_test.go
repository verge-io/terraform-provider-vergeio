package network_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"terraform-provider-vergeio/internal/acctest"
)

func TestAccNetworkIPSec_connection(t *testing.T) {
	networkName := acctest.Name("ipsec-net")
	basic := testAccNetworkIPSecConfig(networkName, "198.51.100.0/24")
	updated := testAccNetworkIPSecConfig(networkName, "198.51.100.0/25")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNetworkIPSecDestroy,
		Steps: []resource.TestStep{
			{
				Config: basic,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckNetworkExists("vergeio_network.test"),
					resource.TestCheckResourceAttrSet("vergeio_network_ipsec.test", "id"),
					resource.TestCheckResourceAttr("vergeio_network_ipsec.test", "enabled", "true"),
					resource.TestCheckResourceAttr("vergeio_network_ipsec.test", "mode", "normal"),
					resource.TestCheckResourceAttr("vergeio_network_ipsec_connection.test", "name", "branch"),
					resource.TestCheckResourceAttr("vergeio_network_ipsec_connection.test", "remote_gateway", "203.0.113.10"),
					resource.TestCheckResourceAttr("vergeio_network_ipsec_connection.test", "ike", "aes256-sha256-modp2048"),
					resource.TestCheckResourceAttr("vergeio_network_ipsec_connection.test", "phase2.name", "lan"),
					resource.TestCheckResourceAttr("vergeio_network_ipsec_connection.test", "phase2.local", "192.168.0.0/24"),
					resource.TestCheckResourceAttr("vergeio_network_ipsec_connection.test", "phase2.remote", "198.51.100.0/24"),
					resource.TestCheckResourceAttrSet("vergeio_network_ipsec_connection.test", "id"),
					resource.TestCheckResourceAttrSet("vergeio_network_ipsec_connection.test", "phase2.id"),
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
					resource.TestCheckResourceAttr("vergeio_network_ipsec_connection.test", "phase2.remote", "198.51.100.0/25"),
				),
			},
			{
				ResourceName:            "vergeio_network_ipsec.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"strongswan_conf", "ipsec_conf", "ipsec_secrets", "cisco_unity", "accept_unencrypted_mainmode", "mss_clamp", "make_before_break"},
			},
			{
				ResourceName:            "vergeio_network_ipsec_connection.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"psk"},
			},
		},
	})
}

func testAccCheckNetworkIPSecDestroy(s *terraform.State) error {
	client, err := acctest.SDKClient()
	if err != nil {
		return err
	}
	if err := acctest.CheckDeleted(s, "vergeio_network_ipsec_connection", func(ctx context.Context, id int) error {
		_, err := client.VNetIPSecPhase1s.Get(ctx, id)
		return err
	}); err != nil {
		return err
	}
	if err := acctest.CheckDeleted(s, "vergeio_network_ipsec", func(ctx context.Context, id int) error {
		_, err := client.VNetIPSecs.Get(ctx, id)
		return err
	}); err != nil {
		return err
	}
	return acctest.CheckDeleted(s, "vergeio_network", func(ctx context.Context, id int) error {
		_, err := client.Networks.Get(ctx, id)
		return err
	})
}

func testAccNetworkIPSecConfig(networkName, remote string) string {
	if err := acctest.RequirePrefix(networkName); err != nil {
		panic(err)
	}
	return acctest.Config(fmt.Sprintf(`
resource "vergeio_network" "test" {
  name    = %q
  type    = "internal"
  enabled = true
}

resource "vergeio_network_ipsec" "test" {
  network_id = vergeio_network.test.id
  enabled    = true
  mode       = "normal"
}

resource "vergeio_network_ipsec_connection" "test" {
  ipsec_id       = vergeio_network_ipsec.test.id
  name           = "branch"
  remote_gateway = "203.0.113.10"
  keyexchange    = "ikev2"
  auth           = "psk"
  ike            = "aes256-sha256-modp2048"
  psk            = "tf-acc-ipsec-preshared-key"
  auto           = "start"

  phase2 {
    name     = "lan"
    local    = "192.168.0.0/24"
    remote   = %q
    protocol = "esp"
  }
}
`, networkName, remote))
}
