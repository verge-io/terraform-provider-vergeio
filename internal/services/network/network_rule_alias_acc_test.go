package network_test

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/verge-io/govergeos"

	"terraform-provider-vergeio/internal/acctest"
)

// TestAccNetworkRule_AliasByID creates a rule that references an alias by
// id (alias:<key>). VergeOS rejects alias:<name>; the provider passes the
// value through unchanged, so documented usage must use the alias id.
func TestAccNetworkRule_AliasByID(t *testing.T) {
	networkName := acctest.Name("network")
	aliasName := acctest.Name("alias")
	config := testAccNetworkRuleAliasByIDConfig(networkName, aliasName, true)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNetworkAndAliasDestroy,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckNetworkExists("vergeio_network.test"),
					resource.TestCheckResourceAttrSet("vergeio_network_rule_alias.test", "id"),
					resource.TestCheckResourceAttrSet("vergeio_network_rule_alias.test", "alias_id"),
					resource.TestCheckResourceAttr("vergeio_network_rule_alias.test", "name", aliasName),
					resource.TestCheckResourceAttrSet("vergeio_network_rule.test", "id"),
					resource.TestCheckResourceAttr("vergeio_network_rule.test", "name", "allow-ssh"),
					testAccCheckRuleSourceIPIsAliasID("vergeio_network_rule.test", "vergeio_network_rule_alias.test"),
				),
			},
			{
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})
}

// TestAccNetworkRule_AliasByNameRejected shows VergeOS rejects alias:<name>.
// The provider passes the value through; there is no name-to-key lookup.
func TestAccNetworkRule_AliasByNameRejected(t *testing.T) {
	networkName := acctest.Name("network")
	aliasName := acctest.Name("alias")
	config := testAccNetworkRuleAliasByIDConfig(networkName, aliasName, false)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNetworkAndAliasDestroy,
		Steps: []resource.TestStep{
			{
				Config:      config,
				ExpectError: regexp.MustCompile(`(?i)Unable to (generate rule references|create rule reference)|No such file or directory|Error Saving Network Rule`),
			},
		},
	})
}

// TestAccNetworkRules_AliasByID owns the rule list with destination_ip set
// to alias:<id> and expects a clean second plan.
func TestAccNetworkRules_AliasByID(t *testing.T) {
	networkName := acctest.Name("network")
	aliasName := acctest.Name("alias")
	config := testAccNetworkRulesAliasByIDConfig(networkName, aliasName)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNetworkAndAliasDestroy,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckNetworkExists("vergeio_network.test"),
					resource.TestCheckResourceAttrSet("vergeio_network_rule_alias.test", "id"),
					resource.TestCheckResourceAttrSet("vergeio_network_rule_alias.test", "alias_id"),
					resource.TestCheckResourceAttr("vergeio_network_rules.test", "rule.#", "1"),
					resource.TestCheckResourceAttr("vergeio_network_rules.test", "rule.0.name", "allow-from-alias"),
					testAccCheckRuleDestinationIPIsAliasID("vergeio_network_rules.test", "vergeio_network_rule_alias.test"),
				),
			},
			{
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})
}

func testAccNetworkRuleAliasByIDConfig(networkName, aliasName string, byID bool) string {
	if err := acctest.RequirePrefix(networkName); err != nil {
		panic(err)
	}
	if err := acctest.RequirePrefix(aliasName); err != nil {
		panic(err)
	}
	ref := "alias:${vergeio_network_rule_alias.test.id}"
	if !byID {
		ref = "alias:${vergeio_network_rule_alias.test.name}"
	}
	return acctest.Config(fmt.Sprintf(`
resource "vergeio_network" "test" {
  name       = %q
  type       = "internal"
  enabled    = true
  powerstate = false
}

resource "vergeio_network_rule_alias" "test" {
  name  = %q
  value = "10.9.0.0/16"
}

resource "vergeio_network_rule" "test" {
  vnet              = vergeio_network.test.id
  name              = "allow-ssh"
  protocol          = "tcp"
  destination_ports = "22"
  source_ip         = %q
}
`, networkName, aliasName, ref))
}

func testAccNetworkRulesAliasByIDConfig(networkName, aliasName string) string {
	if err := acctest.RequirePrefix(networkName); err != nil {
		panic(err)
	}
	if err := acctest.RequirePrefix(aliasName); err != nil {
		panic(err)
	}
	return acctest.Config(fmt.Sprintf(`
resource "vergeio_network" "test" {
  name       = %q
  type       = "internal"
  enabled    = true
  powerstate = false
}

resource "vergeio_network_rule_alias" "test" {
  name  = %q
  value = "10.9.0.0/16"
}

resource "vergeio_network_rules" "test" {
  vnet = vergeio_network.test.id

  rule = [
    {
      name              = "allow-from-alias"
      protocol          = "tcp"
      destination_ports = "22"
      destination_ip    = "alias:${vergeio_network_rule_alias.test.id}"
    },
  ]
}
`, networkName, aliasName))
}

func testAccCheckRuleSourceIPIsAliasID(ruleResource, aliasResource string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		aliasID, err := testAccResourceAttr(s, aliasResource, "id")
		if err != nil {
			return err
		}
		got, err := testAccResourceAttr(s, ruleResource, "source_ip")
		if err != nil {
			return err
		}
		want := "alias:" + aliasID
		if got != want {
			return fmt.Errorf("%s source_ip = %q, want %q", ruleResource, got, want)
		}
		return nil
	}
}

func testAccCheckRuleDestinationIPIsAliasID(rulesResource, aliasResource string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		aliasID, err := testAccResourceAttr(s, aliasResource, "id")
		if err != nil {
			return err
		}
		got, err := testAccResourceAttr(s, rulesResource, "rule.0.destination_ip")
		if err != nil {
			return err
		}
		want := "alias:" + aliasID
		if got != want {
			return fmt.Errorf("%s rule.0.destination_ip = %q, want %q", rulesResource, got, want)
		}
		return nil
	}
}

func testAccCheckNetworkAndAliasDestroy(s *terraform.State) error {
	if err := testAccCheckNetworkDestroy(s); err != nil {
		return err
	}
	return testAccCheckNetworkRuleAliasDestroy(s)
}

func testAccCheckNetworkRuleAliasDestroy(s *terraform.State) error {
	client, err := acctest.SDKClient()
	if err != nil {
		return err
	}
	return acctest.CheckDeletedMatching(s, "vergeio_network_rule_alias", func(ctx context.Context, id int, attrs map[string]string) error {
		got, err := client.VNetRuleAliases.Get(ctx, id)
		if err != nil {
			return err
		}
		// alias_id is stronger than name: VergeOS can reuse the key for an
		// alias that happens to share the configured name (#228/#231).
		if aliasID := attrs["alias_id"]; aliasID != "" && got.ID != aliasID {
			return &vergeos.NotFoundError{Resource: "VNetRuleAlias", ID: id}
		}
		return nil
	})
}


// TestAccNetworkRuleAlias_RenameAsDrift covers #231: renaming an alias outside
// Terraform must plan an in-place name update, not abandon the row and create
// a duplicate under the configured name.
func TestAccNetworkRuleAlias_RenameAsDrift(t *testing.T) {
	acctest.PreCheck(t)
	aliasName := acctest.Name("alias-ren231")
	config := testAccNetworkRuleAliasOnlyConfig(aliasName)

	var managedKey int
	var managedAliasID string

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNetworkRuleAliasDestroy,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("vergeio_network_rule_alias.test", "id"),
					resource.TestCheckResourceAttrSet("vergeio_network_rule_alias.test", "alias_id"),
					resource.TestCheckResourceAttr("vergeio_network_rule_alias.test", "name", aliasName),
					func(s *terraform.State) error {
						rs, ok := s.RootModule().Resources["vergeio_network_rule_alias.test"]
						if !ok {
							return fmt.Errorf("missing vergeio_network_rule_alias.test")
						}
						id, err := strconv.Atoi(rs.Primary.ID)
						if err != nil || id <= 0 {
							return fmt.Errorf("alias id %q: %v", rs.Primary.ID, err)
						}
						managedKey = id
						managedAliasID = rs.Primary.Attributes["alias_id"]
						if managedAliasID == "" {
							return fmt.Errorf("alias_id empty")
						}
						return nil
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
					renamed := aliasName + "-renamed"
					if _, err := client.VNetRuleAliases.Update(ctx, managedKey, &vergeos.VNetRuleAliasUpdateRequest{
						Name: &renamed,
					}); err != nil {
						t.Fatalf("rename alias %d outside terraform: %v", managedKey, err)
					}
				},
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vergeio_network_rule_alias.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_network_rule_alias.test", "name", aliasName),
					resource.TestCheckResourceAttrSet("vergeio_network_rule_alias.test", "alias_id"),
					func(s *terraform.State) error {
						rs := s.RootModule().Resources["vergeio_network_rule_alias.test"]
						if rs.Primary.ID != strconv.Itoa(managedKey) {
							return fmt.Errorf("key changed from %d to %s (created duplicate?)", managedKey, rs.Primary.ID)
						}
						if rs.Primary.Attributes["alias_id"] != managedAliasID {
							return fmt.Errorf("alias_id changed from %s to %s", managedAliasID, rs.Primary.Attributes["alias_id"])
						}
						client, err := acctest.SDKClient()
						if err != nil {
							return err
						}
						got, err := client.VNetRuleAliases.Get(context.Background(), managedKey)
						if err != nil {
							return err
						}
						if got.Name != aliasName {
							return fmt.Errorf("alias name = %q, want restored %q", got.Name, aliasName)
						}
						if got.ID != managedAliasID {
							return fmt.Errorf("API alias id = %q, want %q", got.ID, managedAliasID)
						}
						return nil
					},
				),
			},
		},
	})
}

// TestAccNetworkRuleAlias_KeyReuseDoesNotAdoptForeign covers #227/#231: when
// VergeOS reuses a vnet_rule_aliases key after an outside delete, refresh must
// treat the foreign row as gone (plan create) instead of adopting it.
func TestAccNetworkRuleAlias_KeyReuseDoesNotAdoptForeign(t *testing.T) {
	acctest.PreCheck(t)
	aliasName := acctest.Name("alias-kr231")
	foreignName := acctest.Name("alias-kr231-f")
	config := testAccNetworkRuleAliasOnlyConfig(aliasName)

	var managedKey int
	var managedAliasID string
	var foreignKey int
	var keyReused bool

	t.Cleanup(func() {
		client, err := acctest.SDKClient()
		if err != nil || foreignKey <= 0 {
			return
		}
		_ = client.VNetRuleAliases.Delete(context.Background(), foreignKey)
	})

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNetworkRuleAliasDestroy,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("vergeio_network_rule_alias.test", "id"),
					resource.TestCheckResourceAttrSet("vergeio_network_rule_alias.test", "alias_id"),
					func(s *terraform.State) error {
						rs, ok := s.RootModule().Resources["vergeio_network_rule_alias.test"]
						if !ok {
							return fmt.Errorf("missing vergeio_network_rule_alias.test")
						}
						id, err := strconv.Atoi(rs.Primary.ID)
						if err != nil || id <= 0 {
							return fmt.Errorf("alias id %q: %v", rs.Primary.ID, err)
						}
						managedKey = id
						managedAliasID = rs.Primary.Attributes["alias_id"]
						if managedAliasID == "" {
							return fmt.Errorf("alias_id empty")
						}
						return nil
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
					if err := client.VNetRuleAliases.Delete(ctx, managedKey); err != nil && !vergeos.IsNotFoundError(err) {
						t.Fatalf("delete managed alias %d: %v", managedKey, err)
					}
					deadline := time.Now().Add(2 * time.Minute)
					for {
						_, err := client.VNetRuleAliases.Get(ctx, managedKey)
						if vergeos.IsNotFoundError(err) {
							break
						}
						if time.Now().After(deadline) {
							t.Fatalf("alias %d still present after delete: %v", managedKey, err)
						}
						time.Sleep(time.Second)
					}
					// Key reuse may need a short window after the row is gone.
					t.Logf("alias row gone; waiting 15s for key reuse window")
					time.Sleep(15 * time.Second)

					created, err := client.VNetRuleAliases.Create(ctx, &vergeos.VNetRuleAliasCreateRequest{
						Name:  foreignName,
						Value: "10.250.231.20",
					})
					if err != nil {
						t.Fatalf("create foreign alias: %v", err)
					}
					foreignKey = created.Key.Int()
					keyReused = foreignKey == managedKey
					t.Logf("managed alias key=%d alias_id=%s; foreign key=%d alias_id=%s reused=%v",
						managedKey, managedAliasID, foreignKey, created.ID, keyReused)
				},
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vergeio_network_rule_alias.test", plancheck.ResourceActionCreate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_network_rule_alias.test", "name", aliasName),
					resource.TestCheckResourceAttrSet("vergeio_network_rule_alias.test", "alias_id"),
					func(s *terraform.State) error {
						client, err := acctest.SDKClient()
						if err != nil {
							return err
						}
						ctx := context.Background()
						got, err := client.VNetRuleAliases.Get(ctx, foreignKey)
						if err != nil {
							return fmt.Errorf("foreign alias %d missing after apply (key reused=%v): %w", foreignKey, keyReused, err)
						}
						if got.Name != foreignName {
							return fmt.Errorf("foreign alias name = %q, want %q (adopted/renamed? reused=%v)", got.Name, foreignName, keyReused)
						}
						rs := s.RootModule().Resources["vergeio_network_rule_alias.test"]
						managedID, err := strconv.Atoi(rs.Primary.ID)
						if err != nil {
							return err
						}
						managedAID := rs.Primary.Attributes["alias_id"]
						if managedAID == "" {
							return fmt.Errorf("managed alias_id empty after recreate")
						}
						if managedAID == managedAliasID {
							return fmt.Errorf("managed alias kept old alias_id %s after outside delete", managedAliasID)
						}
						if keyReused && managedID == foreignKey {
							return fmt.Errorf("managed alias adopted foreign key %d", foreignKey)
						}
						if got.ID == managedAID {
							return fmt.Errorf("managed alias_id matches foreign alias_id %s", managedAID)
						}
						return nil
					},
				),
			},
		},
	})
}

func testAccNetworkRuleAliasOnlyConfig(aliasName string) string {
	if err := acctest.RequirePrefix(aliasName); err != nil {
		panic(err)
	}
	return acctest.Config(fmt.Sprintf(`
resource "vergeio_network_rule_alias" "test" {
  name  = %q
  value = "10.250.231.10"
}
`, aliasName))
}
