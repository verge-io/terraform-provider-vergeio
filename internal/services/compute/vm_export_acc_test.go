// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package compute_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/verge-io/govergeos"

	"terraform-provider-vergeio/internal/acctest"
)

func TestVMExportAcceptanceConfigParses(t *testing.T) {
	src := testAccVMExportConfig("tf-acc-export-net", "tf-acc-export-vm", "tf-acc-export-user", "tf-acc-export-vol", false, 2)
	if _, diags := hclsyntax.ParseConfig([]byte(src), "acc.tf", hcl.InitialPos); diags.HasErrors() {
		t.Fatal(diags.Error())
	}
}

func TestAccVMExport_Volume(t *testing.T) {
	networkName := acctest.Name("exportnet")
	vmName := acctest.Name("exportvm")
	userName := acctest.Name("exportuser")
	volumeName := acctest.Name("exportvol")
	for _, name := range []string{networkName, vmName, userName, volumeName} {
		if err := acctest.RequirePrefix(name); err != nil {
			t.Fatal(err)
		}
	}
	created := testAccVMExportConfig(networkName, vmName, userName, volumeName, false, 2)
	updated := testAccVMExportConfig(networkName, vmName, userName, volumeName, true, 3)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckVMExportDestroy,
		Steps: []resource.TestStep{
			{
				Config: created,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_vm_export.test", "quiesced", "false"),
					resource.TestCheckResourceAttr("vergeio_vm_export.test", "create_current", "true"),
					resource.TestCheckResourceAttr("vergeio_vm_export.test", "max_exports", "2"),
					resource.TestCheckResourceAttrSet("vergeio_vm_export.test", "id"),
					resource.TestCheckResourceAttrPair("vergeio_vm_export.test", "volume", "vergeio_nas_volume.test", "id"),
				),
			},
			{
				Config: created,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{
				Config: updated,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_vm_export.test", "quiesced", "true"),
					resource.TestCheckResourceAttr("vergeio_vm_export.test", "max_exports", "3"),
				),
			},
			{
				ResourceName:      "vergeio_vm_export.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccCheckVMExportDestroy(s *terraform.State) error {
	sdk, err := acctest.SDKClient()
	if err != nil {
		return err
	}
	if err := acctest.CheckDeleted(s, "vergeio_vm_export", func(ctx context.Context, id int) error {
		_, err := sdk.VMExports.Get(ctx, id)
		return err
	}); err != nil {
		return err
	}
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "vergeio_nas_volume" || rs.Primary.ID == "" {
			continue
		}
		_, err := sdk.Volumes.Get(context.Background(), rs.Primary.ID)
		if err == nil {
			return fmt.Errorf("nas volume %s still exists after destroy", rs.Primary.ID)
		}
		if !vergeos.IsNotFoundError(err) {
			return fmt.Errorf("checking nas volume %s was destroyed: %w", rs.Primary.ID, err)
		}
	}
	if err := acctest.CheckDeleted(s, "vergeio_nas_service", func(ctx context.Context, id int) error {
		_, err := sdk.NASServices.Get(ctx, id)
		return err
	}); err != nil {
		return err
	}
	return acctest.CheckDeleted(s, "vergeio_network", func(ctx context.Context, id int) error {
		_, err := sdk.Networks.Get(ctx, id)
		return err
	})
}

func testAccVMExportConfig(networkName, vmName, userName, volumeName string, quiesced bool, maxExports int) string {
	for _, name := range []string{networkName, vmName, userName, volumeName} {
		if err := acctest.RequirePrefix(name); err != nil {
			panic(err)
		}
	}
	return acctest.Config(fmt.Sprintf(`
resource "vergeio_network" "test" {
  name         = %q
  type         = "internal"
  enabled      = true
  network      = "10.50.86.0/24"
  ipaddress    = "10.50.86.1"
  dhcp_enabled = true
  dhcp_start   = "10.50.86.10"
  dhcp_stop    = "10.50.86.200"
  powerstate   = true
}

resource "vergeio_nas_service" "test" {
  name       = %q
  network_id = vergeio_network.test.id

  user {
    name                = %q
    password_wo         = "TerraformTest123!"
    password_wo_version = 1
    display_name        = "Files"
    enabled             = true
  }
}

resource "vergeio_nas_volume" "test" {
  service_id = vergeio_nas_service.test.id
  name       = %q
  enabled    = true
}

resource "vergeio_vm_export" "test" {
  volume         = vergeio_nas_volume.test.id
  quiesced       = %t
  create_current = true
  max_exports    = %d
}
`, networkName, vmName, userName, volumeName, quiesced, maxExports))
}
