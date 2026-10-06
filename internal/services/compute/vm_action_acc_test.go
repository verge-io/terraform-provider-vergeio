// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package compute_test

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/verge-io/govergeos"

	"terraform-provider-vergeio/internal/acctest"
)

// TestAccVMSnapshotAction takes a snapshot from a before_update trigger, then
// deletes that snapshot so destroy can remove the VM. The trigger is on
// terraform_data so the VM resource is not updated. Skipped without TF_ACC
// and on OpenTofu.
func TestAccVMSnapshotAction(t *testing.T) {
	acctest.RequireActions(t)
	vmName := acctest.Name("vm-snap")
	snapName := acctest.Name("vm-snap-shot")
	created := testAccVMActionConfig(vmName, "created", "", "")
	updated := testAccVMActionConfig(vmName, "snapshot", vmSnapshotTrigger, fmt.Sprintf(`
action "vergeio_vm_snapshot" "before" {
  config {
    vm_id             = vergeio_vm.test.id
    name              = %q
    retention_seconds = 3600
  }
}
`, snapName))

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.RequireActions(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckVMDestroy,
		Steps: []resource.TestStep{
			{Config: created},
			{
				Config: updated,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccVMHasSnapshot("vergeio_vm.test", snapName),
					testAccDeleteVMSnapshot("vergeio_vm.test", snapName),
				),
			},
		},
	})
}

// TestAccVMPowerAction powers an empty VM on, resets it, then shuts it down
// with a short timeout and force so a guest without ACPI does not block the
// test. The trigger is on terraform_data. power_on leaves the guest running
// while vergeio_vm.powerstate stays false, so that step's refresh plan is not
// empty. reset then declares powerstate true, matching the refreshed state, so
// the VM resource does not power the guest off. shutdown leaves the guest
// stopped while powerstate stays true, so that refresh plan is not empty either.
func TestAccVMPowerAction(t *testing.T) {
	acctest.RequireActions(t)
	vmName := acctest.Name("vm-power")
	stopped := testAccVMPowerStep(vmName, "created", "false", "", "")
	on := testAccVMPowerStep(vmName, "on", "false", "power_on", "")
	reset := testAccVMPowerStep(vmName, "reset", "true", "reset", "")
	off := testAccVMPowerStep(vmName, "off", "true", "shutdown", `
    timeout_seconds = 5
    force           = true
`)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.RequireActions(t) },
		ProtoV6ProviderFactories: acctest.ProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckVMDestroy,
		Steps: []resource.TestStep{
			{
				Config: stopped,
				Check:  testAccVMPower("vergeio_vm.test", false),
			},
			{
				Config:             on,
				ExpectNonEmptyPlan: true,
				Check:              testAccVMPower("vergeio_vm.test", true),
			},
			{
				Config: reset,
				Check:  testAccVMPower("vergeio_vm.test", true),
			},
			{
				Config:             off,
				ExpectNonEmptyPlan: true,
				Check:              testAccVMPower("vergeio_vm.test", false),
			},
		},
	})
}

func TestVMActionAcceptanceConfigParses(t *testing.T) {
	for _, src := range []string{
		testAccVMActionConfig("tf-acc-vm", "created", "", ""),
		testAccVMActionConfig("tf-acc-vm", "snapshot", vmSnapshotTrigger, `
action "vergeio_vm_snapshot" "before" {
  config {
    vm_id             = vergeio_vm.test.id
    name              = "tf-acc-vm-snap"
    retention_seconds = 3600
  }
}
`),
		testAccVMPowerStep("tf-acc-vm", "off", "true", "shutdown", `
    timeout_seconds = 5
    force           = true
`),
	} {
		if _, diags := hclsyntax.ParseConfig([]byte(src), "acc.tf", hcl.InitialPos); diags.HasErrors() {
			t.Fatalf("acceptance config did not parse: %s\n%s", diags.Error(), src)
		}
	}
}

const vmSnapshotTrigger = `
  lifecycle {
    action_trigger {
      events  = [before_update]
      actions = [action.vergeio_vm_snapshot.before]
    }
  }`

const vmPowerTrigger = `
  lifecycle {
    action_trigger {
      events  = [before_update]
      actions = [action.vergeio_vm_power.maintenance]
    }
  }`

func testAccVMPowerStep(vmName, tick, powerstate, operation, extra string) string {
	trigger := ""
	action := ""
	if operation != "" {
		trigger = vmPowerTrigger
		action = fmt.Sprintf(`
action "vergeio_vm_power" "maintenance" {
  config {
    vm_id     = vergeio_vm.test.id
    operation = %q
%s  }
}
`, operation, extra)
	}
	return testAccVMConfig(vmName, tick, fmt.Sprintf("\n  powerstate = %s", powerstate), trigger, action)
}

func testAccVMActionConfig(vmName, tick, lifecycle, action string) string {
	return testAccVMConfig(vmName, tick, "\n  powerstate = false", lifecycle, action)
}

func testAccVMConfig(vmName, tick, vmBody, lifecycle, action string) string {
	if err := acctest.RequirePrefix(vmName); err != nil {
		panic(err)
	}
	return acctest.Config(fmt.Sprintf(`
resource "vergeio_vm" "test" {
  name       = %q%s
}

resource "terraform_data" "tick" {
  input = %q
%s
}
%s
`, vmName, vmBody, tick, lifecycle, action))
}

func testAccVMHasSnapshot(resourceName, snapName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		id, err := accVMID(s, resourceName)
		if err != nil {
			return err
		}
		client, err := acctest.SDKClient()
		if err != nil {
			return err
		}
		snaps, err := client.VMSnapshots.ListByVM(context.Background(), id)
		if err != nil {
			return err
		}
		for _, snap := range snaps {
			if snap.Name == snapName {
				return nil
			}
		}
		return fmt.Errorf("vm %d has no snapshot %q", id, snapName)
	}
}

func testAccDeleteVMSnapshot(resourceName, snapName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		id, err := accVMID(s, resourceName)
		if err != nil {
			return err
		}
		client, err := acctest.SDKClient()
		if err != nil {
			return err
		}
		snaps, err := client.VMSnapshots.ListByVM(context.Background(), id)
		if err != nil {
			return err
		}
		for _, snap := range snaps {
			if snap.Name != snapName && !acctest.HasPrefix(snap.Name) {
				continue
			}
			if err := client.VMSnapshots.Delete(context.Background(), snap.Key.Int()); err != nil && !vergeos.IsNotFoundError(err) {
				return fmt.Errorf("delete vm snapshot %d: %w", snap.Key.Int(), err)
			}
		}
		return nil
	}
}

func testAccVMPower(resourceName string, want bool) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		id, err := accVMID(s, resourceName)
		if err != nil {
			return err
		}
		client, err := acctest.SDKClient()
		if err != nil {
			return err
		}
		vm, err := client.VMs.Get(context.Background(), id)
		if err != nil {
			return err
		}
		if vm.PowerState != want {
			return fmt.Errorf("vm %d powerstate = %v, want %v", id, vm.PowerState, want)
		}
		return nil
	}
}

func accVMID(s *terraform.State, resourceName string) (int, error) {
	rs, ok := s.RootModule().Resources[resourceName]
	if !ok {
		return 0, fmt.Errorf("resource not found: %s", resourceName)
	}
	id, err := strconv.Atoi(rs.Primary.ID)
	if err != nil {
		return 0, fmt.Errorf("vm id %q: %w", rs.Primary.ID, err)
	}
	return id, nil
}
