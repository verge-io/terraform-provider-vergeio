package network_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

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
	ref := fmt.Sprintf("alias:${vergeio_network_rule_alias.test.id}")
	if !byID {
		ref = fmt.Sprintf("alias:${vergeio_network_rule_alias.test.name}")
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
	client, err := acctest.SDKClient()
	if err != nil {
		return err
	}
	return acctest.CheckDeleted(s, "vergeio_network_rule_alias", func(ctx context.Context, id int) error {
		_, err := client.VNetRuleAliases.Get(ctx, id)
		return err
	})
}
