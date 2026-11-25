package network_test

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/verge-io/govergeos"

	"terraform-provider-vergeio/internal/acctest"
)

// TestAccNetworkIPSec_ParentNetworkGoneRecovers covers #323. Deleting the
// network outside Terraform leaves the IPsec rows. Refresh drops the IPsec
// resources, apply recreates them, and the new network has one IPsec tree.
func TestAccNetworkIPSec_ParentNetworkGoneRecovers(t *testing.T) {
	networkName := acctest.Name("ipsec-gone")
	config := testAccNetworkIPSecConfig(networkName, "198.51.100.0/24")

	var networkID, ipsecID, phase1ID int

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNetworkIPSecDestroy,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("vergeio_network.test", "id"),
					resource.TestCheckResourceAttrSet("vergeio_network_ipsec.test", "id"),
					resource.TestCheckResourceAttrSet("vergeio_network_ipsec_connection.test", "id"),
					func(s *terraform.State) error {
						var err error
						networkID, err = acctest.ResourceID(s, "vergeio_network.test")
						if err != nil {
							return err
						}
						ipsecID, err = acctest.ResourceID(s, "vergeio_network_ipsec.test")
						if err != nil {
							return err
						}
						phase1ID, err = acctest.ResourceID(s, "vergeio_network_ipsec_connection.test")
						return err
					},
				),
			},
			{
				PreConfig: func() {
					client, err := acctest.SDKClient()
					if err != nil {
						t.Fatalf("sdk client: %v", err)
					}
					ctx := context.Background()
					if err := client.Networks.Delete(ctx, networkID); err != nil && !vergeos.IsNotFoundError(err) {
						t.Fatalf("delete network %d: %v", networkID, err)
					}
					acctest.WaitUntilMissing(t, func() error {
						_, err := client.Networks.Get(ctx, networkID)
						return err
					}, fmt.Sprintf("network %d", networkID))
					t.Logf("deleted network %d outside Terraform; ipsec %d phase1 %d left for the provider", networkID, ipsecID, phase1ID)
				},
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vergeio_network.test", plancheck.ResourceActionCreate),
						plancheck.ExpectResourceAction("vergeio_network_ipsec.test", plancheck.ResourceActionCreate),
						plancheck.ExpectResourceAction("vergeio_network_ipsec_connection.test", plancheck.ResourceActionCreate),
					},
				},
				Check: func(s *terraform.State) error {
					newNetwork, err := acctest.ResourceID(s, "vergeio_network.test")
					if err != nil {
						return err
					}
					newIPSec, err := acctest.ResourceID(s, "vergeio_network_ipsec.test")
					if err != nil {
						return err
					}
					newPhase1, err := acctest.ResourceID(s, "vergeio_network_ipsec_connection.test")
					if err != nil {
						return err
					}
					return assertOneIPSecTree(newNetwork, newIPSec, newPhase1)
				},
			},
		},
	})
}

func assertOneIPSecTree(networkID, ipsecID, phase1ID int) error {
	client, err := acctest.SDKClient()
	if err != nil {
		return err
	}
	ctx := context.Background()
	rows, err := client.VNetIPSecs.List(ctx, vergeos.WithFilter(fmt.Sprintf("vnet eq %d", networkID)))
	if err != nil {
		return err
	}
	if len(rows) != 1 {
		return fmt.Errorf("network %d has %d IPsec configurations, want 1", networkID, len(rows))
	}
	if rows[0].Key.Int() != ipsecID {
		return fmt.Errorf("IPsec on network %d is %d, state has %d", networkID, rows[0].Key.Int(), ipsecID)
	}
	phase1s, err := client.VNetIPSecPhase1s.ListByIPSec(ctx, ipsecID)
	if err != nil {
		return err
	}
	if len(phase1s) != 1 || phase1s[0].Key.Int() != phase1ID || phase1s[0].Name != "branch" {
		return fmt.Errorf("phase 1 rows = %#v, want one branch row %d", phase1s, phase1ID)
	}
	phase2s, err := client.VNetIPSecPhase2s.ListByPhase1(ctx, phase1ID)
	if err != nil {
		return err
	}
	if len(phase2s) != 1 || phase2s[0].Name != "lan" {
		return fmt.Errorf("phase 2 rows = %#v, want one lan row", phase2s)
	}
	return nil
}

// TestAccNetworkIPSec_KeyReuseDoesNotAdoptForeign covers a reused IPsec key
// that now belongs to another network. vnet_ipsecs has no created column on
// the pinned client, so the parent network is the identity.
func TestAccNetworkIPSec_KeyReuseDoesNotAdoptForeign(t *testing.T) {
	name := acctest.Name("ipsec-kr")
	foreignNet := acctest.Name("ipsec-kr-net")
	config := testAccNetworkIPSecOnly(name)

	var managedKey int
	var foreignKey int
	var foreignNetworkID int
	var keyReused bool

	t.Cleanup(func() {
		cleanupForeignNetwork(foreignNetworkID)
	})

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNetworkIPSecDestroy,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: func(s *terraform.State) error {
					id, err := acctest.ResourceID(s, "vergeio_network_ipsec.test")
					managedKey = id
					return err
				},
			},
			{
				PreConfig: func() {
					client, err := acctest.SDKClient()
					if err != nil {
						t.Fatalf("sdk client: %v", err)
					}
					ctx := context.Background()
					if err := client.VNetIPSecs.Delete(ctx, managedKey); err != nil && !vergeos.IsNotFoundError(err) {
						t.Fatalf("delete managed IPsec %d: %v", managedKey, err)
					}
					acctest.WaitUntilMissing(t, func() error {
						_, err := client.VNetIPSecs.Get(ctx, managedKey)
						return err
					}, fmt.Sprintf("IPsec %d", managedKey))
					foreignNetworkID, foreignKey, keyReused = createForeignIPSec(t, client, foreignNet, managedKey)
				},
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vergeio_network_ipsec.test", plancheck.ResourceActionCreate),
					},
				},
				Check: func(s *terraform.State) error {
					return foreignIPSecUntouched(s, foreignKey, foreignNetworkID, keyReused)
				},
			},
		},
	})
}

// TestAccNetworkIPSec_KeyReuseDestroyRefreshOff covers destroy without
// refresh. The row at the stored key is left alone when it belongs to
// another network.
func TestAccNetworkIPSec_KeyReuseDestroyRefreshOff(t *testing.T) {
	name := acctest.Name("ipsec-kr322")
	foreignNet := acctest.Name("ipsec-kr322-net")
	config := testAccNetworkIPSecOnly(name)

	var managedKey int
	var foreignKey int
	var foreignNetworkID int
	var keyReused bool

	t.Cleanup(func() {
		cleanupForeignNetwork(foreignNetworkID)
	})

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		AdditionalCLIOptions:     acctest.PlanWithoutRefresh(),
		CheckDestroy:             testAccCheckNetworkIPSecDestroy,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: func(s *terraform.State) error {
					id, err := acctest.ResourceID(s, "vergeio_network_ipsec.test")
					managedKey = id
					return err
				},
			},
			{
				PreConfig: func() {
					client, err := acctest.SDKClient()
					if err != nil {
						t.Fatalf("sdk client: %v", err)
					}
					ctx := context.Background()
					if err := client.VNetIPSecs.Delete(ctx, managedKey); err != nil && !vergeos.IsNotFoundError(err) {
						t.Fatalf("delete managed IPsec %d: %v", managedKey, err)
					}
					acctest.WaitUntilMissing(t, func() error {
						_, err := client.VNetIPSecs.Get(ctx, managedKey)
						return err
					}, fmt.Sprintf("IPsec %d", managedKey))
					foreignNetworkID, foreignKey, keyReused = createForeignIPSec(t, client, foreignNet, managedKey)
				},
				Config: acctest.Config(""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vergeio_network_ipsec.test", plancheck.ResourceActionDestroy),
					},
				},
				Check: func(*terraform.State) error {
					client, err := acctest.SDKClient()
					if err != nil {
						return err
					}
					got, err := client.VNetIPSecs.Get(context.Background(), foreignKey)
					if err != nil {
						return fmt.Errorf("foreign IPsec %d missing after destroy -refresh=false (key reused=%v): %w", foreignKey, keyReused, err)
					}
					if got.VNet.Int() != foreignNetworkID {
						return fmt.Errorf("foreign IPsec vnet = %d, want %d (key reused=%v)", got.VNet.Int(), foreignNetworkID, keyReused)
					}
					cleanupForeignNetwork(foreignNetworkID)
					foreignNetworkID = 0
					return nil
				},
			},
		},
	})
}

// TestAccNetworkIPSecConnection_KeyReuseDoesNotAdoptForeign covers a reused
// phase 1 key that now belongs to another IPsec configuration.
func TestAccNetworkIPSecConnection_KeyReuseDoesNotAdoptForeign(t *testing.T) {
	name := acctest.Name("ipsec-conn-kr")
	foreignName := acctest.Name("ipsec-conn-kr-f")
	config := testAccNetworkIPSecConfig(name, "198.51.100.0/24")

	var managedKey int
	var phase2Key int
	var foreignKey int
	var foreignNetworkID int
	var keyReused bool

	t.Cleanup(func() {
		cleanupForeignNetwork(foreignNetworkID)
	})

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNetworkIPSecDestroy,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: func(s *terraform.State) error {
					id, err := acctest.ResourceID(s, "vergeio_network_ipsec_connection.test")
					if err != nil {
						return err
					}
					managedKey = id
					phase2Key, err = phase2StateID(s)
					return err
				},
			},
			{
				PreConfig: func() {
					client, err := acctest.SDKClient()
					if err != nil {
						t.Fatalf("sdk client: %v", err)
					}
					ctx := context.Background()
					if err := client.VNetIPSecPhase2s.Delete(ctx, phase2Key); err != nil && !vergeos.IsNotFoundError(err) {
						t.Fatalf("delete phase 2 %d: %v", phase2Key, err)
					}
					if err := client.VNetIPSecPhase1s.Delete(ctx, managedKey); err != nil && !vergeos.IsNotFoundError(err) {
						t.Fatalf("delete phase 1 %d: %v", managedKey, err)
					}
					acctest.WaitUntilMissing(t, func() error {
						_, err := client.VNetIPSecPhase1s.Get(ctx, managedKey)
						return err
					}, fmt.Sprintf("phase 1 %d", managedKey))
					foreignNetworkID, foreignKey, keyReused = createForeignPhase1(t, client, foreignName, managedKey)
				},
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vergeio_network_ipsec_connection.test", plancheck.ResourceActionCreate),
					},
				},
				Check: func(s *terraform.State) error {
					if err := acctest.ForeignUntouched(s, "vergeio_network_ipsec_connection.test", foreignKey, keyReused, func(ctx context.Context) (string, int, error) {
						client, err := acctest.SDKClient()
						if err != nil {
							return "", 0, err
						}
						got, err := client.VNetIPSecPhase1s.Get(ctx, foreignKey)
						if err != nil {
							return "", 0, err
						}
						return got.Name, got.Key.Int(), nil
					}, foreignName); err != nil {
						return err
					}
					return nil
				},
			},
		},
	})
}

// TestAccNetworkIPSecConnection_KeyReuseDestroyRefreshOff covers destroy
// without refresh for a phase 1 key that now belongs to another IPsec
// configuration. The foreign phase 1 and its phase 2 rows are left alone.
func TestAccNetworkIPSecConnection_KeyReuseDestroyRefreshOff(t *testing.T) {
	name := acctest.Name("ipsec-conn-kr322")
	foreignName := acctest.Name("ipsec-conn-kr322-f")
	config := testAccNetworkIPSecConfig(name, "198.51.100.0/24")

	var managedKey int
	var phase2Key int
	var foreignKey int
	var foreignNetworkID int
	var keyReused bool

	t.Cleanup(func() {
		cleanupForeignNetwork(foreignNetworkID)
	})

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		AdditionalCLIOptions:     acctest.PlanWithoutRefresh(),
		CheckDestroy:             testAccCheckNetworkIPSecDestroy,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: func(s *terraform.State) error {
					id, err := acctest.ResourceID(s, "vergeio_network_ipsec_connection.test")
					if err != nil {
						return err
					}
					managedKey = id
					phase2Key, err = phase2StateID(s)
					return err
				},
			},
			{
				PreConfig: func() {
					client, err := acctest.SDKClient()
					if err != nil {
						t.Fatalf("sdk client: %v", err)
					}
					ctx := context.Background()
					if err := client.VNetIPSecPhase2s.Delete(ctx, phase2Key); err != nil && !vergeos.IsNotFoundError(err) {
						t.Fatalf("delete phase 2 %d: %v", phase2Key, err)
					}
					if err := client.VNetIPSecPhase1s.Delete(ctx, managedKey); err != nil && !vergeos.IsNotFoundError(err) {
						t.Fatalf("delete phase 1 %d: %v", managedKey, err)
					}
					acctest.WaitUntilMissing(t, func() error {
						_, err := client.VNetIPSecPhase1s.Get(ctx, managedKey)
						return err
					}, fmt.Sprintf("phase 1 %d", managedKey))
					foreignNetworkID, foreignKey, keyReused = createForeignPhase1(t, client, foreignName, managedKey)
				},
				Config: acctest.Config(""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vergeio_network_ipsec_connection.test", plancheck.ResourceActionDestroy),
					},
				},
				Check: func(*terraform.State) error {
					if err := acctest.ForeignPresent(foreignKey, foreignName, keyReused, func(ctx context.Context) (string, error) {
						client, err := acctest.SDKClient()
						if err != nil {
							return "", err
						}
						got, err := client.VNetIPSecPhase1s.Get(ctx, foreignKey)
						if err != nil {
							return "", err
						}
						return got.Name, nil
					}); err != nil {
						return err
					}
					cleanupForeignNetwork(foreignNetworkID)
					foreignNetworkID = 0
					return nil
				},
			},
		},
	})
}

func testAccNetworkIPSecOnly(networkName string) string {
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
`, networkName))
}

func phase2StateID(s *terraform.State) (int, error) {
	rs, ok := s.RootModule().Resources["vergeio_network_ipsec_connection.test"]
	if !ok {
		return 0, fmt.Errorf("missing vergeio_network_ipsec_connection.test")
	}
	raw := rs.Primary.Attributes["phase2.id"]
	id, err := strconv.Atoi(raw)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("phase2.id %q: %v", raw, err)
	}
	return id, nil
}

func createForeignIPSec(t *testing.T, client *vergeos.Client, networkName string, managedKey int) (int, int, bool) {
	t.Helper()
	ctx := context.Background()
	enabled := true
	network, err := client.Networks.Create(ctx, &vergeos.NetworkCreateRequest{
		Name:    networkName,
		Enabled: &enabled,
	})
	if err != nil {
		t.Fatalf("create foreign network: %v", err)
	}
	created, err := client.VNetIPSecs.Create(ctx, &vergeos.VNetIPSecCreateRequest{VNet: network.Key.Int()})
	if err != nil {
		_ = client.Networks.Delete(ctx, network.Key.Int())
		t.Fatalf("create foreign IPsec: %v", err)
	}
	keyReused := created.Key.Int() == managedKey
	t.Logf("managed IPsec key=%d; foreign key=%d network=%d reused=%v", managedKey, created.Key.Int(), network.Key.Int(), keyReused)
	return network.Key.Int(), created.Key.Int(), keyReused
}

func createForeignPhase1(t *testing.T, client *vergeos.Client, name string, managedKey int) (int, int, bool) {
	t.Helper()
	ctx := context.Background()
	networkID, ipsecID, _ := createForeignIPSec(t, client, name+"-net", 0)
	ike := "aes256-sha256-modp2048"
	created, err := client.VNetIPSecPhase1s.Create(ctx, &vergeos.VNetIPSecPhase1CreateRequest{
		IPSec:         ipsecID,
		Name:          name,
		RemoteGateway: "203.0.113.20",
		IKE:           &ike,
	})
	if err != nil {
		cleanupForeignNetwork(networkID)
		t.Fatalf("create foreign phase 1: %v", err)
	}
	keyReused := created.Key.Int() == managedKey
	t.Logf("managed phase 1 key=%d; foreign key=%d ipsec=%d reused=%v", managedKey, created.Key.Int(), ipsecID, keyReused)
	return networkID, created.Key.Int(), keyReused
}

func foreignIPSecUntouched(s *terraform.State, foreignKey, foreignNetworkID int, keyReused bool) error {
	client, err := acctest.SDKClient()
	if err != nil {
		return err
	}
	got, err := client.VNetIPSecs.Get(context.Background(), foreignKey)
	if err != nil {
		return fmt.Errorf("foreign IPsec %d missing after apply (key reused=%v): %w", foreignKey, keyReused, err)
	}
	if got.VNet.Int() != foreignNetworkID {
		return fmt.Errorf("foreign IPsec vnet = %d, want %d (key reused=%v)", got.VNet.Int(), foreignNetworkID, keyReused)
	}
	managedID, err := acctest.ResourceID(s, "vergeio_network_ipsec.test")
	if err != nil {
		return err
	}
	if keyReused && managedID == foreignKey {
		return fmt.Errorf("managed IPsec adopted foreign key %d", foreignKey)
	}
	if got.Key.Int() == managedID {
		return fmt.Errorf("managed id %d matches foreign id", managedID)
	}
	return nil
}

func cleanupForeignNetwork(networkID int) {
	if networkID <= 0 {
		return
	}
	client, err := acctest.SDKClient()
	if err != nil {
		return
	}
	ctx := context.Background()
	rows, err := client.VNetIPSecs.List(ctx, vergeos.WithFilter(fmt.Sprintf("vnet eq %d", networkID)))
	if err == nil {
		for _, row := range rows {
			phase1s, listErr := client.VNetIPSecPhase1s.ListByIPSec(ctx, row.Key.Int())
			if listErr == nil {
				for _, phase1 := range phase1s {
					phase2s, phase2Err := client.VNetIPSecPhase2s.ListByPhase1(ctx, phase1.Key.Int())
					if phase2Err == nil {
						for _, phase2 := range phase2s {
							_ = client.VNetIPSecPhase2s.Delete(ctx, phase2.Key.Int())
						}
					}
					_ = client.VNetIPSecPhase1s.Delete(ctx, phase1.Key.Int())
				}
			}
			_ = client.VNetIPSecs.Delete(ctx, row.Key.Int())
		}
	}
	_ = client.Networks.Delete(ctx, networkID)
}
