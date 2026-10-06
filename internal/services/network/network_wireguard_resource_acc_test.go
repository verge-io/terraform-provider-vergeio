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

func TestAccNetworkWireGuard_peer(t *testing.T) {
	networkName := acctest.Name("wg-net")
	basic := testAccNetworkWireGuardConfig(networkName, "10.0.0.0/24")
	updated := testAccNetworkWireGuardConfig(networkName, "10.0.0.0/24,10.1.0.0/24")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNetworkWireGuardDestroy,
		Steps: []resource.TestStep{
			{
				Config: basic,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckNetworkExists("vergeio_network.test"),
					resource.TestCheckResourceAttr("vergeio_network_wireguard.test", "name", "wg0"),
					resource.TestCheckResourceAttr("vergeio_network_wireguard.test", "ip", "192.168.255.1/24"),
					resource.TestCheckResourceAttr("vergeio_network_wireguard.test", "apply", "true"),
					resource.TestCheckResourceAttrSet("vergeio_network_wireguard.test", "id"),
					resource.TestCheckResourceAttrSet("vergeio_network_wireguard.test", "public_key"),
					resource.TestCheckResourceAttr("vergeio_network_wireguard_peer.test", "name", "office"),
					resource.TestCheckResourceAttr("vergeio_network_wireguard_peer.test", "allowed_ips", "10.0.0.0/24"),
					resource.TestCheckResourceAttr("vergeio_network_wireguard_peer.test", "configure_firewall", "site-to-site"),
					resource.TestCheckResourceAttr("vergeio_network_wireguard_peer.test", "apply", "true"),
					resource.TestCheckResourceAttrSet("vergeio_network_wireguard_peer.test", "id"),
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
					resource.TestCheckResourceAttr("vergeio_network_wireguard_peer.test", "allowed_ips", "10.0.0.0/24,10.1.0.0/24"),
				),
			},
			{
				ResourceName:            "vergeio_network_wireguard.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"private_key", "configure_firewall", "external_ip"},
			},
			{
				ResourceName:            "vergeio_network_wireguard_peer.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"preshared_key", "peer_config"},
			},
		},
	})
}

func testAccCheckNetworkWireGuardDestroy(s *terraform.State) error {
	client, err := acctest.SDKClient()
	if err != nil {
		return err
	}
	if err := acctest.CheckDeleted(s, "vergeio_network_wireguard_peer", func(ctx context.Context, id int) error {
		_, err := client.VNetWireGuardPeers.Get(ctx, id)
		return err
	}); err != nil {
		return err
	}
	if err := acctest.CheckDeleted(s, "vergeio_network_wireguard", func(ctx context.Context, id int) error {
		_, err := client.VNetWireGuards.Get(ctx, id)
		return err
	}); err != nil {
		return err
	}
	return acctest.CheckDeleted(s, "vergeio_network", func(ctx context.Context, id int) error {
		_, err := client.Networks.Get(ctx, id)
		return err
	})
}

func testAccNetworkWireGuardConfig(networkName, allowed string) string {
	if err := acctest.RequirePrefix(networkName); err != nil {
		panic(err)
	}
	return acctest.Config(fmt.Sprintf(`
resource "vergeio_network" "test" {
  name       = %q
  type       = "internal"
  enabled    = true
  powerstate = true
}

resource "vergeio_network_wireguard" "test" {
  network_id  = vergeio_network.test.id
  name        = "wg0"
  ip          = "192.168.255.1/24"
  listen_port = 51820
  apply       = true
}

resource "vergeio_network_wireguard_peer" "test" {
  wireguard_id       = vergeio_network_wireguard.test.id
  name               = "office"
  peer_ip            = "192.168.255.2"
  public_key         = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
  allowed_ips        = %q
  configure_firewall = "site-to-site"
  endpoint           = "203.0.113.20"
  port               = 51820
  keepalive          = 25
  apply              = true
}
`, networkName, allowed))
}
