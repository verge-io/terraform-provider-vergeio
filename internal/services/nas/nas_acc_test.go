// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package nas_test

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/verge-io/govergeos"

	"terraform-provider-vergeio/internal/acctest"
)

func TestNASAcceptanceConfigParses(t *testing.T) {
	for _, src := range []string{
		testAccNASConfig("tf-acc-nas-net", "tf-acc-nas-vm", "tf-acc-nas-user", "tf-acc-nas-vol", "tf-acc-nas-cifs", "tf-acc-nas-nfs", "Documents", "10.0.0.0/8", 1),
		testAccNASConfig("tf-acc-nas-net", "tf-acc-nas-vm", "tf-acc-nas-user", "tf-acc-nas-vol", "tf-acc-nas-cifs", "tf-acc-nas-nfs", "Shared documents", "192.0.2.0/24", 2),
	} {
		if _, diags := hclsyntax.ParseConfig([]byte(src), "acc.tf", hcl.InitialPos); diags.HasErrors() {
			t.Fatalf("acceptance config did not parse: %s\n%s", diags.Error(), src)
		}
	}
}

func TestAccNAS_ServiceVolumeShares(t *testing.T) {
	networkName := acctest.Name("nasnet")
	vmName := acctest.Name("nasvm")
	userName := acctest.Name("nasuser")
	volumeName := acctest.Name("nasvol")
	cifsName := acctest.Name("nascifs")
	nfsName := acctest.Name("nasnfs")
	created := testAccNASConfig(networkName, vmName, userName, volumeName, cifsName, nfsName, "Documents", "10.0.0.0/8", 1)
	updated := testAccNASConfig(networkName, vmName, userName, volumeName, cifsName, nfsName, "Shared documents", "192.0.2.0/24", 2)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNASDestroy,
		Steps: []resource.TestStep{
			{
				Config: created,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_nas_service.test", "name", vmName),
					resource.TestCheckResourceAttrSet("vergeio_nas_service.test", "id"),
					resource.TestCheckResourceAttrSet("vergeio_nas_service.test", "vm_id"),
					resource.TestCheckResourceAttr("vergeio_nas_service.test", "max_imports", "4"),
					resource.TestCheckResourceAttr("vergeio_nas_service.test", "max_syncs", "1"),
					resource.TestCheckResourceAttr("vergeio_nas_service.test", "user.0.name", userName),
					resource.TestCheckResourceAttr("vergeio_nas_service.test", "user.0.display_name", "Files"),
					resource.TestCheckResourceAttr("vergeio_nas_service.test", "user.0.password_wo_version", "1"),
					resource.TestCheckResourceAttrSet("vergeio_nas_volume.test", "id"),
					resource.TestCheckResourceAttr("vergeio_nas_volume.test", "name", volumeName),
					resource.TestCheckResourceAttr("vergeio_nas_volume.test", "description", "Documents"),
					resource.TestCheckResourceAttr("vergeio_nas_volume.test", "enabled", "true"),
					resource.TestCheckResourceAttrSet("vergeio_nas_cifs_share.test", "id"),
					resource.TestCheckResourceAttr("vergeio_nas_cifs_share.test", "name", cifsName),
					resource.TestCheckResourceAttrSet("vergeio_nas_nfs_share.test", "id"),
					resource.TestCheckResourceAttr("vergeio_nas_nfs_share.test", "allowed_hosts", "10.0.0.0/8"),
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
					resource.TestCheckResourceAttr("vergeio_nas_service.test", "max_syncs", "2"),
					resource.TestCheckResourceAttr("vergeio_nas_volume.test", "description", "Shared documents"),
					resource.TestCheckResourceAttr("vergeio_nas_nfs_share.test", "allowed_hosts", "192.0.2.0/24"),
					resource.TestCheckResourceAttr("vergeio_nas_volume.test", "enabled", "true"),
				),
			},
			{
				ResourceName:            "vergeio_nas_service.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"user.0.password_wo_version", "network_id"},
			},
			{
				ResourceName:            "vergeio_nas_volume.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"modified"},
			},
			{
				ResourceName:            "vergeio_nas_cifs_share.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"modified", "status"},
			},
			{
				ResourceName:            "vergeio_nas_nfs_share.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"modified", "status"},
			},
		},
	})
}

func testAccCheckNASDestroy(s *terraform.State) error {
	client, err := acctest.SDKClient()
	if err != nil {
		return err
	}
	ctx := context.Background()
	stringChecks := []struct {
		resourceType string
		get          func(string) error
	}{
		{"vergeio_nas_cifs_share", func(id string) error {
			_, err := client.VolumeCIFSShares.Get(ctx, id)
			return err
		}},
		{"vergeio_nas_nfs_share", func(id string) error {
			_, err := client.VolumeNFSShares.Get(ctx, id)
			return err
		}},
		{"vergeio_nas_volume", func(id string) error {
			_, err := client.Volumes.Get(ctx, id)
			return err
		}},
	}
	for _, check := range stringChecks {
		if err := checkStringDeleted(s, check.resourceType, check.get); err != nil {
			return err
		}
	}
	if err := acctest.CheckDeleted(s, "vergeio_nas_service", func(ctx context.Context, id int) error {
		_, err := client.NASServices.Get(ctx, id)
		return err
	}); err != nil {
		return err
	}
	if err := checkNASVMDeleted(s, client); err != nil {
		return err
	}
	return acctest.CheckDeleted(s, "vergeio_network", func(ctx context.Context, id int) error {
		_, err := client.Networks.Get(ctx, id)
		return err
	})
}

func checkNASVMDeleted(s *terraform.State, client *vergeos.Client) error {
	ctx := context.Background()
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "vergeio_nas_service" {
			continue
		}
		vmID, err := strconv.Atoi(rs.Primary.Attributes["vm_id"])
		if err != nil || vmID <= 0 {
			return fmt.Errorf("NAS service %s has no virtual machine id", rs.Primary.ID)
		}
		_, err = client.VMs.Get(ctx, vmID)
		if err == nil {
			return fmt.Errorf("virtual machine %d for NAS service %s still exists after destroy", vmID, rs.Primary.ID)
		}
		if !vergeos.IsNotFoundError(err) {
			return fmt.Errorf("checking virtual machine %d was destroyed: %w", vmID, err)
		}
	}
	return nil
}

func checkStringDeleted(s *terraform.State, resourceType string, get func(string) error) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != resourceType || rs.Primary.ID == "" {
			continue
		}
		err := get(rs.Primary.ID)
		if err == nil {
			return fmt.Errorf("%s %s still exists after destroy", resourceType, rs.Primary.ID)
		}
		if !vergeos.IsNotFoundError(err) {
			return fmt.Errorf("checking %s %s was destroyed: %w", resourceType, rs.Primary.ID, err)
		}
	}
	return nil
}

func testAccNASConfig(networkName, vmName, userName, volumeName, cifsName, nfsName, volumeDesc, hosts string, maxSyncs int) string {
	for _, name := range []string{networkName, vmName, userName, volumeName, cifsName, nfsName} {
		if err := acctest.RequirePrefix(name); err != nil {
			panic(err)
		}
	}
	return acctest.Config(fmt.Sprintf(`
resource "vergeio_network" "test" {
  name         = %q
  type         = "internal"
  enabled      = true
  network      = "10.50.8.0/24"
  ipaddress    = "10.50.8.1"
  dhcp_enabled = true
  dhcp_start   = "10.50.8.10"
  dhcp_stop    = "10.50.8.200"
  powerstate   = true
}

resource "vergeio_nas_service" "test" {
  name                  = %q
  network_id            = vergeio_network.test.id
  max_imports           = 4
  max_syncs             = %d
  disable_swap          = false
  read_ahead_kb_default = "128"

  user {
    name                = %q
    password_wo         = "TerraformTest123"
    password_wo_version = 1
    display_name        = "Files"
    description         = "Acceptance user"
    enabled             = true
    home_drive          = "H"
  }
}

resource "vergeio_nas_volume" "test" {
  service_id  = vergeio_nas_service.test.id
  name        = %q
  description = %q
  enabled     = true
}

resource "vergeio_nas_cifs_share" "test" {
  volume_id   = vergeio_nas_volume.test.id
  name        = %q
  comment     = "docs"
  enabled     = true
  browseable  = true
}

resource "vergeio_nas_nfs_share" "test" {
  volume_id     = vergeio_nas_volume.test.id
  name          = %q
  allowed_hosts = %q
  squash        = "root_squash"
  data_access   = "rw"
  enabled       = true
}
`, networkName, vmName, maxSyncs, userName, volumeName, volumeDesc, cifsName, nfsName, hosts))
}
