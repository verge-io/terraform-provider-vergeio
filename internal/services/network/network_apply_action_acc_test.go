// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package network_test

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"terraform-provider-vergeio/internal/acctest"
)

// TestAccNetworkApplyAction stages a firewall rule with apply = false, then
// refreshes the running network from an action trigger. The trigger is on
// terraform_data so the network resource is not updated. Skipped without
// TF_ACC and on OpenTofu.
func TestAccNetworkApplyAction(t *testing.T) {
	acctest.RequireActions(t)
	networkName := acctest.Name("network-apply")
	staged := testAccNetworkApplyConfig(networkName, "staged", false)
	applied := testAccNetworkApplyConfig(networkName, "applied", true)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.RequireActions(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNetworkDestroy,
		Steps: []resource.TestStep{
			{
				Config: staged,
				Check:  testAccNetworkNeedFWApply("vergeio_network.test", true),
			},
			{
				Config: applied,
				Check:  testAccNetworkNeedFWApply("vergeio_network.test", false),
			},
		},
	})
}

func TestNetworkApplyAcceptanceConfigParses(t *testing.T) {
	for _, withAction := range []bool{false, true} {
		src := testAccNetworkApplyConfig("tf-acc-network", "tick", withAction)
		if _, diags := hclsyntax.ParseConfig([]byte(src), "acc.tf", hcl.InitialPos); diags.HasErrors() {
			t.Fatalf("acceptance config did not parse: %s\n%s", diags.Error(), src)
		}
	}
}

func testAccNetworkApplyConfig(networkName, tick string, withAction bool) string {
	if err := acctest.RequirePrefix(networkName); err != nil {
		panic(err)
	}
	lifecycle := ""
	actionBlock := ""
	if withAction {
		lifecycle = `
  lifecycle {
    action_trigger {
      events  = [before_update]
      actions = [action.vergeio_network_apply.rules]
    }
  }`
		actionBlock = `
action "vergeio_network_apply" "rules" {
  config {
    network_id = vergeio_network.test.id
    target     = "rules"
  }
}
`
	}
	return acctest.Config(fmt.Sprintf(`
resource "vergeio_network" "test" {
  name       = %q
  type       = "internal"
  enabled    = true
  powerstate = true
}

resource "vergeio_network_rule" "ssh" {
  vnet              = vergeio_network.test.id
  name              = "allow-ssh"
  protocol          = "tcp"
  direction         = "incoming"
  action            = "accept"
  source_ip         = "192.0.2.0/24"
  destination_ports = "22"
  apply             = false
}

resource "terraform_data" "tick" {
  input = %q
%s
}
%s
`, networkName, tick, lifecycle, actionBlock))
}

func testAccNetworkNeedFWApply(resourceName string, want bool) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("resource not found: %s", resourceName)
		}
		id, err := strconv.Atoi(rs.Primary.ID)
		if err != nil {
			return fmt.Errorf("network id %q: %w", rs.Primary.ID, err)
		}
		client, err := acctest.SDKClient()
		if err != nil {
			return err
		}
		deadline := time.Now().Add(30 * time.Second)
		var last bool
		for {
			network, err := client.Networks.Get(context.Background(), id)
			if err != nil {
				return err
			}
			last = network.NeedFWApply
			if last == want {
				return nil
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("network %d need_fw_apply = %v, want %v", id, last, want)
			}
			time.Sleep(time.Second)
		}
	}
}
