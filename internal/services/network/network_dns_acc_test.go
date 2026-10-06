// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package network_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"terraform-provider-vergeio/internal/acctest"
)

func TestNetworkDNSAcceptanceConfigParses(t *testing.T) {
	networkName := "tf-acc-dns-net"
	for _, src := range []string{
		testAccDNSStackConfig(networkName, "192.0.2.10", false),
		testAccDNSStackConfig(networkName, "192.0.2.11", false),
		testAccDNSRunningConfig(networkName),
		testAccDNSVMConfig(networkName, "tf-acc-dns-vm", false),
		testAccDNSVMConfig(networkName, "tf-acc-dns-vm", true),
	} {
		if _, diags := hclsyntax.ParseConfig([]byte(src), "acc.tf", hcl.InitialPos); diags.HasErrors() {
			t.Fatalf("acceptance config did not parse: %s\n%s", diags.Error(), src)
		}
	}
}

func TestAccNetworkDNS_ViewZoneRecord(t *testing.T) {
	networkName := acctest.Name("dns-net")
	created := testAccDNSStackConfig(networkName, "192.0.2.10", false)
	updated := testAccDNSStackConfig(networkName, "192.0.2.11", false)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDNSDestroy,
		Steps: []resource.TestStep{
			{
				Config: created,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("vergeio_network_dns_view.test", "id"),
					resource.TestCheckResourceAttr("vergeio_network_dns_view.test", "name", "internal"),
					resource.TestCheckResourceAttrSet("vergeio_network_dns_zone.test", "id"),
					resource.TestCheckResourceAttr("vergeio_network_dns_zone.test", "domain", "example.com"),
					resource.TestCheckResourceAttr("vergeio_network_dns_zone.test", "type", "master"),
					resource.TestCheckResourceAttrSet("vergeio_network_dns_record.test", "id"),
					resource.TestCheckResourceAttr("vergeio_network_dns_record.test", "host", "www"),
					resource.TestCheckResourceAttr("vergeio_network_dns_record.test", "type", "A"),
					resource.TestCheckResourceAttr("vergeio_network_dns_record.test", "value", "192.0.2.10"),
				),
			},
			{
				Config: created,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			{
				Config: updated,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_network_dns_record.test", "value", "192.0.2.11"),
				),
			},
			{
				ResourceName:            "vergeio_network_dns_view.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"modified"},
			},
			{
				ResourceName:            "vergeio_network_dns_zone.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"modified"},
			},
			{
				ResourceName:            "vergeio_network_dns_record.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"modified"},
			},
		},
	})
}

func TestAccNetworkDNS_ApplyRunning(t *testing.T) {
	networkName := acctest.Name("dns-apply")
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDNSDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccDNSRunningConfig(networkName),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccNetworkRunning("vergeio_network.test"),
					resource.TestCheckResourceAttrSet("vergeio_network_dns_record.test", "id"),
					testAccNetworkNeedDNSApply("vergeio_network.test", false),
				),
			},
		},
	})
}

func TestAccNetworkDNS_VMAddress(t *testing.T) {
	networkName := acctest.Name("dns-vm-net")
	vmName := acctest.Name("dns-vm")
	byValue := testAccDNSVMConfig(networkName, vmName, false)
	byNIC := testAccDNSVMConfig(networkName, vmName, true)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDNSDestroy,
		Steps: []resource.TestStep{
			{
				Config: byValue,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_vm_nic.test", "ipaddress", "10.50.74.40"),
					resource.TestCheckResourceAttr("vergeio_network_dns_record.test", "value", "10.50.74.40"),
					resource.TestCheckResourceAttr("vergeio_network_dns_record.test", "type", "A"),
				),
			},
			{
				Config: byValue,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			{
				ResourceName:            "vergeio_network_dns_record.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"modified"},
			},
			{
				Config: byNIC,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("vergeio_network_dns_record.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("vergeio_network_dns_record.test", "vm_nic_id"),
					resource.TestCheckResourceAttr("vergeio_network_dns_record.test", "value", "10.50.74.40"),
				),
			},
			{
				ResourceName:            "vergeio_network_dns_record.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"vm_nic_id", "modified"},
			},
		},
	})
}

func testAccCheckDNSDestroy(s *terraform.State) error {
	client, err := acctest.SDKClient()
	if err != nil {
		return err
	}
	checks := []struct {
		resourceType string
		get          func(context.Context, int) error
	}{
		{"vergeio_network_dns_record", func(ctx context.Context, id int) error {
			_, err := client.VNetDNSRecords.Get(ctx, id)
			return err
		}},
		{"vergeio_network_dns_zone", func(ctx context.Context, id int) error {
			_, err := client.VNetDNSZones.Get(ctx, id)
			return err
		}},
		{"vergeio_network_dns_view", func(ctx context.Context, id int) error {
			_, err := client.VNetDNSViews.Get(ctx, id)
			return err
		}},
		{"vergeio_vm_nic", func(ctx context.Context, id int) error {
			_, err := client.VMNICs.Get(ctx, id)
			return err
		}},
		{"vergeio_vm", func(ctx context.Context, id int) error {
			_, err := client.VMs.Get(ctx, id)
			return err
		}},
	}
	for _, check := range checks {
		if err := acctest.CheckDeleted(s, check.resourceType, check.get); err != nil {
			return err
		}
	}
	return testAccCheckNetworkDestroy(s)
}

func testAccNetworkNeedDNSApply(resourceName string, want bool) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		id, err := accNetworkID(s, resourceName)
		if err != nil {
			return err
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
			last = network.NeedDNSApply
			if last == want {
				return nil
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("network %d need_dns_apply = %v, want %v", id, last, want)
			}
			time.Sleep(time.Second)
		}
	}
}

func testAccDNSStackConfig(networkName, value string, running bool) string {
	if err := acctest.RequirePrefix(networkName); err != nil {
		panic(err)
	}
	power := "false"
	if running {
		power = "true"
	}
	return acctest.Config(fmt.Sprintf(`
resource "vergeio_network" "test" {
  name       = %q
  type       = "internal"
  enabled    = true
  powerstate = %s
}

resource "vergeio_network_dns_view" "test" {
  network_id    = vergeio_network.test.id
  name          = "internal"
  recursion     = true
  match_clients = "10.0.0.0/8;"
}

resource "vergeio_network_dns_zone" "test" {
  view_id    = vergeio_network_dns_view.test.id
  domain     = "example.com"
  type       = "master"
  nameserver = "ns1.example.com"
  email      = "hostmaster@example.com"
}

resource "vergeio_network_dns_record" "test" {
  zone_id = vergeio_network_dns_zone.test.id
  host    = "www"
  type    = "A"
  value   = %q
}
`, networkName, power, value))
}

func testAccDNSRunningConfig(networkName string) string {
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

resource "vergeio_network_dns_view" "test" {
  network_id = vergeio_network.test.id
  name       = "internal"
  apply      = false
}

resource "vergeio_network_dns_zone" "test" {
  view_id = vergeio_network_dns_view.test.id
  domain  = "example.com"
  type    = "master"
  apply   = false
}

resource "vergeio_network_dns_record" "test" {
  zone_id = vergeio_network_dns_zone.test.id
  host    = "www"
  type    = "A"
  value   = "192.0.2.10"

  depends_on = [
    vergeio_network_dns_view.test,
    vergeio_network_dns_zone.test,
  ]
}
`, networkName))
}

func testAccDNSVMConfig(networkName, vmName string, byNIC bool) string {
	if err := acctest.RequirePrefix(networkName); err != nil {
		panic(err)
	}
	if err := acctest.RequirePrefix(vmName); err != nil {
		panic(err)
	}
	value := `value = vergeio_vm_nic.test.ipaddress`
	if byNIC {
		value = `vm_nic_id = vergeio_vm_nic.test.id`
	}
	return acctest.Config(fmt.Sprintf(`
resource "vergeio_network" "test" {
  name       = %q
  type       = "internal"
  enabled    = true
  powerstate = false
  network    = "10.50.74.0/24"
  ipaddress  = "10.50.74.1"
}

resource "vergeio_vm" "test" {
  name       = %q
  enabled    = true
  cpu_cores  = 2
  ram        = 2048
  powerstate = false

  boot_disk {
    name = "os"
    size = 5
  }
}

resource "vergeio_vm_nic" "test" {
  vm_id            = vergeio_vm.test.id
  name             = "nic0"
  interface        = "virtio"
  vnet             = tonumber(vergeio_network.test.id)
  assign_ipaddress = true
  ipaddress        = "10.50.74.40"
}

resource "vergeio_network_dns_view" "test" {
  network_id = vergeio_network.test.id
  name       = "internal"
  apply      = false
}

resource "vergeio_network_dns_zone" "test" {
  view_id = vergeio_network_dns_view.test.id
  domain  = "example.com"
  type    = "master"
  apply   = false
}

resource "vergeio_network_dns_record" "test" {
  zone_id = vergeio_network_dns_zone.test.id
  host    = "web"
  type    = "A"
  %s

  depends_on = [
    vergeio_network_dns_view.test,
    vergeio_network_dns_zone.test,
  ]
}
`, networkName, vmName, value))
}
