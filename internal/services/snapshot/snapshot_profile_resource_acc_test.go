// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package snapshot_test

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

// TestAccSnapshotProfile creates a profile, points a VM at it, updates the
// period retention and schedule, and imports the profile. It is skipped
// unless TF_ACC=1.
//
// Retention is checked on the period row. A missing retention must not be
// stored as 86400 seconds.
func TestAccSnapshotProfile(t *testing.T) {
	profileName := acctest.Name("snapshot")
	vmName := acctest.Name("snapshot-vm")
	if err := acctest.RequirePrefix(profileName); err != nil {
		t.Fatal(err)
	}
	if err := acctest.RequirePrefix(vmName); err != nil {
		t.Fatal(err)
	}
	basic := testAccSnapshotProfileConfig(profileName, vmName, "Nightly VM snapshots", false)
	updated := testAccSnapshotProfileConfig(profileName, vmName, "Nightly and weekly VM snapshots", true)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSnapshotProfileDestroy,
		Steps: []resource.TestStep{
			{
				Config: basic,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("vergeio_snapshot_profile.test", "name", profileName),
					resource.TestCheckResourceAttr("vergeio_snapshot_profile.test", "description", "Nightly VM snapshots"),
					resource.TestCheckResourceAttr("vergeio_snapshot_profile.test", "period.0.name", "nightly"),
					resource.TestCheckResourceAttr("vergeio_snapshot_profile.test", "period.0.frequency", "daily"),
					resource.TestCheckResourceAttr("vergeio_snapshot_profile.test", "period.0.hour", "2"),
					resource.TestCheckResourceAttr("vergeio_snapshot_profile.test", "period.0.minute", "0"),
					resource.TestCheckResourceAttr("vergeio_snapshot_profile.test", "period.0.retention", "604800"),
					resource.TestCheckResourceAttr("vergeio_snapshot_profile.test", "period.0.quiesce", "true"),
					resource.TestCheckResourceAttrSet("vergeio_snapshot_profile.test", "id"),
					resource.TestCheckResourceAttrSet("vergeio_snapshot_profile.test", "period.0.key"),
					testAccCheckVMSnapshotProfile("vergeio_vm.test", "vergeio_snapshot_profile.test"),
					testAccCheckPeriodRetention("vergeio_snapshot_profile.test", "nightly", 604800, true),
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
					resource.TestCheckResourceAttr("vergeio_snapshot_profile.test", "description", "Nightly and weekly VM snapshots"),
					resource.TestCheckResourceAttr("vergeio_snapshot_profile.test", "period.0.retention", "1209600"),
					resource.TestCheckResourceAttr("vergeio_snapshot_profile.test", "period.0.quiesce", "false"),
					resource.TestCheckResourceAttr("vergeio_snapshot_profile.test", "period.1.name", "weekly"),
					resource.TestCheckResourceAttr("vergeio_snapshot_profile.test", "period.1.frequency", "weekly"),
					resource.TestCheckResourceAttr("vergeio_snapshot_profile.test", "period.1.day_of_week", "sun"),
					resource.TestCheckResourceAttr("vergeio_snapshot_profile.test", "period.1.retention", "2419200"),
					testAccCheckVMSnapshotProfile("vergeio_vm.test", "vergeio_snapshot_profile.test"),
					testAccCheckPeriodRetention("vergeio_snapshot_profile.test", "nightly", 1209600, false),
					testAccCheckPeriodRetention("vergeio_snapshot_profile.test", "weekly", 2419200, true),
				),
			},
			{
				Config: updated,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			{
				ResourceName:      "vergeio_snapshot_profile.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccCheckVMSnapshotProfile(vmName, profileName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		vm, ok := s.RootModule().Resources[vmName]
		if !ok {
			return fmt.Errorf("resource not found: %s", vmName)
		}
		profile, ok := s.RootModule().Resources[profileName]
		if !ok {
			return fmt.Errorf("resource not found: %s", profileName)
		}
		if vm.Primary.Attributes["snapshot_profile"] != profile.Primary.ID {
			return fmt.Errorf("snapshot_profile = %s, want %s", vm.Primary.Attributes["snapshot_profile"], profile.Primary.ID)
		}
		client, err := acctest.SDKClient()
		if err != nil {
			return err
		}
		vmID, err := strconv.Atoi(vm.Primary.ID)
		if err != nil {
			return err
		}
		profileID, err := strconv.Atoi(profile.Primary.ID)
		if err != nil {
			return err
		}
		got, err := client.VMs.Get(context.Background(), vmID)
		if err != nil {
			return err
		}
		if got.SnapshotProfile.Int() != profileID {
			return fmt.Errorf("vm snapshot profile = %d, want %d", got.SnapshotProfile.Int(), profileID)
		}
		return nil
	}
}

func testAccCheckPeriodRetention(profileName, periodName string, retention int, quiesce bool) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		profile, ok := s.RootModule().Resources[profileName]
		if !ok {
			return fmt.Errorf("resource not found: %s", profileName)
		}
		id, err := strconv.Atoi(profile.Primary.ID)
		if err != nil {
			return err
		}
		client, err := acctest.SDKClient()
		if err != nil {
			return err
		}
		periods, err := client.SnapshotProfilePeriods.ListByProfile(context.Background(), id)
		if err != nil {
			return err
		}
		for _, period := range periods {
			if period.Name != periodName {
				continue
			}
			if period.Retention != retention {
				return fmt.Errorf("period %s retention = %d, want %d", periodName, period.Retention, retention)
			}
			if period.Quiesce != quiesce {
				return fmt.Errorf("period %s quiesce = %t, want %t", periodName, period.Quiesce, quiesce)
			}
			return nil
		}
		return fmt.Errorf("period %s not found", periodName)
	}
}

func testAccCheckSnapshotProfileDestroy(s *terraform.State) error {
	client, err := acctest.SDKClient()
	if err != nil {
		return err
	}
	if err := acctest.CheckDeleted(s, "vergeio_vm", func(ctx context.Context, id int) error {
		_, err := client.VMs.Get(ctx, id)
		return err
	}); err != nil {
		return err
	}
	return acctest.CheckDeleted(s, "vergeio_snapshot_profile", func(ctx context.Context, id int) error {
		_, err := client.SnapshotProfiles.Get(ctx, id)
		if err == nil {
			return nil
		}
		if !vergeos.IsNotFoundError(err) {
			return err
		}
		periods, listErr := client.SnapshotProfilePeriods.ListByProfile(ctx, id)
		if listErr != nil {
			return listErr
		}
		if len(periods) > 0 {
			return fmt.Errorf("%d periods still exist for snapshot profile %d", len(periods), id)
		}
		return err
	})
}

func testAccSnapshotProfileConfig(profileName, vmName, description string, weekly bool) string {
	weeklyBlock := ""
	retention := 604800
	quiesce := "true"
	if weekly {
		retention = 1209600
		quiesce = "false"
		weeklyBlock = `
  period {
    name        = "weekly"
    frequency   = "weekly"
    day_of_week = "sun"
    hour        = 1
    minute      = 0
    retention   = 2419200
    quiesce     = true
  }
`
	}
	return acctest.Config(fmt.Sprintf(`
resource "vergeio_snapshot_profile" "test" {
  name        = %q
  description = %q

  period {
    name      = "nightly"
    frequency = "daily"
    hour      = 2
    minute    = 0
    retention = %d
    quiesce   = %s
  }
%s
}

resource "vergeio_vm" "test" {
  name             = %q
  snapshot_profile = tonumber(vergeio_snapshot_profile.test.id)
}
`, profileName, description, retention, quiesce, weeklyBlock, vmName))
}
